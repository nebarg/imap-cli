package mailbox

import (
	"fmt"
	"time"

	"github.com/emersion/go-imap/v2"
)

// ReadParams describes a single-message read.
type ReadParams struct {
	Folder      string
	UID         uint32
	IncludeHTML bool
}

// Read fetches one full message by UID without marking it as seen.
func (c *Client) Read(p ReadParams) (*Message, error) {
	folder := p.Folder
	if folder == "" {
		folder = "INBOX"
	}
	if _, err := c.imap.Select(folder, &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
		return nil, fmt.Errorf("selecting %q: %w", folder, err)
	}

	uidSet := imap.UIDSetNum(imap.UID(p.UID))
	bodySection := &imap.FetchItemBodySection{Peek: true} // Peek: never set \Seen
	fetchOpts := &imap.FetchOptions{
		UID:         true,
		Envelope:    true,
		Flags:       true,
		RFC822Size:  true,
		BodySection: []*imap.FetchItemBodySection{bodySection},
	}

	msgs, err := c.imap.Fetch(uidSet, fetchOpts).Collect()
	if err != nil {
		return nil, fmt.Errorf("fetching message: %w", err)
	}
	if len(msgs) == 0 {
		return nil, fmt.Errorf("no message with UID %d in %q", p.UID, folder)
	}
	m := msgs[0]

	msg := &Message{
		UID:         uint32(m.UID),
		Folder:      folder,
		Size:        m.RFC822Size,
		Attachments: []Attachment{},
	}
	msg.Flags, msg.Seen = flagStrings(m.Flags)
	if m.Envelope != nil {
		msg.From = toAddresses(m.Envelope.From)
		msg.To = toAddresses(m.Envelope.To)
		msg.Cc = toAddresses(m.Envelope.Cc)
		msg.ReplyTo = toAddresses(m.Envelope.ReplyTo)
		msg.Subject = m.Envelope.Subject
		msg.MessageID = m.Envelope.MessageID
		if !m.Envelope.Date.IsZero() {
			msg.Date = m.Envelope.Date.Format(time.RFC3339)
		}
	}

	if raw := m.FindBodySection(bodySection); raw != nil {
		pb, err := parseBody(raw)
		if err != nil {
			return nil, fmt.Errorf("parsing body: %w", err)
		}
		msg.BodyText = pb.text
		if p.IncludeHTML {
			msg.BodyHTML = pb.html
		}
		msg.Attachments = pb.attachments
	}

	return msg, nil
}
