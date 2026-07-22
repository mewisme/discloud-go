package ui

import (
	"bytes"
	"net/url"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/mewisme/discloud-go/internal/client"
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

func TestResolveArg(t *testing.T) {
	t.Parallel()
	got, err := ResolveArg("abc", "SHA-256: ")
	if err != nil || got != "abc" {
		t.Fatalf("got %q %v", got, err)
	}
	JSON = true
	t.Cleanup(func() { JSON = false })
	_, err = ResolveArg("", "SHA-256: ")
	if err == nil || !strings.Contains(err.Error(), "SHA-256 is required") {
		t.Fatalf("want required error, got %v", err)
	}
}

func TestFieldName(t *testing.T) {
	t.Parallel()
	if got := fieldName("Username: "); got != "Username" {
		t.Fatalf("got %q", got)
	}
}

func TestReadLinePrefill(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	// prefill "ab", backspace once, type "c", enter → "ac"
	got, err := readLinePrefill(strings.NewReader("\x7fc\r"), &out, []byte("ab"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "ac" {
		t.Fatalf("got %q want ac", got)
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
			got := Confirm(&out, strings.NewReader(tc.in), "Sure?")
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	t.Run("eof", func(t *testing.T) {
		var out bytes.Buffer
		if Confirm(&out, strings.NewReader(""), "Sure?") {
			t.Fatal("EOF should be No")
		}
	})
}

func TestResolvePasswordConflict(t *testing.T) {
	t.Parallel()
	_, err := ResolvePassword("pos", true)
	if err == nil {
		t.Fatal("expected conflict error")
	}
}

func TestProgressBarFinish100(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	bar := NewProgressBar(&buf)
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

func TestTableRule(t *testing.T) {
	t.Parallel()
	got := tableRule([]int{2, 3})
	want := "+----+-----+"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	row := tableRow([]string{"ab", "cde"}, false)
	if row != "| ab | cde |" {
		t.Fatalf("row %q", row)
	}
	span := tableSpanRule([]int{2, 3})
	// inner = 2+3+3 = 8, dashes = 10 → +----------+
	if span != "+----------+" {
		t.Fatalf("span rule %q", span)
	}
}

func TestPaintTyped(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw  string
		want string // ansi code when on=true; "" means no wrap beyond stringCode
		code string
	}{
		{"-", ansiDim, ""},
		{"true", ansiGreen, ""},
		{"public", ansiGreen, ""},
		{"false", ansiRed, ""},
		{"private", ansiYellow, ""},
		{"42", ansiCyan, ""},
		{"1.5 MB", ansiCyan, ""},
		{"2024-01-02", ansiDim, ""},
		{"hello", ansiCyan, ansiCyan},
		{"hello", "", ""},
	}
	for _, tc := range cases {
		got := paintTyped(true, tc.raw, tc.raw, tc.code)
		if tc.want == "" {
			if got != tc.raw {
				t.Fatalf("raw=%q got %q want plain", tc.raw, got)
			}
			continue
		}
		if !strings.HasPrefix(got, tc.want) || !strings.HasSuffix(got, ansiReset) {
			t.Fatalf("raw=%q got %q want prefix %q", tc.raw, got, tc.want)
		}
	}
}

func TestContentWidthsAndFit(t *testing.T) {
	t.Parallel()
	headers := []string{"ID", "NAME", "SIZE"}
	rows := [][]string{
		{"abcd", "hello", "1 B"},
		{"xy", "longer-name", "10 KiB"},
	}
	w := contentWidths(headers, rows)
	if w[0] != 4 || w[1] != 11 || w[2] != 6 {
		t.Fatalf("widths %v", w)
	}
	fitWidths(w, 30, 1) // overhead 3*3+1=10, content 4+11+6=21 → total 31, need shrink 1 from NAME
	if w[1] != 10 {
		t.Fatalf("after fit NAME=%d want 10 (widths=%v)", w[1], w)
	}
}

func TestPadRight(t *testing.T) {
	t.Parallel()
	if got := padRight("abcdef", 4); got != "abc…" {
		t.Fatalf("trunc got %q", got)
	}
	if got := padRight("ab", 4); got != "ab  " {
		t.Fatalf("pad got %q", got)
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

func TestPickFileTyped(t *testing.T) {
	t.Parallel()
	files := []File{
		{ID: "aaa", Name: "a.bin", Size: 1, Visibility: "public"},
		{ID: "bbb", Name: "b.bin", Size: 2, Visibility: "private"},
	}
	var out bytes.Buffer
	idx, err := PickFile(&out, strings.NewReader("2\n"), files)
	if err != nil {
		t.Fatal(err)
	}
	if idx != 1 {
		t.Fatalf("got %d", idx)
	}
	if !strings.Contains(out.String(), "#") || !strings.Contains(out.String(), "b.bin") {
		t.Fatalf("expected pick table, got %q", out.String())
	}
}

func TestPickChoiceTyped(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	got, _, err := PickChoice(&out, strings.NewReader("2\n"), "Visibility", []string{"public", "private"}, "public")
	if err != nil {
		t.Fatal(err)
	}
	if got != "private" {
		t.Fatalf("got %q", got)
	}
	s := out.String()
	if !strings.Contains(s, "Visibility (↑↓):") {
		t.Fatalf("expected single-line hint, got %q", s)
	}
}

func TestPickChoiceDefaultEmptyEnter(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	got, extra, err := PickChoice(&out, strings.NewReader("\n"), "Visibility", []string{"public", "private"}, "private")
	if err != nil {
		t.Fatal(err)
	}
	if got != "private" {
		t.Fatalf("got %q, want default private", got)
	}
	if extra != 1 {
		t.Fatalf("clearExtra = %d, want 1", extra)
	}
}

func TestPrintGlyphTable(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	if err := PrintGlyphTable(&out, [][]string{
		{IconOK, "Visibility", "private"},
		{IconOK, "Token", "tok"},
	}); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "FIELD") || !strings.Contains(s, "VALUE") {
		t.Fatalf("expected headers, got %q", s)
	}
	if !strings.Contains(s, IconOK) || !strings.Contains(s, "Visibility") || !strings.Contains(s, "private") {
		t.Fatalf("got %q", s)
	}
	if !strings.Contains(s, "+") || !strings.Contains(s, "|") {
		t.Fatalf("expected table borders, got %q", s)
	}
}

func TestDisplayWidthGlyph(t *testing.T) {
	t.Parallel()
	if displayWidth(IconOK) != 1 {
		t.Fatalf("✓: got %d want 1", displayWidth(IconOK))
	}
	if displayWidth(IconKey) != 2 {
		t.Fatalf("🔑: got %d want 2", displayWidth(IconKey))
	}
	if got := padRight(IconOK, 2); displayWidth(got) != 2 || got != IconOK+" " {
		t.Fatalf("pad ✓ to 2: %q dw=%d", got, displayWidth(got))
	}
	if got := padRight(IconKey, 2); displayWidth(got) != 2 || got != IconKey {
		t.Fatalf("pad 🔑 to 2: %q dw=%d", got, displayWidth(got))
	}
	widths := contentWidths([]string{"", "FIELD", "VALUE"}, [][]string{
		{IconOK, "a", "b"},
		{IconKey, "c", "d"},
	})
	if widths[0] != 2 {
		t.Fatalf("glyph col width %d, want 2", widths[0])
	}
}

func TestDescribeError(t *testing.T) {
	t.Parallel()
	title, detail, hint := describeError(&client.Error{Status: 401, Message: "Not signed in"})
	if title != "Not signed in" || detail != "HTTP 401" || hint == "" {
		t.Fatalf("api: %q %q %q", title, detail, hint)
	}
	title, detail, hint = describeError(&url.Error{
		Op:  "Get",
		URL: "http://localhost:8080/healthz",
		Err: syscall.ECONNREFUSED,
	})
	if title != "Cannot reach API" || !strings.Contains(detail, "localhost") || hint == "" {
		t.Fatalf("conn: %q %q %q", title, detail, hint)
	}
}

func TestColorNO_COLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if ColorOn(os.Stdout) {
		t.Fatal("NO_COLOR should disable color")
	}
	if Green(false, "x") != "x" {
		t.Fatal("paint disabled should be plain")
	}
	if Green(true, "x") == "x" {
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
