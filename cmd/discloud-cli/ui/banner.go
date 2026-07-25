package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// JSON is set by main from --json; when true, human UI (banner, spinner, prompts) is skipped.
var JSON bool

// cliBanner lines are left-aligned to each other; centerLines pads the whole block.
const cliBanner = `
██████╗ ██╗███████╗ ██████╗██╗      ██████╗ ██╗   ██╗██████╗
██╔══██╗██║██╔════╝██╔════╝██║     ██╔═══██╗██║   ██║██╔══██╗
██║  ██║██║███████╗██║     ██║     ██║   ██║██║   ██║██║  ██║
██║  ██║██║╚════██║██║     ██║     ██║   ██║██║   ██║██║  ██║
██████╔╝██║███████║╚██████╗███████╗╚██████╔╝╚██████╔╝██████╔╝
╚═════╝ ╚═╝╚══════╝ ╚═════╝╚══════╝ ╚═════╝  ╚═════╝ ╚═════╝
`

// PrintHeader clears the screen (TTY only), then prints the DISCLOUD banner.
// Skipped for --json and cobra completion (keeps stdout/machine output clean).
func PrintHeader(cmd *cobra.Command) {
	if JSON {
		return
	}
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "completion" {
			return
		}
	}
	ClearAndPrintBanner()
}

// ClearAndPrintBanner clears the TTY and reprints the banner when the terminal
// is wide enough; on narrow terminals it only clears (banner would clip).
func ClearAndPrintBanner() {
	if JSON {
		return
	}
	clearScreen()
	tw := termWidth(os.Stderr)
	if tw >= BannerContentWidth() {
		fmt.Fprint(os.Stderr, FormatBanner(tw))
	}
}

func clearScreen() {
	if IsTTY(os.Stderr) {
		fmt.Fprint(os.Stderr, "\033[2J\033[H")
	}
}

// BannerContentWidth is the display width of the widest banner art line.
func BannerContentWidth() int {
	max := 0
	for _, line := range strings.Split(strings.Trim(cliBanner, "\n"), "\n") {
		trim := strings.TrimRight(line, " \t")
		if trim == "" {
			continue
		}
		if w := displayWidth(trim); w > max {
			max = w
		}
	}
	return max
}

// bannerLayout returns left pad and content width used to place the banner in termW.
// Matches centerLines (odd term widths floor to even).
func bannerLayout(termW int) (leftPad, contentW int) {
	contentW = BannerContentWidth()
	if termW < 1 {
		return 0, contentW
	}
	w := termW
	if w%2 != 0 {
		w--
	}
	leftPad = (w - contentW) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	return leftPad, contentW
}

// FormatBanner returns the centered DISCLOUD banner block for the given width
// (including leading blank lines). Used by PrintHeader and embedded TUIs.
func FormatBanner(width int) string {
	var b strings.Builder
	b.WriteByte('\n')
	b.WriteByte('\n')
	text := centerLines(cliBanner, width)
	if ColorOn(os.Stderr) {
		text = Dim(true, text)
	}
	b.WriteString(text)
	if !strings.HasSuffix(text, "\n") {
		b.WriteByte('\n')
	}
	return b.String()
}

// BannerHeight is how many terminal rows FormatBanner occupies at width.
func BannerHeight(width int) int {
	s := FormatBanner(width)
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

// centerLines centers the whole block using one shared left pad from the widest line.
// Odd widths are floored to even (width-1).
func centerLines(text string, width int) string {
	if width < 1 {
		return text
	}
	if width%2 != 0 {
		width--
	}
	raw := strings.Split(strings.Trim(text, "\n"), "\n")
	lines := make([]string, 0, len(raw))
	maxW := 0
	for _, line := range raw {
		trim := strings.TrimRight(line, " \t")
		lines = append(lines, trim)
		if trim == "" {
			continue
		}
		if w := displayWidth(trim); w > maxW {
			maxW = w
		}
	}
	pad := (width - maxW) / 2
	if pad < 0 {
		pad = 0
	}
	left := strings.Repeat(" ", pad)
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		if line == "" {
			continue
		}
		b.WriteString(left)
		b.WriteString(line)
	}
	b.WriteByte('\n')
	return b.String()
}
