package ui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

type payeesSection int

const (
	payeesSectionPayees       payeesSection = iota
	payeesSectionDescriptions
)

type payeesMode int

const (
	payeesModeList     payeesMode = iota
	payeesModeTxDetail
	payeesModeAssign
)

const payeesSectionHeaderH = 1

type payeesDescRow struct {
	description string
	fids        []string
	count       int
}

// PayeesTab shows all payees and unassigned descriptions with inline payee assignment.
type PayeesTab struct {
	width, height int
	client        floatv1connect.LedgerServiceClient
	styles        Styles
	section       payeesSection
	mode          payeesMode

	payeesBase  panelBase
	payees      []string
	payeesTable table.Model

	descBase  panelBase
	descRows  []payeesDescRow
	descTable table.Model

	selectedPayee string
	txPanel       TransactionsPanel

	assignDesc       string
	assignFids       []string
	assignInput      textinput.Model
	assignErr        string
	assignSubmitting bool
}

func NewPayeesTab(client floatv1connect.LedgerServiceClient, st Styles) PayeesTab {
	inp := textinput.New()
	inp.Placeholder = "Payee name"

	return PayeesTab{
		client:      client,
		styles:      st,
		section:     payeesSectionPayees,
		mode:        payeesModeList,
		payeesBase:  newPanelBase(),
		payeesTable: newPayeesTable(st),
		descBase:    newPanelBase(),
		descTable:   newDescTable(st),
		txPanel:     newTransactionsPanel(st),
		assignInput: inp,
	}
}

func newPayeesTable(st Styles) table.Model {
	return table.New(
		table.WithColumns([]table.Column{{Title: "Payee", Width: 40}}),
		table.WithStyles(styledTableStyles(st)),
		table.WithFocused(true),
	)
}

func newDescTable(st Styles) table.Model {
	return table.New(
		table.WithColumns([]table.Column{
			{Title: "Description", Width: 40},
			{Title: "Count", Width: 8},
		}),
		table.WithStyles(styledTableStyles(st)),
		table.WithFocused(true),
	)
}

func (m PayeesTab) setStyles(st Styles) PayeesTab {
	m.styles = st
	m.payeesTable.SetStyles(styledTableStyles(st))
	m.descTable.SetStyles(styledTableStyles(st))
	m.txPanel.setStyles(st)
	return m
}

func (m PayeesTab) SetSize(w, h int) PayeesTab {
	m.width = w
	m.height = h
	contentH := h - payeesSectionHeaderH
	if contentH < 1 {
		contentH = 1
	}

	m.payeesBase.width = w
	m.payeesBase.height = contentH
	m.payeesTable.SetWidth(w)
	m.payeesTable.SetHeight(contentH)
	m.payeesTable.SetColumns([]table.Column{{Title: "Payee", Width: w - 1}})

	countW := 8
	descW := w - countW - 1
	if descW < 10 {
		descW = 10
	}
	m.descBase.width = w
	m.descBase.height = contentH
	m.descTable.SetWidth(w)
	m.descTable.SetHeight(contentH)
	m.descTable.SetColumns([]table.Column{
		{Title: "Description", Width: descW},
		{Title: "Count", Width: countW},
	})

	txHeaderH := 2
	m.txPanel.SetSize(w, h-txHeaderH)

	modalW := calcModalWidth(w)
	border := m.styles.FocusedBorder.Padding(modalVertPad, modalHorizPad)
	innerW := modalW - border.GetHorizontalFrameSize()
	if innerW < 1 {
		innerW = 1
	}
	m.assignInput.SetWidth(innerW)

	return m
}

func (m PayeesTab) Init() tea.Cmd {
	return tea.Batch(
		m.payeesBase.spinner.Tick(),
		m.descBase.spinner.Tick(),
		FetchPayees(m.client),
		FetchNoPayeeTransactions(m.client),
	)
}

func (m PayeesTab) capturesAllKeys() bool {
	return m.mode == payeesModeAssign
}

func (m PayeesTab) KeyMap() help.KeyMap {
	switch m.mode {
	case payeesModeTxDetail:
		return PayeesTxKeyMap{}
	case payeesModeAssign:
		return PayeesAssignKeyMap{}
	}
	return PayeesListKeyMap{}
}

func (m PayeesTab) Update(msg tea.Msg) (PayeesTab, tea.Cmd) {
	switch msg := msg.(type) {
	case PayeesMsg:
		if msg.Err != nil {
			m.payeesBase.SetError(msg.Err.Error())
		} else {
			m.payees = msg.Payees
			m.payeesBase.state = stateLoaded
			rows := make([]table.Row, len(msg.Payees))
			for i, p := range msg.Payees {
				rows[i] = table.Row{p}
			}
			m.payeesTable.SetRows(rows)
		}
		return m, nil

	case NoPayeeTransactionsMsg:
		if msg.Err != nil {
			m.descBase.SetError(msg.Err.Error())
		} else {
			m.descRows = groupByDescription(msg.Transactions)
			m.descBase.state = stateLoaded
			m.descTable.SetRows(descTableRows(m.descRows))
		}
		return m, nil

	case PayeeTransactionsMsg:
		if msg.Payee != m.selectedPayee {
			return m, nil
		}
		if msg.Err != nil {
			m.txPanel.SetError(msg.Err.Error())
		} else {
			m.txPanel.SetTransactions(msg.Transactions)
		}
		return m, nil

	case SetPayeeMsg:
		m.assignSubmitting = false
		if msg.Err != nil {
			m.assignErr = msg.Err.Error()
			return m, nil
		}
		m.mode = payeesModeList
		m.section = payeesSectionDescriptions
		m.assignDesc = ""
		m.assignFids = nil
		m.assignErr = ""
		m.assignInput.SetValue("")
		m.payeesBase = newPanelBase()
		m.descBase = newPanelBase()
		m.payeesBase.width = m.width
		m.payeesBase.height = m.height - payeesSectionHeaderH
		m.descBase.width = m.width
		m.descBase.height = m.height - payeesSectionHeaderH
		return m, tea.Batch(
			m.payeesBase.spinner.Tick(),
			m.descBase.spinner.Tick(),
			FetchPayees(m.client),
			FetchNoPayeeTransactions(m.client),
		)

	case tea.KeyMsg:
		switch m.mode {
		case payeesModeList:
			return m.updateList(msg)
		case payeesModeTxDetail:
			return m.updateTxDetail(msg)
		case payeesModeAssign:
			return m.updateAssign(msg)
		}

	default:
		switch m.mode {
		case payeesModeList:
			cmd1 := m.payeesBase.handleSpinnerTick(msg)
			cmd2 := m.descBase.handleSpinnerTick(msg)
			return m, tea.Batch(cmd1, cmd2)
		case payeesModeTxDetail:
			cmd := m.txPanel.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m PayeesTab) updateList(msg tea.KeyMsg) (PayeesTab, tea.Cmd) {
	switch msg.String() {
	case "h", "left":
		m.section = payeesSectionPayees
		return m, nil
	case "l", "right":
		m.section = payeesSectionDescriptions
		return m, nil
	case "r":
		m.payeesBase = newPanelBase()
		m.descBase = newPanelBase()
		m.payeesBase.width = m.width
		m.payeesBase.height = m.height - payeesSectionHeaderH
		m.descBase.width = m.width
		m.descBase.height = m.height - payeesSectionHeaderH
		return m, tea.Batch(
			m.payeesBase.spinner.Tick(),
			m.descBase.spinner.Tick(),
			FetchPayees(m.client),
			FetchNoPayeeTransactions(m.client),
		)
	case "enter":
		if m.section == payeesSectionPayees {
			payee := m.payeeAtCursor()
			if payee == "" {
				return m, nil
			}
			m.mode = payeesModeTxDetail
			m.selectedPayee = payee
			m.txPanel = newTransactionsPanel(m.styles)
			txHeaderH := 2
			m.txPanel.SetSize(m.width, m.height-txHeaderH)
			m.txPanel.Focus()
			return m, tea.Batch(
				m.txPanel.spinner.Tick(),
				FetchPayeeTransactions(m.client, payee),
			)
		}
		row := m.descRowAtCursor()
		if row == nil {
			return m, nil
		}
		m.mode = payeesModeAssign
		m.assignDesc = row.description
		m.assignFids = row.fids
		m.assignErr = ""
		m.assignInput.SetValue("")
		m.assignInput.Focus()
		return m, nil
	default:
		if m.section == payeesSectionPayees && m.payeesBase.state == stateLoaded {
			var cmd tea.Cmd
			m.payeesTable, cmd = m.payeesTable.Update(msg)
			return m, cmd
		}
		if m.section == payeesSectionDescriptions && m.descBase.state == stateLoaded {
			var cmd tea.Cmd
			m.descTable, cmd = m.descTable.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m PayeesTab) updateTxDetail(msg tea.KeyMsg) (PayeesTab, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = payeesModeList
		return m, nil
	case "r":
		m.txPanel.panelBase = newPanelBase()
		return m, tea.Batch(
			m.txPanel.spinner.Tick(),
			FetchPayeeTransactions(m.client, m.selectedPayee),
		)
	default:
		cmd := m.txPanel.Update(msg)
		return m, cmd
	}
}

func (m PayeesTab) updateAssign(msg tea.KeyMsg) (PayeesTab, tea.Cmd) {
	if m.assignSubmitting {
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.mode = payeesModeList
		m.assignErr = ""
		m.assignInput.SetValue("")
		return m, nil
	case "enter":
		payee := strings.TrimSpace(m.assignInput.Value())
		if payee == "" {
			return m, nil
		}
		m.assignErr = ""
		m.assignSubmitting = true
		return m, BulkEditSetPayeeCmd(m.client, m.assignFids, payee)
	default:
		var cmd tea.Cmd
		m.assignInput, cmd = m.assignInput.Update(msg)
		return m, cmd
	}
}

func (m PayeesTab) payeeAtCursor() string {
	if len(m.payees) == 0 {
		return ""
	}
	c := m.payeesTable.Cursor()
	if c < 0 || c >= len(m.payees) {
		return ""
	}
	return m.payees[c]
}

func (m PayeesTab) descRowAtCursor() *payeesDescRow {
	if len(m.descRows) == 0 {
		return nil
	}
	c := m.descTable.Cursor()
	if c < 0 || c >= len(m.descRows) {
		return nil
	}
	return &m.descRows[c]
}

func (m PayeesTab) View() string {
	switch m.mode {
	case payeesModeTxDetail:
		header := m.styles.TabInactive.Render("← esc") + "  Payee: " + m.selectedPayee
		header = lipgloss.NewStyle().Width(m.width).Render(header)
		return lipgloss.JoinVertical(lipgloss.Left, header, m.txPanel.View())
	case payeesModeAssign:
		return RenderModal(m.width, m.height, "Set Payee", m.viewAssignForm(), m.styles)
	}
	sectionHeader := m.renderSectionHeader()
	var content string
	if m.section == payeesSectionPayees {
		content = m.viewPayeesSection()
	} else {
		content = m.viewDescSection()
	}
	return lipgloss.JoinVertical(lipgloss.Left, sectionHeader, content)
}

func (m PayeesTab) renderSectionHeader() string {
	payeesLabel := "Payees"
	descLabel := "Unassigned"
	if m.descBase.state == stateLoaded {
		descLabel = fmt.Sprintf("Unassigned (%d)", len(m.descRows))
	}

	var payeesRendered, descRendered string
	if m.section == payeesSectionPayees {
		payeesRendered = m.styles.TabActive.Render("[ " + payeesLabel + " ]")
		descRendered = m.styles.TabInactive.Render(descLabel)
	} else {
		payeesRendered = m.styles.TabInactive.Render(payeesLabel)
		descRendered = m.styles.TabActive.Render("[ " + descLabel + " ]")
	}
	line := payeesRendered + "  " + descRendered + "  " + m.styles.Help.Render("h/l to switch")
	return lipgloss.NewStyle().Width(m.width).Render(line)
}

func (m PayeesTab) viewPayeesSection() string {
	switch m.payeesBase.state {
	case stateLoading:
		return m.payeesBase.renderLoading()
	case stateError:
		return m.payeesBase.renderError(true)
	case stateLoaded:
		if len(m.payees) == 0 {
			return lipgloss.NewStyle().
				Width(m.payeesBase.width).Height(m.payeesBase.height).
				Align(lipgloss.Center, lipgloss.Center).
				Render("No payees found.")
		}
		return m.payeesTable.View()
	}
	return ""
}

func (m PayeesTab) viewDescSection() string {
	switch m.descBase.state {
	case stateLoading:
		return m.descBase.renderLoading()
	case stateError:
		return m.descBase.renderError(true)
	case stateLoaded:
		if len(m.descRows) == 0 {
			return lipgloss.NewStyle().
				Width(m.descBase.width).Height(m.descBase.height).
				Align(lipgloss.Center, lipgloss.Center).
				Render("All transactions have a payee assigned.")
		}
		return m.descTable.View()
	}
	return ""
}

func (m PayeesTab) viewAssignForm() string {
	desc := m.assignDesc
	if len(desc) > 40 {
		desc = desc[:37] + "..."
	}
	lines := []string{
		m.styles.Help.Render("Description: ") + desc,
		"",
		m.assignInput.View(),
		"",
	}
	if m.assignSubmitting {
		lines = append(lines, m.styles.Help.Render("Saving…"))
	} else if m.assignErr != "" {
		lines = append(lines, m.styles.Error.Render("! "+m.assignErr))
		lines = append(lines, m.styles.Help.Render("enter to set  esc to cancel"))
	} else {
		lines = append(lines, m.styles.Help.Render("enter to set  esc to cancel"))
	}
	return strings.Join(lines, "\n")
}

func groupByDescription(txs []*floatv1.Transaction) []payeesDescRow {
	m := make(map[string][]string)
	for _, tx := range txs {
		m[tx.Description] = append(m[tx.Description], tx.Fid)
	}
	rows := make([]payeesDescRow, 0, len(m))
	for desc, fids := range m {
		rows = append(rows, payeesDescRow{description: desc, fids: fids, count: len(fids)})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].count != rows[j].count {
			return rows[i].count > rows[j].count
		}
		return rows[i].description < rows[j].description
	})
	return rows
}

func descTableRows(rows []payeesDescRow) []table.Row {
	result := make([]table.Row, len(rows))
	for i, r := range rows {
		result[i] = table.Row{r.description, fmt.Sprintf("%d", r.count)}
	}
	return result
}
