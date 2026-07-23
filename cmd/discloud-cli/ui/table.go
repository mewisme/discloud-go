package ui

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

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

// KVBlock is one labeled section inside a combined FIELD/VALUE table.
type KVBlock struct {
	Title string
	Rows  [][]string
}

// printTable draws a content-sized ASCII table with dim +-| borders.
// rightAlign marks numeric columns; shrinkPrefer is the order to shrink when
// the table is wider than the terminal (defaults to last column).
func printTable(w io.Writer, headers []string, rows [][]string, rightAlign []bool, shrinkPrefer ...int) error {
	if len(headers) == 0 {
		return nil
	}
	on := false
	if f, ok := w.(*os.File); ok {
		on = ColorOn(f)
	}
	widths := contentWidths(headers, rows)
	prefer := shrinkPrefer
	if len(prefer) == 0 {
		prefer = []int{len(headers) - 1}
	}
	fitWidths(widths, termWidth(w), prefer...)

	rule := Dim(on, tableRule(widths))
	fmt.Fprintln(w, rule)
	fmt.Fprintln(w, tableRow(paintRow(headers, widths, rightAlign, on, true), on))
	fmt.Fprintln(w, rule)
	for _, row := range rows {
		fmt.Fprintln(w, tableRow(paintRow(row, widths, rightAlign, on, false), on))
	}
	fmt.Fprintln(w, rule)
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
	on := false
	if f, ok := w.(*os.File); ok {
		on = ColorOn(f)
	}
	headers := []string{"", "FIELD", "VALUE"}
	widths := contentWidths(headers, rows)
	glyphW := widths[0]
	fitWidths(widths, termWidth(w), 2)
	widths[0] = glyphW // emoji are display-width 2; never shrink glyph col

	rule := Dim(on, tableRule(widths))
	fmt.Fprintln(w, rule)
	fmt.Fprintln(w, tableRow(paintRow(headers, widths, nil, on, true), on))
	fmt.Fprintln(w, rule)
	for _, row := range rows {
		glyph, field, value := "", "", ""
		if len(row) > 0 {
			glyph = row[0]
		}
		if len(row) > 1 {
			field = row[1]
		}
		if len(row) > 2 {
			value = row[2]
		}
		cells := []string{
			paintGlyph(on, padRight(glyph, widths[0])),
			Dim(on, padRight(field, widths[1])),
			paintTyped(on, padRight(value, widths[2]), value, ansiCyan),
		}
		fmt.Fprintln(w, tableRow(cells, on))
	}
	fmt.Fprintln(w, rule)
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

// PrintKVBlocks draws one table with full-width section title blocks.
func PrintKVBlocks(w io.Writer, blocks []KVBlock) error {
	if len(blocks) == 0 {
		return nil
	}
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
	fitWidths(widths, termWidth(w), 1)

	// Only the table's top border is solid (no middle +); everything else uses columns.
	spanRule := Dim(on, tableSpanRule(widths))
	colRule := Dim(on, tableRule(widths))
	for i, b := range blocks {
		if i == 0 {
			fmt.Fprintln(w, spanRule)
		} else {
			fmt.Fprintln(w, colRule)
		}
		fmt.Fprintln(w, tableSpanRow(b.Title, widths, on))
		fmt.Fprintln(w, colRule)
		for _, row := range b.Rows {
			fmt.Fprintln(w, tableRow(paintRow(row, widths, nil, on, false), on))
		}
		if i == len(blocks)-1 {
			fmt.Fprintln(w, colRule)
		}
	}
	return nil
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

// PrintFileTable prints a file listing table.
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

// printPickFileTable is PrintFileTable with a 1-based # column for selection.
func printPickFileTable(w io.Writer, files []File) error {
	headers := []string{"#", "ID", "NAME", "SIZE", "STATUS", "VISIBILITY", "EXPIRES"}
	rows := make([][]string, len(files))
	for i, f := range files {
		rows[i] = []string{
			strconv.Itoa(i + 1),
			f.ID,
			f.Name,
			client.FormatBytes(f.Size),
			ShortStatus(f.Status),
			ShortVis(f.Visibility),
			formatTimeShort(f.Expires),
		}
	}
	return printTable(w, headers, rows, []bool{true, false, false, true, false, false, false}, 2, 1)
}

// PrintInspect prints file analytics as KV blocks.
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

func shareDownloadLine(item Inspect) string {
	if item.MaxDownloads == nil {
		return fmt.Sprintf("%d / unlimited", item.DownloadCount)
	}
	return fmt.Sprintf("%d / %d", item.DownloadCount, *item.MaxDownloads)
}

// contentWidths returns max terminal display width per column.
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

// displayWidth is terminal cell count. Non-BMP (emoji) = 2; ASCII/BMP = 1.
func displayWidth(s string) int {
	n := 0
	for _, r := range s {
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
			// KV values: typed color; plain strings stay cyan.
			out[i] = paintTyped(on, plain, c, ansiCyan)
		case i == 0:
			// First column (usually IDs): typed color; ids/strings cyan.
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

// ShortVis returns a short visibility string for tables.
func ShortVis(v string) string {
	switch v {
	case "private", "public":
		return v
	default:
		return v
	}
}

// ShortStatus returns a short file status for tables.
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
	if width == 1 {
		return "…"
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := runeWidth(r)
		if w+rw > width-1 {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	b.WriteRune('…')
	out := b.String()
	if dw := displayWidth(out); dw < width {
		out += strings.Repeat(" ", width-dw)
	}
	return out
}
