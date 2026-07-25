//go:build !windows

package ui

import (
	"os"
	"time"

	"golang.org/x/term"
)

// pickerEnterPressed checks stdin for Enter without blocking the countdown.
func pickerEnterPressed() bool {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return false
	}
	// Brief non-blocking poll via read deadline.
	_ = os.Stdin.SetReadDeadline(time.Now().Add(1 * time.Millisecond))
	defer os.Stdin.SetReadDeadline(time.Time{})

	var buf [1]byte
	n, err := os.Stdin.Read(buf[:])
	if err != nil || n == 0 {
		return false
	}
	return buf[0] == '\r' || buf[0] == '\n'
}
