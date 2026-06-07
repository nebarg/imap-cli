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

// attachmentPart is a non-text MIME part: its public metadata plus the IMAP
// part path, transfer-encoding, and charset needed to fetch and decode it on
// demand. (charset only matters for text attachments extracted with --as-text.)
type attachmentPart struct {
	Attachment        // Filename, ContentType, Size
	path       []int  // IMAP part number, e.g. [2] or [1 2]
	encoding   string // Content-Transfer-Encoding
	charset    string // charset param, if any
}

// planMessage walks a BODYSTRUCTURE (no payload downloaded) and decides which
// text parts to fetch and which parts are attachments. It picks the first
// non-attachment text/plain part and the first non-attachment text/html part;
// everything else (files, inline images, nested messages) is listed as an
// attachment with its size, which BODYSTRUCTURE reports for free. The
// attachment order is the DFS order of the MIME tree, which is the order
// callers index by.
func planMessage(bs imap.BodyStructure) (plain, html *bodyTarget, attachments []attachmentPart) {
	attachments = []attachmentPart{}
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
			attachments = append(attachments, attachmentPart{
				Attachment: Attachment{
					Filename:    sp.Filename(),
					ContentType: mediaType,
					Size:        int64(sp.Size),
				},
				path:     append([]int(nil), path...), // copy: Walk reuses its buffer
				encoding: sp.Encoding,
				charset:  paramCharset(sp.Params),
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

// transferDecode undoes a part's Content-Transfer-Encoding (base64 or
// quoted-printable), returning the raw payload bytes. It is best-effort and
// tolerates truncated input (a partial fetch may cut a token mid-way): on a
// decode error it uses whatever it managed to decode.
func transferDecode(raw []byte, encoding string) []byte {
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
			return d
		}
	case "quoted-printable":
		// io.ReadAll returns what it decoded even if it stops on a dangling
		// soft-break at a truncation boundary; use that rather than the raw.
		if d, _ := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(raw))); len(d) > 0 {
			return d
		}
	}
	return raw
}

// charsetDecode converts already-transfer-decoded bytes from the named charset
// to UTF-8. UTF-8/ASCII pass through; an unknown charset or a decode error
// leaves the bytes as-is (best-effort).
func charsetDecode(data []byte, charsetName string) []byte {
	switch strings.ToLower(charsetName) {
	case "", "utf-8", "utf8", "us-ascii", "ascii":
		return data
	}
	if r, err := charset.Reader(charsetName, bytes.NewReader(data)); err == nil {
		if conv, err := io.ReadAll(r); err == nil {
			return conv
		}
	}
	return data
}

// decodeText turns a raw fetched part into a UTF-8 string, undoing its
// Content-Transfer-Encoding and charset. It is best-effort and tolerates
// truncated input.
func decodeText(raw []byte, encoding, charsetName string) string {
	return string(charsetDecode(transferDecode(raw, encoding), charsetName))
}
