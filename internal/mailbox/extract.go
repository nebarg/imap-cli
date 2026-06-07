package mailbox

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// extractText pulls readable text from an attachment's bytes. It supports
// text/* (returned as-is) and PDF; other types return an error directing the
// caller to fetch the raw bytes instead.
func extractText(contentType, filename string, data []byte) (string, error) {
	ct := strings.ToLower(contentType)
	switch {
	case strings.HasPrefix(ct, "text/"):
		return string(data), nil
	case ct == "application/pdf" || strings.HasSuffix(strings.ToLower(filename), ".pdf"):
		return extractPDF(data)
	default:
		return "", fmt.Errorf("text extraction not supported for content type %q; omit --as-text to get the raw bytes", contentType)
	}
}

// extractPDF returns the plain text of a PDF. The underlying reader can panic on
// malformed input, so recover and surface it as an error.
func extractPDF(data []byte) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("could not extract text from PDF: %v", r)
		}
	}()

	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("reading PDF: %w", err)
	}
	tr, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("extracting PDF text: %w", err)
	}
	var buf strings.Builder
	if _, err := io.Copy(&buf, tr); err != nil {
		return "", fmt.Errorf("reading PDF text: %w", err)
	}
	return buf.String(), nil
}
