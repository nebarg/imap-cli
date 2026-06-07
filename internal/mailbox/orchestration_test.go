package mailbox

import (
	"slices"
	"testing"

	"github.com/emersion/go-imap/v2"
)

// mixedWithPDF is multipart/mixed: [1] text/plain body, [2] PDF attachment.
func mixedWithPDF() imap.BodyStructure {
	return &imap.BodyStructureMultiPart{
		Subtype: "mixed",
		Children: []imap.BodyStructure{
			textSP("plain"),
			fileSP("application", "pdf", "invoice.pdf", "base64", 4000),
		},
	}
}

func TestReadMultiUIDOrderAndNotFound(t *testing.T) {
	fake := &fakeSession{
		fetches: []fetchResponse{
			// Phase 1: structure for UID 10 only; 12 is missing entirely.
			{msgs: []*imapMsg{structMsg(10, "Hello", mixedWithPDF())}},
			// Phase 2: the text part [1] for UID 10.
			{msgs: []*imapMsg{bodyMsg(10, sectionBytes([]int{1}, "the body text"))}},
		},
	}
	c := &Client{sess: fake}

	out, err := c.Read(ReadParams{Folder: "INBOX", UIDs: []uint32{10, 12}})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 results, got %d", len(out))
	}
	// Results stay in requested order.
	if out[0].UID != 10 || !out[0].Found {
		t.Fatalf("result[0] = %+v, want UID 10 found", out[0])
	}
	if out[1].UID != 12 || out[1].Found {
		t.Fatalf("result[1] = %+v, want UID 12 not found", out[1])
	}
	if got := out[0].Message.BodyText; got != "the body text" {
		t.Fatalf("body text = %q", got)
	}
	if n := len(out[0].Message.Attachments); n != 1 || out[0].Message.Attachments[0].Filename != "invoice.pdf" {
		t.Fatalf("attachments = %+v", out[0].Message.Attachments)
	}

	// The PDF lives at part [2]; read must never have fetched it.
	if paths := fake.requestedBodyPaths(); slices.Contains(paths, "[2]") {
		t.Fatalf("read fetched the attachment part [2]; body fetches were %v", paths)
	}
}

func TestAttachmentIndexMapsToPart(t *testing.T) {
	// [1] text body, [2] PDF (index 1), [3] PNG (index 2).
	bs := &imap.BodyStructureMultiPart{
		Subtype: "mixed",
		Children: []imap.BodyStructure{
			textSP("plain"),
			fileSP("application", "pdf", "invoice.pdf", "base64", 4000),
			fileSP("image", "png", "logo.png", "base64", 1000),
		},
	}
	// "PNGDATA" base64-encoded is the body the server returns for part [3].
	fake := &fakeSession{
		fetches: []fetchResponse{
			{msgs: []*imapMsg{structMsg(10, "Hello", bs)}},
			{msgs: []*imapMsg{bodyMsg(10, sectionBytes([]int{3}, "UE5HREFUQQ=="))}},
		},
	}
	c := &Client{sess: fake}

	res, err := c.Attachment(AttachmentParams{Folder: "INBOX", UID: 10, Index: 2, Base64: true})
	if err != nil {
		t.Fatalf("Attachment: %v", err)
	}
	if res.ContentType != "image/png" || res.Filename != "logo.png" {
		t.Fatalf("wrong attachment selected: %+v", res)
	}
	// base64 of the decoded "PNGDATA" round-trips.
	if res.Base64 != "UE5HREFUQQ==" || res.SizeBytes != int64(len("PNGDATA")) {
		t.Fatalf("decoded bytes wrong: base64=%q size=%d", res.Base64, res.SizeBytes)
	}
	if paths := fake.requestedBodyPaths(); !slices.Contains(paths, "[3]") || slices.Contains(paths, "[2]") {
		t.Fatalf("index 2 should fetch part [3] only; fetched %v", paths)
	}
}

func TestAttachmentIndexErrors(t *testing.T) {
	// Out of range: message has one attachment, ask for index 5.
	fake := &fakeSession{fetches: []fetchResponse{
		{msgs: []*imapMsg{structMsg(10, "Hello", mixedWithPDF())}},
	}}
	if _, err := (&Client{sess: fake}).Attachment(AttachmentParams{UID: 10, Index: 5, Base64: true}); err == nil {
		t.Fatal("out-of-range index should error")
	}

	// No attachments: a plain single-part message.
	fake2 := &fakeSession{fetches: []fetchResponse{
		{msgs: []*imapMsg{structMsg(10, "Hello", textSP("plain"))}},
	}}
	if _, err := (&Client{sess: fake2}).Attachment(AttachmentParams{UID: 10, Index: 1, Base64: true}); err == nil {
		t.Fatal("no-attachment message should error")
	}
}

func TestSearchSnippetSkipsAttachment(t *testing.T) {
	fake := &fakeSession{
		searchUIDs: []imap.UID{20},
		fetches: []fetchResponse{
			// Phase 1: summary + structure.
			{msgs: []*imapMsg{structMsg(20, "Subject", mixedWithPDF())}},
			// Phase 2: snippet group for the text part [1].
			{msgs: []*imapMsg{bodyMsg(20, sectionBytes([]int{1}, "preview body content"))}},
		},
	}
	c := &Client{sess: fake}

	got, err := c.Search(SearchParams{Folders: []string{"INBOX"}, WithSnippet: true, Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 summary, got %d", len(got))
	}
	if got[0].Snippet != "preview body content" {
		t.Fatalf("snippet = %q", got[0].Snippet)
	}
	if paths := fake.requestedBodyPaths(); slices.Contains(paths, "[2]") {
		t.Fatalf("snippet fetched the attachment part [2]; fetched %v", paths)
	}
}
