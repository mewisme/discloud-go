package main

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

const (
	iconOK     = "✓"
	iconFail   = "✗"
	iconInfo   = "•"
	iconKey    = "🔑"
	iconLock   = "🔒"
	iconUnlock = "🔓"
	iconUp     = "↑"
)

func colorOn(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return isTTY(f)
}

func paint(enabled bool, code, s string) string {
	if !enabled || s == "" {
		return s
	}
	return code + s + ansiReset
}

func bold(enabled bool, s string) string   { return paint(enabled, ansiBold, s) }
func dim(enabled bool, s string) string    { return paint(enabled, ansiDim, s) }
func green(enabled bool, s string) string  { return paint(enabled, ansiGreen, s) }
func red(enabled bool, s string) string    { return paint(enabled, ansiRed, s) }
func yellow(enabled bool, s string) string { return paint(enabled, ansiYellow, s) }
func cyan(enabled bool, s string) string   { return paint(enabled, ansiCyan, s) }

func successMsg(msg string) string {
	on := colorOn(os.Stdout)
	return green(on, iconOK) + " " + msg
}

func infoMsg(msg string) string {
	on := colorOn(os.Stdout)
	return cyan(on, iconInfo) + " " + msg
}

func errorMsg(msg string) string {
	on := colorOn(os.Stderr)
	return red(on, iconFail) + " " + msg
}

func visibilityLabel(enabled bool, v string) string {
	switch v {
	case "private":
		return yellow(enabled, iconLock+" private")
	case "public":
		return green(enabled, iconUnlock+" public")
	default:
		return v
	}
}

func printSuccess(format string, args ...any) {
	fmt.Println(successMsg(fmt.Sprintf(format, args...)))
}

func printInfo(format string, args ...any) {
	fmt.Println(infoMsg(fmt.Sprintf(format, args...)))
}
