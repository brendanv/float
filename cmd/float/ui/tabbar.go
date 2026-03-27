package ui

import "charm.land/lipgloss/v2"

// RenderTabBar returns a 1-line string showing tabs.
// activeTab: 0 = Home, 1 = Manager.
func RenderTabBar(activeTab int, width int) string {
	tabs := []struct {
		label  string
		active bool
	}{
		{"Home", activeTab == TabHome},
		{"Manager", activeTab == TabManager},
		{"Trends", activeTab == TabTrends},
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
