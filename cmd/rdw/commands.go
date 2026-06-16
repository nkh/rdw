package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/nkh/rdw/internal/discovery"
	"github.com/nkh/rdw/internal/layout"
	"github.com/nkh/rdw/internal/mirror"
	pipepkg "github.com/nkh/rdw/internal/pipe"
	"github.com/nkh/rdw/internal/selftest"
	"github.com/nkh/rdw/internal/session"
	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------------------
// REST client helper
// ---------------------------------------------------------------------------

type restClient struct {
	base  string
	token string
}

func newRestClient(port int) (*restClient, error) {
	resolved, err := discovery.Resolve(port)
	if err != nil {
		return nil, err
	}
	return &restClient{base: fmt.Sprintf("http://127.0.0.1:%d", resolved)}, nil
}

func (c *restClient) do(method, path string, body interface{}) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connecting to rdw server: %w", err)
	}
	return resp, nil
}

func (c *restClient) get(path string) (*http.Response, error) {
	return c.do("GET", path, nil)
}
func (c *restClient) post(path string, b interface{}) (*http.Response, error) {
	return c.do("POST", path, b)
}
func (c *restClient) put(path string, b interface{}) (*http.Response, error) {
	return c.do("PUT", path, b)
}
func (c *restClient) delete(path string) (*http.Response, error) {
	return c.do("DELETE", path, nil)
}
func (c *restClient) patch(path string, b interface{}) (*http.Response, error) {
	return c.do("PATCH", path, b)
}

func decodeBody(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

func checkStatus(resp *http.Response, expected int) error {
	defer resp.Body.Close()
	if resp.StatusCode == expected {
		return nil
	}
	var errBody map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&errBody)
	if msg, ok := errBody["error"]; ok {
		return fmt.Errorf("server error (%d): %s", resp.StatusCode, msg)
	}
	return fmt.Errorf("unexpected status %d", resp.StatusCode)
}

// portFlag reads the persistent --port flag from the root command.
func portFlag(cmd *cobra.Command) int {
	p, _ := cmd.Root().PersistentFlags().GetInt("port")
	return p
}

// ---------------------------------------------------------------------------
// rdw pipe
// ---------------------------------------------------------------------------

var pipeCmd = &cobra.Command{
	Use:   "pipe",
	Short: "Relay stdin to a named pane",
	Long: `Read lines from stdin and forward them to the server pane identified
by --id. Inline control sequences are interpreted by the server.

Binary payloads must be base64-encoded with a "b64:" prefix.

If --layout is given, the layout is looked up by name (a saved preset) or
loaded from a file path. If the layout is not yet active on the server it is
created and displayed in the browser. The stream is then routed into the pane
matching --id within that layout.

If --window is given without --layout, the stream is routed into the named
window. If neither flag is given the server routes to any pane already
registered for --id.`,
	RunE: runPipe,
}

func init() {
	rootCmd.AddCommand(pipeCmd)
	pipeCmd.Flags().StringP("id", "i", "", "target pane ID (required)")
	pipeCmd.Flags().StringP("layout", "l", "",
		"layout preset name or file path; created in browser if not yet active")
	pipeCmd.Flags().StringP("window", "w", "",
		"window name to route into (used when --layout is not specified)")
	pipeCmd.Flags().String("forward", "",
		"also forward to: rdw, rd, or both (default: rdw only)")
	pipeCmd.Flags().Bool("allow-unassigned", false,
		"create pane dynamically if target ID has no registered pane")
	pipeCmd.Flags().String("forward-to-file", "",
		"mirror stream to a local file or FIFO path")
	pipeCmd.Flags().String("forward-to-cmd", "",
		"mirror stream as stdin to this shell command")
	_ = pipeCmd.MarkFlagRequired("id")
}

func runPipe(cmd *cobra.Command, _ []string) error {
	port := portFlag(cmd)
	idStr, _ := cmd.Flags().GetString("id")
	layoutRef, _ := cmd.Flags().GetString("layout")
	_ = cmd.Flags().Lookup("window")

	id, err := session.ParseTargetID(idStr)
	if err != nil {
		return fmt.Errorf("invalid target ID %q: %w", idStr, err)
	}

	resolved, err := discovery.Resolve(port)
	if err != nil {
		return err
	}

	// If --layout is given and resolves to a file, apply it first.
	if layoutRef != "" {
		rc := &restClient{base: fmt.Sprintf("http://127.0.0.1:%d", resolved)}
		if err := applyLayoutRef(rc, layoutRef); err != nil {
			return fmt.Errorf("applying layout %q: %w", layoutRef, err)
		}
	}

	socketPath, _ := unixSocketPath(fmt.Sprintf("%d", resolved))

	src := io.Reader(os.Stdin)

	// --forward rd|rdw|both: tee to bash-rd via its socket/protocol.
	if fwdMode, _ := cmd.Flags().GetString("forward"); fwdMode == "rd" || fwdMode == "both" {
		rdSink, err := mirror.CmdSync("rd 2>/dev/null || true")
		if err == nil {
			src = mirror.Tee(src, rdSink)
		}
	}

	if fwdFile, _ := cmd.Flags().GetString("forward-to-file"); fwdFile != "" {
		sink, err := mirror.FileSync(fwdFile)
		if err != nil {
			return err
		}

		src = mirror.Tee(src, sink)
	}

	if fwdCmd, _ := cmd.Flags().GetString("forward-to-cmd"); fwdCmd != "" {
		sink, err := mirror.CmdSync(fwdCmd)
		if err != nil {
			return err
		}

		src = mirror.Tee(src, sink)
	}

	return pipepkg.Relay(context.Background(), src, pipepkg.Options{
		TargetID:          id,
		Port:              resolved,
		SocketPath:        socketPath,
		ReconnectQueueLen: 1000,
	})
}

// applyLayoutRef applies a layout by preset name or file path.
func applyLayoutRef(rc *restClient, ref string) error {
	// Try preset first.
	resp, err := rc.post("/api/v1/layouts/"+ref+"/apply", nil)
	if err == nil && resp.StatusCode == http.StatusNoContent {
		resp.Body.Close()
		return nil
	}
	if resp != nil {
		resp.Body.Close()
	}

	// Try as file path.
	l, err := layout.LoadFile(ref)
	if err != nil {
		// Also try ref + ".yaml"
		l, err = layout.LoadFile(ref + ".yaml")
		if err != nil {
			return fmt.Errorf("not a saved preset and cannot load as file: %w", err)
		}
	}

	// Upload to server as a new preset.
	name := l.Name
	if name == "" {
		name = ref
	}

	// Save it then apply.
	data, _ := json.Marshal(map[string]string{"name": name})
	resp, err = rc.do("POST", "/api/v1/layouts", json.RawMessage(data))
	if err != nil {
		return err
	}
	resp.Body.Close()

	resp2, err := rc.post("/api/v1/layouts/"+name+"/apply", nil)
	if err != nil {
		return err
	}
	return checkStatus(resp2, http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// rdw window
// ---------------------------------------------------------------------------

var windowCmd = &cobra.Command{
	Use:   "window",
	Short: "Manage windows in the active server session",
}

var windowCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new empty window",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rc, err := newRestClient(portFlag(cmd))
		if err != nil {
			return err
		}
		resp, err := rc.post("/api/v1/windows", map[string]string{"name": args[0]})
		if err != nil {
			return err
		}
		if err := checkStatus(resp, http.StatusCreated); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "window %q created\n", args[0])
		return nil
	},
}

var windowCloseCmd = &cobra.Command{
	Use:   "close <name>",
	Short: "Close a window and all its panes",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rc, err := newRestClient(portFlag(cmd))
		if err != nil {
			return err
		}
		resp, err := rc.delete("/api/v1/windows/" + args[0])
		if err != nil {
			return err
		}
		return checkStatus(resp, http.StatusNoContent)
	},
}

var windowRenameCmd = &cobra.Command{
	Use:   "rename <old-name> <new-name>",
	Short: "Rename a window",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		rc, err := newRestClient(portFlag(cmd))
		if err != nil {
			return err
		}
		resp, err := rc.patch("/api/v1/windows/"+args[0],
			map[string]string{"name": args[1]})
		if err != nil {
			return err
		}
		return checkStatus(resp, http.StatusNoContent)
	},
}

var windowListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all windows in the active session",
	RunE: func(cmd *cobra.Command, _ []string) error {
		rc, err := newRestClient(portFlag(cmd))
		if err != nil {
			return err
		}
		resp, err := rc.get("/api/v1/windows")
		if err != nil {
			return err
		}
		var body map[string]interface{}
		if err := decodeBody(resp, &body); err != nil {
			return err
		}
		wins, _ := body["windows"].([]interface{})
		if len(wins) == 0 {
			fmt.Println("no windows")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tPANES")
		for _, v := range wins {
			win := v.(map[string]interface{})
			name, _ := win["name"].(string)
			panes, _ := win["panes"].([]interface{})
			fmt.Fprintf(w, "%s\t%d\n", name, len(panes))
		}
		return w.Flush()
	},
}

var windowFocusCmd = &cobra.Command{
	Use:   "focus <name>",
	Short: "Switch the browser to the named window",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rc, err := newRestClient(portFlag(cmd))
		if err != nil {
			return err
		}
		resp, err := rc.post("/api/v1/windows/"+args[0]+"/focus", nil)
		if err != nil {
			return err
		}
		return checkStatus(resp, http.StatusNoContent)
	},
}

func init() {
	rootCmd.AddCommand(windowCmd)
	windowCmd.AddCommand(windowCreateCmd)
	windowCmd.AddCommand(windowCloseCmd)
	windowCmd.AddCommand(windowRenameCmd)
	windowCmd.AddCommand(windowListCmd)
	windowCmd.AddCommand(windowFocusCmd)
	windowCreateCmd.Flags().StringP("layout", "l", "",
		"layout file or preset name to populate the new window")
}

// ---------------------------------------------------------------------------
// rdw pane
// ---------------------------------------------------------------------------

var paneCmd = &cobra.Command{
	Use:   "pane",
	Short: "Manage panes within a window",
}

var paneSplitCmd = &cobra.Command{
	Use:   "split <parent-id> <h|v> <new-id>",
	Short: "Split a pane horizontally or vertically",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		rc, err := newRestClient(portFlag(cmd))
		if err != nil {
			return err
		}
		group, _ := cmd.Flags().GetString("group")
		private, _ := cmd.Flags().GetBool("private")
		body := map[string]interface{}{
			"direction": args[1],
			"new_id":    args[2],
		}
		if group != "" {
			body["group"] = group
		}
		if private {
			body["private"] = true
		}
		resp, err := rc.post("/api/v1/panes/"+args[0]+"/split", body)
		if err != nil {
			return err
		}
		return checkStatus(resp, http.StatusCreated)
	},
}

var paneResizeCmd = &cobra.Command{
	Use:   "resize <pane-id> <left|right|up|down> <size>",
	Short: "Resize a pane (columns, Npx, N%)",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, _, err := layout.ParseResizeArg(args[2]); err != nil {
			return fmt.Errorf("invalid size %q: %w", args[2], err)
		}
		rc, err := newRestClient(portFlag(cmd))
		if err != nil {
			return err
		}
		resp, err := rc.post("/api/v1/panes/"+args[0]+"/resize",
			map[string]string{"direction": args[1], "size": args[2]})
		if err != nil {
			return err
		}
		return checkStatus(resp, http.StatusNoContent)
	},
}

var paneZoomCmd = &cobra.Command{
	Use:   "zoom <pane-id>",
	Short: "Toggle a pane between normal and full-window zoom",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rc, err := newRestClient(portFlag(cmd))
		if err != nil {
			return err
		}
		resp, err := rc.post("/api/v1/panes/"+args[0]+"/zoom", nil)
		if err != nil {
			return err
		}
		return checkStatus(resp, http.StatusNoContent)
	},
}

var paneSwapCmd = &cobra.Command{
	Use:   "swap <pane-id-a> <pane-id-b>",
	Short: "Swap the positions of two panes",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Root().PersistentFlags().GetInt("port")
		rc, err := newRestClient(port)
		if err != nil {
			return err
		}
		resp, err := rc.post("/api/v1/panes/"+args[0]+"/swap",
			map[string]string{"target": args[1]})
		if err != nil {
			return err
		}
		return checkStatus(resp, http.StatusNoContent)
	},
}

var paneCloseCmd = &cobra.Command{
	Use:   "close <pane-id>",
	Short: "Close a pane",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rc, err := newRestClient(portFlag(cmd))
		if err != nil {
			return err
		}
		resp, err := rc.delete("/api/v1/panes/" + args[0])
		if err != nil {
			return err
		}
		return checkStatus(resp, http.StatusNoContent)
	},
}

func init() {
	rootCmd.AddCommand(paneCmd)
	paneCmd.AddCommand(paneSplitCmd)
	paneCmd.AddCommand(paneResizeCmd)
	paneCmd.AddCommand(paneZoomCmd)
	paneCmd.AddCommand(paneSwapCmd)
	paneCmd.AddCommand(paneCloseCmd)
	paneSplitCmd.Flags().StringP("group", "g", "", "assign pane to a group")
	paneSplitCmd.Flags().Bool("private", false, "hide pane from non-owner tokens")
}

// ---------------------------------------------------------------------------
// rdw layout
// ---------------------------------------------------------------------------

var layoutCmd = &cobra.Command{
	Use:   "layout",
	Short: "Manage and apply layout presets",
}

var layoutSaveCmd = &cobra.Command{
	Use:   "save --name <name>",
	Short: "Snapshot the current layout as a named preset",
	RunE: func(cmd *cobra.Command, _ []string) error {
		name, _ := cmd.Flags().GetString("name")
		rc, err := newRestClient(portFlag(cmd))
		if err != nil {
			return err
		}
		resp, err := rc.post("/api/v1/layouts", map[string]string{"name": name})
		if err != nil {
			return err
		}
		if err := checkStatus(resp, http.StatusCreated); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "layout %q saved\n", name)
		return nil
	},
}

var layoutApplyCmd = &cobra.Command{
	Use:   "apply <name-or-path>",
	Short: "Apply a layout preset or file to the active session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rc, err := newRestClient(portFlag(cmd))
		if err != nil {
			return err
		}
		return applyLayoutRef(rc, args[0])
	},
}

var layoutListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved layout presets",
	RunE: func(cmd *cobra.Command, _ []string) error {
		rc, err := newRestClient(portFlag(cmd))
		if err != nil {
			return err
		}
		resp, err := rc.get("/api/v1/layouts")
		if err != nil {
			return err
		}
		var body map[string]interface{}
		if err := decodeBody(resp, &body); err != nil {
			return err
		}
		layouts, _ := body["layouts"].([]interface{})
		if len(layouts) == 0 {
			fmt.Println("no saved layouts")
			return nil
		}
		for _, v := range layouts {
			fmt.Println(v)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(layoutCmd)
	layoutCmd.AddCommand(layoutSaveCmd)
	layoutCmd.AddCommand(layoutApplyCmd)
	layoutCmd.AddCommand(layoutListCmd)
	layoutSaveCmd.Flags().String("name", "", "preset name (required)")
	_ = layoutSaveCmd.MarkFlagRequired("name")
}

// ---------------------------------------------------------------------------
// rdw kv
// ---------------------------------------------------------------------------

var kvCmd = &cobra.Command{
	Use:   "kv",
	Short: "Interact with the session key-value store",
}

var kvSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Write a value into the KV store",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		rc, err := newRestClient(portFlag(cmd))
		if err != nil {
			return err
		}
		resp, err := rc.put("/api/v1/kv/"+args[0],
			map[string]string{"value": args[1]})
		if err != nil {
			return err
		}
		return checkStatus(resp, http.StatusNoContent)
	},
}

var kvGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Read a value from the KV store",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rc, err := newRestClient(portFlag(cmd))
		if err != nil {
			return err
		}
		resp, err := rc.get("/api/v1/kv/" + args[0])
		if err != nil {
			return err
		}
		var body map[string]string
		if err := decodeBody(resp, &body); err != nil {
			return err
		}
		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("key %q not found", args[0])
		}
		fmt.Println(body["value"])
		return nil
	},
}

var kvDeleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Delete a key from the KV store",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rc, err := newRestClient(portFlag(cmd))
		if err != nil {
			return err
		}
		resp, err := rc.delete("/api/v1/kv/" + args[0])
		if err != nil {
			return err
		}
		return checkStatus(resp, http.StatusNoContent)
	},
}

func init() {
	rootCmd.AddCommand(kvCmd)
	kvCmd.AddCommand(kvSetCmd, kvGetCmd, kvDeleteCmd)
}

// ---------------------------------------------------------------------------
// rdw token
// ---------------------------------------------------------------------------

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Manage session access tokens",
}

var tokenCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an access token",
	RunE: func(cmd *cobra.Command, _ []string) error {
		expiry, _ := cmd.Flags().GetDuration("expiry")
		panesStr, _ := cmd.Flags().GetString("panes")
		windowsStr, _ := cmd.Flags().GetString("windows")

		var panes, windows []string
		if panesStr != "" {
			panes = strings.Split(panesStr, ",")
		}
		if windowsStr != "" {
			windows = strings.Split(windowsStr, ",")
		}

		rc, err := newRestClient(portFlag(cmd))
		if err != nil {
			return err
		}

		body := map[string]interface{}{
			"expiry_seconds": int64(expiry.Seconds()),
		}
		if len(panes) > 0 {
			body["panes"] = panes
		}
		if len(windows) > 0 {
			body["windows"] = windows
		}

		resp, err := rc.post("/api/v1/tokens", body)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusCreated {
			return checkStatus(resp, http.StatusCreated)
		}

		var result map[string]interface{}
		if err := decodeBody(resp, &result); err != nil {
			return err
		}

		fmt.Fprintf(os.Stdout, "token: %s\n", result["token"])
		fmt.Fprintf(os.Stdout, "id:    %s\n", result["id"])
		if exp, ok := result["expires_at"].(string); ok && exp != "" {
			fmt.Fprintf(os.Stdout, "expires: %s\n", exp)
		}
		fmt.Fprintln(os.Stdout, "\nThis token will not be shown again.")
		return nil
	},
}

var tokenRevokeCmd = &cobra.Command{
	Use:   "revoke <token-id>",
	Short: "Revoke an access token immediately",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rc, err := newRestClient(portFlag(cmd))
		if err != nil {
			return err
		}
		resp, err := rc.delete("/api/v1/tokens/" + args[0])
		if err != nil {
			return err
		}
		if err := checkStatus(resp, http.StatusNoContent); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "token %q revoked\n", args[0])
		return nil
	},
}

var tokenListCmd = &cobra.Command{
	Use:   "list",
	Short: "List active tokens",
	RunE: func(cmd *cobra.Command, _ []string) error {
		rc, err := newRestClient(portFlag(cmd))
		if err != nil {
			return err
		}
		resp, err := rc.get("/api/v1/tokens")
		if err != nil {
			return err
		}
		var body map[string]interface{}
		if err := decodeBody(resp, &body); err != nil {
			return err
		}
		tokens, _ := body["tokens"].([]interface{})
		if len(tokens) == 0 {
			fmt.Println("no active tokens")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tEXPIRES\tPANES\tWINDOWS")
		for _, v := range tokens {
			tok := v.(map[string]interface{})
			id, _ := tok["id"].(string)
			exp, _ := tok["expires_at"].(string)
			if exp == "" {
				exp = "never"
			}
			panes := ifaceSliceStr(tok["panes"])
			wins := ifaceSliceStr(tok["windows"])
			panesStr := strings.Join(panes, ",")
			winsStr := strings.Join(wins, ",")
			if panesStr == "" {
				panesStr = "all"
			}
			if winsStr == "" {
				winsStr = "all"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", id, exp, panesStr, winsStr)
		}
		return w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(tokenCmd)
	tokenCmd.AddCommand(tokenCreateCmd)
	tokenCmd.AddCommand(tokenRevokeCmd)
	tokenCmd.AddCommand(tokenListCmd)

	tokenCreateCmd.Flags().Duration("expiry", 24*time.Hour,
		"token lifetime (0 = no expiry)")
	tokenCreateCmd.Flags().String("panes", "",
		"comma-separated list of pane IDs this token can access")
	tokenCreateCmd.Flags().String("windows", "",
		"comma-separated list of window names this token can access")
}

// ---------------------------------------------------------------------------
// rdw group
// ---------------------------------------------------------------------------

var groupCmd = &cobra.Command{
	Use:   "group",
	Short: "Manage pane groups",
}

func groupAction(action string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		rc, err := newRestClient(portFlag(cmd))
		if err != nil {
			return err
		}
		resp, err := rc.post(fmt.Sprintf("/api/v1/groups/%s/%s", args[0], action), nil)
		if err != nil {
			return err
		}
		return checkStatus(resp, http.StatusNoContent)
	}
}

func init() {
	rootCmd.AddCommand(groupCmd)
	for _, action := range []string{"hide", "show", "focus", "kill"} {
		a := action
		groupCmd.AddCommand(&cobra.Command{
			Use:   a + " <group-name>",
			Short: strings.Title(a) + " all panes in a group",
			Args:  cobra.ExactArgs(1),
			RunE:  groupAction(a),
		})
	}
}

// ---------------------------------------------------------------------------
// rdw save
// ---------------------------------------------------------------------------

var saveCmd = &cobra.Command{
	Use:   "save",
	Short: "Export scrollback history to a Markdown bundle",
}

var savePaneCmd = &cobra.Command{
	Use:   "pane",
	Short: "Export a single pane",
	RunE: func(cmd *cobra.Command, _ []string) error {
		id, _ := cmd.Flags().GetString("target-id")
		outDir, _ := cmd.Flags().GetString("out-dir")
		rc, err := newRestClient(portFlag(cmd))
		if err != nil {
			return err
		}
		resp, err := rc.post("/api/v1/export/pane",
			map[string]string{"target_id": id, "out_dir": outDir})
		if err != nil {
			return err
		}
		return checkStatus(resp, http.StatusNoContent)
	},
}

var saveWindowCmd = &cobra.Command{
	Use:   "window",
	Short: "Export all panes in a window",
	RunE: func(cmd *cobra.Command, _ []string) error {
		name, _ := cmd.Flags().GetString("name")
		outDir, _ := cmd.Flags().GetString("out-dir")
		rc, err := newRestClient(portFlag(cmd))
		if err != nil {
			return err
		}
		resp, err := rc.post("/api/v1/export/window",
			map[string]string{"name": name, "out_dir": outDir})
		if err != nil {
			return err
		}
		return checkStatus(resp, http.StatusNoContent)
	},
}

var saveAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Export all windows in the session",
	RunE: func(cmd *cobra.Command, _ []string) error {
		outDir, _ := cmd.Flags().GetString("out-dir")
		rc, err := newRestClient(portFlag(cmd))
		if err != nil {
			return err
		}
		resp, err := rc.post("/api/v1/export/all",
			map[string]string{"out_dir": outDir})
		if err != nil {
			return err
		}
		return checkStatus(resp, http.StatusNoContent)
	},
}

func init() {
	rootCmd.AddCommand(saveCmd)
	saveCmd.AddCommand(savePaneCmd)
	saveCmd.AddCommand(saveWindowCmd)
	saveCmd.AddCommand(saveAllCmd)

	savePaneCmd.Flags().String("target-id", "", "target pane ID (required)")
	savePaneCmd.Flags().String("out-dir", ".", "output directory")
	_ = savePaneCmd.MarkFlagRequired("target-id")

	saveWindowCmd.Flags().String("name", "", "window name (required)")
	saveWindowCmd.Flags().String("out-dir", ".", "output directory")
	_ = saveWindowCmd.MarkFlagRequired("name")

	saveAllCmd.Flags().String("out-dir", ".", "output directory")
}

// ---------------------------------------------------------------------------
// rdw selftest
// ---------------------------------------------------------------------------

var selftestCmd = &cobra.Command{
	Use:   "selftest",
	Short: "Run built-in smoke tests",
	Long: `Start an in-process test suite that exercises the core pipeline,
KV store, and control sequence subsystems without requiring a browser.

Exits 0 on success, non-zero on failure. Suitable for CI smoke testing.`,
	RunE: runSelftest,
}

func init() { rootCmd.AddCommand(selftestCmd) }

func runSelftest(_ *cobra.Command, _ []string) error {
	result := selftest.Run(context.Background(), os.Stdout)
	if !result.Passed {
		return fmt.Errorf("selftest: one or more checks failed")
	}
	return nil
}

// ---------------------------------------------------------------------------
// rdw completion
// ---------------------------------------------------------------------------

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:                   "completion bash",
		Short:                 "Output bash completion script to stdout",
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Root().GenBashCompletion(os.Stdout)
		},
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func ifaceSliceStr(v interface{}) []string {
	if v == nil {
		return nil
	}
	s, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, len(s))
	for i, item := range s {
		out[i], _ = item.(string)
	}
	return out
}
