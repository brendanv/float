package ui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ThemeSelectedMsg is emitted by SettingsTab when the user applies a theme.
type ThemeSelectedMsg struct {
	Theme Theme
}

// SettingsTab is the TUI settings page. Currently it only exposes theme selection.
type SettingsTab struct {
	width  int
	height int
	styles Styles
	// cursor is the index into ThemeNames() for the highlighted row.
	cursor int
	// applied is the currently applied (saved) theme.
	applied Theme
}

func NewSettingsTab(st Styles, theme Theme) SettingsTab {
	return SettingsTab{
		styles:  st,
		cursor:  int(theme),
		applied: theme,
	}
}

func (m SettingsTab) setStyles(st Styles) SettingsTab {
	m.styles = st
	return m
}

// setApplied updates which theme is shown as currently applied (called by the
// root model after a theme change is confirmed).
func (m SettingsTab) setApplied(theme Theme) SettingsTab {
	m.applied = theme
	m.cursor = int(theme)
	return m
}

func (m SettingsTab) SetSize(w, h int) SettingsTab {
	m.width = w
	m.height = h
	return m
}

func (m SettingsTab) Init() tea.Cmd { return nil }

func (m SettingsTab) Update(msg tea.Msg) (SettingsTab, tea.Cmd) {
	names := ThemeNames()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if m.cursor < len(names)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "enter", " ":
			selected := Theme(m.cursor)
			m.applied = selected
			return m, func() tea.Msg { return ThemeSelectedMsg{Theme: selected} }
		}
	}
	return m, nil
}

func (m SettingsTab) KeyMap() help.KeyMap { return SettingsKeyMap{} }

func (m SettingsTab) View() string {
	names := ThemeNames()
	st := m.styles

	// Build the theme list rows.
	var rows []string
	for i, name := range names {
		theme := Theme(i)
		prefix := "  "
		label := name
		var row string
		switch {
		case i == m.cursor && theme == m.applied:
			// Highlighted and applied.
			row = st.Active.Bold(true).Render(prefix+"▶ "+label) +
				st.Help.Render("  (applied)")
		case i == m.cursor:
			// Highlighted but not yet applied.
			row = st.Active.Render(prefix + "▶ " + label)
		case theme == m.applied:
			// Applied but not highlighted.
			row = st.Base.Render(prefix+"  "+label) +
				st.Help.Render("  (applied)")
		default:
			row = st.Help.Render(prefix + "  " + label)
		}
		rows = append(rows, row)
	}

	// Build section: heading + rows.
	heading := st.Active.Bold(true).Render("Theme")
	hint := st.Help.Render("j/k to move  enter to apply")

	body := heading + "\n\n" +
		strings.Join(rows, "\n") +
		"\n\n" + hint

	// Center the body in the available area.
	inner := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Left, lipgloss.Center).
		PaddingLeft(4).
		Render(body)

	return inner
}
