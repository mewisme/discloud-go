package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mewisme/discloud-go/internal/client"
)

// contentAlignMode is left|center; synced from customize via SyncContentAlign.
var contentAlignMode = client.DefaultAlign

// TermWidth returns the terminal width for w, or a sensible default.
func TermWidth(w io.Writer) int {
	return termWidth(w)
}

// ContentBandWidth is the preferred content width: banner art width, or the
// terminal when narrower than the banner (same as tables).
func ContentBandWidth(termW int) int {
	return tableTargetOuter(termW)
}

// SyncContentAlign loads customize.align into the process-wide content pad mode.
func SyncContentAlign() {
	c, err := client.LoadCustomize()
	if err != nil {
		contentAlignMode = client.DefaultAlign
		return
	}
	contentAlignMode = c.AlignOrDefault()
}

// SetContentAlign sets the process-wide content pad mode (left|center).
// Invalid values fall back to center. Used after customize writes and in tests.
func SetContentAlign(a string) {
	n, err := client.NormalizeAlign(a)
	if err != nil {
		n = client.DefaultAlign
	}
	contentAlignMode = n
}

// ContentAlign returns the active content alignment (left|center).
func ContentAlign() string {
	return contentAlignMode
}

// ContentLeftPad returns left pad for contentW.
// "left" (default) is 0; "center" uses the banner axis.
func ContentLeftPad(termW, contentW int) int {
	if contentAlignMode == "left" {
		return 0
	}
	return tableLeftPad(termW, contentW)
}

// PadUnderBanner left-pads each line of s so the block sits on the banner axis.
// If contentW <= 0, uses the widest display line of s (centers short status
// lines under the banner the same way a narrow table is centered).
func PadUnderBanner(s string, termW, contentW int) string {
	if s == "" || termW < 1 {
		return s
	}
	if contentW <= 0 {
		contentW = blockDisplayWidth(s)
		if contentW < 1 {
			contentW = 1
		}
	}
	return padLines(s, ContentLeftPad(termW, contentW))
}

// PrintUnderBanner writes s + newline, centered under the banner when
// contentAlignMode is center (same axis as tables). Uses cursor-column
// positioning on TTYs so leading alignment cannot be dropped by the console.
func PrintUnderBanner(w io.Writer, s string) {
	if w == nil {
		w = os.Stdout
	}
	s = strings.TrimRight(s, "\n")
	tw := layoutTermWidth()
	cw := blockDisplayWidth(s)
	if cw < 1 {
		cw = 1
	}
	left := ContentLeftPad(tw, cw)
	lines := strings.Split(s, "\n")
	useCol := false
	if f, ok := w.(*os.File); ok && IsTTY(f) && left > 0 {
		useCol = true
	}
	for _, line := range lines {
		if useCol {
			// CHA: move to 1-based column, then print (avoids stripped leading spaces).
			fmt.Fprintf(w, "\033[%dG%s\n", left+1, line)
			continue
		}
		if left > 0 {
			fmt.Fprintln(w, strings.Repeat(" ", left)+line)
		} else {
			fmt.Fprintln(w, line)
		}
	}
}

// LayoutTermWidth is the terminal width used for banner/content alignment.
// Prefers stderr (where the banner is drawn) so stdout size mismatches do not
// shove content to the left edge of the terminal.
func LayoutTermWidth() int {
	return layoutTermWidth()
}

// layoutTermWidth matches the banner's width source (stderr first).
func layoutTermWidth() int {
	if w := termWidth(os.Stderr); w >= 40 {
		return w
	}
	return termWidth(os.Stdout)
}

func padLines(s string, left int) string {
	if left <= 0 || s == "" {
		return s
	}
	pad := strings.Repeat(" ", left)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line == "" && i == len(lines)-1 {
			continue
		}
		lines[i] = pad + line
	}
	return strings.Join(lines, "\n")
}

func blockDisplayWidth(s string) int {
	max := 0
	for _, line := range strings.Split(s, "\n") {
		if w := displayWidth(line); w > max {
			max = w
		}
	}
	return max
}
