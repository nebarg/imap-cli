package mailbox

import (
	"fmt"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// ReadParams describes a read of one or more messages by UID.
type ReadParams struct {
	Folder      string
	UIDs        []uint32
	IncludeHTML bool
}

// Read fetches one or more full messages by UID without marking them as seen.
// It returns one ReadResult per requested UID, in request order; UIDs with no
// matching message come back with Found=false rather than being dropped.
func (c *Client) Read(p ReadParams) ([]ReadResult, error) {
	folder := p.Folder
	if folder == "" {
		folder = "INBOX"
	}
	if len(p.UIDs) == 0 {
		return nil, fmt.Errorf("no UIDs given")
	}
	if _, err := c.imap.Select(folder, &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
		return nil, fmt.Errorf("selecting %q: %w", folder, err)
	}

	uids := make([]imap.UID, len(p.UIDs))
	for i, u := range p.UIDs {
		uids[i] = imap.UID(u)
	}
	uidSet := imap.UIDSetNum(uids...)
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
		return nil, fmt.Errorf("fetching messages: %w", err)
	}

	// Servers return FETCH results in their own order, so index by UID and
	// emit in the order the caller requested.
	byUID := make(map[uint32]*Message, len(msgs))
	for _, m := range msgs {
		msg, err := c.buildMessage(m, folder, bodySection, p.IncludeHTML)
		if err != nil {
			return nil, err
		}
		byUID[msg.UID] = msg
	}

	out := make([]ReadResult, 0, len(p.UIDs))
	for _, u := range p.UIDs {
		if msg, ok := byUID[u]; ok {
			out = append(out, ReadResult{UID: u, Found: true, Message: msg})
		} else {
			out = append(out, ReadResult{UID: u, Found: false})
		}
	}
	return out, nil
}

// buildMessage converts a fetched message into a *Message, parsing its body.
func (c *Client) buildMessage(m *imapclient.FetchMessageBuffer, folder string, bodySection *imap.FetchItemBodySection, includeHTML bool) (*Message, error) {
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
			return nil, fmt.Errorf("parsing body of UID %d: %w", msg.UID, err)
		}
		msg.BodyText = pb.text
		if includeHTML {
			msg.BodyHTML = pb.html
		}
		msg.Attachments = pb.attachments
	}

	return msg, nil
}
