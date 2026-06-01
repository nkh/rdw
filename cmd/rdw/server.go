package cmd

import (
	"fmt"

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

	// --port on start overrides the persistent --port; use a local flag so the
	// help text reads naturally ("start on port X").
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
	port, _ := cmd.Flags().GetInt("port")
	_ = cfgPath
	_ = port

	fmt.Println("server start: not yet implemented")

	return nil
}

func runServerStop(cmd *cobra.Command, _ []string) error {
	port, _ := cmd.Root().PersistentFlags().GetInt("port")
	_ = port

	fmt.Println("server stop: not yet implemented")

	return nil
}

func runServerList(_ *cobra.Command, _ []string) error {
	fmt.Println("server list: not yet implemented")

	return nil
}
