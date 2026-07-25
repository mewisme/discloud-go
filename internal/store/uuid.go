package store

import "strings"

// UuidHex normalizes a Postgres uuid text scan (dashed) to 32-char lowercase hex.
func UuidHex(s string) string {
	return strings.ReplaceAll(s, "-", "")
}
