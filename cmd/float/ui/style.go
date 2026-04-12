package ui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
	"github.com/charmbracelet/x/ansi"
)

var (
	colorBorder    = compat.AdaptiveColor{Light: lipgloss.Color("#A49FA5"), Dark: lipgloss.Color("#555555")}
	colorFocused   = compat.AdaptiveColor{Light: lipgloss.Color("#6C91BF"), Dark: lipgloss.Color("#7DC4E4")}
	colorTabActive = compat.AdaptiveColor{Light: lipgloss.Color("#1C1C1C"), Dark: lipgloss.Color("#EEEEEE")}
	colorHelp      = compat.AdaptiveColor{Light: lipgloss.Color("#9B9B9B"), Dark: lipgloss.Color("#626262")}

	BorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder)

	FocusedBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorFocused)

	TabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorTabActive)

	TabInactiveStyle = lipgloss.NewStyle().
				Foreground(colorHelp)

	HelpStyle = lipgloss.NewStyle().
			Foreground(colorHelp)
)

// injectBorderTitle post-processes a rendered panel string (with a rounded
// border) to embed a title in the top border. The title color matches the
// border color (focused or unfocused).
func injectBorderTitle(rendered, title string, focused bool) string {
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

	var fg compat.AdaptiveColor
	if focused {
		fg = colorFocused
	} else {
		fg = colorBorder
	}
	lines[0] = lipgloss.NewStyle().Foreground(fg).Render(newTop)
	return strings.Join(lines, "\n")
}

// NewHelpModel returns a help.Model styled with the app's help color palette.
func NewHelpModel() help.Model {
	h := help.New()
	s := h.Styles
	s.ShortKey = s.ShortKey.Foreground(colorHelp)
	s.ShortDesc = s.ShortDesc.Foreground(colorHelp)
	s.ShortSeparator = s.ShortSeparator.Foreground(colorHelp)
	s.FullKey = s.FullKey.Foreground(colorHelp)
	s.FullDesc = s.FullDesc.Foreground(colorHelp)
	s.FullSeparator = s.FullSeparator.Foreground(colorHelp)
	s.Ellipsis = s.Ellipsis.Foreground(colorHelp)
	h.Styles = s
	return h
}
