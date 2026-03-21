package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ManagerTab is the manager tab placeholder.
type ManagerTab struct {
	width  int
	height int
}

func NewManagerTab() ManagerTab {
	return ManagerTab{}
}

func (m ManagerTab) SetSize(w, h int) ManagerTab {
	m.width = w
	m.height = h
	return m
}

func (m ManagerTab) Init() tea.Cmd {
	return nil
}

func (m ManagerTab) Update(msg tea.Msg) (ManagerTab, tea.Cmd) {
	return m, nil
}

func (m ManagerTab) HelpContext() HelpContext {
	return HelpContext{ActiveTab: TabManager}
}

func (m ManagerTab) View() string {
	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center).
		Render("coming soon")
}
