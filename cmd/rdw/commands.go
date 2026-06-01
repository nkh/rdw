package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/nkh/rdw/internal/selftest"
	"github.com/spf13/cobra"
)

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
	port, _ := cmd.Root().PersistentFlags().GetInt("port")
	id, _ := cmd.Flags().GetString("id")
	layoutRef, _ := cmd.Flags().GetString("layout")
	window, _ := cmd.Flags().GetString("window")
	_ = port
	_ = id
	_ = layoutRef
	_ = window

	fmt.Println("pipe: not yet implemented")

	return nil
}

// ---------------------------------------------------------------------------
// rdw window
// ---------------------------------------------------------------------------

var windowCmd = &cobra.Command{
	Use:   "window",
	Short: "Manage windows in the active server session",
	Long: `Manage server-managed windows.

Windows are views within the browser page, not browser tabs. The browser
displays one window at a time and shows a header bar listing all window names.
Use the keyboard bindings or click the header to switch between windows.`,
}

var windowCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new empty window",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Root().PersistentFlags().GetInt("port")
		_ = port
		fmt.Printf("window create %q: not yet implemented\n", args[0])
		return nil
	},
}

var windowCloseCmd = &cobra.Command{
	Use:   "close <name>",
	Short: "Close a window and all its panes",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Root().PersistentFlags().GetInt("port")
		_ = port
		fmt.Printf("window close %q: not yet implemented\n", args[0])
		return nil
	},
}

var windowRenameCmd = &cobra.Command{
	Use:   "rename <old-name> <new-name>",
	Short: "Rename a window",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Root().PersistentFlags().GetInt("port")
		_ = port
		fmt.Printf("window rename %q -> %q: not yet implemented\n", args[0], args[1])
		return nil
	},
}

var windowListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all windows in the active session",
	RunE: func(cmd *cobra.Command, _ []string) error {
		port, _ := cmd.Root().PersistentFlags().GetInt("port")
		_ = port
		fmt.Println("window list: not yet implemented")
		return nil
	},
}

var windowFocusCmd = &cobra.Command{
	Use:   "focus <name>",
	Short: "Switch the browser to the named window",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Root().PersistentFlags().GetInt("port")
		_ = port
		fmt.Printf("window focus %q: not yet implemented\n", args[0])
		return nil
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
	Use:   "split <target-id> <h|v> <new-id>",
	Short: "Split a pane horizontally or vertically",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Root().PersistentFlags().GetInt("port")
		_ = port
		fmt.Printf("pane split %q %s -> %q: not yet implemented\n", args[0], args[1], args[2])
		return nil
	},
}

var paneResizeCmd = &cobra.Command{
	Use:   "resize <pane-id> <left|right|up|down> <size>",
	Short: "Resize a pane (columns, Npx, N%)",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Root().PersistentFlags().GetInt("port")
		_ = port
		fmt.Printf("pane resize %q %s %q: not yet implemented\n", args[0], args[1], args[2])
		return nil
	},
}

var paneZoomCmd = &cobra.Command{
	Use:   "zoom <pane-id>",
	Short: "Toggle a pane between normal and full-window zoom",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Root().PersistentFlags().GetInt("port")
		_ = port
		fmt.Printf("pane zoom %q: not yet implemented\n", args[0])
		return nil
	},
}

var paneSwapCmd = &cobra.Command{
	Use:   "swap <pane-id-a> <pane-id-b>",
	Short: "Swap the positions of two panes",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Root().PersistentFlags().GetInt("port")
		_ = port
		fmt.Printf("pane swap %q %q: not yet implemented\n", args[0], args[1])
		return nil
	},
}

var paneCloseCmd = &cobra.Command{
	Use:   "close <pane-id>",
	Short: "Close a pane",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Root().PersistentFlags().GetInt("port")
		_ = port
		fmt.Printf("pane close %q: not yet implemented\n", args[0])
		return nil
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
		port, _ := cmd.Root().PersistentFlags().GetInt("port")
		name, _ := cmd.Flags().GetString("name")
		_ = port
		fmt.Printf("layout save %q: not yet implemented\n", name)
		return nil
	},
}

var layoutApplyCmd = &cobra.Command{
	Use:   "apply <name-or-path>",
	Short: "Apply a layout preset or file to the active session",
	Long: `Apply a layout to the active session.

The argument is either a saved preset name or a path to a YAML layout file.
If the layout is already active (same name or matching structure) it is reused
rather than recreated.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Root().PersistentFlags().GetInt("port")
		_ = port
		fmt.Printf("layout apply %q: not yet implemented\n", args[0])
		return nil
	},
}

var layoutListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved layout presets",
	RunE: func(cmd *cobra.Command, _ []string) error {
		port, _ := cmd.Root().PersistentFlags().GetInt("port")
		_ = port
		fmt.Println("layout list: not yet implemented")
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
		port, _ := cmd.Root().PersistentFlags().GetInt("port")
		_ = port
		fmt.Printf("kv set %q = %q: not yet implemented\n", args[0], args[1])
		return nil
	},
}

var kvGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Read a value from the KV store",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Root().PersistentFlags().GetInt("port")
		_ = port
		fmt.Printf("kv get %q: not yet implemented\n", args[0])
		return nil
	},
}

var kvDeleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Delete a key from the KV store",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Root().PersistentFlags().GetInt("port")
		_ = port
		fmt.Printf("kv delete %q: not yet implemented\n", args[0])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(kvCmd)
	kvCmd.AddCommand(kvSetCmd, kvGetCmd, kvDeleteCmd)
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

func init() {
	rootCmd.AddCommand(selftestCmd)
}

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
