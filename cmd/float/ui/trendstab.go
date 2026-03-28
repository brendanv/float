package ui

import (
	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"

	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

// TrendsTab shows the net worth over time chart.
type TrendsTab struct {
	width    int
	height   int
	client   floatv1connect.LedgerServiceClient
	netWorth NetWorthPanel
}

func NewTrendsTab(client floatv1connect.LedgerServiceClient) TrendsTab {
	return TrendsTab{
		client:   client,
		netWorth: NewNetWorthPanel(),
	}
}

func (m TrendsTab) SetSize(w, h int) TrendsTab {
	m.width = w
	m.height = h
	m.netWorth.SetSize(w, h)
	return m
}

func (m TrendsTab) Init() tea.Cmd {
	return tea.Batch(
		m.netWorth.spinner.Tick(),
		FetchNetWorth(m.client),
	)
}

func (m TrendsTab) Update(msg tea.Msg) (TrendsTab, tea.Cmd) {
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

func (m TrendsTab) KeyMap() help.KeyMap {
	return TrendsKeyMap{}
}

func (m TrendsTab) View() string {
	return m.netWorth.View()
}
