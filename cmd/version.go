package cmd

import (
	"github.com/spf13/cobra"
)

// version is the build version. It defaults to "dev" for local builds and is
// overridden at release time via the linker:
//   -ldflags="-X go-imap-cli/cmd.version=v0.1.0"
var version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the build version",
	Args:  cobra.NoArgs,
	Run: func(_ *cobra.Command, _ []string) {
		success(map[string]string{"version": version})
	},
}
