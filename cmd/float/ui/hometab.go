package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

type HomeTab struct {
	leftWidth  int
	rightWidth int
	height     int
	client     floatv1connect.LedgerServiceClient
	accounts     AccountsPanel
	transactions TransactionsPanel
	filter       FilterInput
	query        []string
	focused      int
}

func NewHomeTab(client floatv1connect.LedgerServiceClient) HomeTab {
	m := HomeTab{
		client:       client,
		accounts:     NewAccountsPanel(),
		transactions: newTransactionsPanel(),
		filter:       NewFilterInput(client),
	}
	m.accounts.Focus()
	return m
}

func (m HomeTab) SetSize(w, h int) HomeTab {
	layout := CalcLayout(w, h+2)
	m.leftWidth = layout.LeftWidth
	m.rightWidth = layout.RightWidth
	m.height = h
	leftInnerW, leftInnerH := innerSize(m.leftWidth, m.height, BorderStyle)
	m.accounts.SetSize(leftInnerW, leftInnerH)
	rightInnerW, rightInnerH := innerSize(m.rightWidth, m.height, BorderStyle)
	txH := rightInnerH
	if m.filter.Active() {
		txH--
	}
	if txH < 0 {
		txH = 0
	}
	m.transactions.SetSize(rightInnerW, txH)
	return m
}

func (m HomeTab) Init() tea.Cmd {
	return tea.Batch(
		m.accounts.spinner.Tick(),
		m.transactions.spinner.Tick(),
		FetchAccounts(m.client),
		FetchBalances(m.client, 0, nil),
		FetchTransactions(m.client, nil),
	)
}

func (m HomeTab) Update(msg tea.Msg) (HomeTab, tea.Cmd) {
	switch msg := msg.(type) {
	case AccountsMsg:
		if msg.Err != nil {
			m.accounts.SetError(msg.Err.Error())
		} else {
			m.accounts.SetAccounts(msg.Accounts)
		}
		return m, nil

	case BalancesMsg:
		if msg.Err != nil {
			m.accounts.SetError(msg.Err.Error())
		} else {
			m.accounts.SetBalances(msg.Report)
		}
		return m, nil

	case TransactionsMsg:
		if msg.Err != nil {
			m.transactions.SetError(msg.Err.Error())
		} else {
			m.transactions.SetTransactions(msg.Transactions)
		}
		return m, nil

	case RetryFetchMsg:
		m.accounts.state = stateLoading
		m.transactions.state = stateLoading
		return m, tea.Batch(
			FetchAccounts(m.client),
			FetchBalances(m.client, 0, nil),
			FetchTransactions(m.client, m.query),
		)

	case tea.KeyMsg:
		if !m.filter.Active() {
			switch msg.String() {
			case "r":
				m.accounts.state = stateLoading
				m.transactions.state = stateLoading
				return m, tea.Batch(
					FetchAccounts(m.client),
					FetchBalances(m.client, 0, nil),
					FetchTransactions(m.client, m.query),
				)
			case "tab", "l", "h":
				m.focused = 1 - m.focused
				if m.focused == 0 {
					m.accounts.Focus()
					m.transactions.Blur()
				} else {
					m.accounts.Blur()
					m.transactions.Focus()
				}
				return m, nil
			case "/":
				if m.focused == 1 {
					m.filter.Activate()
					_, rightInnerH := innerSize(m.rightWidth, m.height, BorderStyle)
					txH := rightInnerH - 1
					if txH < 0 {
						txH = 0
					}
					rightInnerW, _ := innerSize(m.rightWidth, m.height, BorderStyle)
					m.transactions.SetSize(rightInnerW, txH)
					return m, nil
				}
			}
		}

		if m.filter.Active() {
			switch msg.String() {
			case "esc":
				cmd := m.filter.Deactivate()
				m.query = nil
				m.transactions.state = stateLoading
				_, rightInnerH := innerSize(m.rightWidth, m.height, BorderStyle)
				rightInnerW, _ := innerSize(m.rightWidth, m.height, BorderStyle)
				m.transactions.SetSize(rightInnerW, rightInnerH)
				return m, cmd
			case "enter":
				m.query = m.filter.Query()
				m.transactions.state = stateLoading
				return m, FetchTransactions(m.client, m.query)
			default:
				newFilter, cmd := m.filter.Update(msg)
				m.filter = newFilter
				return m, cmd
			}
		}

		if m.focused == 0 {
			cmd := m.accounts.Update(msg)
			return m, cmd
		}
		cmd := m.transactions.Update(msg)
		return m, cmd

	default:
		cmd1 := m.accounts.Update(msg)
		cmd2 := m.transactions.Update(msg)
		return m, tea.Batch(cmd1, cmd2)
	}
}

func (m HomeTab) View() string {
	leftInnerW, leftInnerH := innerSize(m.leftWidth, m.height, BorderStyle)
	rightInnerW, rightInnerH := innerSize(m.rightWidth, m.height, BorderStyle)

	leftContent := lipgloss.NewStyle().
		Width(leftInnerW).
		Height(leftInnerH).
		Render(m.accounts.View())

	var rightContent string
	if m.filter.Active() {
		txContent := lipgloss.NewStyle().
			Width(rightInnerW).
			Height(rightInnerH - 1).
			Render(m.transactions.View())
		filterLine := lipgloss.NewStyle().
			Width(rightInnerW).
			Render(m.filter.View())
		rightContent = lipgloss.JoinVertical(lipgloss.Left, txContent, filterLine)
	} else {
		rightContent = lipgloss.NewStyle().
			Width(rightInnerW).
			Height(rightInnerH).
			Render(m.transactions.View())
	}

	leftBorder := BorderStyle
	rightBorder := BorderStyle
	if m.focused == 0 {
		leftBorder = FocusedBorderStyle
	} else {
		rightBorder = FocusedBorderStyle
	}

	leftPanel := leftBorder.
		Width(leftInnerW).
		Height(leftInnerH).
		Render(leftContent)

	rightPanel := rightBorder.
		Width(rightInnerW).
		Height(rightInnerH).
		Render(rightContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
}

func (m HomeTab) HelpContext() HelpContext {
	return HelpContext{
		ActiveTab:    TabHome,
		HomeFocused:  m.focused,
		FilterActive: m.filter.Active(),
	}
}

func innerSize(outerW, outerH int, s lipgloss.Style) (int, int) {
	w := outerW - s.GetHorizontalFrameSize()
	h := outerH - s.GetVerticalFrameSize()
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return w, h
}
