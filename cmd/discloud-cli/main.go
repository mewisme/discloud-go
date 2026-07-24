// Command discloud-cli is the DisCloud API client.
package main

import (
	"os"

	"github.com/mewisme/discloud-go/cmd/discloud-cli/ui"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		// Error already printed by runE. Exit 0 on a TTY so `go run` does not
		// append its own "exit status 1" line; non-TTY (scripts/CI) still get 1.
		if ui.IsTTY(os.Stderr) || ui.IsTTY(os.Stdout) {
			return
		}
		os.Exit(1)
	}
}
