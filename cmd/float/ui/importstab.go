package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

type importsMode int

const (
	importsModeList   importsMode = iota
	importsModeDetail importsMode = iota
)

// ImportsTab shows import history and lets the user drill into a specific batch.
type ImportsTab struct {
	width    int
	height   int
	client   floatv1connect.LedgerServiceClient
	styles   Styles
	mode     importsMode
	list     importsListPanel
	detail   importsDetailPanel

	addTxForm       AddTxForm
	confirmDeleteTx *floatv1.Transaction
	deleteErrMsg    string
	statusErrMsg    string
}

func NewImportsTab(client floatv1connect.LedgerServiceClient, st Styles) ImportsTab {
	return ImportsTab{
		client:    client,
		styles:    st,
		mode:      importsModeList,
		list:      newImportsListPanel(st),
		detail:    newImportsDetailPanel(st),
		addTxForm: NewAddTxForm(client, st),
	}
}

func (m ImportsTab) setStyles(st Styles) ImportsTab {
	m.styles = st
	m.list.setStyles(st)
	m.detail.setStyles(st)
	m.addTxForm.setStyles(st)
	return m
}

func (m ImportsTab) SetSize(w, h int) ImportsTab {
	m.width = w
	m.height = h
	m.list.SetSize(w, h)
	m.detail.SetSize(w, h)
	m.addTxForm.SetSize(w, h)
	return m
}

func (m ImportsTab) Init() tea.Cmd {
	return tea.Batch(
		m.list.spinner.Tick(),
		FetchImports(m.client),
		FetchAccounts(m.client),
	)
}

func (m ImportsTab) KeyMap() help.KeyMap {
	switch {
	case m.mode == importsModeDetail && m.addTxForm.Active():
		return HomeFormKeyMap{}
	case m.mode == importsModeDetail && m.confirmDeleteTx != nil:
		return DeleteConfirmKeyMap{}
	case m.mode == importsModeDetail:
		return ImportsDetailKeyMap{}
	default:
		return ImportsListKeyMap{}
	}
}

func (m ImportsTab) Update(msg tea.Msg) (ImportsTab, tea.Cmd) {
	switch msg := msg.(type) {
	case AccountsMsg:
		if msg.Err == nil {
			m.addTxForm.SetAccounts(msg.Accounts)
		}
		return m, nil

	case UpdateTransactionMsg:
		m.addTxForm.submitting = false
		if msg.Err != nil {
			m.addTxForm.errMsg = msg.Err.Error()
			return m, nil
		}
		m.addTxForm.Deactivate()
		m.detail.txPanel.state = stateLoading
		return m, FetchImportedTransactions(m.client, m.detail.batchId)

	case DeleteTransactionMsg:
		m.confirmDeleteTx = nil
		if msg.Err != nil {
			m.deleteErrMsg = msg.Err.Error()
			return m, nil
		}
		m.deleteErrMsg = ""
		m.detail.txPanel.state = stateLoading
		return m, FetchImportedTransactions(m.client, m.detail.batchId)

	case UpdateTransactionStatusMsg:
		if msg.Err != nil {
			m.statusErrMsg = msg.Err.Error()
			return m, nil
		}
		m.statusErrMsg = ""
		m.detail.txPanel.state = stateLoading
		return m, FetchImportedTransactions(m.client, m.detail.batchId)

	case ImportsMsg:
		if msg.Err != nil {
			m.list.SetError(msg.Err.Error())
		} else {
			m.list.setImports(msg.Imports)
		}
		return m, nil

	case ImportedTransactionsMsg:
		if msg.BatchId != m.detail.batchId {
			return m, nil
		}
		if msg.Err != nil {
			m.detail.txPanel.SetError(msg.Err.Error())
		} else {
			m.detail.txPanel.SetTransactions(msg.Transactions)
		}
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case importsModeList:
			switch msg.String() {
			case "r":
				m.list.panelBase = newPanelBase()
				return m, tea.Batch(m.list.spinner.Tick(), FetchImports(m.client))
			case "enter":
				imp := m.list.selected()
				if imp == nil {
					return m, nil
				}
				m.mode = importsModeDetail
				m.detail = newImportsDetailPanel(m.styles)
				m.detail.SetSize(m.width, m.height)
				m.detail.batchId = imp.ImportBatchId
				m.detail.txPanel.Focus()
				return m, tea.Batch(
					m.detail.txPanel.spinner.Tick(),
					FetchImportedTransactions(m.client, imp.ImportBatchId),
				)
			default:
				cmd := m.list.Update(msg)
				return m, cmd
			}

		case importsModeDetail:
			if m.addTxForm.Active() {
				newForm, cmd := m.addTxForm.Update(msg)
				m.addTxForm = newForm
				return m, cmd
			}
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
			switch msg.String() {
			case "esc":
				m.mode = importsModeList
				m.statusErrMsg = ""
				return m, nil
			case "r":
				m.detail.txPanel.state = stateLoading
				m.statusErrMsg = ""
				return m, FetchImportedTransactions(m.client, m.detail.batchId)
			case "e":
				if tx := m.detail.txPanel.SelectedTransaction(); tx != nil && tx.Fid != "" {
					m.addTxForm.ActivateEdit(tx)
				}
				return m, nil
			case "d":
				if tx := m.detail.txPanel.SelectedTransaction(); tx != nil && tx.Fid != "" {
					m.confirmDeleteTx = tx
					m.deleteErrMsg = ""
				}
				return m, nil
			case "c":
				if tx := m.detail.txPanel.SelectedTransaction(); tx != nil && tx.Fid != "" {
					newStatus := "Cleared"
					if tx.Status == "Cleared" {
						newStatus = "Pending"
					}
					m.statusErrMsg = ""
					return m, UpdateTransactionStatusCmd(m.client, tx.Fid, newStatus)
				}
				return m, nil
			default:
				cmd := m.detail.txPanel.Update(msg)
				return m, cmd
			}
		}

	default:
		switch m.mode {
		case importsModeList:
			cmd := m.list.Update(msg)
			return m, cmd
		case importsModeDetail:
			cmd := m.detail.txPanel.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m ImportsTab) View() string {
	switch m.mode {
	case importsModeList:
		return m.list.View()
	case importsModeDetail:
		if m.addTxForm.Active() {
			return m.addTxForm.View()
		}
		header := m.detail.renderHeader()
		var body string
		switch {
		case m.confirmDeleteTx != nil:
			body = lipgloss.NewStyle().Width(m.width).Height(m.height - 2).Render(m.renderDeleteConfirm(m.width))
		default:
			txView := m.detail.txPanel.View()
			if m.statusErrMsg != "" {
				errLine := m.styles.Error.Width(m.width).Render("  Error: " + m.statusErrMsg)
				body = lipgloss.JoinVertical(lipgloss.Left, txView, errLine)
			} else {
				body = txView
			}
		}
		return lipgloss.JoinVertical(lipgloss.Left, header, body)
	}
	return ""
}

func (m ImportsTab) renderDeleteConfirm(w int) string {
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

// ── list panel ───────────────────────────────────────────────────────────────

type importsListPanel struct {
	panelBase
	styles  Styles
	imports []*floatv1.ImportSummary
	table   table.Model
}

func newImportsListTable(st Styles) table.Model {
	return table.New(
		table.WithColumns([]table.Column{
			{Title: "Date", Width: 12},
			{Title: "Batch ID", Width: 24},
			{Title: "Txns", Width: 6},
		}),
		table.WithStyles(styledTableStyles(st)),
		table.WithFocused(true),
	)
}

func newImportsListPanel(st Styles) importsListPanel {
	return importsListPanel{
		styles:    st,
		panelBase: newPanelBase(),
		table:     newImportsListTable(st),
	}
}

func (p *importsListPanel) setStyles(st Styles) {
	p.styles = st
	p.table.SetStyles(styledTableStyles(st))
}

func (p *importsListPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
	// 1 (leading pad) + 12 (date) + 2 (sep) + 2 (sep) + 6 (txns) = 23 fixed
	batchW := w - 23
	if batchW < 10 {
		batchW = 10
	}
	p.table.SetColumns([]table.Column{
		{Title: "Date", Width: 12},
		{Title: "Batch ID", Width: batchW},
		{Title: "Txns", Width: 6},
	})
	p.table.SetWidth(w)
	p.table.SetHeight(h)
}

func (p *importsListPanel) setImports(imports []*floatv1.ImportSummary) {
	p.imports = imports
	p.state = stateLoaded
	rows := make([]table.Row, len(imports))
	for i, imp := range imports {
		rows[i] = table.Row{
			imp.Date,
			imp.ImportBatchId,
			fmt.Sprintf("%d", imp.TransactionCount),
		}
	}
	p.table.SetRows(rows)
}

func (p *importsListPanel) selected() *floatv1.ImportSummary {
	if len(p.imports) == 0 {
		return nil
	}
	c := p.table.Cursor()
	if c < 0 || c >= len(p.imports) {
		return nil
	}
	return p.imports[c]
}

func (p *importsListPanel) Update(msg tea.Msg) tea.Cmd {
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

func (p importsListPanel) View() string {
	switch p.state {
	case stateLoading:
		return p.renderLoading()
	case stateError:
		return p.renderError(true)
	case stateLoaded:
		if len(p.imports) == 0 {
			return lipgloss.NewStyle().
				Width(p.width).Height(p.height).
				Align(lipgloss.Center, lipgloss.Center).
				Render("No imports yet.")
		}
		return p.table.View()
	}
	return ""
}

// ── detail panel ─────────────────────────────────────────────────────────────

type importsDetailPanel struct {
	width   int
	height  int
	styles  Styles
	batchId string
	txPanel TransactionsPanel
}

func newImportsDetailPanel(st Styles) importsDetailPanel {
	return importsDetailPanel{
		styles:  st,
		txPanel: newTransactionsPanel(st),
	}
}

func (p *importsDetailPanel) setStyles(st Styles) {
	p.styles = st
	p.txPanel.setStyles(st)
}

func (p *importsDetailPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
	headerH := 2
	p.txPanel.SetSize(w, h-headerH)
}

func (p importsDetailPanel) renderHeader() string {
	header := p.styles.TabInactive.Render("← esc") + "  Import " + p.batchId
	return lipgloss.NewStyle().Width(p.width).Render(header)
}

func (p importsDetailPanel) View() string {
	return lipgloss.JoinVertical(lipgloss.Left, p.renderHeader(), p.txPanel.View())
}
