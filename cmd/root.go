// Package cmd implements the imap-cli command-line interface.
package cmd

import (
	"github.com/spf13/cobra"
)

var (
	envPath     string
	accountName string
	timeoutSecs int
)

var rootCmd = &cobra.Command{
	Use:   "imap-cli",
	Short: "Read-only IMAP access that returns JSON, designed as an LLM tool",
	Long: `imap-cli connects to an IMAP server and prints results as JSON.

It is read-only: messages are never modified, deleted, or marked as seen.
Every command prints a single JSON object to stdout in the form
{"ok": true, "data": ...} or {"ok": false, "error": "..."}.

Configuration is read from a .env file (and the environment):
  IMAP_HOST, IMAP_PORT, IMAP_USERNAME, IMAP_PASSWORD, IMAP_TLS`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command. Errors that Cobra surfaces (bad flags,
// missing required flags, validation) are emitted as the same JSON error
// envelope used elsewhere, then the process exits non-zero — so every
// invocation yields parseable JSON on stdout.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fail(err)
	}
	return nil
}

func init() {
	pf := rootCmd.PersistentFlags()
	pf.StringVar(&envPath, "env", ".env", "path to the .env config file")
	pf.StringVar(&accountName, "account", "", "named account to use (reads IMAP_<NAME>_* vars)")
	pf.IntVar(&timeoutSecs, "timeout", 30, "connection timeout in seconds")

	rootCmd.AddCommand(foldersCmd, searchCmd, readCmd)
}
