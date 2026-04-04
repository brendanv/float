package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

type HomeTab struct {
	leftWidth   int
	rightWidth  int
	height      int
	client      floatv1connect.LedgerServiceClient
	accounts    AccountsPanel
	transactions TransactionsPanel
	filter      FilterInput
	period      PeriodSelector
	insights    InsightsPanel
	addTxForm   AddTxForm
	query       []string // filter query from /
	focused     int
	// left column sub-layout dimensions (inner, after border)
	leftInnerW  int
	leftInnerH  int
	accountsH   int
	insightsH   int
	showInsights bool

	// right column inner dimensions (after border)
	rightInnerW int
	rightInnerH int

	// delete confirmation state
	confirmDeleteTx *floatv1.Transaction
	deleteErrMsg    string

	// status update error
	statusErrMsg string

	presetIdx int // index into txFilterPresets; 0 = "All" (no extra filter)
}

func NewHomeTab(client floatv1connect.LedgerServiceClient) HomeTab {
	m := HomeTab{
		client:       client,
		accounts:     NewAccountsPanel(),
		transactions: newTransactionsPanel(),
		filter:       NewFilterInput(client),
		period:       NewPeriodSelector(),
		insights:     NewInsightsPanel(),
		addTxForm:    NewAddTxForm(client),
	}
	m.accounts.Focus()
	return m
}

// periodAndFilterQuery combines the period query with the user's filter query.
func (m HomeTab) periodAndFilterQuery() []string {
	q := []string{m.period.Query()}
	q = append(q, m.query...)
	q = append(q, txFilterPresets[m.presetIdx].tokens...)
	return q
}

func (m HomeTab) SetSize(w, h int) HomeTab {
	// Compute column widths directly — no CalcLayout roundtrip needed.
	m.leftWidth = clamp(w*30/100, 25, 45)
	m.rightWidth = w - m.leftWidth
	if m.rightWidth < 0 {
		m.rightWidth = 0
	}
	m.height = h

	// Left column: subtract border frame to get inner dimensions.
	leftInnerW, leftInnerH := innerSize(m.leftWidth, m.height, BorderStyle)
	m.leftInnerW = leftInnerW
	m.leftInnerH = leftInnerH

	// Left column sub-layout: accounts | period (1 line) | insights
	m.showInsights = leftInnerH >= 15
	if m.showInsights {
		m.accountsH = leftInnerH * 55 / 100
		if m.accountsH < 5 {
			m.accountsH = 5
		}
		m.insightsH = leftInnerH - m.accountsH - 1
		if m.insightsH < 3 {
			m.showInsights = false
		}
	}
	if !m.showInsights {
		m.accountsH = leftInnerH - 1 // leave 1 line for period selector
		m.insightsH = 0
	}

	m.accounts.SetSize(leftInnerW, m.accountsH)
	m.period.SetWidth(leftInnerW)
	m.insights.SetSize(leftInnerW, m.insightsH)

	// Right column: subtract border frame and store inner dimensions.
	rightInnerW, rightInnerH := innerSize(m.rightWidth, m.height, BorderStyle)
	m.rightInnerW = rightInnerW
	m.rightInnerH = rightInnerH

	txH := rightInnerH
	if m.filter.Active() {
		txH--
	}
	if m.presetIdx != 0 {
		txH--
	}
	if txH < 0 {
		txH = 0
	}
	m.transactions.SetSize(rightInnerW, txH)
	m.addTxForm.SetSize(rightInnerW, rightInnerH)
	return m
}

func (m HomeTab) Init() tea.Cmd {
	return tea.Batch(
		m.accounts.spinner.Tick(),
		m.transactions.spinner.Tick(),
		m.insights.spinner.Tick(),
		FetchAccounts(m.client),
		FetchBalances(m.client, 0, []string{m.period.Query()}),
		FetchTransactions(m.client, m.periodAndFilterQuery()),
		FetchInsights(m.client, m.period.Query()),
	)
}

func (m HomeTab) refreshAll() (HomeTab, tea.Cmd) {
	m.accounts.state = stateLoading
	m.transactions.state = stateLoading
	m.insights.state = stateLoading
	return m, tea.Batch(
		FetchAccounts(m.client),
		FetchBalances(m.client, 0, []string{m.period.Query()}),
		FetchTransactions(m.client, m.periodAndFilterQuery()),
		FetchInsights(m.client, m.period.Query()),
	)
}

func (m HomeTab) Update(msg tea.Msg) (HomeTab, tea.Cmd) {
	switch msg := msg.(type) {
	case AddTransactionMsg:
		m.addTxForm.submitting = false
		if msg.Err != nil {
			m.addTxForm.errMsg = msg.Err.Error()
			return m, nil
		}
		m.addTxForm.Deactivate()
		return m.refreshAll()

	case UpdateTransactionMsg:
		m.addTxForm.submitting = false
		if msg.Err != nil {
			m.addTxForm.errMsg = msg.Err.Error()
			return m, nil
		}
		m.addTxForm.Deactivate()
		return m.refreshAll()

	case DeleteTransactionMsg:
		m.confirmDeleteTx = nil
		if msg.Err != nil {
			m.deleteErrMsg = msg.Err.Error()
			return m, nil
		}
		m.deleteErrMsg = ""
		return m.refreshAll()

	case UpdateTransactionStatusMsg:
		if msg.Err != nil {
			m.statusErrMsg = msg.Err.Error()
			return m, nil
		}
		m.statusErrMsg = ""
		return m.refreshAll()

	case AccountsMsg:
		if msg.Err != nil {
			m.accounts.SetError(msg.Err.Error())
		} else {
			m.accounts.SetAccounts(msg.Accounts)
			m.addTxForm.SetAccounts(msg.Accounts)
		}
		return m, nil

	case BalancesMsg:
		if msg.Err != nil {
			m.accounts.SetError(msg.Err.Error())
		} else {
			m.accounts.SetBalances(msg.Report)
		}
		return m, nil

	case InsightsMsg:
		if msg.Err != nil {
			m.insights.SetError(msg.Err.Error())
		} else {
			m.insights.SetData(msg.Report)
		}
		return m, nil

	case TransactionsMsg:
		if msg.Err != nil {
			m.transactions.SetError(msg.Err.Error())
		} else {
			m.transactions.SetTransactions(msg.Transactions)
		}
		return m, nil

	case PeriodChangedMsg:
		m.accounts.state = stateLoading
		m.transactions.state = stateLoading
		m.insights.state = stateLoading
		return m, tea.Batch(
			FetchBalances(m.client, 0, []string{m.period.Query()}),
			FetchTransactions(m.client, m.periodAndFilterQuery()),
			FetchInsights(m.client, m.period.Query()),
		)

	case RetryFetchMsg:
		return m.refreshAll()

	case tea.KeyMsg:
		if m.addTxForm.Active() {
			newForm, cmd := m.addTxForm.Update(msg)
			m.addTxForm = newForm
			return m, cmd
		}

		// Delete confirmation intercepts all keys when active
		if m.confirmDeleteTx != nil {
			switch msg.String() {
			case "y":
				fid := m.confirmDeleteTx.Fid
				m.confirmDeleteTx = nil
				return m, DeleteTransactionCmd(m.client, fid)
			case "esc", "n":
				m.confirmDeleteTx = nil
				m.deleteErrMsg = ""
				return m, nil
			}
			return m, nil
		}

		if !m.filter.Active() {
			switch msg.String() {
			case "r":
				return m.refreshAll()
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
			case "[", "]":
				newPeriod, cmd := m.period.Update(msg)
				m.period = newPeriod
				return m, cmd
			case "/":
				if m.focused == 1 {
					m.filter.Activate()
					txH := m.rightInnerH - 1
					if txH < 0 {
						txH = 0
					}
					m.transactions.SetSize(m.rightInnerW, txH)
					return m, nil
				}
			case "a":
				if m.focused == 1 {
					m.addTxForm.Activate()
					return m, nil
				}
			case "e":
				if m.focused == 1 {
					if tx := m.transactions.SelectedTransaction(); tx != nil && tx.Fid != "" {
						m.addTxForm.ActivateEdit(tx)
					}
					return m, nil
				}
			case "d":
				if m.focused == 1 {
					if tx := m.transactions.SelectedTransaction(); tx != nil && tx.Fid != "" {
						m.confirmDeleteTx = tx
						m.deleteErrMsg = ""
					}
					return m, nil
				}
			case "c":
				if m.focused == 1 {
					if tx := m.transactions.SelectedTransaction(); tx != nil && tx.Fid != "" {
						newStatus := "Cleared"
						if tx.Status == "Cleared" {
							newStatus = "Pending"
						}
						m.statusErrMsg = ""
						return m, UpdateTransactionStatusCmd(m.client, tx.Fid, newStatus)
					}
					return m, nil
				}
			case "v":
				if m.focused == 1 {
					m.presetIdx = (m.presetIdx + 1) % len(txFilterPresets)
					txH := m.rightInnerH
					if m.filter.Active() {
						txH--
					}
					if m.presetIdx != 0 {
						txH--
					}
					if txH < 0 {
						txH = 0
					}
					m.transactions.SetSize(m.rightInnerW, txH)
					m.transactions.state = stateLoading
					return m, FetchTransactions(m.client, m.periodAndFilterQuery())
				}
			}
		}

		if m.filter.Active() {
			switch msg.String() {
			case "esc":
				cmd := m.filter.Deactivate()
				m.query = nil
				m.transactions.state = stateLoading
				m.transactions.SetSize(m.rightInnerW, m.rightInnerH)
				return m, cmd
			case "enter":
				m.query = m.filter.Query()
				m.transactions.state = stateLoading
				return m, FetchTransactions(m.client, m.periodAndFilterQuery())
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
		cmd3 := m.insights.Update(msg)
		return m, tea.Batch(cmd1, cmd2, cmd3)
	}
}

func (m HomeTab) View() string {
	// Build left column content as a vertical stack using stored inner dimensions.
	accountsView := lipgloss.NewStyle().
		Width(m.leftInnerW).
		Height(m.accountsH).
		Render(m.accounts.View())
	periodView := m.period.View()

	var leftContent string
	if m.showInsights {
		insightsView := lipgloss.NewStyle().
			Width(m.leftInnerW).
			Height(m.insightsH).
			Render(m.insights.View())
		leftContent = lipgloss.JoinVertical(lipgloss.Left, accountsView, periodView, insightsView)
	} else {
		leftContent = lipgloss.JoinVertical(lipgloss.Left, accountsView, periodView)
	}
	// Pad to full inner height.
	leftContent = lipgloss.NewStyle().
		Width(m.leftInnerW).
		Height(m.leftInnerH).
		Render(leftContent)

	var rightContent string
	switch {
	case m.addTxForm.Active():
		rightContent = lipgloss.NewStyle().
			Width(m.rightInnerW).
			Height(m.rightInnerH).
			Render(m.addTxForm.View())
	case m.confirmDeleteTx != nil:
		rightContent = lipgloss.NewStyle().
			Width(m.rightInnerW).
			Height(m.rightInnerH).
			Render(m.renderDeleteConfirm(m.rightInnerW))
	case m.filter.Active():
		txH := m.rightInnerH - 1
		if m.presetIdx != 0 {
			txH--
		}
		txContent := lipgloss.NewStyle().
			Width(m.rightInnerW).
			Height(txH).
			Render(m.transactions.View())
		filterLine := lipgloss.NewStyle().
			Width(m.rightInnerW).
			Render(m.filter.View())
		if m.presetIdx != 0 {
			statusLine := m.renderPresetLine()
			rightContent = lipgloss.JoinVertical(lipgloss.Left, statusLine, txContent, filterLine)
		} else {
			rightContent = lipgloss.JoinVertical(lipgloss.Left, txContent, filterLine)
		}
	default:
		if m.presetIdx != 0 {
			statusLine := m.renderPresetLine()
			txContent := lipgloss.NewStyle().
				Width(m.rightInnerW).
				Height(m.rightInnerH - 1).
				Render(m.transactions.View())
			rightContent = lipgloss.JoinVertical(lipgloss.Left, statusLine, txContent)
		} else {
			rightContent = lipgloss.NewStyle().
				Width(m.rightInnerW).
				Height(m.rightInnerH).
				Render(m.transactions.View())
		}
	}

	leftBorder := BorderStyle
	rightBorder := BorderStyle
	if m.focused == 0 {
		leftBorder = FocusedBorderStyle
	} else {
		rightBorder = FocusedBorderStyle
	}

	leftPanel := leftBorder.
		Width(m.leftWidth).
		Height(m.height).
		Render(leftContent)

	rightPanel := rightBorder.
		Width(m.rightWidth).
		Height(m.height).
		Render(rightContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
}

func (m HomeTab) renderDeleteConfirm(w int) string {
	tx := m.confirmDeleteTx
	var lines []string

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5555")).Render("Delete Transaction?")
	lines = append(lines, title)
	lines = append(lines, HelpStyle.Render(strings.Repeat("─", w)))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("  %s  %s", tx.Date, tx.Description))
	if len(tx.Postings) > 0 {
		lines = append(lines, "")
		const indent = 4
		const amtW = 14
		acctW := w - indent - amtW
		if acctW < 10 {
			acctW = 10
		}
		for _, p := range tx.Postings {
			amt := formatBalance(p.Amounts)
			line := strings.Repeat(" ", indent) + padRight(p.Account, acctW) + fmt.Sprintf("%*s", amtW, amt)
			lines = append(lines, line)
		}
	}
	lines = append(lines, "")
	if m.deleteErrMsg != "" {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555"))
		lines = append(lines, errStyle.Render("  Error: "+m.deleteErrMsg))
		lines = append(lines, "")
	}
	lines = append(lines, HelpStyle.Render("  Press y to confirm, esc to cancel"))

	return strings.Join(lines, "\n")
}

func (m HomeTab) renderPresetLine() string {
	preset := txFilterPresets[m.presetIdx]
	label := "[ " + preset.label + " ]"
	return lipgloss.NewStyle().
		Foreground(colorFocused).
		Width(m.rightInnerW).
		Render(label)
}

func (m HomeTab) KeyMap() help.KeyMap {
	switch {
	case m.addTxForm.Active():
		return HomeFormKeyMap{}
	case m.confirmDeleteTx != nil:
		return HomeDeleteKeyMap{}
	case m.filter.Active():
		return HomeFilterKeyMap{}
	case m.focused == 1:
		return HomeTxKeyMap{}
	default:
		return HomeDefaultKeyMap{}
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
