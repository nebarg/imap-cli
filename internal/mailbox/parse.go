package mailbox

import (
	"strings"

	"github.com/emersion/go-imap/v2"
)

// toAddresses converts go-imap envelope addresses to serialization-friendly ones.
func toAddresses(addrs []imap.Address) []Address {
	out := make([]Address, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, Address{Name: a.Name, Email: a.Addr()})
	}
	return out
}

// flagStrings converts IMAP flags to plain strings and reports whether the
// message carries the \Seen flag.
func flagStrings(flags []imap.Flag) ([]string, bool) {
	out := make([]string, 0, len(flags))
	seen := false
	for _, f := range flags {
		if f == imap.FlagSeen {
			seen = true
		}
		out = append(out, string(f))
	}
	return out, seen
}

// snippet returns a single-line preview of up to n runes.
func snippet(text string, n int) string {
	text = strings.Join(strings.Fields(text), " ")
	r := []rune(text)
	if len(r) <= n {
		return text
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}
