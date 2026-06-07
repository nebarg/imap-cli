package mailbox

import (
	"encoding/base64"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/emersion/go-imap/v2"
)

// AttachmentParams selects one attachment of one message and how to return it.
type AttachmentParams struct {
	Folder string
	UID    uint32
	Index  int  // 1-based position in the message's attachments list (from read)
	AsText bool // extract readable text (PDF/text) instead of raw bytes
	Base64 bool // return bytes inline as base64 instead of saving a file
	OutDir string
}

// AttachmentResult describes the extracted attachment. Exactly one of Path,
// Base64, or Text is populated, depending on the requested delivery. SizeBytes
// is the decoded byte count — the real file size, which differs from read's
// encoded_size_bytes for base64-encoded parts.
type AttachmentResult struct {
	UID         uint32 `json:"uid"`
	Index       int    `json:"index"`
	Filename    string `json:"filename,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	SizeBytes   int64  `json:"decoded_size_bytes"`
	Path        string `json:"path,omitempty"`
	Base64      string `json:"base64,omitempty"`
	Text        string `json:"text,omitempty"`
}

// Attachment fetches a single attachment by UID and index without marking the
// message seen and without downloading the message body or other attachments.
// Like read, it is two-phase: BODYSTRUCTURE to map the index to a MIME part,
// then a targeted fetch of just that part.
func (c *Client) Attachment(p AttachmentParams) (*AttachmentResult, error) {
	if p.AsText && p.Base64 {
		return nil, fmt.Errorf("--as-text and --base64 are mutually exclusive")
	}

	att, data, err := c.fetchAttachment(p)
	if err != nil {
		return nil, err
	}

	res := &AttachmentResult{
		UID:         p.UID,
		Index:       p.Index,
		Filename:    att.Filename,
		ContentType: att.ContentType,
		SizeBytes:   int64(len(data)),
	}
	switch {
	case p.AsText:
		text, err := extractText(att.ContentType, att.Filename, att.charset, data)
		if err != nil {
			return nil, err
		}
		res.Text = text
	case p.Base64:
		res.Base64 = base64.StdEncoding.EncodeToString(data)
	default:
		path, err := saveAttachment(p.OutDir, p.UID, p.Index, att.Filename, att.ContentType, data)
		if err != nil {
			return nil, err
		}
		res.Path = path
	}
	return res, nil
}

// fetchAttachment resolves the indexed attachment and returns its metadata and
// decoded bytes. Shared by the raw and (later) text-extraction paths.
func (c *Client) fetchAttachment(p AttachmentParams) (attachmentPart, []byte, error) {
	var zero attachmentPart
	folder := p.Folder
	if folder == "" {
		folder = "INBOX"
	}
	if p.Index < 1 {
		return zero, nil, fmt.Errorf("--index must be 1 or greater")
	}
	if _, err := c.imap.Select(folder, &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
		return zero, nil, fmt.Errorf("selecting %q: %w", folder, err)
	}

	uidSet := imap.UIDSetNum(imap.UID(p.UID))
	metaMsgs, err := c.imap.Fetch(uidSet, &imap.FetchOptions{
		UID:           true,
		BodyStructure: &imap.FetchItemBodyStructure{Extended: true},
	}).Collect()
	if err != nil {
		return zero, nil, fmt.Errorf("fetching structure: %w", err)
	}
	if len(metaMsgs) == 0 || metaMsgs[0].BodyStructure == nil {
		return zero, nil, fmt.Errorf("no message with UID %d in %q", p.UID, folder)
	}

	_, _, atts := planMessage(metaMsgs[0].BodyStructure)
	if len(atts) == 0 {
		return zero, nil, fmt.Errorf("message UID %d has no attachments", p.UID)
	}
	if p.Index > len(atts) {
		return zero, nil, fmt.Errorf("message UID %d has %d attachment(s); --index must be 1..%d", p.UID, len(atts), len(atts))
	}
	att := atts[p.Index-1]

	section := &imap.FetchItemBodySection{Part: att.path, Peek: true}
	bodyMsgs, err := c.imap.Fetch(uidSet, &imap.FetchOptions{
		BodySection: []*imap.FetchItemBodySection{section},
	}).Collect()
	if err != nil {
		return zero, nil, fmt.Errorf("fetching attachment: %w", err)
	}
	if len(bodyMsgs) == 0 {
		return zero, nil, fmt.Errorf("attachment body not returned for UID %d", p.UID)
	}
	raw := bodyMsgs[0].FindBodySection(section)
	if raw == nil {
		return zero, nil, fmt.Errorf("attachment part %v not returned for UID %d", att.path, p.UID)
	}
	return att, transferDecode(raw, att.encoding), nil
}

// saveAttachment writes the bytes under outDir (default <temp>/imap-cli),
// returning the file path. The name is prefixed with the UID and index to avoid
// collisions and sanitised to stay inside outDir.
func saveAttachment(outDir string, uid uint32, index int, filename, contentType string, data []byte) (string, error) {
	if outDir == "" {
		outDir = filepath.Join(os.TempDir(), "imap-cli")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("creating %q: %w", outDir, err)
	}
	name := safeFilename(filename)
	if name == "" {
		name = "attachment" + extForType(contentType)
	}
	dest := filepath.Join(outDir, fmt.Sprintf("%d-%d-%s", uid, index, name))
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return "", fmt.Errorf("writing %q: %w", dest, err)
	}
	return dest, nil
}

var unsafeFilenameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// safeFilename reduces an attacker-controllable attachment name to a bare,
// path-safe base name (no directory components, no traversal).
func safeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name)) // strip any path components
	if name == "." || name == ".." || name == string(filepath.Separator) {
		return ""
	}
	name = unsafeFilenameChars.ReplaceAllString(name, "_")
	return strings.Trim(name, "._")
}

// extForType picks a file extension for a content type when the attachment has
// no usable filename.
func extForType(contentType string) string {
	if mt, _, err := mime.ParseMediaType(contentType); err == nil {
		if exts, err := mime.ExtensionsByType(mt); err == nil && len(exts) > 0 {
			return exts[0]
		}
	}
	return ".bin"
}
