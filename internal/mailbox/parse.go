package mailbox

import (
	"bytes"
	"io"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-message/mail"
	"github.com/jaytaylor/html2text"

	// Register additional charset decoders (e.g. ISO-8859-1) used by mail.
	_ "github.com/emersion/go-message/charset"
)

// toAddresses converts go-imap envelope addresses to JSON-friendly ones.
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

// parsedBody holds the decoded contents of a message body.
type parsedBody struct {
	text        string
	html        string
	attachments []Attachment
}

// parseBody decodes a raw RFC 822 message into plain text, HTML, and
// attachment metadata. When only HTML is present, text is derived from it.
func parseBody(raw []byte) (parsedBody, error) {
	var pb parsedBody

	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return pb, err
	}

	pb.attachments = []Attachment{}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		} else if err != nil {
			return pb, err
		}

		switch h := part.Header.(type) {
		case *mail.InlineHeader:
			ct, _, _ := h.ContentType()
			body, err := io.ReadAll(part.Body)
			if err != nil {
				return pb, err
			}
			switch {
			case strings.EqualFold(ct, "text/html"):
				if pb.html == "" {
					pb.html = string(body)
				}
			default: // text/plain and anything else inline
				if pb.text == "" {
					pb.text = string(body)
				}
			}
		case *mail.AttachmentHeader:
			filename, _ := h.Filename()
			ct, _, _ := h.ContentType()
			pb.attachments = append(pb.attachments, Attachment{
				Filename:    filename,
				ContentType: ct,
			})
		}
	}

	// Fall back to HTML-derived text when no plain-text part exists.
	if strings.TrimSpace(pb.text) == "" && pb.html != "" {
		if txt, err := html2text.FromString(pb.html, html2text.Options{TextOnly: true}); err == nil {
			pb.text = txt
		}
	}

	return pb, nil
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
