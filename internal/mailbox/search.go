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
	Text        string   // matches any header or the body
	Or          []string // matches if ANY term appears in any header or body
	Since       time.Time
	Before      time.Time
	SinceHours  int   // >0: only messages received within the last N hours (exact)
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

	// --since-hours needs an exact, sub-day window. IMAP SINCE is date-granular,
	// so it can only narrow the server-side scan to the relevant day(s); the
	// precise cutoff is applied client-side against each message's received time.
	var cutoff time.Time
	if p.SinceHours > 0 {
		cutoff = time.Now().Add(-time.Duration(p.SinceHours) * time.Hour)
		if criteria.Since.IsZero() || cutoff.After(criteria.Since) {
			criteria.Since = cutoff
		}
	}

	data, err := c.imap.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	uids := data.AllUIDs()
	// Newest first: higher UIDs are more recent within a mailbox.
	sort.Slice(uids, func(i, j int) bool { return uids[i] > uids[j] })

	// Without an exact cutoff we can page at the UID level before fetching, so we
	// only pull headers for the page we return. With a cutoff we must fetch the
	// candidates' received times first, trim, then page.
	if cutoff.IsZero() {
		uids = paginate(uids, p.Offset, p.Limit)
		return c.fetchSummaries(folder, uids, p.WithSnippet)
	}

	summaries, err := c.fetchSummaries(folder, uids, p.WithSnippet)
	if err != nil {
		return nil, err
	}
	summaries = filterReceivedSince(summaries, cutoff)
	return paginate(summaries, p.Offset, p.Limit), nil
}

// fetchSummaries fetches envelope/flags/size/received for the given UIDs and
// returns summaries sorted newest-first.
func (c *Client) fetchSummaries(folder string, uids []imap.UID, withSnippet bool) ([]MessageSummary, error) {
	if len(uids) == 0 {
		return []MessageSummary{}, nil
	}

	fetchOpts := &imap.FetchOptions{
		UID:          true,
		Envelope:     true,
		Flags:        true,
		RFC822Size:   true,
		InternalDate: true,
	}
	var bodySection *imap.FetchItemBodySection
	if withSnippet {
		bodySection = &imap.FetchItemBodySection{Peek: true}
		fetchOpts.BodySection = []*imap.FetchItemBodySection{bodySection}
	}

	msgs, err := c.imap.Fetch(imap.UIDSetNum(uids...), fetchOpts).Collect()
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
		if !m.InternalDate.IsZero() {
			s.Received = m.InternalDate.Format(time.RFC3339)
		}
		if m.Envelope != nil {
			s.From = toAddresses(m.Envelope.From)
			s.To = toAddresses(m.Envelope.To)
			s.Subject = m.Envelope.Subject
			if !m.Envelope.Date.IsZero() {
				s.Date = m.Envelope.Date.Format(time.RFC3339)
			}
		}
		if withSnippet && bodySection != nil {
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

// filterReceivedSince keeps summaries whose received time is at or after cutoff.
func filterReceivedSince(summaries []MessageSummary, cutoff time.Time) []MessageSummary {
	out := summaries[:0]
	for _, s := range summaries {
		// Keep messages whose received time is unknown rather than dropping them.
		if s.Received == "" {
			out = append(out, s)
			continue
		}
		t, err := time.Parse(time.RFC3339, s.Received)
		if err != nil || !t.Before(cutoff) {
			out = append(out, s)
		}
	}
	return out
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
	if oc := orCriteria(p.Or); oc != nil {
		c.And(oc)
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

// orCriteria folds N free-text terms into a single criteria that matches when
// ANY of them appears in any header or the body. IMAP's OR is binary, so the
// terms are combined right-to-left into a nested OR expression.
func orCriteria(terms []string) *imap.SearchCriteria {
	term := func(t string) imap.SearchCriteria {
		return imap.SearchCriteria{Text: []string{t}}
	}
	switch len(terms) {
	case 0:
		return nil
	case 1:
		c := term(terms[0])
		return &c
	}
	acc := term(terms[len(terms)-1])
	for i := len(terms) - 2; i >= 0; i-- {
		acc = imap.SearchCriteria{Or: [][2]imap.SearchCriteria{{term(terms[i]), acc}}}
	}
	return &acc
}

func paginate[T any](items []T, offset, limit int) []T {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(items) {
		return []T{}
	}
	items = items[offset:]
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	return items
}
