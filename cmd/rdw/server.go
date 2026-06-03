package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"text/tabwriter"
	"time"

	"github.com/nkh/rdw/internal/config"
	"github.com/nkh/rdw/internal/discovery"
	"github.com/nkh/rdw/internal/server"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Manage rdw server daemon instances",
	Long: `Manage rdw server daemon instances.

Multiple servers can run simultaneously on different ports. Use --port on
start to select the port; use --port on all other commands to target a
specific instance.`,
}

var serverStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start an rdw server daemon",
	Long: `Start an rdw server daemon on the specified port (default: 7681).

Multiple servers can run simultaneously on different ports. Each server
maintains its own session, KV store, layout, and token registry.

The server registers itself in the local server registry so that client
commands can discover it automatically when --port is omitted.

The admin console is restricted to loopback unless --network-expose is set.`,
	RunE: runServerStart,
}

var serverStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a running rdw server",
	Long: `Stop the rdw server on the target port.

If --port is omitted the default port (7681) is used. If multiple servers
are running and --port is not specified, an error lists the active instances.`,
	RunE: runServerStop,
}

var serverListCmd = &cobra.Command{
	Use:   "list",
	Short: "List running rdw server instances",
	RunE:  runServerList,
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.AddCommand(serverStartCmd)
	serverCmd.AddCommand(serverStopCmd)
	serverCmd.AddCommand(serverListCmd)

	serverStartCmd.Flags().Int("port", 0,
		"port to listen on (default: 7681)")
	serverStartCmd.Flags().Bool("network-expose", false,
		"bind to all interfaces instead of loopback only")
	serverStartCmd.Flags().Bool("no-auth", false,
		"disable token authentication (development only)")
	serverStartCmd.Flags().Bool("open-browser", false,
		"open the default browser after starting")
	serverStartCmd.Flags().Bool("restore", false,
		"restore the last saved session state on startup")
	serverStartCmd.Flags().String("kv-persist", "",
		"path to SQLite file for KV store persistence")
}

func runServerStart(cmd *cobra.Command, _ []string) error {
	cfgPath, _ := cmd.Root().PersistentFlags().GetString("config")

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Local --port overrides config; persistent --port is for client use.
	if port, _ := cmd.Flags().GetInt("port"); port != 0 {
		cfg.Server.Port = port
	}

	if v, _ := cmd.Flags().GetBool("network-expose"); v {
		cfg.Server.NetworkExpose = v
	}

	if v, _ := cmd.Flags().GetBool("no-auth"); v {
		cfg.Auth.NoAuth = v
	}

	if v, _ := cmd.Flags().GetString("kv-persist"); v != "" {
		cfg.KV.PersistPath = v
	}

	sessionID := fmt.Sprintf("%d", cfg.Server.Port)
	s := server.New(cfg, server.Options{SessionID: sessionID})

	return s.Run(context.Background())
}

func runServerStop(cmd *cobra.Command, _ []string) error {
	port, _ := cmd.Root().PersistentFlags().GetInt("port")

	resolvedPort, err := discovery.Resolve(port)
	if err != nil {
		return err
	}

	sockPath, err := unixSocketPath(fmt.Sprintf("%d", resolvedPort))
	if err != nil {
		return fmt.Errorf("resolving socket path: %w", err)
	}

	resp, err := sendUnixCommand(sockPath, server.UnixCommand{Action: "stop"})
	if err != nil {
		return fmt.Errorf("sending stop command: %w", err)
	}

	if !resp.OK {
		return fmt.Errorf("server error: %s", resp.Error)
	}

	fmt.Fprintf(os.Stdout, "rdw server on port %d stopping\n", resolvedPort)

	return nil
}

func runServerList(_ *cobra.Command, _ []string) error {
	// Prune stale entries first.
	_ = discovery.PruneStale()

	servers, err := discovery.ReadRegistry()
	if err != nil || len(servers) == 0 {
		fmt.Println("no rdw servers registered")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PORT\tPID\tSTARTED\tSOCKET")

	for _, s := range servers {
		fmt.Fprintf(w, "%d\t%d\t%s\t%s\n",
			s.Port, s.PID, s.StartedAt, s.SocketPath)
	}

	return w.Flush()
}

// sendUnixCommand connects to the server's Unix socket and sends a command.
func sendUnixCommand(socketPath string, cmd server.UnixCommand) (server.UnixResponse, error) {
	conn, err := net.DialTimeout("unix", socketPath, 3*time.Second)
	if err != nil {
		return server.UnixResponse{}, fmt.Errorf("connecting to %q: %w", socketPath, err)
	}
	defer conn.Close()

	if err = json.NewEncoder(conn).Encode(cmd); err != nil {
		return server.UnixResponse{}, fmt.Errorf("sending command: %w", err)
	}

	var resp server.UnixResponse
	if err = json.NewDecoder(conn).Decode(&resp); err != nil {
		return server.UnixResponse{}, fmt.Errorf("reading response: %w", err)
	}

	return resp, nil
}

// unixSocketPath returns the Unix socket path for the given session ID.
func unixSocketPath(sessionID string) (string, error) {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = os.TempDir()
	}
	return base + "/rdw/" + sessionID + ".sock", nil
}
