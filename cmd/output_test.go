package cmd

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	toon "github.com/toon-format/toon-go"

	"go-imap-cli/internal/mailbox"
)

// sampleResponse is a realistic payload exercising nested objects, arrays of
// objects, embedded newlines, delimiter characters, and omitempty fields.
func sampleResponse() response {
	return response{OK: true, Data: []mailbox.ReadResult{
		{
			UID:   4213,
			Found: true,
			Message: &mailbox.Message{
				UID:       4213,
				Folder:    "INBOX",
				From:      []mailbox.Address{{Name: "Jones, Bob", Email: "bob@example.com"}},
				To:        []mailbox.Address{{Email: "me@example.com"}},
				Subject:   "Re: order #12345",
				Date:      "2026-08-01T10:04:05Z",
				MessageID: "<abc@example.com>",
				Flags:     []string{"\\Seen"},
				Seen:      true,
				Size:      24193,
				BodyText:  "Hi,\n\nYour order shipped.\n\nThanks",
				Attachments: []mailbox.Attachment{
					{Filename: "invoice.pdf", ContentType: "application/pdf", Size: 81876},
				},
			},
		},
		{UID: 4214, Found: false},
	}}
}

// TestResponseTagParity guards the invariant that makes --format toon safe: the
// `toon` tag on every field reachable from a response must match its `json`
// tag, so both encodings name and omit exactly the same fields.
func TestResponseTagParity(t *testing.T) {
	roots := []any{
		response{},
		[]mailbox.Folder{},
		[]mailbox.MessageSummary{},
		[]mailbox.ReadResult{},
		mailbox.AttachmentResult{},
	}

	seen := map[reflect.Type]bool{}
	var walk func(reflect.Type)
	walk = func(rt reflect.Type) {
		for rt.Kind() == reflect.Pointer || rt.Kind() == reflect.Slice || rt.Kind() == reflect.Array {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct || seen[rt] {
			return
		}
		seen[rt] = true
		for i := range rt.NumField() {
			f := rt.Field(i)
			if !f.IsExported() {
				continue
			}
			jsonTag, toonTag := f.Tag.Get("json"), f.Tag.Get("toon")
			if jsonTag == "" {
				t.Errorf("%s.%s: missing json tag", rt.Name(), f.Name)
			}
			if jsonTag != toonTag {
				t.Errorf("%s.%s: json tag %q != toon tag %q", rt.Name(), f.Name, jsonTag, toonTag)
			}
			walk(f.Type)
		}
	}
	for _, root := range roots {
		walk(reflect.TypeOf(root))
	}

	if !seen[reflect.TypeOf(mailbox.Message{})] {
		t.Fatal("walk did not reach Message; the seed types no longer cover the response payloads")
	}
}

// TestEncodeResponseTOONMatchesJSON is the semantic check behind the tag
// parity: decoding the TOON encoding must yield exactly the data the JSON
// encoding carries. This covers quoting of commas, newlines, and backslashes.
func TestEncodeResponseTOONMatchesJSON(t *testing.T) {
	r := sampleResponse()

	var jsonBuf, toonBuf bytes.Buffer
	if err := encodeResponse(&jsonBuf, formatJSON, r); err != nil {
		t.Fatalf("encode json: %v", err)
	}
	if err := encodeResponse(&toonBuf, formatTOON, r); err != nil {
		t.Fatalf("encode toon: %v", err)
	}

	var fromJSON any
	if err := json.Unmarshal(jsonBuf.Bytes(), &fromJSON); err != nil {
		t.Fatalf("json output is not valid JSON: %v", err)
	}
	fromTOON, err := toon.Decode(toonBuf.Bytes())
	if err != nil {
		t.Fatalf("toon output is not valid TOON: %v\n%s", err, toonBuf.String())
	}

	if !reflect.DeepEqual(fromJSON, fromTOON) {
		t.Fatalf("TOON and JSON encodings disagree\njson: %#v\ntoon: %#v", fromJSON, fromTOON)
	}
}

// TestEncodeResponseTOONIsSmaller records the point of the feature: TOON should
// be materially more compact than indented JSON for list-shaped results.
func TestEncodeResponseTOONIsSmaller(t *testing.T) {
	r := response{OK: true, Data: []mailbox.Folder{
		{Name: "INBOX", Role: "inbox", Selectable: true, Delimiter: "/", Flags: []string{"\\HasNoChildren"}},
		{Name: "Sent", Role: "sent", Selectable: true, Delimiter: "/", Flags: []string{"\\HasNoChildren"}},
		{Name: "Archive", Role: "archive", Selectable: true, Delimiter: "/", Flags: []string{"\\HasNoChildren"}},
	}}

	var jsonBuf, toonBuf bytes.Buffer
	if err := encodeResponse(&jsonBuf, formatJSON, r); err != nil {
		t.Fatalf("encode json: %v", err)
	}
	if err := encodeResponse(&toonBuf, formatTOON, r); err != nil {
		t.Fatalf("encode toon: %v", err)
	}

	if toonBuf.Len() >= jsonBuf.Len() {
		t.Errorf("toon (%d bytes) is not smaller than json (%d bytes)", toonBuf.Len(), jsonBuf.Len())
	}

	// Folders do not collapse into TOON's tabular form because `flags` is a
	// nested array; only all-primitive objects qualify. Attachments do, and
	// that is where the format is at its most compact.
	tabular := response{OK: true, Data: []mailbox.Attachment{
		{Filename: "invoice.pdf", ContentType: "application/pdf", Size: 81876},
		{Filename: "notes.txt", ContentType: "text/plain", Size: 214},
	}}
	var tabularBuf bytes.Buffer
	if err := encodeResponse(&tabularBuf, formatTOON, tabular); err != nil {
		t.Fatalf("encode toon: %v", err)
	}
	if !strings.Contains(tabularBuf.String(), "{filename,content_type,encoded_size_bytes}") {
		t.Errorf("expected a tabular header for uniform attachments, got:\n%s", tabularBuf.String())
	}
}

// TestEncodeResponseError checks the failure envelope encodes in both formats.
func TestEncodeResponseError(t *testing.T) {
	r := response{OK: false, Error: "connect: no such host"}

	for _, format := range []string{formatJSON, formatTOON} {
		var buf bytes.Buffer
		if err := encodeResponse(&buf, format, r); err != nil {
			t.Fatalf("%s: %v", format, err)
		}
		out := buf.String()
		if !strings.Contains(out, "no such host") {
			t.Errorf("%s: error text missing from output: %s", format, out)
		}
		// data is omitted, so ok must be the only other field.
		if strings.Contains(out, "data") {
			t.Errorf("%s: empty data should be omitted: %s", format, out)
		}
	}
}

// TestUnknownFormat covers both guards: flag validation, and encodeResponse
// refusing a format it does not know rather than silently emitting JSON.
func TestUnknownFormat(t *testing.T) {
	t.Cleanup(func() { outputFormat = formatJSON })

	outputFormat = "yaml"
	if err := validateFormat(outputFormat); err == nil {
		t.Fatal("validateFormat accepted an unknown format")
	}
	if outputFormat != formatJSON {
		t.Errorf("rejected format should fall back to json so the error is encodable, got %q", outputFormat)
	}

	var buf bytes.Buffer
	if err := encodeResponse(&buf, "yaml", response{OK: true}); err == nil {
		t.Fatal("encodeResponse accepted an unknown format")
	}
	if buf.Len() != 0 {
		t.Errorf("nothing should be written for an unknown format, got %q", buf.String())
	}

	for _, format := range []string{formatJSON, formatTOON} {
		if err := validateFormat(format); err != nil {
			t.Errorf("validateFormat(%q): %v", format, err)
		}
	}
}
