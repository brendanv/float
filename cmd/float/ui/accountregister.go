package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	floatv1 "github.com/brendanv/float/gen/float/v1"
)

// AccountRegisterPanel displays a scrollable register for a single account:
// every transaction touching that account, with a running balance.
type AccountRegisterPanel struct {
	panelBase
	styles  Styles
	account string
	total   int32
	rows    []*floatv1.AccountRegisterRow
	table   table.Model
}

func newAccountRegisterTable(st Styles) table.Model {
	return table.New(
		table.WithColumns([]table.Column{
			{Title: "St", Width: 2},
			{Title: "Date", Width: 10},
			{Title: "Description", Width: 20},
			{Title: "Other Accounts", Width: 20},
			{Title: "Change", Width: 13},
			{Title: "Balance", Width: 13},
		}),
		table.WithStyles(styledTableStyles(st)),
		table.WithFocused(true),
	)
}

func NewAccountRegisterPanel(st Styles) AccountRegisterPanel {
	return AccountRegisterPanel{
		styles:    st,
		panelBase: newPanelBase(),
		table:     newAccountRegisterTable(st),
	}
}

func (p *AccountRegisterPanel) setStyles(st Styles) {
	p.styles = st
	p.table.SetStyles(styledTableStyles(st))
}

func (p *AccountRegisterPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
	// Fixed columns: St(2) + Date(10) + Change(13) + Balance(13) + 6 cols * 2 padding = 50.
	fixed := 2 + 10 + 13 + 13 + (6 * 2)
	remaining := w - fixed
	if remaining < 4 {
		remaining = 4
	}
	descW := remaining * 45 / 100
	otherW := remaining - descW
	if descW < 2 {
		descW = 2
	}
	if otherW < 2 {
		otherW = 2
	}
	p.table.SetColumns([]table.Column{
		{Title: "St", Width: 2},
		{Title: "Date", Width: 10},
		{Title: "Description", Width: descW},
		{Title: "Other Accounts", Width: otherW},
		{Title: "Change", Width: 13},
		{Title: "Balance", Width: 13},
	})
	p.table.SetWidth(w)
	p.table.SetHeight(h)
}

func (p *AccountRegisterPanel) SetRows(account string, rows []*floatv1.AccountRegisterRow, total int32) {
	p.account = account
	p.rows = rows
	p.total = total
	if p.state != stateError {
		p.state = stateLoaded
	}
	p.rebuildRows()
}

func (p *AccountRegisterPanel) rebuildRows() {
	tableRows := make([]table.Row, len(p.rows))
	for i, r := range p.rows {
		sym := statusSymbol(r.Status)
		desc := r.Description
		if r.Payee != nil {
			desc = r.GetPayee()
			if r.Note != nil {
				desc += " · " + r.GetNote()
			}
		}
		other := strings.Join(r.OtherAccounts, ", ")
		change := formatBalance(r.Change)
		balance := formatBalance(r.RunningTotal)
		tableRows[i] = table.Row{sym, r.Date, desc, other, change, balance}
	}
	p.table.SetRows(tableRows)
}

// Title returns a display string showing the account name and total row count.
func (p *AccountRegisterPanel) Title() string {
	if p.account == "" {
		return "Register"
	}
	if p.total > 0 {
		return fmt.Sprintf("%s (%d)", p.account, p.total)
	}
	return p.account
}

func (p *AccountRegisterPanel) SelectedRow() *floatv1.AccountRegisterRow {
	if len(p.rows) == 0 || p.table.Cursor() >= len(p.rows) {
		return nil
	}
	return p.rows[p.table.Cursor()]
}

func (p *AccountRegisterPanel) SelectedFid() string {
	row := p.SelectedRow()
	if row == nil {
		return ""
	}
	return row.Fid
}

func (p *AccountRegisterPanel) Update(msg tea.Msg) tea.Cmd {
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

func (p AccountRegisterPanel) View() string {
	if p.height < 3 {
		return ""
	}
	switch p.state {
	case stateLoading:
		return p.renderLoading()
	case stateError:
		return p.renderError(true)
	case stateLoaded:
		if len(p.rows) == 0 {
			return lipgloss.NewStyle().
				Width(p.width).Height(p.height).
				Align(lipgloss.Center, lipgloss.Center).
				Render("no transactions")
		}
		return p.table.View()
	}
	return ""
}
