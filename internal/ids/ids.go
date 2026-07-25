// Package ids generates and parses application entity IDs as 32-char lowercase hex UUIDv7.
package ids

import (
	"encoding/hex"

	"github.com/google/uuid"
)

// New returns a new UUIDv7 as a 32-character lowercase hex string (no dashes).
func New() string {
	id, err := uuid.NewV7()
	if err != nil {
		panic(err) // NewV7 only fails if rand fails
	}
	return hex.EncodeToString(id[:])
}

// Parse accepts dashed or hex UUID forms and returns 32-char lowercase hex.
func Parse(s string) (string, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(id[:]), nil
}

// Hex normalizes a possibly dashed UUID string to 32-char lowercase hex.
func Hex(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '-' {
			out = append(out, s[i])
		}
	}
	return string(out)
}
