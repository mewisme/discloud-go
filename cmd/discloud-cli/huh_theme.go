package main

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/mewisme/discloud-go/cmd/discloud-cli/ui"
	"github.com/mewisme/discloud-go/internal/client"
)

type promptThemeOpt struct {
	id, label string
	fn        func(bool) *huh.Styles
}

var promptThemeOpts = []promptThemeOpt{
	{id: "base16", label: "Base 16", fn: huh.ThemeBase16},
	{id: "charm", label: "Charm", fn: huh.ThemeCharm},
	{id: "dracula", label: "Dracula", fn: huh.ThemeDracula},
	{id: "catppuccin", label: "Catppuccin", fn: huh.ThemeCatppuccin},
	{id: "default", label: "Default", fn: huh.ThemeBase},
}

func promptThemeLabel(id string) string {
	id = client.CustomizeConfig{PromptTheme: id}.PromptThemeOrDefault()
	for _, o := range promptThemeOpts {
		if o.id == id {
			return o.label
		}
	}
	return id
}

func huhThemeByID(id string) huh.Theme {
	id = client.CustomizeConfig{PromptTheme: id}.PromptThemeOrDefault()
	for _, o := range promptThemeOpts {
		if o.id == id {
			return huh.ThemeFunc(o.fn)
		}
	}
	return huh.ThemeFunc(huh.ThemeBase16)
}

func currentHuhTheme() huh.Theme {
	c, _ := client.LoadCustomize()
	return huhThemeByID(c.PromptThemeOrDefault())
}

// runHuh runs a single huh field with the configured prompt theme (default Base 16),
// sized and left-padded on the same banner axis as config tables.
func runHuh(field huh.Field) error {
	tw := ui.LayoutTermWidth()
	if tw < 1 {
		tw = 80
	}
	band := ui.ContentBandWidth(tw)
	left := ui.ContentLeftPad(tw, band)
	return huh.NewForm(huh.NewGroup(field)).
		WithTheme(bannerAlignedTheme(currentHuhTheme(), left)).
		WithWidth(band).
		WithShowHelp(false).
		Run()
}

func bannerAlignedTheme(base huh.Theme, left int) huh.Theme {
	if left <= 0 {
		return base
	}
	return huh.ThemeFunc(func(isDark bool) *huh.Styles {
		s := base.Theme(isDark)
		s.Form.Base = s.Form.Base.MarginLeft(left)
		return s
	})
}

// promptPromptTheme picks a huh theme with live restyle as the cursor moves.
func promptPromptTheme(current string) (string, error) {
	cur := client.CustomizeConfig{PromptTheme: current}.PromptThemeOrDefault()
	idx := 0
	for i, o := range promptThemeOpts {
		if o.id == cur {
			idx = i
			break
		}
	}
	isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	m := &themePickModel{cursor: idx, isDark: isDark}
	final, err := tea.NewProgram(m).Run()
	if err != nil {
		return "", err
	}
	out, ok := final.(*themePickModel)
	if !ok || out == nil {
		return "", fmt.Errorf("prompt theme picker ended unexpectedly")
	}
	if out.canceled {
		return "", huh.ErrUserAborted
	}
	return promptThemeOpts[out.cursor].id, nil
}

type themePickModel struct {
	cursor   int
	isDark   bool
	canceled bool
}

func (m *themePickModel) Init() tea.Cmd { return nil }

func (m *themePickModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.canceled = true
			return m, tea.Quit
		case "enter":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(promptThemeOpts)-1 {
				m.cursor++
			}
		}
	}
	return m, nil
}

func (m *themePickModel) View() tea.View {
	st := promptThemeOpts[m.cursor].fn(m.isDark)
	var b strings.Builder
	b.WriteString(st.Focused.Title.Render("Prompt theme"))
	b.WriteString("\n")
	b.WriteString(st.Focused.Description.Render("Live preview — ↑/↓ move, enter save, esc cancel"))
	b.WriteString("\n\n")
	for i, o := range promptThemeOpts {
		if i == m.cursor {
			b.WriteString(st.Focused.SelectSelector.Render("> "))
			b.WriteString(st.Focused.SelectedOption.Render(o.label))
		} else {
			b.WriteString("  ")
			b.WriteString(st.Focused.UnselectedOption.Render(o.label))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	preview := strings.Join([]string{
		st.Focused.NoteTitle.Render("Sample prompt"),
		st.Focused.Description.Render("How the form looks with this theme"),
		st.Focused.TextInput.Prompt.Render("? ") + st.Focused.TextInput.Text.Render("example value"),
		st.Focused.FocusedButton.Render(" Yes ") + " " + st.Focused.BlurredButton.Render(" No "),
	}, "\n")
	b.WriteString(st.Focused.Card.Render(preview))
	b.WriteString("\n")
	tw := ui.TermWidth(os.Stdout)
	band := ui.ContentBandWidth(tw)
	return tea.NewView(ui.PadUnderBanner(b.String(), tw, band))
}
