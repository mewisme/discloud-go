package ui

import (
	"fmt"
	"os"
	"regexp"
)

// ANSI styles — only applied when stdout/stderr is a TTY and NO_COLOR is unset.
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
)

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// stripANSI removes CSI color/style sequences for display-width math.
func stripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}

// Icon glyphs used in human-readable CLI output.
const (
	IconOK     = "✓"
	IconFail   = "✗"
	IconInfo   = "•"
	IconKey    = "🔑"
	IconLock   = "🔒"
	IconUnlock = "🔓"
	IconUp     = "↑"
)

func ColorOn(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return IsTTY(f)
}

func paint(enabled bool, code, s string) string {
	if !enabled || s == "" {
		return s
	}
	return code + s + ansiReset
}

func Bold(enabled bool, s string) string { return paint(enabled, ansiBold, s) }

func Dim(enabled bool, s string) string { return paint(enabled, ansiDim, s) }

func Green(enabled bool, s string) string { return paint(enabled, ansiGreen, s) }

func Red(enabled bool, s string) string { return paint(enabled, ansiRed, s) }

func Yellow(enabled bool, s string) string { return paint(enabled, ansiYellow, s) }

func Cyan(enabled bool, s string) string { return paint(enabled, ansiCyan, s) }

func successMsg(msg string) string {
	on := ColorOn(os.Stdout)
	return Green(on, IconOK) + " " + msg
}

func infoMsg(msg string) string {
	on := ColorOn(os.Stdout)
	return Cyan(on, IconInfo) + " " + msg
}

func errorMsg(msg string) string {
	on := ColorOn(os.Stderr)
	return Red(on, IconFail) + " " + msg
}

func VisibilityLabel(enabled bool, v string) string {
	switch v {
	case "private":
		return Yellow(enabled, IconLock+" private")
	case "public":
		return Green(enabled, IconUnlock+" public")
	default:
		return v
	}
}

func PrintSuccess(format string, args ...any) {
	fmt.Println(successMsg(fmt.Sprintf(format, args...)))
}

func PrintInfo(format string, args ...any) {
	fmt.Println(infoMsg(fmt.Sprintf(format, args...)))
}
