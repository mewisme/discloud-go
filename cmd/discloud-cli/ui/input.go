package ui

import (
	"os"

	"golang.org/x/term"
)

// IsTTY reports whether f is a terminal.
func IsTTY(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// ArgOrEmpty returns args[i] or "" if out of range.
func ArgOrEmpty(args []string, i int) string {
	if i < 0 || i >= len(args) {
		return ""
	}
	return args[i]
}
