package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"go-imap-cli/internal/config"
	"go-imap-cli/internal/mailbox"
)

// response is the uniform JSON envelope printed to stdout for every command.
type response struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

// emit writes a JSON response to stdout.
func emit(r response) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
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
