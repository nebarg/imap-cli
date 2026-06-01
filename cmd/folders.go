package cmd

import (
	"github.com/spf13/cobra"

	"go-imap-cli/internal/mailbox"
)

var foldersAll bool

var foldersCmd = &cobra.Command{
	Use:   "folders",
	Short: "List selectable mailboxes/folders",
	Long: `List the account's mailboxes.

By default only selectable folders are shown. Unselectable folders are structural
containers (e.g. Gmail's "[Gmail]") that can't hold mail and can't be searched;
pass --all to include them.`,
	Run: func(cmd *cobra.Command, args []string) {
		withClient(func(c *mailbox.Client) (any, error) {
			return c.Folders(foldersAll)
		})
	},
}

func init() {
	foldersCmd.Flags().BoolVar(&foldersAll, "all", false, "include unselectable container folders")
}
