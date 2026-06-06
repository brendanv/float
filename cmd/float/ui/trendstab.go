package ui

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

type netWorthRange int

const (
	rangeOneYear  netWorthRange = 0
	rangeTwoYear  netWorthRange = 1
	rangeFiveYear netWorthRange = 2
	rangeAll      netWorthRange = 3
	numRanges                   = 4
)

var rangeLabels = [numRanges]string{"1Y", "2Y", "5Y", "All"}

func (r netWorthRange) begin() string {
	now := time.Now()
	switch r {
	case rangeOneYear:
		return now.AddDate(-1, 0, 0).Format("2006-01-02")
	case rangeTwoYear:
		return now.AddDate(-2, 0, 0).Format("2006-01-02")
	case rangeFiveYear:
		return now.AddDate(-5, 0, 0).Format("2006-01-02")
	default:
		return ""
	}
}

// TrendsTab shows the net worth over time chart with a time range selector.
type TrendsTab struct {
	width    int
	height   int
	client   floatv1connect.LedgerServiceClient
	styles   Styles
	netWorth NetWorthPanel
	rng      netWorthRange
}

func NewTrendsTab(client floatv1connect.LedgerServiceClient, st Styles) TrendsTab {
	return TrendsTab{
		client:   client,
		styles:   st,
		netWorth: NewNetWorthPanel(st),
		rng:      rangeAll,
	}
}

func (m TrendsTab) setStyles(st Styles) TrendsTab {
	m.styles = st
	m.netWorth.setStyles(st)
	return m
}

func (m TrendsTab) SetSize(w, h int) TrendsTab {
	m.width = w
	m.height = h
	// Reserve 1 row for the range selector bar.
	m.netWorth.SetSize(w, h-1)
	return m
}

func (m TrendsTab) Init() tea.Cmd {
	return tea.Batch(
		m.netWorth.spinner.Tick(),
		FetchNetWorth(m.client, m.rng.begin()),
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
	case tea.KeyMsg:
		switch msg.String() {
		case "[":
			if m.rng > 0 {
				m.rng--
				m.netWorth.Reset()
				return m, FetchNetWorth(m.client, m.rng.begin())
			}
		case "]":
			if m.rng < numRanges-1 {
				m.rng++
				m.netWorth.Reset()
				return m, FetchNetWorth(m.client, m.rng.begin())
			}
		}
		cmd := m.netWorth.Update(msg)
		return m, cmd
	default:
		cmd := m.netWorth.Update(msg)
		return m, cmd
	}
}

func (m TrendsTab) KeyMap() help.KeyMap {
	return TrendsKeyMap{}
}

func (m TrendsTab) renderRangeBar() string {
	st := m.styles
	var parts []string
	for i := 0; i < numRanges; i++ {
		label := rangeLabels[i]
		if netWorthRange(i) == m.rng {
			parts = append(parts, st.Active.Bold(true).Render(" "+label+" "))
		} else {
			parts = append(parts, st.Help.Render(" "+label+" "))
		}
	}
	bar := strings.Join(parts, st.Help.Render("·"))
	hint := st.Help.Render("  [/] change range")
	return lipgloss.NewStyle().Width(m.width).Render(bar + hint)
}

func (m TrendsTab) View() string {
	rangeBar := m.renderRangeBar()
	chart := m.netWorth.View()
	return lipgloss.JoinVertical(lipgloss.Left, rangeBar, chart)
}
