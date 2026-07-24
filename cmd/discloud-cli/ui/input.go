package ui

import (
	"os"

	"golang.org/x/term"
)

func IsTTY(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

func ArgOrEmpty(args []string, i int) string {
	if i < 0 || i >= len(args) {
		return ""
	}
	return args[i]
}
