// Package cmd implements the rdw CLI using cobra.
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "rdw",
	Short: "Remote Display Web — pipe process output to a browser",
	Long: `rdw routes data streams from any process into named panes inside a
live browser layout. Each stream is identified by a Target ID; the server
multiplexes them into server-managed windows with optional filtering and
formatting.

Multiple rdw servers can run simultaneously on different ports. Use --port to
select a specific instance; when omitted, the default port (7681) is used.

See 'rdw <command> --help' for command-specific usage.`,
	SilenceUsage: true,
}

// Execute runs the root command and exits with a non-zero code on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringP("config", "c", "", "path to config file")
	rootCmd.PersistentFlags().IntP("port", "p", 0,
		"rdw server port to connect to (default: 7681; auto-detected if omitted)")
}
