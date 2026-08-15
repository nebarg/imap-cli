package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	toon "github.com/toon-format/toon-go"

	"go-imap-cli/internal/config"
	"go-imap-cli/internal/mailbox"
)

// Output formats accepted by --format.
const (
	formatJSON = "json"
	formatTOON = "toon"
)

// response is the uniform envelope printed to stdout for every command. The
// `toon` tags mirror the `json` ones so both formats name and omit fields
// identically (see TestResponseTagParity).
type response struct {
	OK    bool   `json:"ok" toon:"ok"`
	Data  any    `json:"data,omitempty" toon:"data,omitempty"`
	Error string `json:"error,omitempty" toon:"error,omitempty"`
}

// encodeResponse renders r in the named format. An unknown format is an error
// rather than a silent fallback, so a typo can't quietly change the output.
func encodeResponse(w io.Writer, format string, r response) error {
	switch format {
	case formatTOON:
		out, err := toon.MarshalString(r)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, out)
		return err
	case formatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(r)
	default:
		return fmt.Errorf("unknown --format %q (want %s or %s)", format, formatJSON, formatTOON)
	}
}

// emit writes a response to stdout in the selected format.
func emit(r response) {
	if err := encodeResponse(os.Stdout, outputFormat, r); err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode response: %v\n", err)
	}
}

// success prints {ok:true, data:...}.
func success(data any) {
	emit(response{OK: true, Data: data})
}

// fail prints {ok:false, error:...} to stdout and exits non-zero. Diagnostic
// text goes to stderr so stdout stays machine-parseable.
func fail(err error) {
	emit(response{OK: false, Error: err.Error()})
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

// withClient loads config, connects, runs fn, and emits the result. It
// centralises connection lifecycle so each command stays small.
func withClient(fn func(*mailbox.Client) (any, error)) {
	acc, err := config.Load(envPath, accountName)
	if err != nil {
		fail(err)
		return
	}
	c, err := mailbox.Connect(acc, time.Duration(timeoutSecs)*time.Second)
	if err != nil {
		fail(err)
		return
	}
	defer c.Close()

	data, err := fn(c)
	if err != nil {
		fail(err)
		return
	}
	success(data)
}
