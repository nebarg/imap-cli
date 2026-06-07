package mailbox

import "testing"

func TestExtractText(t *testing.T) {
	// text/* is returned as-is.
	got, err := extractText("text/plain", "notes.txt", []byte("hello"))
	if err != nil || got != "hello" {
		t.Fatalf("text/plain: got %q, err %v", got, err)
	}

	// A .csv is text/* too.
	if _, err := extractText("text/csv", "data.csv", []byte("a,b")); err != nil {
		t.Fatalf("text/csv should pass through, err %v", err)
	}

	// Unsupported binary types are rejected with guidance, not a panic.
	if _, err := extractText("image/png", "logo.png", []byte{0x89, 'P', 'N', 'G'}); err == nil {
		t.Fatal("image/png should be unsupported for --as-text")
	}

	// Malformed PDF bytes must surface an error, not crash (recover path).
	if _, err := extractText("application/pdf", "broken.pdf", []byte("not really a pdf")); err == nil {
		t.Fatal("malformed PDF should return an error")
	}
}
