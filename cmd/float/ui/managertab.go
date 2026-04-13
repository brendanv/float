package ui

import (
	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

// ManagerTab shows the account hierarchy and balance summary.
type ManagerTab struct {
	width  int
	height int
	styles Styles
	client floatv1connect.LedgerServiceClient

	// Left column (35%).
	leftWidth    int
	leftInnerW   int
	leftInnerH   int
	summaryH     int
	placeholderH int
	summary      SummaryPanel

	// Right column (65%).
	rightWidth  int
	rightInnerW int
	rightInnerH int
	tree        AccountTree
}

func NewManagerTab(client floatv1connect.LedgerServiceClient, st Styles) ManagerTab {
	return ManagerTab{
		styles:  st,
		client:  client,
		summary: NewSummaryPanel(st),
		tree:    NewAccountTree(),
	}
}

func (m ManagerTab) setStyles(st Styles) ManagerTab {
	m.styles = st
	m.summary.setStyles(st)
	return m
}

func (m ManagerTab) SetSize(w, h int) ManagerTab {
	m.width = w
	m.height = h

	// Left = 35%, floor 20; Right = remainder.
	m.leftWidth = w * 35 / 100
	if m.leftWidth < 20 {
		m.leftWidth = 20
	}
	m.rightWidth = w - m.leftWidth
	if m.rightWidth < 0 {
		m.rightWidth = 0
	}

	m.leftInnerW, m.leftInnerH = innerSize(m.leftWidth, h, m.styles.Border)

	// Left sub-layout: summary top 40%, placeholder fills remainder.
	m.summaryH = m.leftInnerH * 40 / 100
	if m.summaryH < 3 {
		m.summaryH = 3
	}
	m.placeholderH = m.leftInnerH - m.summaryH
	if m.placeholderH < 0 {
		m.placeholderH = 0
	}
	m.summary.SetSize(m.leftInnerW, m.summaryH)

	// Right inner — tree fills full height.
	m.rightInnerW, m.rightInnerH = innerSize(m.rightWidth, h, m.styles.FocusedBorder)
	m.tree.width = m.rightInnerW
	m.tree.height = m.rightInnerH
	m.tree.clampOffset()

	return m
}

func (m ManagerTab) Init() tea.Cmd {
	return tea.Batch(
		m.summary.spinner.Tick(),
		m.tree.spinner.Tick(),
		FetchManagerAccounts(m.client),
		FetchManagerBalances(m.client),
		FetchManagerSummary(m.client),
	)
}

func (m ManagerTab) Update(msg tea.Msg) (ManagerTab, tea.Cmd) {
	switch msg := msg.(type) {
	case ManagerAccountsMsg:
		if msg.Err != nil {
			m.tree.SetError(msg.Err.Error())
		} else {
			m.tree.SetAccounts(msg.Accounts)
		}
		return m, nil

	case ManagerBalancesMsg:
		if msg.Err != nil {
			m.tree.SetError(msg.Err.Error())
		} else {
			m.tree.SetBalances(msg.Report)
		}
		return m, nil

	case ManagerSummaryMsg:
		if msg.Err != nil {
			m.summary.SetError(msg.Err.Error())
		} else {
			m.summary.SetData(msg.Report)
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			m.tree.state = stateLoading
			m.summary.state = stateLoading
			return m, tea.Batch(
				FetchManagerAccounts(m.client),
				FetchManagerBalances(m.client),
				FetchManagerSummary(m.client),
			)
		default:
			m.tree.Update(msg)
		}
		return m, nil

	default:
		cmd1 := m.summary.Update(msg)
		cmd2 := m.tree.Update(msg)
		return m, tea.Batch(cmd1, cmd2)
	}
}

func (m ManagerTab) KeyMap() help.KeyMap {
	return ManagerKeyMap{}
}

func (m ManagerTab) View() string {
	// Left column: summary + placeholder.
	summaryContent := lipgloss.NewStyle().
		Width(m.leftInnerW).
		Height(m.summaryH).
		Render(m.summary.View())

	var leftContent string
	if m.placeholderH > 0 {
		placeholder := m.styles.Help.
			Width(m.leftInnerW).
			Height(m.placeholderH).
			Align(lipgloss.Center, lipgloss.Center).
			Render("chart placeholder")
		leftContent = lipgloss.JoinVertical(lipgloss.Left, summaryContent, placeholder)
	} else {
		leftContent = summaryContent
	}
	leftContent = lipgloss.NewStyle().
		Width(m.leftInnerW).
		Height(m.leftInnerH).
		Render(leftContent)

	leftPanel := renderCard(leftContent, "Summary", false, m.leftWidth, m.height, m.styles)

	// Right column: account tree.
	treeContent := lipgloss.NewStyle().
		Width(m.rightInnerW).
		Height(m.rightInnerH).
		Render(m.tree.View())

	rightPanel := renderCard(treeContent, "Accounts", true, m.rightWidth, m.height, m.styles)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
}
