package ui

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
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
