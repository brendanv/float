package ui

import (
	"image/color"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Styles holds all lipgloss styles for the TUI, built from a specific
// dark-background determination. Create one with NewStyles(hasDark).
type Styles struct {
	Border        lipgloss.Style
	FocusedBorder lipgloss.Style
	TabActive     lipgloss.Style
	TabInactive   lipgloss.Style
	Help          lipgloss.Style
	RevenueBar    lipgloss.Style
	InsightsBar   lipgloss.Style

	// Raw colors for inline Foreground() calls.
	BorderFg  color.Color
	FocusedFg color.Color
}

// NewStyles creates a Styles set tuned for dark or light terminal backgrounds.
func NewStyles(hasDark bool) Styles {
	ld := lipgloss.LightDark(hasDark)

	borderFg := ld(lipgloss.Color("#A49FA5"), lipgloss.Color("#555555"))
	focusedFg := ld(lipgloss.Color("#6C91BF"), lipgloss.Color("#7DC4E4"))
	tabActiveFg := ld(lipgloss.Color("#1C1C1C"), lipgloss.Color("#EEEEEE"))
	helpFg := ld(lipgloss.Color("#9B9B9B"), lipgloss.Color("#626262"))

	return Styles{
		Border:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(borderFg),
		FocusedBorder: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(focusedFg),
		TabActive:     lipgloss.NewStyle().Bold(true).Foreground(tabActiveFg),
		TabInactive:   lipgloss.NewStyle().Foreground(helpFg),
		Help:          lipgloss.NewStyle().Foreground(helpFg),
		RevenueBar:    lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#3D7A4A"), lipgloss.Color("#A6E3A1"))),
		InsightsBar:   lipgloss.NewStyle().Foreground(focusedFg),
		BorderFg:      borderFg,
		FocusedFg:     focusedFg,
	}
}

// injectBorderTitle post-processes a rendered panel string (with a rounded
// border) to embed a title in the top border. The title color matches the
// border color (focused or unfocused).
func injectBorderTitle(rendered, title string, focused bool, st Styles) string {
	if title == "" {
		return rendered
	}
	lines := strings.Split(rendered, "\n")
	if len(lines) == 0 {
		return rendered
	}
	topLine := lines[0]
	totalWidth := ansi.StringWidth(topLine)
	if totalWidth < 4 {
		return rendered
	}

	// Build: ╭─ Title ──────────────╮
	titlePart := "─ " + title + " "
	titleWidth := ansi.StringWidth(titlePart)
	remaining := totalWidth - 2 - titleWidth // -2 for ╭ and ╮
	if remaining < 1 {
		return rendered // title doesn't fit, keep as-is
	}

	newTop := "╭" + titlePart + strings.Repeat("─", remaining) + "╮"

	var fg color.Color
	if focused {
		fg = st.FocusedFg
	} else {
		fg = st.BorderFg
	}
	lines[0] = lipgloss.NewStyle().Foreground(fg).Render(newTop)
	return strings.Join(lines, "\n")
}

// NewHelpModel returns a help.Model styled with the app's help color palette.
func NewHelpModel(st Styles) help.Model {
	h := help.New()
	s := h.Styles
	s.ShortKey = s.ShortKey.Foreground(st.Help.GetForeground())
	s.ShortDesc = s.ShortDesc.Foreground(st.Help.GetForeground())
	s.ShortSeparator = s.ShortSeparator.Foreground(st.Help.GetForeground())
	s.FullKey = s.FullKey.Foreground(st.Help.GetForeground())
	s.FullDesc = s.FullDesc.Foreground(st.Help.GetForeground())
	s.FullSeparator = s.FullSeparator.Foreground(st.Help.GetForeground())
	s.Ellipsis = s.Ellipsis.Foreground(st.Help.GetForeground())
	h.Styles = s
	return h
}
