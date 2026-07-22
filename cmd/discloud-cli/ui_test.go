package main

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestReadPasswordMasked(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	// type a,b, backspace, c, enter
	got, err := readPasswordMasked(strings.NewReader("ab\x7fc\r"), &out)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ac" {
		t.Fatalf("got %q want ac", got)
	}
	if !strings.Contains(out.String(), "***") && !strings.Contains(out.String(), "**") {
		// a* b* backspace erase c* → at least two stars written
	}
	stars := strings.Count(out.String(), "*")
	if stars != 3 { // a, b, c each echo *
		t.Fatalf("stars=%d out=%q", stars, out.String())
	}
}

func TestConfirmDefaultNo(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "\n", false},
		{"n", "n\n", false},
		{"no", "no\n", false},
		{"garbage", "maybe\n", false},
		{"y", "y\n", true},
		{"yes", "yes\n", true},
		{"Y", "Y\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			got := confirm(&out, strings.NewReader(tc.in), "Sure?")
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	t.Run("eof", func(t *testing.T) {
		var out bytes.Buffer
		if confirm(&out, strings.NewReader(""), "Sure?") {
			t.Fatal("EOF should be No")
		}
	})
}

func TestResolvePasswordConflict(t *testing.T) {
	t.Parallel()
	_, err := resolvePassword("pos", true)
	if err == nil {
		t.Fatal("expected conflict error")
	}
}

func TestProgressBarFinish100(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	bar := newProgressBar(&buf)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int64) {
			defer wg.Done()
			bar.Update(n, 100)
		}(int64((i + 1) * 10))
	}
	wg.Wait()
	bar.Finish()
	s := buf.String()
	if !strings.Contains(s, "100%") {
		t.Fatalf("expected 100%% in %q", s)
	}
}

func TestPickIndex(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	idx, err := pickIndex(&out, strings.NewReader("2\n"), []string{"a", "b", "c"})
	if err != nil {
		t.Fatal(err)
	}
	if idx != 1 {
		t.Fatalf("got %d", idx)
	}
	_, err = pickIndex(&out, strings.NewReader("9\n"), []string{"a"})
	if err == nil {
		t.Fatal("expected invalid selection")
	}
}

func TestColorNO_COLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if colorOn(os.Stdout) {
		t.Fatal("NO_COLOR should disable color")
	}
	if green(false, "x") != "x" {
		t.Fatal("paint disabled should be plain")
	}
	if green(true, "x") == "x" {
		t.Fatal("paint enabled should wrap")
	}
}

func TestSpinnerStop(t *testing.T) {
	var buf bytes.Buffer
	s := startSpinner(&buf, "Working…")
	time.Sleep(120 * time.Millisecond)
	s.Stop()
	s.Stop() // idempotent
}

func TestDecodeFileList(t *testing.T) {
	t.Parallel()
	raw := map[string]any{
		"files": []any{
			map[string]any{
				"fileId":     "abc",
				"fileName":   "x.bin",
				"fileSize":   float64(1024),
				"visibility": "public",
			},
		},
	}
	list, err := decode[FilesList](raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Files) != 1 || list.Files[0].FileID != "abc" {
		t.Fatalf("%+v", list)
	}
}
