package ui

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const pickerCountdown = 3 * time.Second

// WaitPicker shows a 3s countdown naming what dialog will open, then clears the
// line and returns so the caller can open the picker. Press Enter to skip the
// wait (clears countdown first, then returns).
func WaitPicker(what string) {
	if JSON || !IsTTY(os.Stderr) {
		return
	}

	deadline := time.Now().Add(pickerCountdown)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		left := time.Until(deadline)
		if left <= 0 {
			clearPickerLine()
			return
		}
		// Check Enter before redraw so a keypress isn't delayed a tick.
		if pickerEnterPressed() {
			clearPickerLine()
			return
		}
		secs := int((left + time.Second - 1) / time.Second)
		on := ColorOn(os.Stderr)
		line := Cyan(on, IconInfo) + " " + fmt.Sprintf("Opening %s in %ds… (Enter to open now)", what, secs)
		tw := termWidth(os.Stderr)
		pad := ContentLeftPad(tw, displayWidth(line))
		fmt.Fprintf(os.Stderr, "\r\033[K%s%s", strings.Repeat(" ", pad), line)

		select {
		case <-ticker.C:
		case <-time.After(left):
			clearPickerLine()
			return
		}
	}
}

func clearPickerLine() {
	fmt.Fprint(os.Stderr, "\r\033[K")
	_ = os.Stderr.Sync()
}

// Selecting prints a status line while the GUI picker is open.
func Selecting(what string) {
	if JSON {
		return
	}
	PrintInfo("Selecting %s…", what)
}

// Selected replaces the Selecting line (TTY) and logs the path(s) chosen.
func Selected(what string, paths ...string) {
	if JSON {
		return
	}
	clearSelectingLine()
	switch len(paths) {
	case 0:
		PrintInfo("Selected %s: (none)", what)
	case 1:
		PrintInfo("Selected %s: %s", what, paths[0])
	default:
		PrintInfo("Selected %s: %d items", what, len(paths))
		for _, p := range paths {
			PrintUnderBanner(os.Stdout, "  "+p)
		}
	}
}

// clearSelectingLine erases the prior "Selecting …" line on a TTY.
func clearSelectingLine() {
	if !IsTTY(os.Stdout) {
		return
	}
	fmt.Fprint(os.Stdout, "\033[1A\033[2K")
}

// ClearSelecting hides the Selecting line without printing Selected (cancel/error).
func ClearSelecting() {
	if JSON {
		return
	}
	clearSelectingLine()
}
