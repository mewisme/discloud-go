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
// "center" (default) uses the banner axis; "left" is 0.
func ContentLeftPad(termW, contentW int) int {
	if contentAlignMode == "left" {
		return 0
	}
	return tableLeftPad(termW, contentW)
}

// PadUnderBanner left-pads each line of s so the block sits on the banner axis.
// If contentW <= 0, uses the widest display line of s.
func PadUnderBanner(s string, termW, contentW int) string {
	if s == "" || termW < 1 {
		return s
	}
	if contentW <= 0 {
		contentW = blockDisplayWidth(s)
	}
	return padLines(s, ContentLeftPad(termW, contentW))
}

// PrintUnderBanner writes s + newline, banner-aligned on w.
func PrintUnderBanner(w io.Writer, s string) {
	if w == nil {
		w = os.Stdout
	}
	s = strings.TrimRight(s, "\n")
	fmt.Fprintln(w, PadUnderBanner(s, termWidth(w), 0))
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
