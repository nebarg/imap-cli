package cmd

import (
	"github.com/spf13/cobra"

	"go-imap-cli/internal/mailbox"
)

var foldersCmd = &cobra.Command{
	Use:   "folders",
	Short: "List available mailboxes/folders",
	Run: func(cmd *cobra.Command, args []string) {
		withClient(func(c *mailbox.Client) (any, error) {
			return c.Folders()
		})
	},
}
