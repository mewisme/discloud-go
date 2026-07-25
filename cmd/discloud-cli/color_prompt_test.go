package main

import "testing"

func TestHexFieldFromPasteReplacesWhenHash(t *testing.T) {
	v, ok := hexFieldFromPaste("#74ebd5")
	if !ok || v != "#74ebd5" {
		t.Fatalf("got %q %v, want #74ebd5 true", v, ok)
	}
	v, ok = hexFieldFromPaste("  #ACB6E5  ")
	if !ok || v != "#acb6e5" {
		t.Fatalf("got %q %v, want #acb6e5 true", v, ok)
	}
	if _, ok := hexFieldFromPaste("74ebd5"); ok {
		t.Fatal("paste without # must not replace")
	}
	if _, ok := hexFieldFromPaste(""); ok {
		t.Fatal("empty paste must not replace")
	}
}
