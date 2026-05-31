package cmd

import (
	"github.com/spf13/cobra"

	"go-imap-cli/internal/mailbox"
)

var readFlags struct {
	folder      string
	uid         uint32
	includeHTML bool
}

var readCmd = &cobra.Command{
	Use:   "read",
	Short: "Read one full message by UID (does not mark it as seen)",
	Long: `Fetch a single message's headers, body text, and attachment metadata.

The message is fetched with PEEK so the \Seen flag is never set.

Example:
  imap-cli read --uid 4213 --folder INBOX`,
	RunE: func(cmd *cobra.Command, args []string) error {
		withClient(func(c *mailbox.Client) (any, error) {
			return c.Read(mailbox.ReadParams{
				Folder:      readFlags.folder,
				UID:         readFlags.uid,
				IncludeHTML: readFlags.includeHTML,
			})
		})
		return nil
	},
}

func init() {
	f := readCmd.Flags()
	f.StringVar(&readFlags.folder, "folder", "INBOX", "mailbox containing the message")
	f.Uint32Var(&readFlags.uid, "uid", 0, "UID of the message to read (required)")
	f.BoolVar(&readFlags.includeHTML, "include-html", false, "include the raw HTML body in addition to text")
	readCmd.MarkFlagRequired("uid")
}
