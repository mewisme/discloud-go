package ids

import (
	"regexp"
	"testing"
)

var hex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestNewFormat(t *testing.T) {
	id := New()
	if !hex32.MatchString(id) {
		t.Fatalf("id=%q", id)
	}
}

func TestParseDashedAndHex(t *testing.T) {
	id := New()
	// Reconstruct dashed form from hex.
	dashed := id[0:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:32]
	got, err := Parse(dashed)
	if err != nil {
		t.Fatal(err)
	}
	if got != id {
		t.Fatalf("got %q want %q", got, id)
	}
	got2, err := Parse(id)
	if err != nil {
		t.Fatal(err)
	}
	if got2 != id {
		t.Fatalf("got %q want %q", got2, id)
	}
}
