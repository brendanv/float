package ui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

// AssertionsTab mirrors the web UI's balance assertions page. It shows every
// asset/liability account with activity, how stale its latest balance assertion
// is, and lets the user add a fresh assertion to the latest transaction.
type AssertionsTab struct {
	width, height int
	client        floatv1connect.LedgerServiceClient
	styles        Styles

	panelBase
	statuses []*floatv1.AccountAssertionStatus
	table    table.Model

	editingAccount string
	editingTx      *floatv1.Transaction
	assertQtyInput textinput.Model
	assertComInput textinput.Model
	editFocused    int
	submitting     bool
	editErrMsg     string
}

func NewAssertionsTab(client floatv1connect.LedgerServiceClient, st Styles) AssertionsTab {
	qty := textinput.New()
	qty.Placeholder = "real balance"
	com := textinput.New()
	com.Placeholder = "commodity"
	return AssertionsTab{
		client:         client,
		styles:         st,
		panelBase:      newPanelBase(),
		table:          newAssertionsTable(st),
		assertQtyInput: qty,
		assertComInput: com,
	}
}

func newAssertionsTable(st Styles) table.Model {
	return table.New(
		table.WithColumns([]table.Column{
			{Title: "Account", Width: 30},
			{Title: "Type", Width: 10},
			{Title: "Balance", Width: 20},
			{Title: "Last", Width: 14},
			{Title: "Status", Width: 18},
		}),
		table.WithRows(nil),
		table.WithStyles(styledTableStyles(st)),
		table.WithFocused(true),
	)
}

func (m AssertionsTab) setStyles(st Styles) AssertionsTab {
	m.styles = st
	m.table.SetStyles(styledTableStyles(st))
	return m
}

func (m AssertionsTab) SetSize(w, h int) AssertionsTab {
	m.width = w
	m.height = h
	m.panelBase.width = w
	m.panelBase.height = h
	m.table.SetWidth(w)
	m.table.SetHeight(h)
	m.resizeColumns()

	modalW := calcModalWidth(w)
	border := m.styles.FocusedBorder.Padding(modalVertPad, modalHorizPad)
	inputW := modalW - border.GetHorizontalFrameSize() - len("Commodity: ") - 2
	if inputW < 12 {
		inputW = 12
	}
	m.assertQtyInput.SetWidth(inputW)
	m.assertComInput.SetWidth(inputW)
	return m
}

func (m *AssertionsTab) resizeColumns() {
	statusW := 24
	lastW := 18
	typeW := 10
	balanceW := 24
	accountW := m.width - statusW - lastW - typeW - balanceW - 1
	if accountW < 24 {
		accountW = 24
	}
	if m.width < 100 {
		balanceW = 18
		lastW = 16
		statusW = 20
		accountW = m.width - statusW - lastW - typeW - balanceW - 1
		if accountW < 18 {
			accountW = 18
		}
	}
	m.table.SetColumns([]table.Column{
		{Title: "Account", Width: accountW},
		{Title: "Type", Width: typeW},
		{Title: "Balance", Width: balanceW},
		{Title: "Last assertion", Width: lastW},
		{Title: "Status", Width: statusW},
	})
}

func (m AssertionsTab) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick(), FetchBalanceAssertionStatus(m.client))
}

func (m AssertionsTab) capturesAllKeys() bool {
	return m.editingTx != nil
}

func (m AssertionsTab) KeyMap() help.KeyMap {
	if m.editingTx != nil {
		return AssertionFormKeyMap{}
	}
	return AssertionsListKeyMap{}
}

func (m AssertionsTab) Update(msg tea.Msg) (AssertionsTab, tea.Cmd) {
	switch msg := msg.(type) {
	case BalanceAssertionStatusMsg:
		if msg.Err != nil {
			m.SetError(msg.Err.Error())
		} else {
			m.setStatuses(msg.Accounts)
		}
		return m, nil

	case UpdateTransactionMsg:
		if m.editingTx == nil {
			return m, nil
		}
		m.submitting = false
		if msg.Err != nil {
			m.editErrMsg = msg.Err.Error()
			return m, nil
		}
		m.closeEditor()
		m.panelBase = newPanelBase()
		return m, tea.Batch(m.spinner.Tick(), FetchBalanceAssertionStatus(m.client))

	case tea.KeyMsg:
		if m.editingTx != nil {
			return m.updateEditor(msg)
		}
		switch msg.String() {
		case "r":
			m.panelBase = newPanelBase()
			return m, tea.Batch(m.spinner.Tick(), FetchBalanceAssertionStatus(m.client))
		case "enter":
			m.openSelectedEditor()
			return m, nil
		}
		if m.state == stateLoaded {
			var cmd tea.Cmd
			m.table, cmd = m.table.Update(msg)
			return m, cmd
		}
	}
	return m, m.handleSpinnerTick(msg)
}

func (m *AssertionsTab) setStatuses(statuses []*floatv1.AccountAssertionStatus) {
	m.statuses = statuses
	m.state = stateLoaded
	rows := make([]table.Row, 0, len(statuses))
	for _, s := range statuses {
		rows = append(rows, table.Row{
			truncateString(s.GetAccount(), 80),
			assertionTypeLabel(s.GetType()),
			formatCollapsedBalance(s.GetBalance()),
			formatAssertionLast(s),
			assertionStatusLabel(s),
		})
	}
	m.table.SetRows(rows)
}

func (m AssertionsTab) selectedStatus() *floatv1.AccountAssertionStatus {
	if len(m.statuses) == 0 {
		return nil
	}
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.statuses) {
		return nil
	}
	return m.statuses[idx]
}

func (m *AssertionsTab) openSelectedEditor() {
	s := m.selectedStatus()
	if s == nil || s.GetLastTransaction() == nil {
		return
	}
	m.editingAccount = s.GetAccount()
	m.editingTx = s.GetLastTransaction()
	m.editErrMsg = ""
	m.submitting = false
	m.editFocused = 0
	qty, commodity := assertionDefaults(m.editingTx, m.editingAccount)
	m.assertQtyInput.Reset()
	m.assertQtyInput.SetValue(qty)
	m.assertComInput.Reset()
	m.assertComInput.SetValue(commodity)
	m.focusEditorField()
}

func (m *AssertionsTab) closeEditor() {
	m.editingAccount = ""
	m.editingTx = nil
	m.editErrMsg = ""
	m.submitting = false
	m.editFocused = 0
	m.assertQtyInput.Blur()
	m.assertComInput.Blur()
}

func (m AssertionsTab) updateEditor(msg tea.KeyMsg) (AssertionsTab, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.closeEditor()
		return m, nil
	case "shift+tab":
		if m.editFocused > 0 {
			m.editFocused--
		}
		m.focusEditorField()
		return m, nil
	case "tab", "enter":
		if m.editFocused < 1 {
			m.editFocused++
		}
		m.focusEditorField()
		return m, nil
	case "shift+enter":
		req, errMsg := m.buildAssertionUpdateRequest()
		if errMsg != "" {
			m.editErrMsg = errMsg
			return m, nil
		}
		m.submitting = true
		m.editErrMsg = ""
		return m, UpdateTransactionCmd(m.client, req)
	}

	var cmd tea.Cmd
	if m.editFocused == 0 {
		m.assertQtyInput, cmd = m.assertQtyInput.Update(msg)
	} else {
		m.assertComInput, cmd = m.assertComInput.Update(msg)
	}
	return m, cmd
}

func (m *AssertionsTab) focusEditorField() {
	if m.editFocused == 0 {
		m.assertQtyInput.Focus()
		m.assertComInput.Blur()
	} else {
		m.assertQtyInput.Blur()
		m.assertComInput.Focus()
	}
}

func (m AssertionsTab) buildAssertionUpdateRequest() (*floatv1.UpdateTransactionRequest, string) {
	if m.editingTx == nil {
		return nil, "no transaction selected"
	}
	quantity := strings.TrimSpace(m.assertQtyInput.Value())
	commodity := strings.TrimSpace(m.assertComInput.Value())
	if quantity == "" {
		return nil, "asserted balance is required"
	}
	if commodity == "" {
		return nil, "commodity is required"
	}

	postings := make([]*floatv1.PostingInput, 0, len(m.editingTx.GetPostings()))
	applied := false
	for _, p := range m.editingTx.GetPostings() {
		in := postingToInput(p)
		if p.GetAccount() == m.editingAccount && !applied {
			in.BalanceAssertion = &floatv1.BalanceAssertion{Amount: &floatv1.Amount{Commodity: commodity, Quantity: quantity}}
			applied = true
		}
		postings = append(postings, in)
	}
	if !applied {
		return nil, "selected account is not on the transaction"
	}

	return &floatv1.UpdateTransactionRequest{
		Fid:         m.editingTx.GetFid(),
		Description: m.editingTx.GetDescription(),
		Date:        m.editingTx.GetDate(),
		Comment:     m.editingTx.GetComment(),
		Postings:    postings,
		Tags:        m.editingTx.GetTags(),
		Status:      m.editingTx.GetStatus(),
	}, ""
}

func postingToInput(p *floatv1.Posting) *floatv1.PostingInput {
	in := &floatv1.PostingInput{
		Account: p.GetAccount(),
		Comment: p.GetComment(),
	}
	if len(p.GetAmounts()) > 0 {
		a := p.GetAmounts()[0]
		in.Commodity = a.GetCommodity()
		in.Quantity = a.GetQuantity()
		in.Cost = a.GetCost()
	}
	if p.GetBalanceAssertion() != nil && p.GetBalanceAssertion().GetAmount() != nil {
		a := p.GetBalanceAssertion().GetAmount()
		in.BalanceAssertion = &floatv1.BalanceAssertion{Amount: &floatv1.Amount{Commodity: a.GetCommodity(), Quantity: a.GetQuantity()}}
	}
	return in
}

func assertionDefaults(tx *floatv1.Transaction, account string) (quantity, commodity string) {
	for _, p := range tx.GetPostings() {
		if p.GetAccount() != account {
			continue
		}
		if ba := p.GetBalanceAssertion(); ba != nil && ba.GetAmount() != nil {
			return ba.GetAmount().GetQuantity(), ba.GetAmount().GetCommodity()
		}
		if len(p.GetAmounts()) > 0 {
			return "", p.GetAmounts()[0].GetCommodity()
		}
	}
	return "", ""
}

func assertionTypeLabel(t string) string {
	switch t {
	case "A":
		return "Asset"
	case "L":
		return "Liability"
	default:
		return t
	}
}

func formatAssertionLast(s *floatv1.AccountAssertionStatus) string {
	date := s.GetLastAssertionDate()
	count := s.GetTransactionsSinceLastAssertion()
	if date == "" {
		return fmt.Sprintf("— (%s)", shortTxCountLabel(count))
	}
	if count == 0 {
		return date
	}
	return fmt.Sprintf("%s (%s)", date, shortTxCountLabel(count))
}

func assertionStatusLabel(s *floatv1.AccountAssertionStatus) string {
	count := s.GetTransactionsSinceLastAssertion()
	if s.GetLastAssertionDate() == "" {
		return "Never asserted"
	}
	if count == 0 {
		return "Up to date"
	}
	return transactionCountLabel(count)
}

func transactionCountLabel(count int32) string {
	if count == 1 {
		return "1 transaction since"
	}
	return fmt.Sprintf("%d transactions since", count)
}

func shortTxCountLabel(count int32) string {
	if count == 1 {
		return "1 tx"
	}
	return fmt.Sprintf("%d tx", count)
}

func formatCollapsedBalance(amounts []*floatv1.Amount) string {
	collapsed := collapseAmountsByCommodity(amounts)
	if len(collapsed) == 0 {
		return "—"
	}
	return formatBalance(collapsed)
}

func collapseAmountsByCommodity(amounts []*floatv1.Amount) []*floatv1.Amount {
	if len(amounts) == 0 {
		return nil
	}
	totals := make(map[string]float64)
	order := make([]string, 0, len(amounts))
	for _, a := range amounts {
		commodity := a.GetCommodity()
		if _, ok := totals[commodity]; !ok {
			order = append(order, commodity)
		}
		qty, _ := strconv.ParseFloat(strings.TrimSpace(a.GetQuantity()), 64)
		totals[commodity] += qty
	}
	out := make([]*floatv1.Amount, 0, len(order))
	for _, commodity := range order {
		out = append(out, &floatv1.Amount{Commodity: commodity, Quantity: strconv.FormatFloat(totals[commodity], 'f', -1, 64)})
	}
	return out
}

func (m AssertionsTab) View() string {
	if m.height < 3 {
		return ""
	}

	var base string
	switch m.state {
	case stateLoading:
		base = m.renderLoading()
	case stateError:
		base = m.renderError(true)
	case stateLoaded:
		if len(m.statuses) == 0 {
			base = lipgloss.NewStyle().Width(m.width).Height(m.height).Align(lipgloss.Center, lipgloss.Center).
				Render("No asset or liability accounts with transactions were found.")
		} else {
			title := m.styles.Active.Bold(true).Render("Balance Assertions")
			desc := m.styles.Help.Render("Track asset/liability accounts by transactions since their last enforced balance assertion. Press enter to record a fresh assertion on the selected account's latest transaction.")
			tableH := m.height - lipgloss.Height(title) - lipgloss.Height(desc) - 1
			if tableH < 1 {
				tableH = 1
			}
			m.table.SetHeight(tableH)
			base = lipgloss.JoinVertical(lipgloss.Left, title, desc, m.table.View())
		}
	default:
		base = ""
	}

	if m.editingTx != nil {
		return m.renderEditor()
	}
	return base
}

func (m AssertionsTab) renderEditor() string {
	if m.editingTx == nil {
		return ""
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("Account:     %s", m.editingAccount))
	lines = append(lines, fmt.Sprintf("Transaction: %s — %s", m.editingTx.GetDate(), m.editingTx.GetDescription()))
	lines = append(lines, "")
	lines = append(lines, m.styles.Help.Render("Enter the account's real balance. Saving writes an hledger '=' balance assertion to the selected account posting."))
	lines = append(lines, "")
	lines = append(lines, "Balance:    "+m.assertQtyInput.View())
	lines = append(lines, "Commodity:  "+m.assertComInput.View())
	lines = append(lines, "")
	lines = append(lines, m.styles.Active.Bold(true).Render("Postings"))
	for _, p := range m.editingTx.GetPostings() {
		marker := "  "
		if p.GetAccount() == m.editingAccount {
			marker = "→ "
		}
		amount := formatBalance(p.GetAmounts())
		if amount == "" {
			amount = "(auto)"
		}
		line := fmt.Sprintf("%s%s  %s", marker, truncateString(p.GetAccount(), 42), amount)
		if ba := p.GetBalanceAssertion(); ba != nil && ba.GetAmount() != nil {
			line += fmt.Sprintf(" = %s %s", ba.GetAmount().GetQuantity(), ba.GetAmount().GetCommodity())
		}
		lines = append(lines, line)
	}
	lines = append(lines, "")
	lines = append(lines, m.styles.Help.Render("tab/enter next field  shift+enter save assertion  esc cancel"))
	if m.editErrMsg != "" {
		lines = append(lines, m.styles.Error.Render("Error: "+m.editErrMsg))
	}
	if m.submitting {
		lines = append(lines, m.styles.Help.Render("Saving..."))
	}
	return RenderModal(m.width, m.height, "Record Balance Assertion", strings.Join(lines, "\n"), m.styles)
}
