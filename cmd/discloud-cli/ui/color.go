package ui

import (
	"fmt"
	"os"
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

// ColorOn reports whether ANSI styles should be applied for f.
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

// Bold wraps s in ANSI bold when enabled.
func Bold(enabled bool, s string) string { return paint(enabled, ansiBold, s) }

// Dim wraps s in ANSI dim when enabled.
func Dim(enabled bool, s string) string { return paint(enabled, ansiDim, s) }

// Green wraps s in ANSI green when enabled.
func Green(enabled bool, s string) string { return paint(enabled, ansiGreen, s) }

// Red wraps s in ANSI red when enabled.
func Red(enabled bool, s string) string { return paint(enabled, ansiRed, s) }

// Yellow wraps s in ANSI yellow when enabled.
func Yellow(enabled bool, s string) string { return paint(enabled, ansiYellow, s) }

// Cyan wraps s in ANSI cyan when enabled.
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

// VisibilityLabel returns a colored private/public label.
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

// PrintSuccess writes a green success line to stdout.
func PrintSuccess(format string, args ...any) {
	fmt.Println(successMsg(fmt.Sprintf(format, args...)))
}

// PrintInfo writes a cyan info line to stdout.
func PrintInfo(format string, args ...any) {
	fmt.Println(infoMsg(fmt.Sprintf(format, args...)))
}
