package ui

import (
	"strings"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	floatv1 "github.com/brendanv/float/gen/float/v1"
)

var accountTypeOrder = []string{"A", "L", "R", "X", "E"}

var accountTypeLabel = map[string]string{
	"A": "Assets",
	"L": "Liabilities",
	"R": "Revenue",
	"E": "Equity",
	"X": "Expenses",
}

type AccountsPanel struct {
	panelBase
	styles   Styles
	accounts []*floatv1.Account
	balances map[string][]*floatv1.Amount
	table    table.Model
}

func newAccountsTable(st Styles) table.Model {
	s := table.DefaultStyles()
	s.Header = s.Header.Bold(true)
	s.Selected = s.Selected.Foreground(st.FocusedFg).Bold(false).Reverse(true)
	return table.New(
		table.WithColumns([]table.Column{
			{Title: "Account", Width: 20},
			{Title: "Balance", Width: 15},
		}),
		table.WithStyles(s),
		table.WithFocused(false),
	)
}

func NewAccountsPanel(st Styles) AccountsPanel {
	return AccountsPanel{
		styles:    st,
		panelBase: newPanelBase(),
		table:     newAccountsTable(st),
	}
}

func (p *AccountsPanel) setStyles(st Styles) {
	p.styles = st
	s := table.DefaultStyles()
	s.Header = s.Header.Bold(true)
	s.Selected = s.Selected.Foreground(st.FocusedFg).Bold(false).Reverse(true)
	p.table.SetStyles(s)
}

func (p *AccountsPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
	nameWidth := w - 16
	if nameWidth < 1 {
		nameWidth = 1
	}
	p.table.SetColumns([]table.Column{
		{Title: "Account", Width: nameWidth},
		{Title: "Balance", Width: 15},
	})
	p.table.SetWidth(w)
	p.table.SetHeight(h)
}

func (p *AccountsPanel) SetAccounts(accounts []*floatv1.Account) {
	p.accounts = accounts
	if p.state != stateError {
		p.state = stateLoaded
	}
	p.rebuildRows()
}

func (p *AccountsPanel) SetBalances(report *floatv1.BalanceReport) {
	if report == nil {
		return
	}
	m := make(map[string][]*floatv1.Amount, len(report.Rows))
	for _, row := range report.Rows {
		m[row.FullName] = row.Amounts
	}
	p.balances = m
	if p.state != stateError {
		p.state = stateLoaded
	}
	p.rebuildRows()
}

func (p *AccountsPanel) rebuildRows() {
	grouped := groupedRows(p.accounts)
	rows := make([]table.Row, 0, len(grouped))
	for _, row := range grouped {
		if row.isHeader {
			rows = append(rows, table.Row{row.label, ""})
		} else {
			rows = append(rows, table.Row{
				"  " + row.account.Name,
				formatBalance(p.balances[row.account.FullName]),
			})
		}
	}
	p.table.SetRows(rows)
}

func (p *AccountsPanel) Focus() {
	p.table.Focus()
}

func (p *AccountsPanel) Blur() {
	p.table.Blur()
}

func (p *AccountsPanel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if p.state != stateLoaded {
			return nil
		}
		var cmd tea.Cmd
		p.table, cmd = p.table.Update(msg)
		return cmd
	}
	return p.handleSpinnerTick(msg)
}

type accountRow struct {
	isHeader bool
	label    string
	account  *floatv1.Account
}

func groupedRows(accounts []*floatv1.Account) []accountRow {
	byType := make(map[string][]*floatv1.Account)
	for _, a := range accounts {
		byType[a.Type] = append(byType[a.Type], a)
	}
	var rows []accountRow
	for _, t := range accountTypeOrder {
		accs := byType[t]
		if len(accs) == 0 {
			continue
		}
		rows = append(rows, accountRow{isHeader: true, label: accountTypeLabel[t]})
		for _, a := range accs {
			rows = append(rows, accountRow{account: a})
		}
	}
	return rows
}

func formatBalance(amounts []*floatv1.Amount) string {
	if len(amounts) == 0 {
		return ""
	}
	parts := make([]string, len(amounts))
	for i, a := range amounts {
		parts[i] = a.Quantity + " " + a.Commodity
	}
	return strings.Join(parts, ", ")
}

func (p AccountsPanel) View() string {
	if p.height < 3 {
		return ""
	}

	switch p.state {
	case stateLoading:
		return p.renderLoading()
	case stateError:
		return p.renderError(true)
	case stateLoaded:
		return p.table.View()
	}
	return ""
}
