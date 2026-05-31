package main

import (
	"os"

	"go-imap-cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		// Command-level errors are already reported as JSON by the commands
		// themselves; this guards anything Cobra surfaces (e.g. bad flags).
		os.Exit(1)
	}
}
