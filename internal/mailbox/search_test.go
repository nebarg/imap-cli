package mailbox

import (
	"testing"
	"time"
)

// TestOrCriteria checks the binary-OR folding for various term counts.
func TestOrCriteria(t *testing.T) {
	if got := orCriteria(nil); got != nil {
		t.Fatalf("empty terms: want nil, got %+v", got)
	}

	// Single term: no OR, just a Text match.
	one := orCriteria([]string{"order"})
	if one == nil || len(one.Text) != 1 || one.Text[0] != "order" || len(one.Or) != 0 {
		t.Fatalf("single term not folded to plain Text: %+v", one)
	}

	// Three terms: a -> OR(a, OR(b, c)).
	three := orCriteria([]string{"a", "b", "c"})
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

// TestBuildCriteriaCombinesFromAndOr verifies a From header AND an OR group
// coexist (the "orders from amazon" case).
func TestBuildCriteriaCombinesFromAndOr(t *testing.T) {
	c := buildCriteria(SearchParams{
		From: "amazon",
		Or:   []string{"order", "receipt"},
	})
	if len(c.Header) != 1 || c.Header[0].Key != "From" || c.Header[0].Value != "amazon" {
		t.Fatalf("From header missing/incorrect: %+v", c.Header)
	}
	if len(c.Or) != 1 {
		t.Fatalf("OR group should be ANDed into criteria, got %d Or pairs", len(c.Or))
	}
}

// TestFilterReceivedSince checks the exact sub-day trimming.
func TestFilterReceivedSince(t *testing.T) {
	now := time.Now()
	cutoff := now.Add(-24 * time.Hour)
	in := []MessageSummary{
		{UID: 1, Received: now.Add(-1 * time.Hour).Format(time.RFC3339)},  // keep
		{UID: 2, Received: now.Add(-48 * time.Hour).Format(time.RFC3339)}, // drop
		{UID: 3, Received: ""},                                            // keep (unknown)
	}
	got := filterReceivedSince(in, cutoff)
	if len(got) != 2 {
		t.Fatalf("want 2 kept, got %d: %+v", len(got), got)
	}
	if got[0].UID != 1 || got[1].UID != 3 {
		t.Fatalf("unexpected survivors: %+v", got)
	}
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
