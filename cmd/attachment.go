package cmd

import (
	"github.com/spf13/cobra"

	"go-imap-cli/internal/mailbox"
)

var attachmentFlags struct {
	folder string
	uid    uint32
	index  int
	base64 bool
	outDir string
}

var attachmentCmd = &cobra.Command{
	Use:   "attachment",
	Short: "Download one attachment by UID and index (does not mark the message seen)",
	Long: `Fetch a single attachment from a message, located by its position in that
message's "attachments" list as returned by read (1-based: --index 1 is the
first attachment).

Only the attachment's MIME part is downloaded — not the message body or the
other attachments — and the message is never marked as seen.

By default the raw bytes are written to a file and the path is returned. Pass
--base64 to get the bytes inline in the JSON instead.

Examples:
  imap-cli attachment --uid 4213 --index 1
  imap-cli attachment --uid 4213 --index 1 --out-dir ~/Downloads
  imap-cli attachment --uid 4213 --index 1 --base64`,
	RunE: func(cmd *cobra.Command, args []string) error {
		withClient(func(c *mailbox.Client) (any, error) {
			return c.Attachment(mailbox.AttachmentParams{
				Folder: attachmentFlags.folder,
				UID:    attachmentFlags.uid,
				Index:  attachmentFlags.index,
				Base64: attachmentFlags.base64,
				OutDir: attachmentFlags.outDir,
			})
		})
		return nil
	},
}

func init() {
	f := attachmentCmd.Flags()
	f.StringVar(&attachmentFlags.folder, "folder", "INBOX", "mailbox containing the message")
	f.Uint32Var(&attachmentFlags.uid, "uid", 0, "`UID` of the message (required)")
	f.IntVar(&attachmentFlags.index, "index", 0, "1-based attachment number from read's attachments list (required)")
	f.BoolVar(&attachmentFlags.base64, "base64", false, "return raw bytes as base64 in JSON instead of saving a file")
	f.StringVar(&attachmentFlags.outDir, "out-dir", "", "directory to save into (default: <temp>/imap-cli)")
	attachmentCmd.MarkFlagRequired("uid")
	attachmentCmd.MarkFlagRequired("index")
}
