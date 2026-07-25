package ui

import (
	"fmt"
	"os"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/mewisme/discloud-go/internal/client"
)

// FileEntry is one candidate for the multi-select file list TUI.
type FileEntry struct {
	Path string // local path (upload) or bundle file id (download)
	Name string // logical path shown in the list
	Size int64
}

// UploadFile is an alias kept for upload call sites.
type UploadFile = FileEntry

type fileListItem struct {
	path, name string
	size       int64
	selected   bool
}

func (i fileListItem) Title() string {
	mark := " "
	if i.selected {
		mark = "✓"
	}
	return fmt.Sprintf("%s %s · %s", mark, i.name, client.FormatBytes(i.size))
}

func (i fileListItem) Description() string { return "" }

func (i fileListItem) FilterValue() string { return i.name }

type fileListKeyMap struct {
	toggle    key.Binding
	selectAll key.Binding
	confirm   key.Binding
	quit      key.Binding
}

func newFileListKeyMap(confirmHelp string) fileListKeyMap {
	return fileListKeyMap{
		// Bubble Tea v2 reports space as "space", not " ".
		toggle: key.NewBinding(
			key.WithKeys("space", " "),
			key.WithHelp("space", "toggle"),
		),
		selectAll: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "select all visible"),
		),
		confirm: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", confirmHelp),
		),
		quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "cancel"),
		),
	}
}

type fileListModel struct {
	list        list.Model
	items       []fileListItem
	keys        fileListKeyMap
	titlePrefix string
	width       int
	quitting    bool
	canceled    bool
	ready       bool
}

func newFileListModel(files []FileEntry, titlePrefix string) fileListModel {
	items := make([]fileListItem, len(files))
	listItems := make([]list.Item, len(files))
	for i, f := range files {
		it := fileListItem{path: f.Path, name: f.Name, size: f.Size}
		items[i] = it
		listItems[i] = it
	}

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetHeight(1)
	delegate.SetSpacing(0)

	confirmHelp := "confirm"
	switch titlePrefix {
	case "Upload files":
		confirmHelp = "upload selected"
	case "Download files":
		confirmHelp = "download selected"
	}

	l := list.New(listItems, delegate, 0, 0)
	l.Title = fileListTitle(titlePrefix, items)
	l.SetFilteringEnabled(true)
	l.InfiniteScrolling = true
	l.DisableQuitKeybindings()
	l.SetShowStatusBar(true)

	keys := newFileListKeyMap(confirmHelp)
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{keys.toggle, keys.selectAll, keys.confirm, keys.quit}
	}

	return fileListModel{list: l, items: items, keys: keys, titlePrefix: titlePrefix}
}

func fileListTitle(prefix string, items []fileListItem) string {
	n, sel := len(items), 0
	for _, it := range items {
		if it.selected {
			sel++
		}
	}
	if prefix == "" {
		prefix = "Files"
	}
	return fmt.Sprintf("%s (%d selected / %d)", prefix, sel, n)
}

func (m fileListModel) Init() tea.Cmd { return nil }

func (m fileListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		h, v := 2, 2
		bannerH := BannerHeight(msg.Width)
		listH := msg.Height - v - bannerH
		if listH < 4 {
			listH = 4
		}
		m.list.SetSize(msg.Width-h, listH)
		m.ready = true
		return m, nil

	case tea.KeyPressMsg:
		if m.list.FilterState() == list.Filtering {
			break
		}
		switch {
		case key.Matches(msg, m.keys.quit):
			m.canceled = true
			m.quitting = true
			return m, tea.Quit
		case key.Matches(msg, m.keys.toggle) || isSpaceKey(msg):
			var cmd tea.Cmd
			m, cmd = m.toggleCursor()
			return m, cmd
		case key.Matches(msg, m.keys.selectAll):
			var cmd tea.Cmd
			m, cmd = m.toggleSelectAllVisible()
			return m, cmd
		case key.Matches(msg, m.keys.confirm):
			if m.selectedCount() == 0 {
				cmd := m.list.NewStatusMessage("Choose at least 1 file")
				return m, cmd
			}
			m.quitting = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func isSpaceKey(msg tea.KeyPressMsg) bool {
	s := msg.String()
	return s == " " || s == "space"
}

func (m fileListModel) selectedCount() int {
	n := 0
	for _, it := range m.items {
		if it.selected {
			n++
		}
	}
	return n
}

func (m fileListModel) toggleCursor() (fileListModel, tea.Cmd) {
	item := m.list.SelectedItem()
	if item == nil {
		return m, nil
	}
	cur, ok := item.(fileListItem)
	if !ok {
		return m, nil
	}
	gi := m.list.GlobalIndex()
	if gi < 0 || gi >= len(m.items) {
		for i := range m.items {
			if m.items[i].path == cur.path {
				gi = i
				break
			}
		}
	}
	if gi < 0 || gi >= len(m.items) {
		return m, nil
	}
	m.items[gi].selected = !m.items[gi].selected
	cmd := m.list.SetItem(gi, m.items[gi])
	m.list.Title = fileListTitle(m.titlePrefix, m.items)
	return m, cmd
}

func (m fileListModel) toggleSelectAllVisible() (fileListModel, tea.Cmd) {
	vis := m.list.VisibleItems()
	if len(vis) == 0 {
		return m, nil
	}
	allOn := true
	paths := make(map[string]struct{}, len(vis))
	for _, v := range vis {
		it, ok := v.(fileListItem)
		if !ok {
			continue
		}
		paths[it.path] = struct{}{}
		if !it.selected {
			allOn = false
		}
	}
	want := !allOn
	for i := range m.items {
		if _, ok := paths[m.items[i].path]; ok {
			m.items[i].selected = want
		}
	}
	return m.syncList()
}

func (m fileListModel) syncList() (fileListModel, tea.Cmd) {
	listItems := make([]list.Item, len(m.items))
	for i, it := range m.items {
		listItems[i] = it
	}
	cmd := m.list.SetItems(listItems)
	m.list.Title = fileListTitle(m.titlePrefix, m.items)
	return m, cmd
}

func (m fileListModel) View() tea.View {
	w := m.width
	if w < 1 {
		w = termWidth(os.Stdout)
	}
	banner := FormatBanner(w)
	if m.quitting {
		// Leave banner as the final frame so the screen is not blank on exit.
		return tea.NewView(banner)
	}
	if !m.ready {
		return tea.NewView(banner + PadUnderBanner("Loading…", w, 0) + "\n")
	}
	band := ContentBandWidth(w)
	body := PadUnderBanner(m.list.View(), w, band)
	return tea.NewView(banner + body)
}

func (m fileListModel) selected() []FileEntry {
	var out []FileEntry
	for _, it := range m.items {
		if it.selected {
			out = append(out, FileEntry{Path: it.path, Name: it.name, Size: it.size})
		}
	}
	return out
}

// SelectFiles runs an interactive multi-select list. Requires a TTY.
// Space toggles, a selects/deselects all visible, Enter confirms, q cancels.
func SelectFiles(files []FileEntry, titlePrefix string) ([]FileEntry, error) {
	if JSON || !IsTTY(os.Stdin) || !IsTTY(os.Stdout) {
		return nil, fmt.Errorf("file selection requires an interactive TTY")
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no files to select")
	}
	if titlePrefix == "" {
		titlePrefix = "Files"
	}
	p := tea.NewProgram(newFileListModel(files, titlePrefix), tea.WithInput(os.Stdin))
	final, err := p.Run()
	// Bubble Tea clears the inline view on exit; restore the CLI banner.
	ClearAndPrintBanner()
	if err != nil {
		return nil, err
	}
	m, ok := final.(fileListModel)
	if !ok {
		return nil, fmt.Errorf("unexpected file list model")
	}
	if m.canceled {
		return nil, fmt.Errorf("aborted")
	}
	sel := m.selected()
	if len(sel) == 0 {
		return nil, fmt.Errorf("no files selected")
	}
	return sel, nil
}

// SelectUploadFiles is SelectFiles with an upload title.
func SelectUploadFiles(files []FileEntry) ([]FileEntry, error) {
	return SelectFiles(files, "Upload files")
}

// SelectDownloadFiles is SelectFiles with a download title.
func SelectDownloadFiles(files []FileEntry) ([]FileEntry, error) {
	return SelectFiles(files, "Download files")
}

// ToggleSelectAllVisible is a pure helper for tests: if every path in visible
// is selected in items, deselect them; otherwise select them. Returns updated items.
func ToggleSelectAllVisible(items []FileEntry, selected map[string]bool, visible []string) map[string]bool {
	out := make(map[string]bool, len(selected))
	for k, v := range selected {
		out[k] = v
	}
	if len(visible) == 0 {
		return out
	}
	allOn := true
	for _, p := range visible {
		if !out[p] {
			allOn = false
			break
		}
	}
	want := !allOn
	for _, p := range visible {
		out[p] = want
	}
	return out
}
