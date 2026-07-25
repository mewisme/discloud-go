package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
)

func TestFitNameDisplayWidth(t *testing.T) {
	t.Parallel()
	const w = 12
	names := []string{
		"a.bin",
		"medium-file.bin",
		"this-is-a-very-long-filename.bin",
		"ファイル.bin",
		"📦-archive.bin",
	}
	for _, n := range names {
		got := fitName(n, w)
		if dw := lipgloss.Width(got); dw != w {
			t.Fatalf("fitName(%q,%d)=%q width %d want %d", n, w, got, dw, w)
		}
	}
	if !strings.Contains(fitName("this-is-a-very-long-filename.bin", 12), "…") {
		t.Fatal("expected ellipsis on long name")
	}
}

func TestProgressLayoutSharedNameWidth(t *testing.T) {
	t.Parallel()
	names := []string{"a.bin", "medium-file.bin", "this-is-a-very-long-filename.bin"}
	layout := NewProgressLayoutSized(names, 24)
	if layout.NameWidth != 24 {
		t.Fatalf("NameWidth=%d want 24 (capped)", layout.NameWidth)
	}
	short := fitName("a.bin", layout.NameWidth)
	long := fitName("this-is-a-very-long-filename.bin", layout.NameWidth)
	if lipgloss.Width(short) != lipgloss.Width(long) {
		t.Fatalf("fitted names differ: %q (%d) vs %q (%d)",
			short, lipgloss.Width(short), long, lipgloss.Width(long))
	}
}

func TestProgressStatusTransitions(t *testing.T) {
	t.Parallel()
	files := []ProgressFile{
		{Name: "a.bin", Total: 100},
		{Name: "b.bin", Total: 100},
	}
	m := newProgressModel(files, NewProgressLayout([]string{"a.bin", "b.bin"}), 120, nil, "Uploading", nil, false)
	if m.rows[0].status != statusWaiting || m.rows[1].status != statusWaiting {
		t.Fatal("expected Waiting")
	}
	_, _ = m.Update(fileProgressMsg{id: 0, sent: 40})
	if m.rows[0].status != statusUploading {
		t.Fatalf("got %v want Uploading", m.rows[0].status)
	}
	if m.rows[1].status != statusWaiting {
		t.Fatal("untouched row must stay Waiting")
	}
	_, _ = m.Update(fileDoneMsg{id: 0})
	if m.rows[0].status != statusComplete || m.rows[0].sent != 100 {
		t.Fatalf("complete: status=%v sent=%d", m.rows[0].status, m.rows[0].sent)
	}
}

func TestProgressStatusIconColored(t *testing.T) {
	t.Parallel()
	cases := []struct {
		s     fileStatus
		glyph string
	}{
		{statusWaiting, "○"},
		{statusUploading, "●"},
		{statusComplete, IconOK},
		{statusAborted, IconFail},
	}
	for _, tc := range cases {
		got := tc.s.statusIcon()
		plain := ansiEscape.ReplaceAllString(got, "")
		if plain != tc.glyph {
			t.Fatalf("%v: plain=%q want %q (raw %q)", tc.s, plain, tc.glyph, got)
		}
		if got == plain {
			t.Fatalf("%v: expected ANSI color on %q", tc.s, tc.glyph)
		}
	}
}

func TestProgressViewColumnAlignment(t *testing.T) {
	t.Parallel()
	names := []string{
		"a.bin",
		"medium-file.bin",
		"this-is-a-very-long-filename.bin",
		"ファイル.bin",
		"📦-archive.bin",
	}
	files := make([]ProgressFile, len(names))
	for i, n := range names {
		files[i] = ProgressFile{Name: n, Total: int64((i + 1) * 1024 * 1024)}
	}
	layout := NewProgressLayoutSized(names, 24)
	m := newProgressModel(files, layout, 140, nil, "Uploading", nil, false)
	_, _ = m.Update(fileProgressMsg{id: 0, sent: 0})
	_, _ = m.Update(fileProgressMsg{id: 1, sent: files[1].Total / 2})
	_, _ = m.Update(fileDoneMsg{id: 2})
	_, _ = m.Update(fileProgressMsg{id: 3, sent: 100})
	// row 4 stays Waiting

	raw := m.renderAllPlain()
	lines := strings.Split(raw, "\n")
	if len(lines) != len(names) {
		t.Fatalf("got %d lines want %d\n%s", len(lines), len(names), raw)
	}
	type cols struct{ bar, cur, pct, spd int }
	var first cols
	for i, line := range lines {
		plain := ansiEscape.ReplaceAllString(line, "")
		c, ok := progressColCells(plain)
		if !ok {
			t.Fatalf("line %d: parse failed: %q", i, plain)
		}
		if i == 0 {
			first = c
			continue
		}
		if c != first {
			t.Fatalf("row %d cols %+v != row 0 %+v\n  %q\n  %q", i, c, first, lines[0], plain)
		}
	}
}

func TestUploadProgressWaitSmoke(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	prog := NewUploadProgress(&buf, []ProgressFile{{Name: "readme.md", Total: 100}})
	bar := prog.Bar(0)
	bar.Update(50, 100)
	time.Sleep(50 * time.Millisecond)
	bar.Done()
	prog.Wait()
	prog.Wait() // idempotent
}

func TestPrintProgressPreview(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	PrintProgressPreview(&buf, "Upload", []string{"#ff0000", "#00ff00"})
	s := buf.String()
	if !strings.Contains(s, "Upload") || !strings.Contains(s, "100%") {
		t.Fatalf("preview=%q", s)
	}
	if !strings.Contains(s, "━") {
		t.Fatalf("expected filled bar: %q", s)
	}
}

// progressColCells returns terminal-cell starts of bar, transferred, %, speed.
func progressColCells(plain string) (c struct{ bar, cur, pct, spd int }, ok bool) {
	barAt := -1
	for i, r := range plain {
		if r == '━' || r == '─' {
			barAt = i
			break
		}
	}
	if barAt < 0 {
		return c, false
	}
	c.bar = lipgloss.Width(plain[:barAt])

	i := barAt
	for _, r := range plain[barAt:] {
		if r != '━' && r != '─' {
			break
		}
		i += len(string(r))
	}
	c.cur = lipgloss.Width(plain[:i])

	pctAt := strings.IndexByte(plain, '%')
	if pctAt < 0 {
		return c, false
	}
	c.pct = lipgloss.Width(plain[:pctAt])
	c.spd = lipgloss.Width(plain[:pctAt+1])
	return c, true
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

func TestPrintUploadTableStatus(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := PrintUploadTable(&buf, []UploadRow{
		{Name: "a.bin", Size: 10, Status: "ready", ID: "id1"},
		{Name: "b.bin", Size: 20, Status: "reused", ID: "id2"},
		{Name: "c.bin", Size: 30, Status: "", ID: "id3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "STATUS") {
		t.Fatalf("missing STATUS header:\n%s", out)
	}
	if !strings.Contains(out, "ready") || !strings.Contains(out, "reused") {
		t.Fatalf("missing status values:\n%s", out)
	}
	// empty Status → ShortStatus defaults to ready (appears twice: row0 + row2)
	if n := strings.Count(out, "ready"); n < 2 {
		t.Fatalf("empty status should show ready; got %d ready in:\n%s", n, out)
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

func TestBannerContentWidth(t *testing.T) {
	t.Parallel()
	if w := BannerContentWidth(); w < 40 {
		t.Fatalf("banner width %d too small", w)
	}
}

func TestTableTargetOuter(t *testing.T) {
	t.Parallel()
	bw := BannerContentWidth()
	if got := tableTargetOuter(bw + 10); got != bw {
		t.Fatalf("wide term: got %d want banner %d", got, bw)
	}
	if got := tableTargetOuter(bw - 5); got != bw-5 {
		t.Fatalf("narrow term: got %d want %d", got, bw-5)
	}
}

func TestTableLeftPadFollowsBanner(t *testing.T) {
	SetContentAlign("center")
	bw := BannerContentWidth()
	termW := bw + 20
	bLeft, _ := bannerLayout(termW)
	// Table same width as banner → same left pad.
	if got := tableLeftPad(termW, bw); got != bLeft {
		t.Fatalf("same width: left=%d want banner left=%d", got, bLeft)
	}
	// Narrower table → centered under banner (more left pad than banner).
	tw := bw - 10
	want := bLeft + (bw-tw)/2
	if got := tableLeftPad(termW, tw); got != want {
		t.Fatalf("narrow table: left=%d want %d", got, want)
	}
}

func TestPadUnderBanner(t *testing.T) {
	bw := BannerContentWidth()
	termW := bw + 20

	SetContentAlign("center")
	left := ContentLeftPad(termW, bw)
	if left != tableLeftPad(termW, bw) {
		t.Fatalf("center pad=%d want tableLeftPad=%d", left, tableLeftPad(termW, bw))
	}
	got := PadUnderBanner("hi\nthere", termW, bw)
	pad := strings.Repeat(" ", left)
	want := pad + "hi\n" + pad + "there"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	// contentW<=0 centers using the text width (narrow-table axis).
	shortLeft := ContentLeftPad(termW, displayWidth("hi"))
	if got0 := PadUnderBanner("hi", termW, 0); got0 != strings.Repeat(" ", shortLeft)+"hi" {
		t.Fatalf("short default: got %q want pad=%d", got0, shortLeft)
	}

	SetContentAlign("left")
	if ContentLeftPad(termW, bw) != 0 {
		t.Fatalf("left align pad want 0")
	}
	bLeft, _ := bannerLayout(termW)
	if bLeft != 0 {
		t.Fatalf("left bannerLayout pad=%d want 0", bLeft)
	}
	art := placeBannerLines("ab\nabcd\n", 10)
	if !strings.HasPrefix(art, "ab\n") {
		t.Fatalf("left banner want no pad, got %q", art)
	}
	SetContentAlign("center") // restore for other tests in package
}

func TestWrapDisplay(t *testing.T) {
	t.Parallel()
	lines := wrapDisplay("abcdefghij", 4)
	if len(lines) != 3 || lines[0] != "abcd" || lines[1] != "efgh" || lines[2] != "ij" {
		t.Fatalf("wrap=%q", lines)
	}
	if got := wrapDisplay("ab", 4); len(got) != 1 || got[0] != "ab" {
		t.Fatalf("short=%q", got)
	}
}

func TestCenterLines(t *testing.T) {
	t.Parallel()
	// Shared pad from widest line ("abcd"=4): pad=(10-4)/2=3 for every line.
	in := "ab\nabcd\n"
	got := centerLines(in, 10)
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if lines[0] != "   ab" {
		t.Fatalf("line0=%q want shared pad", lines[0])
	}
	if lines[1] != "   abcd" {
		t.Fatalf("line1=%q", lines[1])
	}
	// odd width floors to even (11 → 10)
	if got := centerLines(in, 11); got != centerLines(in, 10) {
		t.Fatalf("odd width mismatch:\n%q\n%q", got, centerLines(in, 10))
	}
	// wider than term: no negative pad
	if got := centerLines("abcdefghij", 4); !strings.HasPrefix(got, "abcdefghij") {
		t.Fatalf("overflow=%q", got)
	}
}

func TestDisplayWidthGlyph(t *testing.T) {
	t.Parallel()
	if displayWidth(IconOK) != 1 {
		t.Fatalf("✓: got %d want 1", displayWidth(IconOK))
	}
	if displayWidth(IconKey) != 1 {
		t.Fatalf("K: got %d want 1", displayWidth(IconKey))
	}
	if got := padRight(IconOK, 2); displayWidth(got) != 2 || got != IconOK+" " {
		t.Fatalf("pad ✓ to 2: %q dw=%d", got, displayWidth(got))
	}
	if got := padRight(IconKey, 2); displayWidth(got) != 2 || got != IconKey+" " {
		t.Fatalf("pad K to 2: %q dw=%d", got, displayWidth(got))
	}
	widths := contentWidths([]string{"", "FIELD", "VALUE"}, [][]string{
		{IconOK, "a", "b"},
		{IconKey, "c", "d"},
	})
	if widths[0] != 1 {
		t.Fatalf("glyph col width %d, want 1", widths[0])
	}
}

func TestArgOrEmpty(t *testing.T) {
	t.Parallel()
	if got := ArgOrEmpty([]string{"a", "b"}, 0); got != "a" {
		t.Fatalf("got %q", got)
	}
	if got := ArgOrEmpty([]string{"a"}, 2); got != "" {
		t.Fatalf("got %q", got)
	}
}
