package mailbox

import (
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/jaytaylor/html2text"
)

// ReadParams describes a read of one or more messages by UID.
type ReadParams struct {
	Folder      string
	UIDs        []uint32
	IncludeHTML bool
}

// Read fetches one or more messages by UID without marking them as seen, and
// without downloading attachment payloads. It returns one ReadResult per
// requested UID, in request order; UIDs with no matching message come back
// with Found=false rather than being dropped.
//
// Reading is two-phase, by design: phase 1 fetches each message's envelope and
// BODYSTRUCTURE (metadata only, no bodies), so we learn the MIME layout and
// attachment sizes without transferring any attachment bytes. Phase 2 fetches
// only the text part(s) we actually want, by their part number. This trades a
// second round-trip for never pulling a multi-megabyte attachment we'd discard.
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

	// Phase 1: envelope + structure for every UID in one fetch. No bodies.
	metaOpts := &imap.FetchOptions{
		UID:           true,
		Envelope:      true,
		Flags:         true,
		RFC822Size:    true,
		BodyStructure: &imap.FetchItemBodyStructure{Extended: true},
	}
	metaMsgs, err := c.imap.Fetch(imap.UIDSetNum(uids...), metaOpts).Collect()
	if err != nil {
		return nil, fmt.Errorf("fetching structure: %w", err)
	}
	byUID := make(map[uint32]*imapclient.FetchMessageBuffer, len(metaMsgs))
	for _, m := range metaMsgs {
		byUID[uint32(m.UID)] = m
	}

	out := make([]ReadResult, 0, len(p.UIDs))
	for _, u := range p.UIDs {
		m, ok := byUID[u]
		if !ok {
			out = append(out, ReadResult{UID: u, Found: false})
			continue
		}
		msg, err := c.readMessage(folder, m, p.IncludeHTML)
		if err != nil {
			return nil, err
		}
		out = append(out, ReadResult{UID: u, Found: true, Message: msg})
	}
	return out, nil
}

// readMessage assembles a Message from its already-fetched metadata, then does
// the phase-2 fetch for just its text part(s).
func (c *Client) readMessage(folder string, m *imapclient.FetchMessageBuffer, includeHTML bool) (*Message, error) {
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

	if m.BodyStructure == nil {
		return msg, nil
	}
	plain, html, attachments := planMessage(m.BodyStructure)
	msg.Attachments = attachments

	// We want the HTML part when the caller asked for it, or as a fallback to
	// derive body text when there is no plain-text part.
	wantHTML := html != nil && (includeHTML || plain == nil)

	var sections []*imap.FetchItemBodySection
	if plain != nil {
		sections = append(sections, plain.section)
	}
	if wantHTML {
		sections = append(sections, html.section)
	}
	if len(sections) == 0 {
		return msg, nil // nothing textual to fetch (e.g. attachments only)
	}

	// Phase 2: fetch only the chosen text part(s). Peek keeps \Seen unset.
	bodyMsgs, err := c.imap.Fetch(imap.UIDSetNum(imap.UID(m.UID)), &imap.FetchOptions{BodySection: sections}).Collect()
	if err != nil {
		return nil, fmt.Errorf("fetching body of UID %d: %w", m.UID, err)
	}
	if len(bodyMsgs) == 0 {
		return msg, nil
	}
	bm := bodyMsgs[0]

	if plain != nil {
		if raw := bm.FindBodySection(plain.section); raw != nil {
			msg.BodyText = decodeText(raw, plain.encoding, plain.charset)
		}
	}
	var htmlText string
	if wantHTML {
		if raw := bm.FindBodySection(html.section); raw != nil {
			htmlText = decodeText(raw, html.encoding, html.charset)
		}
	}
	if includeHTML {
		msg.BodyHTML = htmlText
	}
	// Fall back to HTML-derived text when no plain-text part exists.
	if strings.TrimSpace(msg.BodyText) == "" && htmlText != "" {
		if txt, err := html2text.FromString(htmlText, html2text.Options{TextOnly: true}); err == nil {
			msg.BodyText = txt
		}
	}

	return msg, nil
}
