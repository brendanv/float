package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

// ManagerTab shows the net worth over time chart.
type ManagerTab struct {
	width    int
	height   int
	client   floatv1connect.LedgerServiceClient
	netWorth NetWorthPanel
}

func NewManagerTab(client floatv1connect.LedgerServiceClient) ManagerTab {
	return ManagerTab{
		client:   client,
		netWorth: NewNetWorthPanel(),
	}
}

func (m ManagerTab) SetSize(w, h int) ManagerTab {
	m.width = w
	m.height = h
	m.netWorth.SetSize(w, h)
	return m
}

func (m ManagerTab) Init() tea.Cmd {
	return tea.Batch(
		m.netWorth.spinner.Tick(),
		FetchNetWorth(m.client),
	)
}

func (m ManagerTab) Update(msg tea.Msg) (ManagerTab, tea.Cmd) {
	switch msg := msg.(type) {
	case NetWorthMsg:
		if msg.Err != nil {
			m.netWorth.SetError(msg.Err.Error())
		} else {
			m.netWorth.SetData(msg.Snapshots)
		}
		return m, nil
	default:
		cmd := m.netWorth.Update(msg)
		return m, cmd
	}
}

func (m ManagerTab) HelpContext() HelpContext {
	return HelpContext{ActiveTab: TabManager}
}

func (m ManagerTab) View() string {
	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Render(m.netWorth.View())
}
