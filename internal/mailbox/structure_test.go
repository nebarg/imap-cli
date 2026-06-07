package mailbox

import (
	"encoding/base64"
	"testing"

	"github.com/emersion/go-imap/v2"
)

func textPart(subtype, encoding, charset string) *imap.BodyStructureSinglePart {
	return &imap.BodyStructureSinglePart{
		Type:     "text",
		Subtype:  subtype,
		Encoding: encoding,
		Params:   map[string]string{"charset": charset},
	}
}

func filePart(typ, subtype, filename string, size uint32) *imap.BodyStructureSinglePart {
	return &imap.BodyStructureSinglePart{
		Type:    typ,
		Subtype: subtype,
		Size:    size,
		Extended: &imap.BodyStructureSinglePartExt{
			Disposition: &imap.BodyStructureDisposition{
				Value:  "attachment",
				Params: map[string]string{"filename": filename},
			},
		},
	}
}

// TestPlanAlternative: multipart/alternative picks plain at [1], html at [2].
func TestPlanAlternative(t *testing.T) {
	bs := &imap.BodyStructureMultiPart{
		Subtype: "alternative",
		Children: []imap.BodyStructure{
			textPart("plain", "7bit", "utf-8"),
			textPart("html", "7bit", "utf-8"),
		},
	}
	plain, html, atts := planMessage(bs)
	if plain == nil || html == nil {
		t.Fatalf("want both plain and html parts, got plain=%v html=%v", plain, html)
	}
	if got := plain.section.Part; len(got) != 1 || got[0] != 1 {
		t.Fatalf("plain part path = %v, want [1]", got)
	}
	if got := html.section.Part; len(got) != 1 || got[0] != 2 {
		t.Fatalf("html part path = %v, want [2]", got)
	}
	if len(atts) != 0 {
		t.Fatalf("alternative should have no attachments, got %v", atts)
	}
}

// TestPlanMixedWithAttachment: text body plus a PDF attachment is listed (with
// its size) but not treated as a body part.
func TestPlanMixedWithAttachment(t *testing.T) {
	bs := &imap.BodyStructureMultiPart{
		Subtype: "mixed",
		Children: []imap.BodyStructure{
			textPart("plain", "7bit", "utf-8"),
			filePart("application", "pdf", "invoice.pdf", 48213),
		},
	}
	plain, html, atts := planMessage(bs)
	if plain == nil {
		t.Fatal("want a plain part")
	}
	if html != nil {
		t.Fatalf("no html part expected, got %v", html)
	}
	if len(atts) != 1 {
		t.Fatalf("want 1 attachment, got %d", len(atts))
	}
	if atts[0].Filename != "invoice.pdf" || atts[0].ContentType != "application/pdf" || atts[0].Size != 48213 {
		t.Fatalf("attachment metadata wrong: %+v", atts[0])
	}
}

// TestPlanNestedAlternative: multipart/mixed wrapping a multipart/alternative
// resolves the text parts to nested paths and still lists the attachment.
func TestPlanNestedAlternative(t *testing.T) {
	bs := &imap.BodyStructureMultiPart{
		Subtype: "mixed",
		Children: []imap.BodyStructure{
			&imap.BodyStructureMultiPart{
				Subtype: "alternative",
				Children: []imap.BodyStructure{
					textPart("plain", "7bit", "utf-8"),
					textPart("html", "7bit", "utf-8"),
				},
			},
			filePart("image", "png", "logo.png", 1234),
		},
	}
	plain, html, atts := planMessage(bs)
	if plain == nil || html == nil {
		t.Fatal("want nested plain and html parts")
	}
	if got := plain.section.Part; len(got) != 2 || got[0] != 1 || got[1] != 1 {
		t.Fatalf("plain path = %v, want [1 1]", got)
	}
	if got := html.section.Part; len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("html path = %v, want [1 2]", got)
	}
	if len(atts) != 1 || atts[0].Filename != "logo.png" {
		t.Fatalf("want logo.png attachment, got %v", atts)
	}
}

// TestPlanSinglePart: a non-multipart text message has its body at [1].
func TestPlanSinglePart(t *testing.T) {
	plain, html, atts := planMessage(textPart("plain", "7bit", "utf-8"))
	if plain == nil {
		t.Fatal("want a plain part")
	}
	if got := plain.section.Part; len(got) != 1 || got[0] != 1 {
		t.Fatalf("singlepart path = %v, want [1]", got)
	}
	if html != nil || len(atts) != 0 {
		t.Fatalf("unexpected html/attachments: html=%v atts=%v", html, atts)
	}
}

func TestDecodeText(t *testing.T) {
	if got := decodeText([]byte("hello world"), "7bit", "utf-8"); got != "hello world" {
		t.Fatalf("7bit decode = %q", got)
	}

	b64 := base64.StdEncoding.EncodeToString([]byte("héllo"))
	if got := decodeText([]byte(b64), "base64", "utf-8"); got != "héllo" {
		t.Fatalf("base64 decode = %q, want héllo", got)
	}

	if got := decodeText([]byte("caf=C3=A9"), "quoted-printable", "utf-8"); got != "café" {
		t.Fatalf("quoted-printable decode = %q, want café", got)
	}
}
