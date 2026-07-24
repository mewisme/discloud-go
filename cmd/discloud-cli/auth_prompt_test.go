package main

import (
	"strings"
	"testing"
)

func TestReadPasswordStdin(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"lf", "secret\n", "secret", false},
		{"crlf", "secret\r\n", "secret", false},
		{"no newline eof", "secret", "secret", false},
		{"empty", "\n", "", true},
		{"empty eof", "", "", true},
		{"keeps spaces", "  x  \n", "  x  ", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readPasswordStdin(strings.NewReader(tc.in))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestResolvePasswordConflict(t *testing.T) {
	t.Parallel()
	_, err := resolvePassword("pos", true)
	if err == nil {
		t.Fatal("expected conflict error")
	}
}

func TestPromptArgProvided(t *testing.T) {
	t.Parallel()
	got, err := promptArg("  abc  ", "Path")
	if err != nil || got != "abc" {
		t.Fatalf("got %q %v", got, err)
	}
}
