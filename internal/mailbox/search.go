package mailbox

import (
	"fmt"
	"sort"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/jaytaylor/html2text"
)

// SearchParams describes a server-side IMAP search.
//
// The text/header fields are slices: repeating a value ORs it within that
// field, while different fields are ANDed together. So From=[amazon,ebay] and
// Contains=[order] means (from amazon OR from ebay) AND contains order.
type SearchParams struct {
	Folders  []string // mailboxes to search; results are merged (any of)
	From     []string // matches the From header (any of)
	To       []string // matches the To header (any of)
	Subject  []string // matches the Subject header (any of)
	Body     []string // matches the body (any of)
	Contains []string // matches any header or the body
	// ContainsAll switches the Contains values from OR (match any, the default)
	// to AND (a message must match every term). Mixing AND and OR in one query
	// is intentionally not supported; run separate queries for that.
	ContainsAll bool
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

// Search runs a read-only search across one or more folders and returns message
// summaries newest-first. When a single folder is searched, results are ordered
// by UID (descending). When several folders are merged, UIDs are not comparable
// across mailboxes, so results are ordered by received time instead.
func (c *Client) Search(p SearchParams) ([]MessageSummary, error) {
	folders := p.Folders
	if len(folders) == 0 {
		folders = []string{"INBOX"}
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

	// Without an exact cutoff we can bound each folder's fetch to the page we
	// might return, instead of pulling every matching header.
	maxFetch := 0
	if cutoff.IsZero() && p.Limit > 0 {
		maxFetch = p.Offset + p.Limit
	}

	var all []MessageSummary
	for _, folder := range folders {
		summaries, err := c.searchFolder(folder, criteria, cutoff, p.WithSnippet, maxFetch)
		if err != nil {
			return nil, err
		}
		all = append(all, summaries...)
	}

	// searchFolder already orders each folder's results newest-first by UID.
	// Across folders that ordering is meaningless, so re-sort by received time.
	if len(folders) > 1 {
		sort.SliceStable(all, func(i, j int) bool {
			return summaryTime(all[i]).After(summaryTime(all[j]))
		})
	}

	return paginate(all, p.Offset, p.Limit), nil
}

// searchFolder selects one folder read-only, runs the search, and returns its
// summaries newest-first by UID (cutoff-filtered when cutoff is non-zero). cap,
// when > 0, limits how many of the newest matches are fetched.
func (c *Client) searchFolder(folder string, criteria *imap.SearchCriteria, cutoff time.Time, withSnippet bool, maxFetch int) ([]MessageSummary, error) {
	if _, err := c.imap.Select(folder, &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
		return nil, fmt.Errorf("selecting %q: %w", folder, err)
	}

	data, err := c.imap.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("search in %q failed: %w", folder, err)
	}

	uids := data.AllUIDs()
	// Newest first: higher UIDs are more recent within a mailbox.
	sort.Slice(uids, func(i, j int) bool { return uids[i] > uids[j] })
	if maxFetch > 0 && len(uids) > maxFetch {
		uids = uids[:maxFetch]
	}

	summaries, err := c.fetchSummaries(folder, uids, withSnippet)
	if err != nil {
		return nil, err
	}
	if !cutoff.IsZero() {
		summaries = filterReceivedSince(summaries, cutoff)
	}
	return summaries, nil
}

// summaryTime returns the best available timestamp for ordering: the server
// received time, falling back to the sender's Date, then the zero time.
func summaryTime(s MessageSummary) time.Time {
	for _, v := range []string{s.Received, s.Date} {
		if v == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
	}
	return time.Time{}
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
	if withSnippet {
		// Fetch the MIME structure, not the bodies, so the snippet phase can
		// pull just the text part and never the attachments.
		fetchOpts.BodyStructure = &imap.FetchItemBodyStructure{Extended: true}
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
		summaries = append(summaries, s)
	}

	if withSnippet {
		if err := c.addSnippets(msgs, summaries); err != nil {
			return nil, err
		}
	}

	// Fetch may return messages out of order; restore newest-first.
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].UID > summaries[j].UID })
	return summaries, nil
}

type snippetPlan struct {
	section  *imap.FetchItemBodySection
	encoding string
	charset  string
	isHTML   bool
}

// snippetGroup is a set of UIDs whose text body lives at the same part path, so
// they can be fetched together with one section.
type snippetGroup struct {
	section *imap.FetchItemBodySection
	uids    []imap.UID
}

// addSnippets fills MessageSummary.Snippet by fetching only each message's text
// part, located by part number from its already-fetched BODYSTRUCTURE, so
// attachment payloads are never downloaded for a preview.
//
// Messages are grouped by their text-part path and each group is fetched for
// just its own UIDs. We deliberately do NOT fetch the union of all paths for
// all UIDs: part [2] may be text in one message but a PDF in another, so a union
// fetch would pull that PDF — the very waste we're avoiding. The number of
// round-trips is the number of distinct text-part paths, usually one or two.
//
// We fetch the whole text part rather than a leading slice: HTML emails often
// begin with kilobytes of <style>/preamble, so a short prefix yields no visible
// text after stripping. The text part alone is small next to the attachments we
// already skip.
func (c *Client) addSnippets(meta []*imapclient.FetchMessageBuffer, summaries []MessageSummary) error {
	plans := make(map[uint32]snippetPlan, len(meta))
	groups := make(map[string]*snippetGroup)
	for _, m := range meta {
		if m.BodyStructure == nil {
			continue
		}
		plain, html, _ := planMessage(m.BodyStructure)
		t, isHTML := plain, false
		if t == nil {
			t, isHTML = html, true
		}
		if t == nil {
			continue // nothing textual (e.g. attachments only)
		}
		key := fmt.Sprint(t.section.Part)
		g, ok := groups[key]
		if !ok {
			g = &snippetGroup{section: &imap.FetchItemBodySection{Part: t.section.Part, Peek: true}}
			groups[key] = g
		}
		g.uids = append(g.uids, m.UID)
		plans[uint32(m.UID)] = snippetPlan{section: g.section, encoding: t.encoding, charset: t.charset, isHTML: isHTML}
	}
	if len(groups) == 0 {
		return nil
	}

	bodyByUID := make(map[uint32]*imapclient.FetchMessageBuffer, len(plans))
	for _, g := range groups {
		bodyMsgs, err := c.imap.Fetch(imap.UIDSetNum(g.uids...), &imap.FetchOptions{
			BodySection: []*imap.FetchItemBodySection{g.section},
		}).Collect()
		if err != nil {
			return fmt.Errorf("fetching snippet bodies: %w", err)
		}
		for _, m := range bodyMsgs {
			bodyByUID[uint32(m.UID)] = m
		}
	}
	idxByUID := make(map[uint32]int, len(summaries))
	for i, s := range summaries {
		idxByUID[s.UID] = i
	}

	for uid, pl := range plans {
		bm := bodyByUID[uid]
		if bm == nil {
			continue
		}
		raw := bm.FindBodySection(pl.section)
		if raw == nil {
			continue
		}
		text := decodeText(raw, pl.encoding, pl.charset)
		if pl.isHTML {
			if txt, err := html2text.FromString(text, html2text.Options{TextOnly: true}); err == nil {
				text = txt
			}
		}
		if i, ok := idxByUID[uid]; ok {
			summaries[i].Snippet = snippet(text, snippetLen)
		}
	}
	return nil
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

	// Each field's values are ORed together, then ANDed into the criteria.
	andField := func(values []string, mk func(string) imap.SearchCriteria) {
		if g := orField(values, mk); g != nil {
			c.And(g)
		}
	}
	header := func(key string) func(string) imap.SearchCriteria {
		return func(v string) imap.SearchCriteria {
			return imap.SearchCriteria{Header: []imap.SearchCriteriaHeaderField{{Key: key, Value: v}}}
		}
	}
	andField(p.From, header("From"))
	andField(p.To, header("To"))
	andField(p.Subject, header("Subject"))
	andField(p.Body, func(v string) imap.SearchCriteria {
		return imap.SearchCriteria{Body: []string{v}}
	})
	if p.ContainsAll {
		// Multiple TEXT keys are ANDed by the server, so a message must contain
		// every term.
		c.Text = append(c.Text, p.Contains...)
	} else {
		andField(p.Contains, func(v string) imap.SearchCriteria {
			return imap.SearchCriteria{Text: []string{v}}
		})
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

// orField folds N values for one field into a single criteria that matches when
// ANY of them matches, using mk to build the per-value criteria. IMAP's OR is
// binary, so values are combined right-to-left into a nested OR expression. A
// single value yields a plain (un-ORed) criteria.
func orField(values []string, mk func(string) imap.SearchCriteria) *imap.SearchCriteria {
	switch len(values) {
	case 0:
		return nil
	case 1:
		c := mk(values[0])
		return &c
	}
	acc := mk(values[len(values)-1])
	for i := len(values) - 2; i >= 0; i-- {
		acc = imap.SearchCriteria{Or: [][2]imap.SearchCriteria{{mk(values[i]), acc}}}
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
