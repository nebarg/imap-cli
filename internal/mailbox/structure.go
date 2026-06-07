package mailbox

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime/quotedprintable"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-message/charset"
)

// bodyTarget identifies a text MIME part to fetch and how to decode it.
type bodyTarget struct {
	section  *imap.FetchItemBodySection // BODY.PEEK[path]
	encoding string                     // Content-Transfer-Encoding
	charset  string                     // charset param, if any
}

// planMessage walks a BODYSTRUCTURE (no payload downloaded) and decides which
// text parts to fetch and which parts are attachments. It picks the first
// non-attachment text/plain part and the first non-attachment text/html part;
// everything else (files, inline images, nested messages) is listed as an
// attachment with its size, which BODYSTRUCTURE reports for free.
func planMessage(bs imap.BodyStructure) (plain, html *bodyTarget, attachments []Attachment) {
	attachments = []Attachment{}
	bs.Walk(func(path []int, part imap.BodyStructure) bool {
		sp, ok := part.(*imap.BodyStructureSinglePart)
		if !ok {
			return true // multipart container: descend into its children
		}

		mediaType := strings.ToLower(sp.MediaType())
		disp := sp.Disposition()
		isAttachment := disp != nil && strings.EqualFold(disp.Value, "attachment")

		switch {
		case !isAttachment && mediaType == "text/plain" && plain == nil:
			plain = targetFor(path, sp)
		case !isAttachment && mediaType == "text/html" && html == nil:
			html = targetFor(path, sp)
		default:
			attachments = append(attachments, Attachment{
				Filename:    sp.Filename(),
				ContentType: mediaType,
				Size:        int64(sp.Size),
			})
		}
		return true
	})
	return plain, html, attachments
}

// targetFor builds a peeking section fetch for a text part.
func targetFor(path []int, sp *imap.BodyStructureSinglePart) *bodyTarget {
	// IMAP part paths are 1-based and copied because Walk reuses its buffer.
	part := append([]int(nil), path...)
	return &bodyTarget{
		section:  &imap.FetchItemBodySection{Part: part, Peek: true},
		encoding: sp.Encoding,
		charset:  paramCharset(sp.Params),
	}
}

// paramCharset reads the charset parameter case-insensitively.
func paramCharset(params map[string]string) string {
	for k, v := range params {
		if strings.EqualFold(k, "charset") {
			return v
		}
	}
	return ""
}

// decodeText turns a raw fetched part into a UTF-8 string, undoing its
// Content-Transfer-Encoding and charset. It is best-effort and tolerates
// truncated input (a partial fetch may cut base64/quoted-printable mid-token):
// on any decode error it uses whatever it managed to decode.
func decodeText(raw []byte, encoding, charsetName string) string {
	data := raw
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		cleaned := bytes.Map(func(r rune) rune {
			if r == '\r' || r == '\n' {
				return -1
			}
			return r
		}, raw)
		// Drop any trailing partial quantum so a truncated chunk still decodes.
		cleaned = cleaned[:len(cleaned)-len(cleaned)%4]
		if d, err := base64.StdEncoding.DecodeString(string(cleaned)); err == nil {
			data = d
		}
	case "quoted-printable":
		// io.ReadAll returns what it decoded even if it stops on a dangling
		// soft-break at a truncation boundary; use that rather than the raw.
		if d, _ := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(raw))); len(d) > 0 {
			data = d
		}
	}

	switch strings.ToLower(charsetName) {
	case "", "utf-8", "utf8", "us-ascii", "ascii":
		// already UTF-8 compatible
	default:
		if r, err := charset.Reader(charsetName, bytes.NewReader(data)); err == nil {
			if conv, err := io.ReadAll(r); err == nil {
				data = conv
			}
		}
	}
	return string(data)
}
