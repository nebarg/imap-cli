package cmd

import (
	"github.com/spf13/cobra"

	"go-imap-cli/internal/mailbox"
)

var readFlags struct {
	folder      string
	uids        []uint
	includeHTML bool
}

var readCmd = &cobra.Command{
	Use:   "read",
	Short: "Read full messages by UID (does not mark them as seen)",
	Long: `Fetch each message's headers, body text, and attachment metadata.

Messages are fetched with PEEK so the \Seen flag is never set. Pass --uid more
than once (or as a comma list) to read several messages in a single round-trip.

The result is always a JSON array with one entry per requested UID, ordered to
match the UIDs you asked for. Each entry has "uid" and "found"; when found, the
full message is nested under "message". A UID with no matching message comes
back as {"uid": N, "found": false} rather than being dropped.

All UIDs are read from the same --folder.

Examples:
  imap-cli read --uid 4213 --folder INBOX
  imap-cli read --uid 4213 --uid 4214
  imap-cli read --uid 4213,4214,4220`,
	RunE: func(cmd *cobra.Command, args []string) error {
		uids := make([]uint32, len(readFlags.uids))
		for i, u := range readFlags.uids {
			uids[i] = uint32(u)
		}
		withClient(func(c *mailbox.Client) (any, error) {
			return c.Read(mailbox.ReadParams{
				Folder:      readFlags.folder,
				UIDs:        uids,
				IncludeHTML: readFlags.includeHTML,
			})
		})
		return nil
	},
}

func init() {
	f := readCmd.Flags()
	f.StringVar(&readFlags.folder, "folder", "INBOX", "mailbox containing the messages")
	f.UintSliceVar(&readFlags.uids, "uid", nil, "`UID` to read; repeatable or comma-separated (required)")
	f.BoolVar(&readFlags.includeHTML, "include-html", false, "include the raw HTML body in addition to text")
	readCmd.MarkFlagRequired("uid")
}
