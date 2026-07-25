//go:build windows

package ui

import (
	"context"
	"time"

	"golang.org/x/sys/windows"
)

const (
	vkC              = 0x43
	leftCtrlPressed  = 0x0008
	rightCtrlPressed = 0x0004
)

// startConsoleInterruptWatch polls CONIN$ for Ctrl+C. Clears ENABLE_PROCESSED_INPUT
// so ConPTY delivers Ctrl+C as a key event when the Go signal handler misses it.
func startConsoleInterruptWatch(ctx context.Context, cancel context.CancelFunc) func() {
	done := make(chan struct{})
	go func() {
		h, closeH, err := openConsoleInput()
		if err != nil {
			return
		}
		defer closeH()

		var mode uint32
		haveMode := windows.GetConsoleMode(h, &mode) == nil
		if haveMode {
			// Ctrl+C → input buffer instead of a process-wide control event.
			_ = windows.SetConsoleMode(h, mode&^windows.ENABLE_PROCESSED_INPUT)
			defer windows.SetConsoleMode(h, mode)
		}

		t := time.NewTicker(50 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-t.C:
				if consoleCtrlCPressed(h) {
					cancel()
					return
				}
			}
		}
	}()
	return func() {
		select {
		case <-done:
		default:
			close(done)
		}
	}
}

func consoleCtrlCPressed(h windows.Handle) bool {
	var n uint32
	if err := windows.GetNumberOfConsoleInputEvents(h, &n); err != nil || n == 0 {
		return false
	}
	if n > 64 {
		n = 64
	}
	recs := make([]consoleInputRecord, n)
	var read uint32
	if err := peekConsoleInput(h, &recs[0], n, &read); err != nil || read == 0 {
		return false
	}
	for _, r := range recs[:read] {
		if isCtrlCKey(r) {
			_ = windows.FlushConsoleInputBuffer(h)
			return true
		}
	}
	drainConsoleInput(h, read)
	return false
}

func isCtrlCKey(r consoleInputRecord) bool {
	if r.EventType != windows.KEY_EVENT || r.KeyDown == 0 {
		return false
	}
	if r.Char == 0x03 { // ETX
		return true
	}
	ctrl := r.Control&(leftCtrlPressed|rightCtrlPressed) != 0
	return ctrl && (r.VK == vkC || r.Char == 'c' || r.Char == 'C')
}
