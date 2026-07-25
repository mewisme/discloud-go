package ui

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mewisme/discloud-go/internal/client"
	"golang.org/x/term"
)

// File is the UI view of a file row (converted from main's FileItem).
type File struct {
	ID, Name, Visibility, Status string
	Size                         int64
	Expires                      time.Time
}

// Inspect holds the fields PrintInspect needs (same names as InspectResponse).
type Inspect struct {
	FileID            string
	FileName          string
	FileSize          int64
	ChunkSize         int64
	ChunkCount        int
	CreatedAt         time.Time
	ExpiresAt         time.Time
	Visibility        string
	Status            string
	PasswordProtected bool
	ShareMode         string
	MaxDownloads      *int
	DownloadCount     int
	SHA256            string
	Views             int64
	Downloads         int64
	Ranges            int64
	BytesServed       int64
	UniqueVisitors    int64
	LastAccessAt      *time.Time
	URL               string
	LongURL           string
	DownloadURL       string
	LongDownloadURL   string
}

type KVBlock struct {
	Title string
	Rows  [][]string
}

// printTable draws a content-sized ASCII table with dim +-| borders, centered.
// On stdout (non-JSON), clears the screen and reprints the banner first (hidden if term < banner).
// rightAlign marks numeric columns; shrinkPrefer is the order to shrink when
// the table is wider than the terminal (defaults to last column). Cells wrap.
func printTable(w io.Writer, headers []string, rows [][]string, rightAlign []bool, shrinkPrefer ...int) error {
	if len(headers) == 0 {
		return nil
	}
	prepareTableView(w)
	on := false
	if f, ok := w.(*os.File); ok {
		on = ColorOn(f)
	}
	widths := contentWidths(headers, rows)
	prefer := shrinkPrefer
	if len(prefer) == 0 {
		prefer = []int{len(headers) - 1}
	}
	growCol := prefer[0]
	layoutTableWidths(widths, termWidth(w), growCol, prefer...)

	left := tableCenterPad(w, tableOuterWidth(widths))
	rule := Dim(on, tableRule(widths))
	var lines []string
	lines = append(lines, rule)
	lines = appendWrappedRow(lines, headers, widths, rightAlign, on, true)
	lines = append(lines, rule)
	for _, row := range rows {
		lines = appendWrappedRow(lines, row, widths, rightAlign, on, false)
	}
	lines = append(lines, rule)
	writeCentered(w, left, lines...)
	return nil
}

// PrintKVTable is a 2-column FIELD/VALUE table (labels dim, values typed-color).
func PrintKVTable(w io.Writer, rows [][]string) error {
	return printTable(w, []string{"FIELD", "VALUE"}, rows, nil, 1)
}

// PrintGlyphTable is a 3-column table: glyph | FIELD | VALUE (with headers).
// Pass plain IconOK / IconFail / IconKey / etc. in column 0.
func PrintGlyphTable(w io.Writer, rows [][]string) error {
	if len(rows) == 0 {
		return nil
	}
	prepareTableView(w)
	on := false
	if f, ok := w.(*os.File); ok {
		on = ColorOn(f)
	}
	headers := []string{"", "FIELD", "VALUE"}
	widths := contentWidths(headers, rows)
	glyphW := widths[0]
	layoutTableWidths(widths, termWidth(w), 2, 2)
	widths[0] = glyphW // emoji are display-width 2; never shrink glyph col
	if tableOuterWidth(widths) > termWidth(w) && termWidth(w) > 0 {
		fitWidths(widths, termWidth(w), 2)
		widths[0] = glyphW
	}

	left := tableCenterPad(w, tableOuterWidth(widths))
	rule := Dim(on, tableRule(widths))
	var lines []string
	lines = append(lines, rule)
	lines = appendGlyphWrappedRow(lines, headers, widths, on, true)
	lines = append(lines, rule)
	for _, row := range rows {
		lines = appendGlyphWrappedRow(lines, row, widths, on, false)
	}
	lines = append(lines, rule)
	writeCentered(w, left, lines...)
	return nil
}

func paintGlyph(on bool, padded string) string {
	switch strings.TrimSpace(padded) {
	case IconOK:
		return Green(on, padded)
	case IconFail:
		return Red(on, padded)
	case IconKey, IconLock:
		return Yellow(on, padded)
	case IconUnlock, IconInfo:
		return Cyan(on, padded)
	default:
		return padded
	}
}

// PrintKVBlocks draws one table with full-width section title blocks, centered.
// On stdout (non-JSON), clears and shows the banner when the terminal is wide enough.
// Table min width matches the banner; on narrow terminals the banner is hidden and cells wrap.
func PrintKVBlocks(w io.Writer, blocks []KVBlock) error {
	if len(blocks) == 0 {
		return nil
	}
	prepareTableView(w)
	on := false
	if f, ok := w.(*os.File); ok {
		on = ColorOn(f)
	}
	headers := []string{"", ""} // two columns; no header labels
	var all [][]string
	for _, b := range blocks {
		all = append(all, b.Rows...)
		all = append(all, []string{b.Title, ""})
	}
	widths := contentWidths(headers, all)
	layoutTableWidths(widths, termWidth(w), 1, 1)

	var lines []string
	spanRule := Dim(on, tableSpanRule(widths))
	colRule := Dim(on, tableRule(widths))
	for i, b := range blocks {
		if i == 0 {
			lines = append(lines, spanRule)
		} else {
			lines = append(lines, colRule)
		}
		lines = append(lines, tableSpanRow(b.Title, widths, on))
		lines = append(lines, colRule)
		for _, row := range b.Rows {
			lines = appendWrappedRow(lines, row, widths, nil, on, false)
		}
		if i == len(blocks)-1 {
			lines = append(lines, colRule)
		}
	}
	writeCentered(w, tableCenterPad(w, tableOuterWidth(widths)), lines...)
	return nil
}

// prepareTableView clears the TTY; shows the banner only when term width >= banner art.
func prepareTableView(w io.Writer) {
	if JSON {
		return
	}
	f, ok := w.(*os.File)
	if !ok || f != os.Stdout {
		return
	}
	clearScreen()
	tw := termWidth(w)
	if tw >= BannerContentWidth() {
		fmt.Fprint(os.Stderr, FormatBanner(tw))
	}
}

func tableCenterPad(w io.Writer, contentW int) string {
	return strings.Repeat(" ", tableLeftPad(termWidth(w), contentW))
}

// tableLeftPad centers contentW under the banner (same axis as bannerLayout).
// When the terminal is too narrow for the banner, centers in the terminal instead.
func tableLeftPad(termW, contentW int) int {
	if termW < 1 {
		return 0
	}
	bw := BannerContentWidth()
	if termW < bw {
		return centerPad(contentW, termW)
	}
	bLeft, bW := bannerLayout(termW)
	// Center of banner → left edge of table.
	left := bLeft + (bW-contentW)/2
	if left < 0 {
		left = 0
	}
	// Keep table inside the terminal when it is wider than the banner.
	if left+contentW > termW {
		left = centerPad(contentW, termW)
	}
	return left
}

func writeCentered(w io.Writer, left string, lines ...string) {
	for _, line := range lines {
		fmt.Fprintln(w, left+line)
	}
}

// centerPad is left spaces to center contentW in termW.
func centerPad(contentW, termW int) int {
	if termW < 1 || contentW >= termW {
		return 0
	}
	return (termW - contentW) / 2
}

// tableOuterWidth is the bordered table width for column widths.
func tableOuterWidth(widths []int) int {
	n := 3*len(widths) + 1
	for _, w := range widths {
		n += w
	}
	return n
}

// tableTargetOuter is min table width: banner width, or terminal width when narrower than the banner.
func tableTargetOuter(termW int) int {
	bw := BannerContentWidth()
	if termW > 0 && termW < bw {
		return termW
	}
	return bw
}

// layoutTableWidths grows the table to at least the banner (or term) width, then fits to term.
func layoutTableWidths(widths []int, termW, growCol int, prefer ...int) {
	growColToMinOuter(widths, growCol, tableTargetOuter(termW))
	if termW > 0 {
		fitWidths(widths, termW, prefer...)
	}
}

func growColToMinOuter(widths []int, col, minOuter int) {
	if col < 0 || col >= len(widths) || minOuter < 1 {
		return
	}
	cur := tableOuterWidth(widths)
	if cur >= minOuter {
		return
	}
	widths[col] += minOuter - cur
}

// tableSpanRow is a full-width section label with no middle | divider.
func tableSpanRow(title string, widths []int, on bool) string {
	bar := Dim(on, "|")
	return bar + " " + Bold(on, padRight(title, spanInner(widths))) + " " + bar
}

// tableSpanRule is a solid +---+ rule matching tableSpanRow width (no column +).
func tableSpanRule(widths []int) string {
	return "+" + strings.Repeat("-", spanInner(widths)+2) + "+"
}

func spanInner(widths []int) int {
	inner := 0
	for i, w := range widths {
		inner += w
		if i > 0 {
			inner += 3 // " | " between columns
		}
	}
	return inner
}

func PrintFileTable(w io.Writer, files []File) error {
	headers := []string{"ID", "NAME", "SIZE", "STATUS", "VISIBILITY", "EXPIRES"}
	rows := make([][]string, len(files))
	for i, f := range files {
		rows[i] = []string{
			f.ID,
			f.Name,
			client.FormatBytes(f.Size),
			ShortStatus(f.Status),
			ShortVis(f.Visibility),
			formatTimeShort(f.Expires),
		}
	}
	return printTable(w, headers, rows, []bool{false, false, true, false, false, false}, 1, 0)
}

type UploadRow struct {
	Name, ID, SHA256, AccessToken, Status string
	Size                                  int64
}

func PrintUploadTable(w io.Writer, items []UploadRow) error {
	if len(items) == 0 {
		return nil
	}
	showSHA := false
	showTok := false
	for _, it := range items {
		if it.SHA256 != "" {
			showSHA = true
		}
		if it.AccessToken != "" {
			showTok = true
		}
	}
	headers := []string{"NAME", "SIZE", "STATUS", "ID"}
	right := []bool{false, true, false, false}
	prefer := []int{0, 3}
	if showSHA {
		headers = append(headers, "SHA256")
		right = append(right, false)
		prefer = append([]int{len(headers) - 1}, prefer...)
	}
	if showTok {
		headers = append(headers, "TOKEN")
		right = append(right, false)
		prefer = append([]int{len(headers) - 1}, prefer...)
	}
	rows := make([][]string, len(items))
	for i, it := range items {
		row := []string{it.Name, client.FormatBytes(it.Size), ShortStatus(it.Status), it.ID}
		if showSHA {
			row = append(row, sha256OrDash(it.SHA256))
		}
		if showTok {
			tok := it.AccessToken
			if tok == "" {
				tok = "-"
			}
			row = append(row, tok)
		}
		rows[i] = row
	}
	return printTable(w, headers, rows, right, prefer...)
}

func PrintInspect(w io.Writer, item Inspect) error {
	last := "-"
	if item.LastAccessAt != nil && !item.LastAccessAt.IsZero() {
		last = FormatTime(*item.LastAccessAt)
	}
	return PrintKVBlocks(w, []KVBlock{
		{Title: "File", Rows: [][]string{
			{"id", item.FileID},
			{"name", item.FileName},
			{"size", client.FormatBytes(item.FileSize)},
			{"chunks", fmt.Sprintf("%d × %s", item.ChunkCount, client.FormatBytes(item.ChunkSize))},
			{"sha256", sha256OrDash(item.SHA256)},
			{"visibility", ShortVis(item.Visibility)},
			{"status", ShortStatus(item.Status)},
			{"share mode", shareModeOrDefault(item.ShareMode)},
			{"password", fmt.Sprintf("%v", item.PasswordProtected)},
			{"created", FormatTime(item.CreatedAt)},
			{"expires", FormatTime(item.ExpiresAt)},
		}},
		{Title: "Stats", Rows: [][]string{
			{"views", fmt.Sprintf("%d", item.Views)},
			{"downloads", fmt.Sprintf("%d", item.Downloads)},
			{"share downloads", shareDownloadLine(item)},
			{"ranges", fmt.Sprintf("%d", item.Ranges)},
			{"bytes served", client.FormatBytes(item.BytesServed)},
			{"unique visitors", fmt.Sprintf("%d", item.UniqueVisitors)},
			{"last access", last},
		}},
		{Title: "Links", Rows: [][]string{
			{"url", item.URL},
			{"download", item.DownloadURL},
			{"long url", item.LongURL},
			{"long download", item.LongDownloadURL},
		}},
	})
}

func shareModeOrDefault(mode string) string {
	if mode == "" {
		return "download"
	}
	return mode
}

func sha256OrDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func shareDownloadLine(item Inspect) string {
	if item.MaxDownloads == nil {
		return fmt.Sprintf("%d / unlimited", item.DownloadCount)
	}
	return fmt.Sprintf("%d / %d", item.DownloadCount, *item.MaxDownloads)
}

func contentWidths(headers []string, rows [][]string) []int {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = displayWidth(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				break
			}
			if n := displayWidth(cell); n > widths[i] {
				widths[i] = n
			}
		}
	}
	return widths
}

// displayWidth is terminal cell count (ANSI escapes ignored). Non-BMP (emoji) = 2; ASCII/BMP = 1.
func displayWidth(s string) int {
	n := 0
	for _, r := range stripANSI(s) {
		n += runeWidth(r)
	}
	return n
}

func runeWidth(r rune) int {
	switch {
	case r == 0 || r == '\n' || r == '\r' || r == '\t':
		return 0
	case r > 0xffff: // emoji / astral plane — double-width in terminals
		return 2
	default:
		return 1
	}
}

// fitWidths shrinks columns so the bordered table fits termW.
func fitWidths(widths []int, termW int, prefer ...int) {
	const minCol = 4
	overhead := 3*len(widths) + 1
	need := overhead
	for _, w := range widths {
		need += w
	}
	if need <= termW {
		return
	}
	extra := need - termW
	shrink := append([]int{}, prefer...)
	for i := range widths {
		found := false
		for _, p := range prefer {
			if p == i {
				found = true
				break
			}
		}
		if !found {
			shrink = append(shrink, i)
		}
	}
	for _, i := range shrink {
		if extra <= 0 {
			break
		}
		can := widths[i] - minCol
		if can <= 0 {
			continue
		}
		if can > extra {
			widths[i] -= extra
			extra = 0
			break
		}
		widths[i] = minCol
		extra -= can
	}
}

// KVBlocksValueWidth returns the value-column width for progress previews.
// Pass draft rows (empty progress previews) so bars do not inflate the measure.
func KVBlocksValueWidth(w io.Writer, blocks []KVBlock) int {
	if len(blocks) == 0 {
		return 8
	}
	headers := []string{"", ""}
	var all [][]string
	for _, b := range blocks {
		all = append(all, b.Rows...)
		all = append(all, []string{b.Title, ""})
	}
	widths := contentWidths(headers, all)
	layoutTableWidths(widths, termWidth(w), 1, 1)
	if widths[1] < 8 {
		return 8
	}
	return widths[1]
}

// wrapDisplay splits s into lines of at most width terminal cells (ANSI stripped when wrapping).
func wrapDisplay(s string, width int) []string {
	if width < 1 {
		return []string{""}
	}
	if displayWidth(s) <= width {
		return []string{s}
	}
	plain := stripANSI(s)
	var lines []string
	var cur strings.Builder
	curW := 0
	for _, r := range plain {
		rw := runeWidth(r)
		if rw <= 0 {
			continue
		}
		if curW+rw > width && curW > 0 {
			lines = append(lines, cur.String())
			cur.Reset()
			curW = 0
		}
		if rw > width {
			if curW > 0 {
				lines = append(lines, cur.String())
				cur.Reset()
				curW = 0
			}
			lines = append(lines, string(r))
			continue
		}
		cur.WriteRune(r)
		curW += rw
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func padExact(s string, width int, right bool) string {
	dw := displayWidth(s)
	if dw >= width {
		return s
	}
	pad := strings.Repeat(" ", width-dw)
	if right {
		return pad + s
	}
	return s + pad
}

func appendWrappedRow(dst []string, cells []string, widths []int, rightAlign []bool, on, header bool) []string {
	ncols := len(widths)
	wrapped := make([][]string, len(cells))
	maxL := 1
	for i, c := range cells {
		w := 0
		if i < len(widths) {
			w = widths[i]
		}
		parts := wrapDisplay(c, w)
		painted := make([]string, len(parts))
		alignRight := rightAlign != nil && i < len(rightAlign) && rightAlign[i]
		for j, part := range parts {
			plain := padExact(part, w, alignRight)
			switch {
			case header:
				painted[j] = Bold(on, plain)
			case ncols == 2 && i == 0:
				painted[j] = Dim(on, plain)
			case ncols == 2 && i == 1:
				if strings.Contains(part, "\x1b") {
					painted[j] = plain
				} else {
					painted[j] = paintTyped(on, plain, part, ansiCyan)
				}
			case i == 0:
				painted[j] = paintTyped(on, plain, part, ansiCyan)
			default:
				painted[j] = paintTyped(on, plain, part, "")
			}
		}
		wrapped[i] = painted
		if len(painted) > maxL {
			maxL = len(painted)
		}
	}
	for j := 0; j < maxL; j++ {
		row := make([]string, len(widths))
		for i := range row {
			w := widths[i]
			if i < len(wrapped) && j < len(wrapped[i]) {
				row[i] = wrapped[i][j]
			} else {
				row[i] = strings.Repeat(" ", w)
			}
		}
		dst = append(dst, tableRow(row, on))
	}
	return dst
}

func appendGlyphWrappedRow(dst []string, cells []string, widths []int, on, header bool) []string {
	glyph, field, value := "", "", ""
	if len(cells) > 0 {
		glyph = cells[0]
	}
	if len(cells) > 1 {
		field = cells[1]
	}
	if len(cells) > 2 {
		value = cells[2]
	}
	gParts := wrapDisplay(glyph, widths[0])
	fParts := wrapDisplay(field, widths[1])
	vParts := wrapDisplay(value, widths[2])
	maxL := len(gParts)
	if len(fParts) > maxL {
		maxL = len(fParts)
	}
	if len(vParts) > maxL {
		maxL = len(vParts)
	}
	for j := 0; j < maxL; j++ {
		g, f, v := "", "", ""
		if j < len(gParts) {
			g = gParts[j]
		}
		if j < len(fParts) {
			f = fParts[j]
		}
		if j < len(vParts) {
			v = vParts[j]
		}
		gp := padExact(g, widths[0], false)
		fp := padExact(f, widths[1], false)
		vp := padExact(v, widths[2], false)
		var row []string
		if header {
			row = []string{Bold(on, gp), Bold(on, fp), Bold(on, vp)}
		} else {
			row = []string{
				paintGlyph(on, gp),
				Dim(on, fp),
				paintTyped(on, vp, v, ansiCyan),
			}
		}
		dst = append(dst, tableRow(row, on))
	}
	return dst
}

func paintRow(cells []string, widths []int, rightAlign []bool, on, header bool) []string {
	out := make([]string, len(cells))
	ncols := len(widths)
	for i, c := range cells {
		w := 0
		if i < len(widths) {
			w = widths[i]
		}
		alignRight := rightAlign != nil && i < len(rightAlign) && rightAlign[i]
		var plain string
		if alignRight {
			plain = padLeft(c, w)
		} else {
			plain = padRight(c, w)
		}
		switch {
		case header:
			out[i] = Bold(on, plain)
		case ncols == 2 && i == 0:
			out[i] = Dim(on, plain)
		case ncols == 2 && i == 1:
			if strings.Contains(c, "\x1b") {
				out[i] = plain
			} else {
				out[i] = paintTyped(on, plain, c, ansiCyan)
			}
		case i == 0:
			out[i] = paintTyped(on, plain, c, ansiCyan)
		default:
			out[i] = paintTyped(on, plain, c, "")
		}
	}
	return out
}

// paintTyped colors the full padded cell from the raw value's type.
func paintTyped(on bool, padded, raw, stringCode string) string {
	t := strings.TrimSpace(raw)
	switch {
	case t == "" || t == "-":
		return Dim(on, padded)
	case t == "true", t == "ok", t == "ready", t == "up", t == "public":
		return Green(on, padded)
	case t == "reused":
		return Cyan(on, padded)
	case t == "false", t == "fail", t == "error", t == "down":
		return Red(on, padded)
	case t == "private":
		return Yellow(on, padded)
	case looksNumber(t), looksBytes(t):
		return Cyan(on, padded)
	case looksTime(t):
		return Dim(on, padded)
	case stringCode != "":
		return paint(on, stringCode, padded)
	default:
		return padded
	}
}

func looksNumber(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

func looksBytes(s string) bool {
	parts := strings.Fields(s)
	if len(parts) != 2 {
		return false
	}
	if _, err := strconv.ParseFloat(parts[0], 64); err != nil {
		return false
	}
	switch parts[1] {
	case "B", "KB", "MB", "GB", "TB", "KiB", "MiB", "GiB", "TiB":
		return true
	default:
		return false
	}
}

func looksTime(s string) bool {
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		if _, err := time.Parse("2006-01-02", s[:10]); err == nil && (len(s) == 10 || s[10] == 'T') {
			return true
		}
	}
	_, err := time.Parse(time.RFC3339, s)
	return err == nil
}

func tableRule(widths []int) string {
	var b strings.Builder
	b.WriteByte('+')
	for _, w := range widths {
		b.WriteString(strings.Repeat("-", w+2))
		b.WriteByte('+')
	}
	return b.String()
}

func tableRow(cells []string, on bool) string {
	bar := Dim(on, "|")
	var b strings.Builder
	b.WriteString(bar)
	for _, c := range cells {
		b.WriteByte(' ')
		b.WriteString(c)
		b.WriteByte(' ')
		b.WriteString(bar)
	}
	return b.String()
}

func termWidth(w io.Writer) int {
	f, ok := w.(*os.File)
	if !ok {
		return 120
	}
	width, _, err := term.GetSize(int(f.Fd()))
	if err != nil || width < 40 {
		return 120
	}
	return width
}

func ShortVis(v string) string {
	switch v {
	case "private", "public":
		return v
	default:
		return v
	}
}

// ShortStatus maps API status for tables; empty means ready.
func ShortStatus(v string) string {
	switch v {
	case "ready", "reused":
		return v
	case "":
		return "ready"
	default:
		return v
	}
}

// FormatTime formats t as RFC3339 UTC, or "-" if zero.
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}

// FormatTimeOrDash is an alias of FormatTime for call-site clarity.
func FormatTimeOrDash(t time.Time) string {
	return FormatTime(t)
}

func formatTimeShort(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format("2006-01-02")
}

func padRight(s string, width int) string {
	if width <= 0 {
		return ""
	}
	dw := displayWidth(s)
	if dw > width {
		return truncateDisplay(s, width)
	}
	return s + strings.Repeat(" ", width-dw)
}

func padLeft(s string, width int) string {
	if width <= 0 {
		return ""
	}
	dw := displayWidth(s)
	if dw > width {
		return truncateDisplay(s, width)
	}
	return strings.Repeat(" ", width-dw) + s
}

func truncateDisplay(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if displayWidth(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	var b strings.Builder
	vis := 0
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' {
			j := i + 1
			if j < len(s) && s[j] == '[' {
				j++
				for j < len(s) {
					c := s[j]
					j++
					if c >= '@' && c <= '~' {
						break
					}
				}
				b.WriteString(s[i:j])
				i = j
				continue
			}
		}
		r, sz := utf8.DecodeRuneInString(s[i:])
		rw := runeWidth(r)
		if vis+rw > width-1 {
			break
		}
		b.WriteRune(r)
		vis += rw
		i += sz
	}
	b.WriteRune('…')
	if strings.Contains(s, "\x1b") {
		b.WriteString(ansiReset)
	}
	out := b.String()
	if dw := displayWidth(out); dw < width {
		out += strings.Repeat(" ", width-dw)
	}
	return out
}
