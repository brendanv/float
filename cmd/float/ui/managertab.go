package ui

import (
	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

type managerMode int

const (
	managerModeTree     managerMode = iota
	managerModeRegister             // showing account register for a selected account
)

// ManagerTab shows the account hierarchy and balance summary.
// Pressing enter on an account opens its register (transaction history with
// running balance). Pressing esc returns to the tree view.
type ManagerTab struct {
	width  int
	height int
	styles Styles
	client floatv1connect.LedgerServiceClient
	mode   managerMode

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
	register    AccountRegisterPanel
}

func NewManagerTab(client floatv1connect.LedgerServiceClient, st Styles) ManagerTab {
	return ManagerTab{
		styles:   st,
		client:   client,
		summary:  NewSummaryPanel(st),
		tree:     NewAccountTree(),
		register: NewAccountRegisterPanel(st),
	}
}

func (m ManagerTab) setStyles(st Styles) ManagerTab {
	m.styles = st
	m.summary.setStyles(st)
	m.register.setStyles(st)
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

	// Right inner — tree and register both fill the full height.
	m.rightInnerW, m.rightInnerH = innerSize(m.rightWidth, h, m.styles.FocusedBorder)
	m.tree.width = m.rightInnerW
	m.tree.height = m.rightInnerH
	m.tree.clampOffset()
	m.register.SetSize(m.rightInnerW, m.rightInnerH)

	return m
}

func (m ManagerTab) Init() tea.Cmd {
	return tea.Batch(
		m.summary.spinner.Tick(),
		m.tree.spinner.Tick(),
		m.register.spinner.Tick(),
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

	case AccountRegisterMsg:
		if msg.Err != nil {
			m.register.SetError(msg.Err.Error())
		} else {
			m.register.SetRows(msg.Account, msg.Rows, msg.Total)
		}
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case managerModeRegister:
			switch msg.String() {
			case "esc":
				m.mode = managerModeTree
				// Reset register state so re-opening shows a fresh spinner.
				m.register = NewAccountRegisterPanel(m.styles)
				m.register.SetSize(m.rightInnerW, m.rightInnerH)
				return m, m.register.spinner.Tick()
			case "r":
				account := m.register.account
				if account != "" {
					m.register.state = stateLoading
					return m, FetchAccountRegister(m.client, account)
				}
			default:
				cmd := m.register.Update(msg)
				return m, cmd
			}

		case managerModeTree:
			switch msg.String() {
			case "r":
				m.tree.state = stateLoading
				m.summary.state = stateLoading
				return m, tea.Batch(
					FetchManagerAccounts(m.client),
					FetchManagerBalances(m.client),
					FetchManagerSummary(m.client),
				)
			case "enter":
				account := m.tree.SelectedAccount()
				if account != "" {
					m.mode = managerModeRegister
					m.register.state = stateLoading
					m.register.account = account
					return m, tea.Batch(
						m.register.spinner.Tick(),
						FetchAccountRegister(m.client, account),
					)
				}
			default:
				m.tree.Update(msg)
			}
		}
		return m, nil

	default:
		cmd1 := m.summary.Update(msg)
		cmd2 := m.tree.Update(msg)
		cmd3 := m.register.Update(msg)
		return m, tea.Batch(cmd1, cmd2, cmd3)
	}
}

func (m ManagerTab) KeyMap() help.KeyMap {
	if m.mode == managerModeRegister {
		return ManagerRegisterKeyMap{}
	}
	return ManagerTreeKeyMap{}
}

func (m ManagerTab) View() string {
	// Left column: summary + placeholder (same in both modes).
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

	// Right column: account tree or register.
	var rightContent string
	var rightTitle string
	if m.mode == managerModeRegister {
		rightContent = lipgloss.NewStyle().
			Width(m.rightInnerW).
			Height(m.rightInnerH).
			Render(m.register.View())
		rightTitle = m.register.Title()
	} else {
		rightContent = lipgloss.NewStyle().
			Width(m.rightInnerW).
			Height(m.rightInnerH).
			Render(m.tree.View())
		rightTitle = "Accounts"
	}

	rightPanel := renderCard(rightContent, rightTitle, true, m.rightWidth, m.height, m.styles)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
}
