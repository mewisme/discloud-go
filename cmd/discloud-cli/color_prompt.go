package main

import (
	"fmt"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	bubblepicker "github.com/madicen/bubble-color-picker"
	"github.com/mewisme/discloud-go/cmd/discloud-cli/ui"
	"github.com/mewisme/discloud-go/internal/client"
)

// hexClipboardMsg is clipboard text from ctrl+v while hex is focused.
type hexClipboardMsg string

const (
	zoneStepPrev    = "grad-step-prev"
	zoneStepNext    = "grad-step-next"
	zoneStepReset   = "grad-step-reset"
	zoneStepLongHue = "grad-step-long-hue"
	zoneHexInput    = "grad-hex-input"
	minGradSteps    = 1
	maxGradSteps    = 16
)

// promptHexColors edits a fixed-length gradient: live preview, prev/next step, Enter replaces current step.
// Esc saves; ctrl+c cancels. Press h (or click hex) to type a hex value directly.
// Toggle long hue with the nav button or key l.
func promptHexColors(label string, steps []string, longHue bool) (colors []string, outLongHue bool, canceled bool, err error) {
	if len(steps) == 0 {
		return nil, false, true, fmt.Errorf("at least one gradient step required")
	}
	for i, c := range steps {
		if norm, e := client.NormalizeHexColor(c); e == nil {
			steps[i] = norm
		}
	}

	zm := zone.New()
	picker := newStepPicker(steps[0], zm)
	hex := textinput.New()
	hex.Placeholder = "#rrggbb"
	hex.CharLimit = 7
	hex.Width = 7
	hex.Prompt = ""
	hex.SetValue(steps[0])
	hex.Blur()

	m := &colorPickModel{
		label:    label,
		picker:   picker,
		zm:       zm,
		steps:    append([]string(nil), steps...),
		index:    0,
		hexInput: hex,
		longHue:  longHue,
		width:    80,
		height:   24,
	}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseAllMotion())
	final, err := p.Run()
	if err != nil {
		return nil, false, false, err
	}
	out, ok := final.(*colorPickModel)
	if !ok || out == nil {
		return nil, false, true, fmt.Errorf("color picker ended unexpectedly")
	}
	if out.canceled {
		return nil, false, true, nil
	}
	out.applyLiveToStep()
	norm, err := client.NormalizeHexColors(out.steps)
	if err != nil {
		return nil, false, false, err
	}
	return norm, out.longHue, false, nil
}

func newStepPicker(initial string, zm *zone.Manager) bubblepicker.Model {
	picker := bubblepicker.New(
		bubblepicker.WithInitialColor(initial),
		bubblepicker.WithAutoDismiss(false),
	)
	picker.SetZoneManager(zm)
	return picker
}

type colorPickModel struct {
	label      string
	picker     bubblepicker.Model
	zm         *zone.Manager
	steps      []string
	index      int
	hexInput   textinput.Model
	hexFocused bool
	hexErr     string
	longHue    bool
	canceled   bool
	width      int
	height     int
}

func (m *colorPickModel) Init() tea.Cmd { return nil }

func (m *colorPickModel) applyLiveToStep() {
	if m.index < 0 || m.index >= len(m.steps) {
		return
	}
	v := strings.TrimSpace(m.picker.Value())
	if v == "" {
		return
	}
	if norm, err := client.NormalizeHexColor(v); err == nil {
		v = norm
	}
	m.steps[m.index] = v
	if !m.hexFocused {
		m.hexInput.SetValue(v)
		m.hexErr = ""
	}
}

func (m *colorPickModel) focusHex() tea.Cmd {
	m.hexFocused = true
	m.hexErr = ""
	m.hexInput.SetValue(m.steps[m.index])
	return m.hexInput.Focus()
}

func (m *colorPickModel) blurHex() {
	m.hexFocused = false
	m.hexInput.Blur()
	m.hexErr = ""
	m.hexInput.SetValue(m.steps[m.index])
}

func (m *colorPickModel) applyHexTyped() tea.Cmd {
	raw := strings.TrimSpace(m.hexInput.Value())
	norm, err := client.NormalizeHexColor(raw)
	if err != nil {
		m.hexErr = "invalid hex (#RGB or #RRGGBB)"
		return nil
	}
	m.hexErr = ""
	m.steps[m.index] = norm
	m.hexInput.SetValue(norm)
	m.blurHex()
	m.picker = newStepPicker(norm, m.zm)
	updated, cmd := m.picker.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	m.picker = updated.(bubblepicker.Model)
	return cmd
}

func (m *colorPickModel) tryLiveHexPreview() {
	raw := strings.TrimSpace(m.hexInput.Value())
	if raw == "" {
		return
	}
	norm, err := client.NormalizeHexColor(raw)
	if err != nil {
		return
	}
	m.steps[m.index] = norm
}

// hexFieldFromPaste: if paste includes '#', replace the whole hex field
// (avoids "##rrggbb" when the field already starts with '#').
func hexFieldFromPaste(raw string) (value string, replace bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.Contains(raw, "#") {
		return "", false
	}
	if norm, err := client.NormalizeHexColor(raw); err == nil {
		return norm, true
	}
	if len(raw) > 7 {
		raw = raw[:7]
	}
	return raw, true
}

func (m *colorPickModel) applyHexPaste(raw string) bool {
	value, ok := hexFieldFromPaste(raw)
	if !ok {
		return false
	}
	m.hexInput.SetValue(value)
	m.hexErr = ""
	m.tryLiveHexPreview()
	return true
}

func readHexClipboard() tea.Msg {
	s, err := clipboard.ReadAll()
	if err != nil {
		return nil
	}
	return hexClipboardMsg(s)
}

func (m *colorPickModel) setStep(i int) tea.Cmd {
	if len(m.steps) == 0 {
		return nil
	}
	if m.hexFocused {
		m.blurHex()
	} else {
		m.applyLiveToStep()
	}
	if i < 0 {
		i = len(m.steps) - 1
	}
	if i >= len(m.steps) {
		i = 0
	}
	m.index = i
	m.hexInput.SetValue(m.steps[m.index])
	m.picker = newStepPicker(m.steps[m.index], m.zm)
	updated, cmd := m.picker.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	m.picker = updated.(bubblepicker.Model)
	return cmd
}

func (m *colorPickModel) resetToDefault() tea.Cmd {
	if m.hexFocused {
		m.blurHex()
	}
	m.steps = append([]string(nil), ui.DefaultProgressColors()...)
	m.index = 0
	m.hexInput.SetValue(m.steps[0])
	m.picker = newStepPicker(m.steps[0], m.zm)
	updated, cmd := m.picker.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	m.picker = updated.(bubblepicker.Model)
	return cmd
}

func (m *colorPickModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case hexClipboardMsg:
		if m.hexFocused {
			if m.applyHexPaste(string(msg)) {
				return m, nil
			}
			// No '#': insert like a normal bracketed paste.
			m.hexInput, _ = m.hexInput.Update(tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune(msg),
				Paste: true,
			})
			m.tryLiveHexPreview()
			return m, nil
		}
	case bubblepicker.ColorChangedMsg:
		if m.hexFocused {
			return m, nil
		}
		hex := strings.TrimSpace(msg.Color)
		if hex == "" {
			return m, nil
		}
		if norm, err := client.NormalizeHexColor(hex); err == nil {
			hex = norm
		}
		m.steps[m.index] = hex
		m.hexInput.SetValue(hex)
		return m, m.setStep(m.index + 1)
	case bubblepicker.ColorCanceledMsg:
		if m.hexFocused {
			m.blurHex()
			return m, nil
		}
		m.applyLiveToStep()
		return m, tea.Quit
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if m.zm.Get(zoneStepPrev).InBounds(msg) {
				return m, m.setStep(m.index - 1)
			}
			if m.zm.Get(zoneStepNext).InBounds(msg) {
				return m, m.setStep(m.index + 1)
			}
			if m.zm.Get(zoneStepReset).InBounds(msg) {
				return m, m.resetToDefault()
			}
			if m.zm.Get(zoneStepLongHue).InBounds(msg) {
				m.longHue = !m.longHue
				return m, nil
			}
			if m.zm.Get(zoneHexInput).InBounds(msg) {
				return m, m.focusHex()
			}
			if m.hexFocused {
				m.blurHex()
			}
		}
	case tea.KeyMsg:
		if m.hexFocused {
			switch msg.String() {
			case "ctrl+c":
				m.canceled = true
				return m, tea.Quit
			case "esc":
				m.blurHex()
				return m, nil
			case "enter":
				return m, m.applyHexTyped()
			case "tab":
				m.blurHex()
				return m, nil
			case "ctrl+v":
				return m, readHexClipboard
			}
			if msg.Paste && m.applyHexPaste(string(msg.Runes)) {
				return m, nil
			}
			var cmd tea.Cmd
			m.hexInput, cmd = m.hexInput.Update(msg)
			m.tryLiveHexPreview()
			return m, cmd
		}
		switch msg.String() {
		case "ctrl+c":
			m.canceled = true
			return m, tea.Quit
		case "[":
			return m, m.setStep(m.index - 1)
		case "]":
			return m, m.setStep(m.index + 1)
		case "r":
			return m, m.resetToDefault()
		case "l":
			m.longHue = !m.longHue
			return m, nil
		case "h":
			return m, m.focusHex()
		}
	}
	if m.hexFocused {
		return m, nil
	}
	updated, cmd := m.picker.Update(msg)
	m.picker = updated.(bubblepicker.Model)
	m.applyLiveToStep()
	return m, cmd
}

func (m *colorPickModel) stepSwatches() string {
	var b strings.Builder
	for i, hex := range m.steps {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color(hex)).
			Background(lipgloss.Color(hex)).
			Render("  "))
		if i < len(m.steps)-1 {
			b.WriteString(" ")
		}
	}
	return b.String()
}

func (m *colorPickModel) View() string {
	preview := ui.FormatProgressPreviewOpts(m.label, m.steps, m.longHue)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	label := muted.Render(fmt.Sprintf("step %d / %d", m.index+1, len(m.steps)))

	prev := m.zm.Mark(zoneStepPrev, lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Padding(0, 1).
		Render("◀ prev"))
	next := m.zm.Mark(zoneStepNext, lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Padding(0, 1).
		Render("next ▶"))
	reset := m.zm.Mark(zoneStepReset, lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Padding(0, 1).
		Render("reset"))
	longHueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Padding(0, 1)
	if m.longHue {
		longHueStyle = longHueStyle.Foreground(lipgloss.Color("#7E00AF")).Bold(true)
	}
	longHueBtn := m.zm.Mark(zoneStepLongHue, longHueStyle.Render("long hue"))
	nav := lipgloss.JoinHorizontal(lipgloss.Center, prev, "  ", next, "  ", reset, "  ", longHueBtn)

	hexBoxStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))
	hexLabel := muted.Render("hex ")
	hexField := m.zm.Mark(zoneHexInput, hexBoxStyle.Render(m.hexInput.View()))
	hexRow := lipgloss.JoinHorizontal(lipgloss.Center, hexLabel, hexField)
	if m.hexErr != "" {
		hexRow = lipgloss.JoinVertical(lipgloss.Center, hexRow, lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render(m.hexErr))
	}

	help := muted.Render("h hex · [ ] · r reset · l long hue · Esc save · Ctrl+C cancel")
	if m.hexFocused {
		help = muted.Render("type #RGB/#RRGGBB · Enter apply · Esc/Tab back to picker")
	}

	body := lipgloss.JoinVertical(lipgloss.Center,
		preview,
		"",
		m.stepSwatches(),
		label,
		"",
		hexRow,
		"",
		nav,
		"",
		m.picker.View(),
		"",
		help,
	)
	w, h := m.width, m.height
	if w < 1 {
		w = 80
	}
	if h < 1 {
		h = 24
	}
	return m.zm.Scan(lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, body))
}

func seedGradientSteps(n int, current []string) []string {
	defaults := ui.DefaultProgressColors()
	out := make([]string, n)
	for i := 0; i < n; i++ {
		switch {
		case i < len(current) && current[i] != "":
			out[i] = current[i]
		case i < len(defaults):
			out[i] = defaults[i]
		default:
			out[i] = defaults[len(defaults)-1]
		}
	}
	return out
}
