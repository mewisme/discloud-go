package server

import (
	"encoding/hex"
	"testing"

	"github.com/google/uuid"
)

func TestNewID_UUIDv7Hex(t *testing.T) {
	id := newID()
	if len(id) != 32 {
		t.Fatalf("len=%d, want 32", len(id))
	}
	raw, err := hex.DecodeString(id)
	if err != nil {
		t.Fatalf("not hex: %v", err)
	}
	parsed, err := uuid.FromBytes(raw)
	if err != nil {
		t.Fatalf("FromBytes: %v", err)
	}
	if parsed.Version() != 7 {
		t.Fatalf("version=%d, want 7", parsed.Version())
	}
}

func TestParseID_DashedAndHex(t *testing.T) {
	canonical := "019f8a8d-908a-765f-855a-54f9e3b92f79"
	hexForm := "019f8a8d908a765f855a54f9e3b92f79"

	a, err := parseID(canonical)
	if err != nil {
		t.Fatalf("parse dashed: %v", err)
	}
	b, err := parseID(hexForm)
	if err != nil {
		t.Fatalf("parse hex: %v", err)
	}
	if a != hexForm || b != hexForm {
		t.Fatalf("got %q / %q, want %q", a, b, hexForm)
	}
	if _, err := parseID("not-a-uuid"); err == nil {
		t.Fatal("expected error for garbage id")
	}
}
