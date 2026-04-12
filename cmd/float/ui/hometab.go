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

// HomeTab is the landing-page tab. It has a three-panel dashboard layout:
//
//	╭── Chart (60% w, 62% h) ──────────────╮╭── Accounts (40% w) ──╮
//	│  Spending / Net Worth   t=toggle      ││  Account  Balance    │
//	│  [chart content]                      ││  ...                 │
//	│                                       ││  <<< Mar 2026 >>>    │
//	╰───────────────────────────────────────╯╰──────────────────────╯
//	╭── Transaction Review (full width, 38% h) ───────────────────────╮
//	│  3 unreviewed  ·  v=cycle: [Unreviewed] All Reviewed No-payee   │
//	│  St  Date  Description  Amount  Account                         │
//	╰─────────────────────────────────────────────────────────────────╯
type HomeTab struct {
	width, height           int
	topHeight, bottomHeight int // gross outer heights for the two rows
	chartWidth, acctWidth   int // gross outer widths for the top row columns

	// inner content dimensions (after border subtraction)
	chartInnerW, chartInnerH     int
	acctInnerW, acctInnerH       int
	acctTableH                   int // acctInnerH - 1 (reserves 1 row for period)
	unreviewedInnerW, unreviewedInnerH int

	styles     Styles
	client     floatv1connect.LedgerServiceClient
	chart      ChartPanel
	accounts   AccountsPanel
	unreviewed TransactionsPanel // default filter: not:status:*
	addTxForm  AddTxForm
	filter     FilterInput
	period     PeriodSelector

	focused   int  // 0=chart, 1=accounts, 2=unreviewed
	presetIdx int  // index into txFilterPresets; 0 = "Unreviewed"
	query     []string // filter query from /

	// overlays
	confirmDeleteTx *floatv1.Transaction
	deleteErrMsg    string
	statusErrMsg    string
}

func NewHomeTab(client floatv1connect.LedgerServiceClient, st Styles) HomeTab {
	m := HomeTab{
		styles:     st,
		client:     client,
		chart:      NewChartPanel(st),
		accounts:   NewAccountsPanel(st),
		unreviewed: newTransactionsPanel(st),
		addTxForm:  NewAddTxForm(client, st),
		filter:     NewFilterInput(client),
		period:     NewPeriodSelector(),
	}
	// Start focus on the chart panel.
	m.focused = 0
	return m
}

func (m HomeTab) setStyles(st Styles) HomeTab {
	m.styles = st
	m.chart.setStyles(st)
	m.addTxForm.setStyles(st)
	m.accounts.setStyles(st)
	m.unreviewed.setStyles(st)
	return m
}

// unreviewedQuery returns the hledger query tokens for the current preset,
func (m HomeTab) unreviewedQuery() []string {
	q := m.query
	q = append(q, m.period.Query())
	q = append(q, txFilterPresets[m.presetIdx].tokens...)
	return q
}

func (m HomeTab) SetSize(w, h int) HomeTab {
	m.width = w
	m.height = h

	// Vertical split: top row 62%, bottom row 38% (min 5).
	m.topHeight = h * 62 / 100
	if m.topHeight < 8 {
		m.topHeight = 8
	}
	m.bottomHeight = h - m.topHeight
	if m.bottomHeight < 5 {
		m.bottomHeight = 5
	}

	// Horizontal split for top row: chart 60%, accounts 40%.
	m.chartWidth = w * 60 / 100
	if m.chartWidth < 30 {
		m.chartWidth = 30
	}
	m.acctWidth = w - m.chartWidth
	if m.acctWidth < 20 {
		m.acctWidth = 20
	}

	// Compute inner dimensions (subtract border frame) for each panel.
	// Accounts spans the full height; chart and unreviewed share the left column.
	m.chartInnerW, m.chartInnerH = innerSize(m.chartWidth, m.topHeight, m.styles.Border)
	m.acctInnerW, m.acctInnerH = innerSize(m.acctWidth, h, m.styles.Border)
	m.unreviewedInnerW, m.unreviewedInnerH = innerSize(m.chartWidth, m.bottomHeight, m.styles.Border)

	// Accounts inner: leave 1 row at the bottom for the period selector.
	m.acctTableH = m.acctInnerH - 1
	if m.acctTableH < 3 {
		m.acctTableH = 3
	}

	m.chart.SetSize(m.chartInnerW, m.chartInnerH)
	m.accounts.SetSize(m.acctInnerW, m.acctTableH)
	m.period.SetWidth(m.acctInnerW)

	// Transaction review: 1 row reserved for the title/preset line.
	txH := m.unreviewedInnerH - 1
	if m.filter.Active() {
		txH--
	}
	if txH < 3 {
		txH = 3
	}
	m.unreviewed.SetSize(m.unreviewedInnerW, txH)
	m.addTxForm.SetSize(m.unreviewedInnerW, m.unreviewedInnerH)
	return m
}

func (m HomeTab) Init() tea.Cmd {
	return tea.Batch(
		m.chart.insights.spinner.Tick(),
		m.chart.netWorth.spinner.Tick(),
		m.accounts.spinner.Tick(),
		m.unreviewed.spinner.Tick(),
		FetchAccounts(m.client),
		FetchBalances(m.client, 0, []string{m.period.Query()}),
		FetchInsights(m.client, m.period.Query()),
		FetchHomeNetWorth(m.client),
		FetchTransactions(m.client, m.unreviewedQuery()),
	)
}

func (m HomeTab) refreshAll() (HomeTab, tea.Cmd) {
	m.accounts.state = stateLoading
	m.chart.insights.state = stateLoading
	m.chart.netWorth.state = stateLoading
	m.unreviewed.state = stateLoading
	return m, tea.Batch(
		FetchAccounts(m.client),
		FetchBalances(m.client, 0, []string{m.period.Query()}),
		FetchInsights(m.client, m.period.Query()),
		FetchHomeNetWorth(m.client),
		FetchTransactions(m.client, m.unreviewedQuery()),
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
			m.chart.SetInsightsError(msg.Err.Error())
		} else {
			m.chart.SetInsightsData(msg.Report)
		}
		return m, nil

	case HomeNetWorthMsg:
		if msg.Err != nil {
			m.chart.SetNetWorthError(msg.Err.Error())
		} else {
			m.chart.SetNetWorthData(msg.Snapshots)
		}
		return m, nil

	case TransactionsMsg:
		if msg.Err != nil {
			m.unreviewed.SetError(msg.Err.Error())
		} else {
			m.unreviewed.SetTransactions(msg.Transactions)
		}
		return m, nil

	case PeriodChangedMsg:
		m.accounts.state = stateLoading
		m.chart.insights.state = stateLoading
		m.unreviewed.state = stateLoading
		return m, tea.Batch(
			FetchBalances(m.client, 0, []string{m.period.Query()}),
			FetchInsights(m.client, m.period.Query()),
			FetchTransactions(m.client, m.unreviewedQuery()),
		)

	case RetryFetchMsg:
		return m.refreshAll()

	case tea.KeyMsg:
		if m.addTxForm.Active() {
			newForm, cmd := m.addTxForm.Update(msg)
			m.addTxForm = newForm
			return m, cmd
		}

		// Delete confirmation intercepts all keys when active.
		if m.confirmDeleteTx != nil {
			switch msg.String() {
			case "y":
				fid := m.confirmDeleteTx.Fid
				m.confirmDeleteTx = nil
				return m, DeleteTransactionCmd(m.client, fid)
			case "esc", "n":
				m.confirmDeleteTx = nil
				m.deleteErrMsg = ""
			}
			return m, nil
		}

		// Filter input handling (only when transaction review panel has focus).
		if m.filter.Active() {
			switch msg.String() {
			case "esc":
				cmd := m.filter.Deactivate()
				m.query = nil
				m.unreviewed.state = stateLoading
				txH := m.unreviewedInnerH - 1
				if txH < 3 {
					txH = 3
				}
				m.unreviewed.SetSize(m.unreviewedInnerW, txH)
				return m, tea.Batch(cmd, FetchTransactions(m.client, m.unreviewedQuery()))
			case "enter":
				m.query = m.filter.Query()
				m.unreviewed.state = stateLoading
				return m, FetchTransactions(m.client, m.unreviewedQuery())
			default:
				newFilter, cmd := m.filter.Update(msg)
				m.filter = newFilter
				return m, cmd
			}
		}

		// Global shortcuts (any focus).
		switch msg.String() {
		case "r":
			return m.refreshAll()
		case "tab", "shift+tab":
			// Tab cycles forward; shift+tab cycles backward through 3 panels.
			delta := 1
			if msg.String() == "shift+tab" {
				delta = -1
			}
			m.focused = (m.focused + 3 + delta) % 3
			m.updateFocus()
			return m, nil
		case "h":
			m.focused = (m.focused + 3 - 1) % 3
			m.updateFocus()
			return m, nil
		case "l":
			m.focused = (m.focused + 1) % 3
			m.updateFocus()
			return m, nil
		case "[", "]":
			newPeriod, cmd := m.period.Update(msg)
			m.period = newPeriod
			return m, cmd
		}

		// Panel-specific shortcuts.
		switch m.focused {
		case 0: // chart
			if msg.String() == "t" {
				m.chart.Toggle()
				return m, nil
			}
			cmd := m.chart.Update(msg)
			return m, cmd

		case 1: // accounts
			cmd := m.accounts.Update(msg)
			return m, cmd

		case 2: // unreviewed / transaction review
			switch msg.String() {
			case "a":
				m.addTxForm.Activate()
				return m, nil
			case "e":
				if tx := m.unreviewed.SelectedTransaction(); tx != nil && tx.Fid != "" {
					m.addTxForm.ActivateEdit(tx)
				}
				return m, nil
			case "d":
				if tx := m.unreviewed.SelectedTransaction(); tx != nil && tx.Fid != "" {
					m.confirmDeleteTx = tx
					m.deleteErrMsg = ""
				}
				return m, nil
			case "c":
				if tx := m.unreviewed.SelectedTransaction(); tx != nil && tx.Fid != "" {
					newStatus := "Cleared"
					if tx.Status == "Cleared" {
						newStatus = "Pending"
					}
					m.statusErrMsg = ""
					return m, UpdateTransactionStatusCmd(m.client, tx.Fid, newStatus)
				}
				return m, nil
			case "v":
				m.presetIdx = (m.presetIdx + 1) % len(txFilterPresets)
				m.unreviewed.state = stateLoading
				return m, FetchTransactions(m.client, m.unreviewedQuery())
			case "/":
				m.filter.Activate()
				txH := m.unreviewedInnerH - 2 // 1 title row + 1 filter row
				if txH < 3 {
					txH = 3
				}
				m.unreviewed.SetSize(m.unreviewedInnerW, txH)
				return m, nil
			case "s":
				cmd := m.unreviewed.Update(msg)
				return m, cmd
			default:
				cmd := m.unreviewed.Update(msg)
				return m, cmd
			}
		}

	default:
		cmd1 := m.chart.Update(msg)
		cmd2 := m.accounts.Update(msg)
		cmd3 := m.unreviewed.Update(msg)
		return m, tea.Batch(cmd1, cmd2, cmd3)
	}

	return m, nil
}

// updateFocus sets the Focus/Blur state of each panel based on m.focused.
func (m *HomeTab) updateFocus() {
	switch m.focused {
	case 1:
		m.accounts.Focus()
		m.unreviewed.Blur()
	case 2:
		m.accounts.Blur()
		m.unreviewed.Focus()
	default: // 0 = chart
		m.accounts.Blur()
		m.unreviewed.Blur()
	}
}

func (m HomeTab) View() string {
	// ── Chart panel (top-left) ────────────────────────────────────────
	chartContent := lipgloss.NewStyle().
		Width(m.chartInnerW).
		Height(m.chartInnerH).
		Render(m.chart.View())
	chartBorder := m.pickBorder(m.focused == 0)
	chartPanel := chartBorder.
		Width(m.chartWidth).
		Height(m.topHeight).
		Render(chartContent)
	chartTitle := "Spending"
	if m.chart.mode == chartModeNetWorth {
		chartTitle = "Net Worth"
	}
	chartPanel = injectBorderTitle(chartPanel, chartTitle, m.focused == 0, m.styles)

	// ── Accounts panel (top-right) ────────────────────────────────────
	acctTableView := lipgloss.NewStyle().
		Width(m.acctInnerW).
		Height(m.acctTableH).
		Render(m.accounts.View())
	periodView := m.period.View()
	acctContent := lipgloss.NewStyle().
		Width(m.acctInnerW).
		Height(m.acctInnerH).
		Render(lipgloss.JoinVertical(lipgloss.Left, acctTableView, periodView))
	acctBorder := m.pickBorder(m.focused == 1)
	acctPanel := acctBorder.
		Width(m.acctWidth).
		Height(m.height).
		Render(acctContent)
	acctPanel = injectBorderTitle(acctPanel, "Accounts", m.focused == 1, m.styles)

	// ── Transaction review panel (bottom-left, same width as chart) ───────
	var bottomContent string
	var bottomTitle string
	switch {
	case m.addTxForm.Active():
		bottomContent = lipgloss.NewStyle().
			Width(m.unreviewedInnerW).
			Height(m.unreviewedInnerH).
			Render(m.addTxForm.View())
		if m.addTxForm.editFID != "" {
			bottomTitle = "Edit Transaction"
		} else {
			bottomTitle = "Add Transaction"
		}
	case m.confirmDeleteTx != nil:
		bottomContent = lipgloss.NewStyle().
			Width(m.unreviewedInnerW).
			Height(m.unreviewedInnerH).
			Render(m.renderDeleteConfirm(m.unreviewedInnerW))
		bottomTitle = "Delete Transaction"
	default:
		bottomContent = m.renderUnreviewed()
		bottomTitle = "Transactions"
	}
	bottomBorder := m.pickBorder(m.focused == 2)
	bottomPanel := bottomBorder.
		Width(m.chartWidth).
		Height(m.bottomHeight).
		Render(bottomContent)
	bottomPanel = injectBorderTitle(bottomPanel, bottomTitle, m.focused == 2, m.styles)

	// Left column: chart on top, transaction review below.
	leftCol := lipgloss.JoinVertical(lipgloss.Left, chartPanel, bottomPanel)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftCol, acctPanel)
}

// pickBorder returns FocusedBorderStyle when the panel is focused.
func (m HomeTab) pickBorder(focused bool) lipgloss.Style {
	if focused {
		return m.styles.FocusedBorder
	}
	return m.styles.Border
}

// renderUnreviewed builds the content for the transaction review panel.
func (m HomeTab) renderUnreviewed() string {
	// Build the preset selector line.
	var presetParts []string
	for i, p := range txFilterPresets {
		if i == m.presetIdx {
			presetParts = append(presetParts,
				m.styles.Active.Render("["+p.label+"]"))
		} else {
			presetParts = append(presetParts, m.styles.Help.Render(p.label))
		}
	}

	titleLine := lipgloss.NewStyle().MaxWidth(m.unreviewedInnerW).Render(
		m.styles.Help.Render("[v]iew: ") + strings.Join(presetParts, "  "),
	)

	renderTxBody := func(h int) string {
		if m.unreviewed.state == stateLoaded && m.unreviewed.Count() == 0 {
			msg := "No transactions"
			if m.presetIdx == 0 {
				msg = "All caught up"
			}
			return lipgloss.NewStyle().
				Width(m.unreviewedInnerW).
				Height(h).
				AlignHorizontal(lipgloss.Center).
				AlignVertical(lipgloss.Center).
				Render(m.styles.Help.Render(msg))
		}
		return lipgloss.NewStyle().
			Width(m.unreviewedInnerW).
			Height(h).
			Render(m.unreviewed.View())
	}

	txH := m.unreviewedInnerH - 1
	if m.filter.Active() {
		txH--
	}
	if txH < 0 {
		txH = 0
	}
	txView := renderTxBody(txH)

	if m.statusErrMsg != "" {
		errLine := m.styles.Error.
			Width(m.unreviewedInnerW).
			Render("  Error: " + m.statusErrMsg)
		txH--
		txView = renderTxBody(max(txH, 0))
		if m.filter.Active() {
			filterLine := lipgloss.NewStyle().Width(m.unreviewedInnerW).Render(m.filter.View())
			return lipgloss.JoinVertical(lipgloss.Left, titleLine, txView, filterLine, errLine)
		}
		return lipgloss.JoinVertical(lipgloss.Left, titleLine, txView, errLine)
	}

	if m.filter.Active() {
		filterLine := lipgloss.NewStyle().Width(m.unreviewedInnerW).Render(m.filter.View())
		return lipgloss.JoinVertical(lipgloss.Left, titleLine, txView, filterLine)
	}
	return lipgloss.JoinVertical(lipgloss.Left, titleLine, txView)
}

func (m HomeTab) renderDeleteConfirm(w int) string {
	tx := m.confirmDeleteTx
	var lines []string

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
		lines = append(lines, m.styles.Error.Render("  Error: "+m.deleteErrMsg))
		lines = append(lines, "")
	}
	lines = append(lines, m.styles.Help.Render("  Press y to confirm, esc to cancel"))

	return strings.Join(lines, "\n")
}

func (m HomeTab) KeyMap() help.KeyMap {
	switch {
	case m.addTxForm.Active():
		return HomeFormKeyMap{}
	case m.confirmDeleteTx != nil:
		return HomeDeleteKeyMap{}
	case m.filter.Active():
		return HomeFilterKeyMap{}
	case m.focused == 0:
		return HomeChartKeyMap{}
	case m.focused == 1:
		return HomeAccountsKeyMap{}
	default:
		return HomeUnreviewedKeyMap{}
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
