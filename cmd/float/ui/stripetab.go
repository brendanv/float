package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

type stripeMode int

const (
	stripeModeList stripeMode = iota
	stripeModePreview
	stripeModeEdit
	stripeModeUnlink
)

// StripeTab lets the user manage Stripe Financial Connections linked accounts:
// view mappings, trigger transaction imports, edit hledger account mappings,
// and unlink accounts. New-account linking is intentionally not supported
// because the Stripe.js iframe flow can't run in a terminal.
type StripeTab struct {
	width, height int
	client        floatv1connect.LedgerServiceClient
	styles        Styles
	mode          stripeMode

	// Config / availability
	configLoaded bool
	enabled      bool
	configErr    string

	// List mode
	panelBase
	accounts []*floatv1.StripeLinkedAccount
	table    table.Model
	actionErr string

	// Preview mode
	previewLoading bool
	previewErr     string
	previewIsAll   bool
	previewSource  *floatv1.StripeLinkedAccount // nil for "fetch all"
	candidates     []stripePreviewRow
	previewTable   table.Model
	selected       map[int]bool
	importing      bool
	importErr      string
	previewSpinner Spinner

	// Edit mode
	editAcct       *floatv1.StripeLinkedAccount
	editHledger    textinput.Model
	editDisplay    textinput.Model
	editFocused    int
	editErr        string
	editSubmitting bool

	// Unlink mode
	unlinkAcct     *floatv1.StripeLinkedAccount
	unlinkErr      string
	unlinkSubmitting bool
}

type stripePreviewRow struct {
	account   *floatv1.StripeLinkedAccount
	candidate *floatv1.ImportCandidate
}

func NewStripeTab(client floatv1connect.LedgerServiceClient, st Styles) StripeTab {
	hledgerInp := textinput.New()
	hledgerInp.Placeholder = "assets:checking"

	displayInp := textinput.New()
	displayInp.Placeholder = "Chase Checking"

	return StripeTab{
		client:         client,
		styles:         st,
		panelBase:      newPanelBase(),
		table:          newStripeAccountsTable(st),
		previewTable:   newStripePreviewTable(st, false),
		selected:       map[int]bool{},
		editHledger:    hledgerInp,
		editDisplay:    displayInp,
		previewSpinner: NewSpinner(),
	}
}

func (m StripeTab) setStyles(st Styles) StripeTab {
	m.styles = st
	m.table.SetStyles(styledTableStyles(st))
	m.previewTable.SetStyles(styledTableStyles(st))
	return m
}

func (m StripeTab) SetSize(w, h int) StripeTab {
	m.width = w
	m.height = h
	m.panelBase.width = w
	m.panelBase.height = h
	m.table.SetWidth(w)
	m.table.SetHeight(h)
	m.previewTable.SetWidth(w)
	m.previewTable.SetHeight(h - 2) // leave room for footer
	m.resizeListColumns()
	m.resizePreviewColumns()

	modalW := calcModalWidth(w)
	border := m.styles.FocusedBorder.Padding(modalVertPad, modalHorizPad)
	innerW := modalW - border.GetHorizontalFrameSize()
	if innerW < 1 {
		innerW = 1
	}
	m.editHledger.SetWidth(innerW)
	m.editDisplay.SetWidth(innerW)
	return m
}

func (m *StripeTab) resizeListColumns() {
	nameW := 24
	instW := 20
	fetchedW := 18
	idW := 16
	usedFixed := nameW + instW + fetchedW + idW + 4 // separators
	mappedW := m.width - usedFixed
	if mappedW < 12 {
		mappedW = 12
	}
	m.table.SetColumns([]table.Column{
		{Title: "Name", Width: nameW},
		{Title: "Institution", Width: instW},
		{Title: "Mapped To", Width: mappedW},
		{Title: "Last Fetched", Width: fetchedW},
		{Title: "Stripe ID", Width: idW},
	})
}

func (m *StripeTab) resizePreviewColumns() {
	selW := 3
	dateW := 12
	amtW := 12
	ruleW := 5
	dupW := 5
	var cols []table.Column
	if m.previewIsAll {
		acctW := 18
		usedFixed := selW + acctW + dateW + amtW + ruleW + dupW + 6
		descW := m.width - usedFixed
		if descW < 10 {
			descW = 10
		}
		cols = []table.Column{
			{Title: " ", Width: selW},
			{Title: "Account", Width: acctW},
			{Title: "Date", Width: dateW},
			{Title: "Description", Width: descW},
			{Title: "Amount", Width: amtW},
			{Title: "Rule", Width: ruleW},
			{Title: "Dup", Width: dupW},
		}
	} else {
		usedFixed := selW + dateW + amtW + ruleW + dupW + 5
		descW := m.width - usedFixed
		if descW < 10 {
			descW = 10
		}
		cols = []table.Column{
			{Title: " ", Width: selW},
			{Title: "Date", Width: dateW},
			{Title: "Description", Width: descW},
			{Title: "Amount", Width: amtW},
			{Title: "Rule", Width: ruleW},
			{Title: "Dup", Width: dupW},
		}
	}
	m.previewTable.SetColumns(cols)
}

func (m StripeTab) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick(),
		FetchStripeConfig(m.client),
		FetchStripeLinkedAccounts(m.client),
	)
}

func (m StripeTab) capturesAllKeys() bool {
	return m.mode != stripeModeList
}

func (m StripeTab) KeyMap() help.KeyMap {
	switch m.mode {
	case stripeModePreview:
		return StripePreviewKeyMap{}
	case stripeModeEdit:
		return StripeEditKeyMap{}
	case stripeModeUnlink:
		return DeleteConfirmKeyMap{}
	}
	return StripeListKeyMap{}
}

func (m StripeTab) Update(msg tea.Msg) (StripeTab, tea.Cmd) {
	switch msg := msg.(type) {
	case StripeConfigMsg:
		m.configLoaded = true
		if msg.Err != nil {
			m.configErr = msg.Err.Error()
			m.enabled = false
		} else {
			m.enabled = msg.Enabled
			m.configErr = ""
		}
		return m, nil

	case StripeLinkedAccountsMsg:
		if msg.Err != nil {
			m.SetError(msg.Err.Error())
		} else {
			m.setAccounts(msg.Accounts)
		}
		return m, nil

	case StripeFetchOneMsg:
		m.previewLoading = false
		if msg.Err != nil {
			m.previewErr = msg.Err.Error()
			return m, nil
		}
		m.setPreviewSingle(msg.Account, msg.Candidates)
		return m, nil

	case StripeFetchAllMsg:
		m.previewLoading = false
		if msg.Err != nil {
			m.previewErr = msg.Err.Error()
			return m, nil
		}
		m.setPreviewAll(msg.Groups)
		return m, nil

	case StripeImportDoneMsg:
		m.importing = false
		if msg.Err != nil {
			m.importErr = msg.Err.Error()
			return m, nil
		}
		// Back to list and reload (Last Fetched will refresh).
		m.mode = stripeModeList
		m.candidates = nil
		m.selected = map[int]bool{}
		m.previewErr = ""
		m.importErr = ""
		m.actionErr = ""
		m.panelBase = newPanelBase()
		return m, tea.Batch(m.spinner.Tick(), FetchStripeLinkedAccounts(m.client))

	case StripeUnlinkMsg:
		m.unlinkSubmitting = false
		if msg.Err != nil {
			m.unlinkErr = msg.Err.Error()
			return m, nil
		}
		m.mode = stripeModeList
		m.unlinkAcct = nil
		m.actionErr = ""
		m.panelBase = newPanelBase()
		return m, tea.Batch(m.spinner.Tick(), FetchStripeLinkedAccounts(m.client))

	case StripeCompleteLinkingMsg:
		m.editSubmitting = false
		if msg.Err != nil {
			m.editErr = msg.Err.Error()
			return m, nil
		}
		m.mode = stripeModeList
		m.editAcct = nil
		m.actionErr = ""
		m.panelBase = newPanelBase()
		return m, tea.Batch(m.spinner.Tick(), FetchStripeLinkedAccounts(m.client))

	case tea.KeyMsg:
		switch m.mode {
		case stripeModePreview:
			return m.updatePreview(msg)
		case stripeModeEdit:
			return m.updateEdit(msg)
		case stripeModeUnlink:
			return m.updateUnlink(msg)
		default:
			return m.updateList(msg)
		}

	default:
		// Spinner ticks for both the list and the preview/import phases.
		var cmds []tea.Cmd
		if cmd := m.handleSpinnerTick(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if cmd := m.previewSpinner.Update(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if len(cmds) == 0 {
			return m, nil
		}
		return m, tea.Batch(cmds...)
	}
}

func (m StripeTab) updateList(msg tea.KeyMsg) (StripeTab, tea.Cmd) {
	switch msg.String() {
	case "r":
		m.actionErr = ""
		m.panelBase = newPanelBase()
		return m, tea.Batch(
			m.spinner.Tick(),
			FetchStripeConfig(m.client),
			FetchStripeLinkedAccounts(m.client),
		)
	case "f":
		acct := m.selectedAccount()
		if acct == nil {
			return m, nil
		}
		m.mode = stripeModePreview
		m.previewIsAll = false
		m.previewSource = acct
		m.previewTable = newStripePreviewTable(m.styles, false)
		m.previewTable.SetWidth(m.width)
		m.previewTable.SetHeight(m.height - 2)
		m.resizePreviewColumns()
		m.candidates = nil
		m.selected = map[int]bool{}
		m.previewLoading = true
		m.previewErr = ""
		m.importErr = ""
		return m, tea.Batch(m.previewSpinner.Tick(), FetchStripeOneCmd(m.client, acct))
	case "F":
		if len(m.accounts) == 0 {
			return m, nil
		}
		m.mode = stripeModePreview
		m.previewIsAll = true
		m.previewSource = nil
		m.previewTable = newStripePreviewTable(m.styles, true)
		m.previewTable.SetWidth(m.width)
		m.previewTable.SetHeight(m.height - 2)
		m.resizePreviewColumns()
		m.candidates = nil
		m.selected = map[int]bool{}
		m.previewLoading = true
		m.previewErr = ""
		m.importErr = ""
		return m, tea.Batch(m.previewSpinner.Tick(), FetchStripeAllCmd(m.client))
	case "e":
		acct := m.selectedAccount()
		if acct == nil {
			return m, nil
		}
		m.mode = stripeModeEdit
		m.editAcct = acct
		m.editHledger.SetValue(acct.HledgerAccount)
		m.editDisplay.SetValue(acct.DisplayName)
		m.editFocused = 0
		m.editErr = ""
		m.editSubmitting = false
		m.focusEditField()
		return m, nil
	case "d":
		acct := m.selectedAccount()
		if acct == nil {
			return m, nil
		}
		m.mode = stripeModeUnlink
		m.unlinkAcct = acct
		m.unlinkErr = ""
		m.unlinkSubmitting = false
		return m, nil
	default:
		if m.state == stateLoaded {
			var cmd tea.Cmd
			m.table, cmd = m.table.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m StripeTab) updatePreview(msg tea.KeyMsg) (StripeTab, tea.Cmd) {
	if m.importing {
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.mode = stripeModeList
		m.candidates = nil
		m.selected = map[int]bool{}
		m.previewErr = ""
		m.importErr = ""
		return m, nil
	case " ":
		if len(m.candidates) == 0 {
			return m, nil
		}
		i := m.previewTable.Cursor()
		if i < 0 || i >= len(m.candidates) {
			return m, nil
		}
		if m.selected[i] {
			delete(m.selected, i)
		} else {
			m.selected[i] = true
		}
		m.refreshPreviewRows()
		return m, nil
	case "a":
		m.selected = map[int]bool{}
		for i, r := range m.candidates {
			if !r.candidate.IsDuplicate {
				m.selected[i] = true
			}
		}
		m.refreshPreviewRows()
		return m, nil
	case "A":
		m.selected = map[int]bool{}
		for i := range m.candidates {
			m.selected[i] = true
		}
		m.refreshPreviewRows()
		return m, nil
	case "n":
		m.selected = map[int]bool{}
		m.refreshPreviewRows()
		return m, nil
	case "enter":
		batches := m.buildImportBatches()
		if len(batches) == 0 {
			m.importErr = "no transactions selected"
			return m, nil
		}
		m.importing = true
		m.importErr = ""
		return m, tea.Batch(m.previewSpinner.Tick(), ImportStripeCmd(m.client, batches))
	default:
		if !m.previewLoading {
			var cmd tea.Cmd
			m.previewTable, cmd = m.previewTable.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m StripeTab) updateEdit(msg tea.KeyMsg) (StripeTab, tea.Cmd) {
	if m.editSubmitting {
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.mode = stripeModeList
		m.editAcct = nil
		m.editErr = ""
		return m, nil
	case "shift+enter":
		return m.submitEdit()
	case "tab", "enter":
		m.editFocused = (m.editFocused + 1) % 2
		m.focusEditField()
		return m, nil
	case "shift+tab":
		m.editFocused = (m.editFocused + 1) % 2
		m.focusEditField()
		return m, nil
	}
	return m.updateEditField(msg)
}

func (m StripeTab) updateEditField(msg tea.KeyMsg) (StripeTab, tea.Cmd) {
	var cmd tea.Cmd
	switch m.editFocused {
	case 0:
		m.editHledger, cmd = m.editHledger.Update(msg)
	case 1:
		m.editDisplay, cmd = m.editDisplay.Update(msg)
	}
	return m, cmd
}

func (m *StripeTab) focusEditField() {
	inputs := []*textinput.Model{&m.editHledger, &m.editDisplay}
	for i, inp := range inputs {
		if i == m.editFocused {
			inp.Focus()
		} else {
			inp.Blur()
		}
	}
}

func (m StripeTab) submitEdit() (StripeTab, tea.Cmd) {
	hl := strings.TrimSpace(m.editHledger.Value())
	if hl == "" {
		m.editErr = "hledger account is required"
		return m, nil
	}
	disp := strings.TrimSpace(m.editDisplay.Value())
	if disp == "" {
		disp = m.editAcct.DisplayName
	}
	m.editErr = ""
	m.editSubmitting = true
	return m, CompleteStripeLinkingCmd(m.client, &floatv1.LinkedAccountInput{
		StripeAccountId: m.editAcct.StripeAccountId,
		HledgerAccount:  hl,
		DisplayName:     disp,
	})
}

func (m StripeTab) updateUnlink(msg tea.KeyMsg) (StripeTab, tea.Cmd) {
	if m.unlinkSubmitting {
		return m, nil
	}
	switch msg.String() {
	case "y":
		if m.unlinkAcct == nil {
			return m, nil
		}
		m.unlinkSubmitting = true
		return m, UnlinkStripeCmd(m.client, m.unlinkAcct.StripeAccountId)
	case "esc", "n":
		m.mode = stripeModeList
		m.unlinkAcct = nil
		m.unlinkErr = ""
	}
	return m, nil
}

func (m StripeTab) View() string {
	switch m.mode {
	case stripeModePreview:
		return m.viewPreview()
	case stripeModeEdit:
		return RenderModal(m.width, m.height, "Edit Mapping", m.viewEdit(), m.styles)
	case stripeModeUnlink:
		return RenderModal(m.width, m.height, "Confirm Unlink", m.viewUnlink(), m.styles)
	}
	return m.viewList()
}

func (m StripeTab) viewList() string {
	// If the config is loaded and Stripe is disabled, render an explanation
	// instead of an empty/error list.
	if m.configLoaded && !m.enabled {
		msg := "Stripe is not configured.\n\nSet STRIPE_SECRET_KEY and STRIPE_PUBLISHABLE_KEY on floatd."
		if m.configErr != "" {
			msg = "! " + m.configErr
		}
		return lipgloss.NewStyle().
			Width(m.width).Height(m.height).
			Align(lipgloss.Center, lipgloss.Center).
			Render(msg)
	}

	switch m.state {
	case stateLoading:
		return m.renderLoading()
	case stateError:
		return m.renderError(true)
	case stateLoaded:
		if len(m.accounts) == 0 {
			return lipgloss.NewStyle().
				Width(m.width).Height(m.height).
				Align(lipgloss.Center, lipgloss.Center).
				Render("No Stripe accounts linked.\nUse the web UI to link an account.")
		}
		body := m.table.View()
		if m.actionErr != "" {
			errLine := m.styles.Error.Render("! " + m.actionErr)
			body = lipgloss.JoinVertical(lipgloss.Left, body, errLine)
		}
		return body
	}
	return ""
}

func (m StripeTab) viewPreview() string {
	title := "Stripe import — "
	switch {
	case m.previewIsAll:
		title += "All accounts"
	case m.previewSource != nil:
		title += stripeAccountLabel(m.previewSource)
	}
	header := m.styles.TabInactive.Render("← esc") + "  " + title
	headerLine := lipgloss.NewStyle().Width(m.width).Render(header)

	var body string
	switch {
	case m.previewLoading:
		body = lipgloss.NewStyle().
			Width(m.width).Height(m.height - 1).
			Align(lipgloss.Center, lipgloss.Center).
			Render(m.previewSpinner.View() + " Fetching candidates…")
	case m.previewErr != "":
		body = lipgloss.NewStyle().
			Width(m.width).Height(m.height - 1).
			Align(lipgloss.Center, lipgloss.Center).
			Render(m.styles.Error.Render("! " + m.previewErr))
	case len(m.candidates) == 0:
		body = lipgloss.NewStyle().
			Width(m.width).Height(m.height - 1).
			Align(lipgloss.Center, lipgloss.Center).
			Render("No new transactions to import.")
	default:
		body = m.previewTable.View()
		footer := m.renderPreviewFooter()
		body = lipgloss.JoinVertical(lipgloss.Left, body, footer)
	}
	return lipgloss.JoinVertical(lipgloss.Left, headerLine, body)
}

func (m StripeTab) renderPreviewFooter() string {
	sel := len(m.selected)
	total := len(m.candidates)
	parts := []string{fmt.Sprintf("%d/%d selected", sel, total)}
	if m.importing {
		parts = append(parts, m.previewSpinner.View()+" importing…")
	} else if m.importErr != "" {
		return m.styles.Error.Render("! " + m.importErr)
	}
	return m.styles.Help.Render(strings.Join(parts, "  "))
}

func (m StripeTab) viewEdit() string {
	name := ""
	if m.editAcct != nil {
		name = stripeAccountLabel(m.editAcct)
	}
	lines := []string{
		m.styles.Help.Render(name),
		"",
		m.editFieldLabel("hledger account", 0) + m.editHledger.View(),
		m.editFieldLabel("Display name", 1) + m.editDisplay.View(),
		"",
	}
	if m.editSubmitting {
		lines = append(lines, m.styles.Help.Render("Saving…"))
	} else if m.editErr != "" {
		lines = append(lines, m.styles.Error.Render("! "+m.editErr))
	} else {
		lines = append(lines, m.styles.Help.Render("shift+enter to save  esc to cancel  tab to switch field"))
	}
	return strings.Join(lines, "\n")
}

func (m StripeTab) editFieldLabel(name string, idx int) string {
	st := m.styles.Help
	if m.editFocused == idx {
		st = m.styles.Active
	}
	return st.Render(fmt.Sprintf("%-18s ", name))
}

func (m StripeTab) viewUnlink() string {
	name := ""
	if m.unlinkAcct != nil {
		name = stripeAccountLabel(m.unlinkAcct)
	}
	lines := []string{
		fmt.Sprintf("Unlink %q?", name),
		"",
		"This disconnects the account from Stripe and removes",
		"it from float. Existing imported transactions are kept.",
		"",
	}
	if m.unlinkSubmitting {
		lines = append(lines, m.styles.Help.Render("Unlinking…"))
	} else if m.unlinkErr != "" {
		lines = append(lines, m.styles.Error.Render("! "+m.unlinkErr))
		lines = append(lines, "")
		lines = append(lines, m.styles.Help.Render("[y] retry  [esc] cancel"))
	} else {
		lines = append(lines, m.styles.Help.Render("[y] confirm  [esc] cancel"))
	}
	return strings.Join(lines, "\n")
}

func (m *StripeTab) setAccounts(accounts []*floatv1.StripeLinkedAccount) {
	m.accounts = accounts
	m.state = stateLoaded
	rows := make([]table.Row, len(accounts))
	for i, a := range accounts {
		rows[i] = table.Row{
			truncate(displayNameOrID(a), 24),
			truncate(a.InstitutionName, 20),
			truncate(a.HledgerAccount, 30),
			formatLastFetched(a.LastFetchedAt),
			truncate(a.StripeAccountId, 16),
		}
	}
	m.table.SetRows(rows)
}

func (m *StripeTab) setPreviewSingle(account *floatv1.StripeLinkedAccount, cands []*floatv1.ImportCandidate) {
	m.previewSource = account
	m.previewIsAll = false
	m.candidates = make([]stripePreviewRow, 0, len(cands))
	m.selected = map[int]bool{}
	for i, c := range cands {
		m.candidates = append(m.candidates, stripePreviewRow{account: account, candidate: c})
		if !c.IsDuplicate {
			m.selected[i] = true
		}
	}
	m.refreshPreviewRows()
}

func (m *StripeTab) setPreviewAll(groups []*floatv1.AccountCandidates) {
	m.previewIsAll = true
	m.previewSource = nil
	m.candidates = nil
	m.selected = map[int]bool{}
	for _, g := range groups {
		for _, c := range g.Candidates {
			idx := len(m.candidates)
			m.candidates = append(m.candidates, stripePreviewRow{account: g.Account, candidate: c})
			if !c.IsDuplicate {
				m.selected[idx] = true
			}
		}
	}
	m.refreshPreviewRows()
}

func (m *StripeTab) refreshPreviewRows() {
	rows := make([]table.Row, len(m.candidates))
	for i, r := range m.candidates {
		mark := " "
		if m.selected[i] {
			mark = "x"
		}
		ruleMark := ""
		if r.candidate.MatchedRuleId != "" {
			ruleMark = "✓"
		}
		dupMark := ""
		if r.candidate.IsDuplicate {
			dupMark = "✓"
		}
		date := ""
		desc := ""
		amt := ""
		if tx := r.candidate.Transaction; tx != nil {
			date = tx.Date
			desc = tx.Description
			amt = primaryPostingAmount(tx)
		}
		if m.previewIsAll {
			rows[i] = table.Row{
				mark,
				truncate(displayNameOrID(r.account), 18),
				date,
				desc,
				amt,
				ruleMark,
				dupMark,
			}
		} else {
			rows[i] = table.Row{mark, date, desc, amt, ruleMark, dupMark}
		}
	}
	m.previewTable.SetRows(rows)
}

func (m StripeTab) buildImportBatches() []StripeImportBatch {
	byAccount := map[string][]string{}
	order := []string{}
	for i, r := range m.candidates {
		if !m.selected[i] {
			continue
		}
		if r.candidate.SourceId == "" || r.account == nil {
			continue
		}
		id := r.account.StripeAccountId
		if _, ok := byAccount[id]; !ok {
			order = append(order, id)
		}
		byAccount[id] = append(byAccount[id], r.candidate.SourceId)
	}
	batches := make([]StripeImportBatch, 0, len(order))
	for _, id := range order {
		batches = append(batches, StripeImportBatch{
			StripeAccountID:      id,
			StripeTransactionIDs: byAccount[id],
		})
	}
	return batches
}

func (m StripeTab) selectedAccount() *floatv1.StripeLinkedAccount {
	if len(m.accounts) == 0 {
		return nil
	}
	c := m.table.Cursor()
	if c < 0 || c >= len(m.accounts) {
		return nil
	}
	return m.accounts[c]
}

func newStripeAccountsTable(st Styles) table.Model {
	return table.New(
		table.WithColumns([]table.Column{
			{Title: "Name", Width: 24},
			{Title: "Institution", Width: 20},
			{Title: "Mapped To", Width: 30},
			{Title: "Last Fetched", Width: 18},
			{Title: "Stripe ID", Width: 16},
		}),
		table.WithStyles(styledTableStyles(st)),
		table.WithFocused(true),
	)
}

func newStripePreviewTable(st Styles, withAccount bool) table.Model {
	var cols []table.Column
	if withAccount {
		cols = []table.Column{
			{Title: " ", Width: 3},
			{Title: "Account", Width: 18},
			{Title: "Date", Width: 12},
			{Title: "Description", Width: 30},
			{Title: "Amount", Width: 12},
			{Title: "Rule", Width: 5},
			{Title: "Dup", Width: 5},
		}
	} else {
		cols = []table.Column{
			{Title: " ", Width: 3},
			{Title: "Date", Width: 12},
			{Title: "Description", Width: 40},
			{Title: "Amount", Width: 12},
			{Title: "Rule", Width: 5},
			{Title: "Dup", Width: 5},
		}
	}
	return table.New(
		table.WithColumns(cols),
		table.WithStyles(styledTableStyles(st)),
		table.WithFocused(true),
	)
}

func stripeAccountLabel(a *floatv1.StripeLinkedAccount) string {
	if a == nil {
		return ""
	}
	return displayNameOrID(a)
}

func displayNameOrID(a *floatv1.StripeLinkedAccount) string {
	if a == nil {
		return ""
	}
	if a.DisplayName != "" {
		return a.DisplayName
	}
	return a.StripeAccountId
}

func formatLastFetched(ts string) string {
	if ts == "" {
		return "never"
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Local().Format("2006-01-02 15:04")
}

// primaryPostingAmount returns a compact amount string for the first
// non-empty posting in the candidate, used as a quick preview column.
func primaryPostingAmount(tx *floatv1.Transaction) string {
	if tx == nil {
		return ""
	}
	for _, p := range tx.Postings {
		if len(p.Amounts) == 0 {
			continue
		}
		return formatBalance(p.Amounts)
	}
	return ""
}
