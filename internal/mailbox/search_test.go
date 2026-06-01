package mailbox

import (
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
)

// TestOrField checks the binary-OR folding for various value counts.
func TestOrField(t *testing.T) {
	text := func(v string) imap.SearchCriteria {
		return imap.SearchCriteria{Text: []string{v}}
	}

	if got := orField(nil, text); got != nil {
		t.Fatalf("empty values: want nil, got %+v", got)
	}

	// Single value: no OR, just a plain match.
	one := orField([]string{"order"}, text)
	if one == nil || len(one.Text) != 1 || one.Text[0] != "order" || len(one.Or) != 0 {
		t.Fatalf("single value not folded to plain criteria: %+v", one)
	}

	// Three values: a -> OR(a, OR(b, c)).
	three := orField([]string{"a", "b", "c"}, text)
	if len(three.Or) != 1 {
		t.Fatalf("want one top-level OR pair, got %d", len(three.Or))
	}
	left, right := three.Or[0][0], three.Or[0][1]
	if len(left.Text) != 1 || left.Text[0] != "a" {
		t.Fatalf("left branch should be 'a', got %+v", left)
	}
	if len(right.Or) != 1 {
		t.Fatalf("right branch should nest another OR, got %+v", right)
	}
	inL, inR := right.Or[0][0], right.Or[0][1]
	if inL.Text[0] != "b" || inR.Text[0] != "c" {
		t.Fatalf("nested OR should be (b, c), got (%v, %v)", inL.Text, inR.Text)
	}
}

// TestBuildCriteriaCombinesFields verifies that a single From header value and
// a multi-value Contains group coexist as From AND (a OR b) — the "orders from
// amazon" case.
func TestBuildCriteriaCombinesFields(t *testing.T) {
	c := buildCriteria(SearchParams{
		From:     []string{"amazon"},
		Contains: []string{"order", "receipt"},
	})
	if len(c.Header) != 1 || c.Header[0].Key != "From" || c.Header[0].Value != "amazon" {
		t.Fatalf("From header missing/incorrect: %+v", c.Header)
	}
	if len(c.Or) != 1 {
		t.Fatalf("Contains OR group should be ANDed into criteria, got %d Or pairs", len(c.Or))
	}
}

// TestBuildCriteriaOrsRepeatedFrom verifies repeating --from ORs the senders.
func TestBuildCriteriaOrsRepeatedFrom(t *testing.T) {
	c := buildCriteria(SearchParams{From: []string{"amazon", "ebay"}})
	if len(c.Or) != 1 {
		t.Fatalf("two --from values should produce one OR pair, got %d", len(c.Or))
	}
	if len(c.Header) != 0 {
		t.Fatalf("ORed From should live under Or, not Header: %+v", c.Header)
	}
}

// TestFilterReceivedSince checks the exact sub-day trimming.
func TestFilterReceivedSince(t *testing.T) {
	now := time.Now()
	cutoff := now.Add(-24 * time.Hour)
	in := []MessageSummary{
		{UID: 1, Received: now.Add(-1 * time.Hour).Format(time.RFC3339)},  // keep
		{UID: 2, Received: now.Add(-48 * time.Hour).Format(time.RFC3339)}, // drop
		{UID: 3, Received: ""}, // keep (unknown)
	}
	got := filterReceivedSince(in, cutoff)
	if len(got) != 2 {
		t.Fatalf("want 2 kept, got %d: %+v", len(got), got)
	}
	if got[0].UID != 1 || got[1].UID != 3 {
		t.Fatalf("unexpected survivors: %+v", got)
	}
}

// TestSummaryTime checks the ordering key used when merging folders: received
// time preferred, then Date, then zero.
func TestSummaryTime(t *testing.T) {
	rcv := "2026-05-30T10:00:00Z"
	date := "2026-05-29T10:00:00Z"

	if got := summaryTime(MessageSummary{Received: rcv, Date: date}); !got.Equal(mustTime(t, rcv)) {
		t.Fatalf("received should win, got %v", got)
	}
	if got := summaryTime(MessageSummary{Date: date}); !got.Equal(mustTime(t, date)) {
		t.Fatalf("should fall back to Date, got %v", got)
	}
	if got := summaryTime(MessageSummary{}); !got.IsZero() {
		t.Fatalf("no timestamps should yield zero time, got %v", got)
	}

	// A newer received time must sort ahead of an older one.
	newer := MessageSummary{Received: "2026-05-31T00:00:00Z"}
	older := MessageSummary{Received: "2026-05-01T00:00:00Z"}
	if !summaryTime(newer).After(summaryTime(older)) {
		t.Fatal("newer received time should sort after-in-time (i.e. ahead when descending)")
	}
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("bad test time %q: %v", s, err)
	}
	return parsed
}

func TestPaginate(t *testing.T) {
	items := []int{0, 1, 2, 3, 4}
	if got := paginate(items, 1, 2); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("offset/limit window wrong: %v", got)
	}
	if got := paginate(items, 10, 2); len(got) != 0 {
		t.Fatalf("offset past end should be empty, got %v", got)
	}
	if got := paginate(items, 0, 0); len(got) != 5 {
		t.Fatalf("limit 0 should return all, got %v", got)
	}
}
