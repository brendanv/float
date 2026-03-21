package ui

import "github.com/charmbracelet/lipgloss"

// RenderTabBar returns a 1-line string showing tabs.
// activeTab: 0 = Home, 1 = Manager.
func RenderTabBar(activeTab int, width int) string {
	tabs := []struct {
		label  string
		active bool
	}{
		{"Home", activeTab == 0},
		{"Manager", activeTab == 1},
	}

	var rendered string
	for i, tab := range tabs {
		if i > 0 {
			rendered += "  "
		}
		if tab.active {
			rendered += TabActiveStyle.Render("[ " + tab.label + " ]")
		} else {
			rendered += TabInactiveStyle.Render(tab.label)
		}
	}

	return lipgloss.NewStyle().Width(width).Render(rendered)
}
