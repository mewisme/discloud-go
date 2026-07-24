package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mewisme/discloud-go/internal/client"
	"golang.org/x/term"
)

const (
	defaultMaxNameWidth = 24
	defaultTermWidth    = 80
	statusColWidth      = 10
	sentColWidth        = 10
	totalColWidth       = 10
	pctColWidth         = 4
	speedColWidth       = 12
	minBarWidth         = 10
)

type ProgressFile struct {
	Name  string
	Total int64
}

type ProgressLayout struct {
	NameWidth int
}

// NewProgressLayout computes one shared filename column width from names,
// capped at maxNameWidth (default 24).
func NewProgressLayout(names []string) ProgressLayout {
	return NewProgressLayoutSized(names, defaultMaxNameWidth)
}

// NewProgressLayoutSized is NewProgressLayout with an explicit name-width cap.
func NewProgressLayoutSized(names []string, maxNameWidth int) ProgressLayout {
	if maxNameWidth < 1 {
		maxNameWidth = defaultMaxNameWidth
	}
	nw := 0
	for _, n := range names {
		if w := lipgloss.Width(n); w > nw {
			nw = w
		}
	}
	if nw > maxNameWidth {
		nw = maxNameWidth
	}
	if nw < 1 {
		nw = 1
	}
	return ProgressLayout{NameWidth: nw}
}

// fitName pads or truncates s to exactly width terminal cells (lipgloss.Width).
func fitName(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := lipgloss.Width(s)
	if w == width {
		return s
	}
	if w < width {
		return s + strings.Repeat(" ", width-w)
	}
	const ell = "…"
	ew := lipgloss.Width(ell)
	target := width - ew
	if target < 1 {
		return truncateCells(s, width)
	}
	out := truncateCells(s, target) + ell
	if dw := lipgloss.Width(out); dw < width {
		out += strings.Repeat(" ", width-dw)
	}
	return out
}

func truncateCells(s string, width int) string {
	if width <= 0 {
		return ""
	}
	var b strings.Builder
	cur := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if cur+rw > width {
			break
		}
		b.WriteRune(r)
		cur += rw
	}
	out := b.String()
	if dw := lipgloss.Width(out); dw < width {
		out += strings.Repeat(" ", width-dw)
	}
	return out
}

func padLeftWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return strings.Repeat(" ", width-w) + s
}

func padRightWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

type fileStatus int

const (
	statusWaiting fileStatus = iota
	statusUploading
	statusComplete
	statusAborted
)

func (s fileStatus) label() string {
	switch s {
	case statusUploading:
		return "Uploading"
	case statusComplete:
		return "Complete"
	case statusAborted:
		return "Aborted"
	default:
		return "Waiting"
	}
}

// statusIcon returns the row glyph with a status color (✓ green, ✗ red, ● cyan, ○ dim).
func (s fileStatus) statusIcon() string {
	var glyph, fg string
	switch s {
	case statusComplete:
		glyph, fg = IconOK, "2"
	case statusAborted:
		glyph, fg = IconFail, "1"
	case statusUploading:
		glyph, fg = "●", "6"
	default:
		glyph, fg = "○", "8"
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(fg)).Render(glyph)
}

type fileRow struct {
	name    string
	total   int64
	sent    int64
	status  fileStatus
	started time.Time
	bar     progress.Model
}

type fileProgressMsg struct {
	id   int
	sent int64
}

type fileDoneMsg struct{ id int }

type fileAbortMsg struct{ id int }

// hideProgressMsg clears the inline progress area, then quits (no cancel).
// tea.QuitMsg alone exits before Update/render, so the last frame would stick.
type hideProgressMsg struct{}

// userCancelMsg is Ctrl+C / Esc / SIGINT: cancel upload context, then quit.
type userCancelMsg struct{}

type progressModel struct {
	rows     []fileRow
	layout   ProgressLayout
	width    int
	barWidth int
	quitting bool
	cancel   context.CancelFunc
}

func newProgressModel(files []ProgressFile, layout ProgressLayout, termWidth int, cancel context.CancelFunc) *progressModel {
	if termWidth < 1 {
		termWidth = defaultTermWidth
	}
	m := &progressModel{
		rows:   make([]fileRow, len(files)),
		layout: layout,
		width:  termWidth,
		cancel: cancel,
	}
	m.recalcBarWidth()
	for i, f := range files {
		total := f.Total
		if total < 0 {
			total = 0
		}
		m.rows[i] = fileRow{
			name:   f.Name,
			total:  total,
			status: statusWaiting,
			bar:    newFileProgressBar(m.barWidth),
		}
	}
	return m
}

func newFileProgressBar(width int) progress.Model {
	return progress.New(
		progress.WithFillCharacters('━', '─'),
		progress.WithoutPercentage(),
		progress.WithColors(
			lipgloss.Color("#3f51b1"),
			lipgloss.Color("#5a55ae"),
			lipgloss.Color("#7b5fac"),
			lipgloss.Color("#8f6aae"),
			lipgloss.Color("#a86aa4"),
			lipgloss.Color("#cc6b8e"),
			lipgloss.Color("#f18271"),
			lipgloss.Color("#f3a469"),
			lipgloss.Color("#f7c978"),
		),
		progress.WithScaled(true),
		progress.WithWidth(width),
	)
}

func (m *progressModel) fixedColsWidth() int {
	// "○ " + name + "  " + status + "  " + bar + "  " + sent + " / " + total + "  " + pct + "  " + speed
	return 2 + m.layout.NameWidth + 2 + statusColWidth + 2 + 2 + sentColWidth + 3 + totalColWidth + 2 + pctColWidth + 2 + speedColWidth
}

func (m *progressModel) recalcBarWidth() {
	bw := m.width - m.fixedColsWidth()
	if bw < minBarWidth {
		bw = minBarWidth
	}
	m.barWidth = bw
	for i := range m.rows {
		m.rows[i].bar.SetWidth(bw)
	}
}

func (m *progressModel) Init() tea.Cmd { return nil }

func (m *progressModel) requestCancel() tea.Cmd {
	if m.cancel != nil {
		m.cancel()
	}
	m.quitting = true
	return tea.Quit
}

func (m *progressModel) hideAndQuit() tea.Cmd {
	m.quitting = true
	return tea.Quit
}

func (m *progressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
			m.recalcBarWidth()
		}
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, m.requestCancel()
		}
	case fileProgressMsg:
		if msg.id < 0 || msg.id >= len(m.rows) {
			return m, nil
		}
		r := &m.rows[msg.id]
		if r.status == statusComplete || r.status == statusAborted {
			return m, nil
		}
		if r.status == statusWaiting {
			r.status = statusUploading
			r.started = time.Now()
		}
		if msg.sent < 0 {
			msg.sent = 0
		}
		r.sent = msg.sent
	case fileDoneMsg:
		if msg.id < 0 || msg.id >= len(m.rows) {
			return m, nil
		}
		r := &m.rows[msg.id]
		r.status = statusComplete
		r.sent = r.total
	case fileAbortMsg:
		if msg.id < 0 || msg.id >= len(m.rows) {
			return m, nil
		}
		r := &m.rows[msg.id]
		if r.status != statusComplete {
			r.status = statusAborted
		}
	case hideProgressMsg:
		return m, m.hideAndQuit()
	case userCancelMsg:
		return m, m.requestCancel()
	}
	return m, nil
}

func (m *progressModel) View() tea.View {
	if m.quitting || len(m.rows) == 0 {
		return tea.NewView("")
	}
	var b strings.Builder
	for i := range m.rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(m.renderRow(i))
	}
	return tea.NewView(b.String())
}

func (m *progressModel) renderRow(i int) string {
	r := &m.rows[i]

	pct := 0.0
	if r.total > 0 {
		pct = float64(r.sent) / float64(r.total)
		if pct > 1 {
			pct = 1
		}
	} else if r.status == statusComplete {
		pct = 1
	}

	bar := r.bar.ViewAs(pct)
	sent := padLeftWidth(client.FormatBytes(r.sent), sentColWidth)
	total := padLeftWidth(client.FormatBytes(r.total), totalColWidth)
	pctStr := padLeftWidth(fmt.Sprintf("%d%%", int(pct*100+0.5)), pctColWidth)

	speed := ""
	if r.status == statusUploading && !r.started.IsZero() {
		elapsed := time.Since(r.started).Seconds()
		if elapsed > 0 {
			speed = client.FormatBytes(int64(float64(r.sent)/elapsed)) + "/s"
		}
	}
	speed = padLeftWidth(speed, speedColWidth)

	return fmt.Sprintf("%s %s  %s  %s  %s / %s  %s  %s",
		r.status.statusIcon(),
		fitName(r.name, m.layout.NameWidth),
		padRightWidth(r.status.label(), statusColWidth),
		bar,
		sent,
		total,
		pctStr,
		speed,
	)
}

func (m *progressModel) renderAllPlain() string {
	if m.quitting || len(m.rows) == 0 {
		return ""
	}
	var b strings.Builder
	for i := range m.rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(m.renderRow(i))
	}
	return b.String()
}

// UploadProgress hosts a Bubble Tea program for upload bars.
type UploadProgress struct {
	p      *tea.Program
	bars   []*FileBar
	once   sync.Once
	ctx    context.Context
	cancel context.CancelFunc
}

type FileBar struct {
	id    int
	p     *tea.Program
	total int64
	mu    sync.Mutex
	done  bool
}

// NewUploadProgress starts an inline Bubble Tea progress UI on w (stderr if nil).
// Ctrl+C / Esc cancels Context() so callers can stop the upload.
func NewUploadProgress(w io.Writer, files []ProgressFile) *UploadProgress {
	if w == nil {
		w = os.Stderr
	}
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.Name
	}
	layout := NewProgressLayout(names)
	termW := progressTermWidth(w)
	ctx, cancel := context.WithCancel(context.Background())
	m := newProgressModel(files, layout, termW, cancel)
	p := tea.NewProgram(m,
		tea.WithOutput(w),
		// Must read the TTY: Bubble Tea queries DEC modes 2026/2027 and the
		// terminal replies with DECRPM (e.g. "?2027;3$y"). WithInput(nil)
		// leaves that reply in the console buffer → PowerShell "autofills" it.
		tea.WithInput(os.Stdin),
		// Turn SIGINT into userCancelMsg instead of ErrInterrupted
		// (which would exit tea without cancelling the upload goroutine).
		tea.WithFilter(func(_ tea.Model, msg tea.Msg) tea.Msg {
			if _, ok := msg.(tea.InterruptMsg); ok {
				return userCancelMsg{}
			}
			return msg
		}),
	)
	u := &UploadProgress{p: p, bars: make([]*FileBar, len(files)), ctx: ctx, cancel: cancel}
	for i, f := range files {
		total := f.Total
		if total < 0 {
			total = 0
		}
		u.bars[i] = &FileBar{id: i, p: p, total: total}
	}
	go func() { _, _ = p.Run() }()
	return u
}

// Context is cancelled when the user presses Ctrl+C or Esc.
func (u *UploadProgress) Context() context.Context {
	if u == nil || u.ctx == nil {
		return context.Background()
	}
	return u.ctx
}

func progressTermWidth(w io.Writer) int {
	type fdWriter interface{ Fd() uintptr }
	if f, ok := w.(fdWriter); ok {
		if width, _, err := term.GetSize(int(f.Fd())); err == nil && width > 0 {
			return width
		}
	}
	return defaultTermWidth
}

func (u *UploadProgress) Bar(i int) *FileBar {
	if u == nil || i < 0 || i >= len(u.bars) {
		return nil
	}
	return u.bars[i]
}

// Wait clears the progress UI, then blocks until the program has exited.
func (u *UploadProgress) Wait() {
	if u == nil || u.p == nil {
		return
	}
	u.once.Do(func() {
		u.p.Send(hideProgressMsg{})
		u.p.Wait()
	})
}

func (f *FileBar) Update(sent, _ int64) {
	if f == nil || f.p == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.done {
		return
	}
	if sent < 0 {
		sent = 0
	}
	f.p.Send(fileProgressMsg{id: f.id, sent: sent})
}

func (f *FileBar) Done() {
	if f == nil || f.p == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.done {
		return
	}
	f.done = true
	f.p.Send(fileDoneMsg{id: f.id})
}

// Abort marks the bar aborted without forcing 100%.
func (f *FileBar) Abort() {
	if f == nil || f.p == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.done {
		return
	}
	f.done = true
	f.p.Send(fileAbortMsg{id: f.id})
}
