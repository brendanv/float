package ui

import (
	"fmt"

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
}

func NewImportsTab(client floatv1connect.LedgerServiceClient, st Styles) ImportsTab {
	return ImportsTab{
		client: client,
		styles: st,
		mode:   importsModeList,
		list:   newImportsListPanel(st),
		detail: newImportsDetailPanel(st),
	}
}

func (m ImportsTab) setStyles(st Styles) ImportsTab {
	m.styles = st
	m.list.setStyles(st)
	m.detail.setStyles(st)
	return m
}

func (m ImportsTab) SetSize(w, h int) ImportsTab {
	m.width = w
	m.height = h
	m.list.SetSize(w, h)
	m.detail.SetSize(w, h)
	return m
}

func (m ImportsTab) Init() tea.Cmd {
	return tea.Batch(
		m.list.spinner.Tick(),
		FetchImports(m.client),
	)
}

func (m ImportsTab) KeyMap() help.KeyMap {
	if m.mode == importsModeDetail {
		return ImportsDetailKeyMap{}
	}
	return ImportsListKeyMap{}
}

func (m ImportsTab) Update(msg tea.Msg) (ImportsTab, tea.Cmd) {
	switch msg := msg.(type) {
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
				return m, tea.Batch(
					m.detail.txPanel.spinner.Tick(),
					FetchImportedTransactions(m.client, imp.ImportBatchId),
				)
			default:
				cmd := m.list.Update(msg)
				return m, cmd
			}
		case importsModeDetail:
			switch msg.String() {
			case "esc":
				m.mode = importsModeList
				return m, nil
			case "r":
				m.detail.txPanel.panelBase = newPanelBase()
				return m, tea.Batch(
					m.detail.txPanel.spinner.Tick(),
					FetchImportedTransactions(m.client, m.detail.batchId),
				)
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
		return m.detail.View()
	}
	return ""
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

func (p importsDetailPanel) View() string {
	header := p.styles.TabInactive.Render("← esc") + "  Import " + p.batchId
	header = lipgloss.NewStyle().Width(p.width).Render(header)
	return lipgloss.JoinVertical(lipgloss.Left, header, p.txPanel.View())
}
