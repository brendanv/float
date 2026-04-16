package ui

import (
	"strings"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	floatv1 "github.com/brendanv/float/gen/float/v1"
)

type TransactionsPanel struct {
	panelBase
	styles       Styles
	transactions []*floatv1.Transaction
	rowToTx      []int
	splitView    bool
	table        table.Model
}

func newTransactionsTable(st Styles) table.Model {
	return table.New(
		table.WithColumns([]table.Column{
			{Title: "St", Width: 2},
			{Title: "Date", Width: 10},
			{Title: "Description", Width: 20},
			{Title: "Amount", Width: 13},
			{Title: "Account", Width: 20},
		}),
		table.WithStyles(styledTableStyles(st)),
		table.WithFocused(false),
	)
}

func newTransactionsPanel(st Styles) TransactionsPanel {
	return TransactionsPanel{
		styles:    st,
		panelBase: newPanelBase(),
		table:     newTransactionsTable(st),
	}
}

func (p *TransactionsPanel) setStyles(st Styles) {
	p.styles = st
	p.table.SetStyles(styledTableStyles(st))
}

func (p *TransactionsPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
	remaining := w - 2 - 10 - 13 - 4 // subtract St(2) + Date(10) + Amount(13) + separators(4)
	if remaining < 2 {
		remaining = 2
	}
	descWidth := remaining * 40 / 100
	acctWidth := remaining - descWidth
	if descWidth < 1 {
		descWidth = 1
	}
	if acctWidth < 1 {
		acctWidth = 1
	}
	p.table.SetColumns([]table.Column{
		{Title: "St", Width: 2},
		{Title: "Date", Width: 10},
		{Title: "Description", Width: descWidth},
		{Title: "Amount", Width: 13},
		{Title: "Account", Width: acctWidth},
	})
	p.table.SetWidth(w)
	p.table.SetHeight(h)
}

func (p *TransactionsPanel) SetTransactions(txs []*floatv1.Transaction) {
	p.transactions = txs
	if p.state != stateError {
		p.state = stateLoaded
	}
	p.rebuildRows()
}

func primaryPosting(tx *floatv1.Transaction) *floatv1.Posting {
	for _, post := range tx.Postings {
		a := strings.ToLower(post.Account)
		if strings.HasPrefix(a, "expenses:") || strings.HasPrefix(a, "income:") {
			return post
		}
	}
	if len(tx.Postings) > 0 {
		return tx.Postings[0]
	}
	return nil
}

func (p *TransactionsPanel) formatDescription(tx *floatv1.Transaction) string {
	if tx.Payee != nil {
		if tx.Note != nil {
			return tx.GetPayee() + " · " + tx.GetNote()
		}
		return tx.GetPayee()
	}
	return tx.Description
}

func statusSymbol(status string) string {
	switch status {
	case "Pending":
		return "!"
	case "Cleared":
		return "*"
	default:
		return " "
	}
}

func (p *TransactionsPanel) rebuildRows() {
	p.rowToTx = nil
	var rows []table.Row
	for i, tx := range p.transactions {
		sym := statusSymbol(tx.Status)
		if !p.splitView {
			post := primaryPosting(tx)
			acct := ""
			amt := ""
			if post != nil {
				acct = post.Account
				amt = formatBalance(post.Amounts)
			}
			rows = append(rows, table.Row{sym, tx.Date, p.formatDescription(tx), amt, acct})
			p.rowToTx = append(p.rowToTx, i)
		} else {
			for _, post := range tx.Postings {
				rows = append(rows, table.Row{sym, tx.Date, p.formatDescription(tx), formatBalance(post.Amounts), post.Account})
				p.rowToTx = append(p.rowToTx, i)
			}
		}
	}
	p.table.SetRows(rows)
}

func (p *TransactionsPanel) SelectedTransaction() *floatv1.Transaction {
	if len(p.rowToTx) == 0 || p.table.Cursor() >= len(p.rowToTx) {
		return nil
	}
	idx := p.rowToTx[p.table.Cursor()]
	if idx >= len(p.transactions) {
		return nil
	}
	return p.transactions[idx]
}

// Count returns the number of transactions currently loaded.
func (p *TransactionsPanel) Count() int {
	return len(p.transactions)
}

func (p *TransactionsPanel) Focus() {
	p.table.Focus()
}

func (p *TransactionsPanel) Blur() {
	p.table.Blur()
}

func (p *TransactionsPanel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if p.state != stateLoaded {
			return nil
		}
		if msg.String() == "s" {
			p.splitView = !p.splitView
			p.rebuildRows()
			return nil
		}
		var cmd tea.Cmd
		p.table, cmd = p.table.Update(msg)
		return cmd
	}
	return p.handleSpinnerTick(msg)
}

func (p TransactionsPanel) View() string {
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
