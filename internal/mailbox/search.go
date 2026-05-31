package mailbox

import (
	"fmt"
	"sort"
	"time"

	"github.com/emersion/go-imap/v2"
)

// SearchParams describes a server-side IMAP search.
type SearchParams struct {
	Folder      string
	From        string
	To          string
	Subject     string
	Body        string
	Text        string // matches any header or the body
	Since       time.Time
	Before      time.Time
	Seen        *bool // nil: any; true: \Seen only; false: unseen only
	Flagged     *bool
	Limit       int
	Offset      int
	WithSnippet bool
}

const snippetLen = 200

// Search runs a read-only search and returns message summaries, newest first.
func (c *Client) Search(p SearchParams) ([]MessageSummary, error) {
	folder := p.Folder
	if folder == "" {
		folder = "INBOX"
	}
	if _, err := c.imap.Select(folder, &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
		return nil, fmt.Errorf("selecting %q: %w", folder, err)
	}

	criteria := buildCriteria(p)
	data, err := c.imap.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	uids := data.AllUIDs()
	// Newest first: higher UIDs are more recent within a mailbox.
	sort.Slice(uids, func(i, j int) bool { return uids[i] > uids[j] })

	uids = paginate(uids, p.Offset, p.Limit)
	if len(uids) == 0 {
		return []MessageSummary{}, nil
	}

	uidSet := imap.UIDSetNum(uids...)
	fetchOpts := &imap.FetchOptions{
		UID:        true,
		Envelope:   true,
		Flags:      true,
		RFC822Size: true,
	}

	var bodySection *imap.FetchItemBodySection
	if p.WithSnippet {
		bodySection = &imap.FetchItemBodySection{Peek: true}
		fetchOpts.BodySection = []*imap.FetchItemBodySection{bodySection}
	}

	msgs, err := c.imap.Fetch(uidSet, fetchOpts).Collect()
	if err != nil {
		return nil, fmt.Errorf("fetching summaries: %w", err)
	}

	summaries := make([]MessageSummary, 0, len(msgs))
	for _, m := range msgs {
		s := MessageSummary{
			UID:    uint32(m.UID),
			Folder: folder,
			Size:   m.RFC822Size,
		}
		s.Flags, s.Seen = flagStrings(m.Flags)
		if m.Envelope != nil {
			s.From = toAddresses(m.Envelope.From)
			s.To = toAddresses(m.Envelope.To)
			s.Subject = m.Envelope.Subject
			if !m.Envelope.Date.IsZero() {
				s.Date = m.Envelope.Date.Format(time.RFC3339)
			}
		}
		if p.WithSnippet && bodySection != nil {
			if raw := m.FindBodySection(bodySection); raw != nil {
				if pb, err := parseBody(raw); err == nil {
					s.Snippet = snippet(pb.text, snippetLen)
				}
			}
		}
		summaries = append(summaries, s)
	}

	// Fetch may return messages out of order; restore newest-first.
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].UID > summaries[j].UID })
	return summaries, nil
}

func buildCriteria(p SearchParams) *imap.SearchCriteria {
	c := &imap.SearchCriteria{}

	addHeader := func(key, val string) {
		if val != "" {
			c.Header = append(c.Header, imap.SearchCriteriaHeaderField{Key: key, Value: val})
		}
	}
	addHeader("From", p.From)
	addHeader("To", p.To)
	addHeader("Subject", p.Subject)

	if p.Body != "" {
		c.Body = append(c.Body, p.Body)
	}
	if p.Text != "" {
		c.Text = append(c.Text, p.Text)
	}
	if !p.Since.IsZero() {
		c.Since = p.Since
	}
	if !p.Before.IsZero() {
		c.Before = p.Before
	}
	if p.Seen != nil {
		if *p.Seen {
			c.Flag = append(c.Flag, imap.FlagSeen)
		} else {
			c.NotFlag = append(c.NotFlag, imap.FlagSeen)
		}
	}
	if p.Flagged != nil {
		if *p.Flagged {
			c.Flag = append(c.Flag, imap.FlagFlagged)
		} else {
			c.NotFlag = append(c.NotFlag, imap.FlagFlagged)
		}
	}
	return c
}

func paginate[T any](items []T, offset, limit int) []T {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(items) {
		return nil
	}
	items = items[offset:]
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	return items
}
