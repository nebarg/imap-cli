package mailbox

import "testing"

func TestExtractText(t *testing.T) {
	// text/* (UTF-8) is returned as-is.
	got, err := extractText("text/plain", "notes.txt", "utf-8", []byte("hello"))
	if err != nil || got != "hello" {
		t.Fatalf("text/plain: got %q, err %v", got, err)
	}

	// A .csv is text/* too.
	if _, err := extractText("text/csv", "data.csv", "", []byte("a,b")); err != nil {
		t.Fatalf("text/csv should pass through, err %v", err)
	}

	// A non-UTF-8 text attachment is charset-decoded: 0xE9 in ISO-8859-1 is é.
	got, err = extractText("text/plain", "latin.txt", "iso-8859-1", []byte{'c', 'a', 'f', 0xE9})
	if err != nil || got != "café" {
		t.Fatalf("iso-8859-1: got %q, err %v; want café", got, err)
	}

	// Unsupported binary types are rejected with guidance, not a panic.
	if _, err := extractText("image/png", "logo.png", "", []byte{0x89, 'P', 'N', 'G'}); err == nil {
		t.Fatal("image/png should be unsupported for --as-text")
	}

	// Malformed PDF bytes must surface an error, not crash (recover path).
	if _, err := extractText("application/pdf", "broken.pdf", "", []byte("not really a pdf")); err == nil {
		t.Fatal("malformed PDF should return an error")
	}
}
