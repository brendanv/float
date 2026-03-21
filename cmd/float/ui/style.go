package ui

import "github.com/charmbracelet/lipgloss"

var (
	colorSubtle   = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
	colorBorder   = lipgloss.AdaptiveColor{Light: "#A49FA5", Dark: "#555555"}
	colorFocused  = lipgloss.AdaptiveColor{Light: "#6C91BF", Dark: "#7DC4E4"}
	colorTabActive = lipgloss.AdaptiveColor{Light: "#1C1C1C", Dark: "#EEEEEE"}
	colorHelp     = lipgloss.AdaptiveColor{Light: "#9B9B9B", Dark: "#626262"}

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
