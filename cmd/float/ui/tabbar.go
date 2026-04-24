package ui

import "charm.land/lipgloss/v2"

// RenderTabBar returns a 1-line string showing tabs.
func RenderTabBar(activeTab int, width int, st Styles) string {
	tabs := []struct {
		label  string
		active bool
	}{
		{"Home", activeTab == TabHome},
		{"Accounts", activeTab == TabAccounts},
		{"Trends", activeTab == TabTrends},
		{"Manage", activeTab == TabManage},
		{"Settings", activeTab == TabSettings},
	}

	var rendered string
	for i, tab := range tabs {
		if i > 0 {
			rendered += "  "
		}
		if tab.active {
			rendered += st.TabActive.Render("[ " + tab.label + " ]")
		} else {
			rendered += st.TabInactive.Render(tab.label)
		}
	}

	return lipgloss.NewStyle().Width(width).Render(rendered)
}
