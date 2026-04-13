package ui

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

type rulesMode int

const (
	rulesModeList    rulesMode = iota // browsing rules table (default)
	rulesModeForm                     // add / edit form active (right panel)
	rulesModePreview                  // full-screen apply-preview
)

// RulesTab is the fourth TUI tab. It mirrors the web UI's rules management
// page: list rules, add/edit/delete rules, test a pattern, and
// preview/apply rules to existing transactions.
type RulesTab struct {
	width  int
	height int
	styles Styles
	client floatv1connect.LedgerServiceClient

	// Layout
	leftWidth   int
	leftInnerW  int
	leftInnerH  int
	rightWidth  int
	rightInnerW int
	rightInnerH int

	// Core state
	mode      rulesMode
	loadState loadState
	spinner   Spinner
	errMsg    string
	rules     []*floatv1.TransactionRule
	accounts  []string // flat list of account names for autocomplete

	// Rules list table
	rulesTable table.Model

	// Delete confirmation
	confirmDeleteID string

	// ─── Form state (add / edit) ───────────────────────────────────────
	editingID      string // non-empty = edit mode
	formField      int    // 0=pattern 1=priority 2=payee 3=account 4=tags
	patternInput   textinput.Model
	priorityInput  textinput.Model
	payeeInput     textinput.Model
	accountInput   textinput.Model
	tagsInput      textinput.Model
	formErr        string
	formSubmitting bool

	// ─── Pattern tester (right panel in list mode) ─────────────────────
	testInput   textinput.Model
	testFocused bool    // true = test input has keyboard focus
	testMatch   string  // display name of the first matching rule, "" if none

	// ─── Preview / apply state ─────────────────────────────────────────
	previews        []*floatv1.RuleApplicationPreview
	selectedFIDs    map[string]bool
	previewCursor   int
	previewErr      string
	previewLoading  bool
	applyResult     string
	previewTable    table.Model
}

// ─── Constructor ────────────────────────────────────────────────────────────

func NewRulesTab(client floatv1connect.LedgerServiceClient, st Styles) RulesTab {
	patternIn := textinput.New()
	patternIn.Placeholder = "regex pattern (required)"

	priorityIn := textinput.New()
	priorityIn.Placeholder = "priority (default 0, lower = higher priority)"

	payeeIn := textinput.New()
	payeeIn.Placeholder = "set payee (optional)"

	accountIn := textinput.New()
	accountIn.Placeholder = "set account (optional)"

	tagsIn := textinput.New()
	tagsIn.Placeholder = "tags: key=val key2=val2 (optional)"

	testIn := textinput.New()
	testIn.Placeholder = "type a description to test rules…"

	return RulesTab{
		styles:       st,
		client:       client,
		loadState:    stateLoading,
		spinner:      NewSpinner(),
		rulesTable:   newRulesTable(st),
		previewTable: newPreviewTable(st),
		patternInput: patternIn,
		priorityInput: priorityIn,
		payeeInput:   payeeIn,
		accountInput: accountIn,
		tagsInput:    tagsIn,
		testInput:    testIn,
		selectedFIDs: make(map[string]bool),
	}
}

func (m RulesTab) setStyles(st Styles) RulesTab {
	m.styles = st
	return m
}

func newRulesTable(st Styles) table.Model {
	return table.New(
		table.WithColumns([]table.Column{
			{Title: "Pri", Width: 3},
			{Title: "Pattern", Width: 30},
			{Title: "Payee", Width: 15},
			{Title: "Account", Width: 20},
			{Title: "Tags", Width: 15},
		}),
		table.WithStyles(styledTableStyles(st)),
		table.WithFocused(true),
	)
}

func newPreviewTable(st Styles) table.Model {
	return table.New(
		table.WithColumns([]table.Column{
			{Title: "Sel", Width: 3},
			{Title: "Description", Width: 30},
			{Title: "Current Account", Width: 20},
			{Title: "New Account", Width: 20},
			{Title: "New Payee", Width: 15},
		}),
		table.WithStyles(styledTableStyles(st)),
		table.WithFocused(true),
	)
}

// ─── SetSize ────────────────────────────────────────────────────────────────

func (m RulesTab) SetSize(w, h int) RulesTab {
	m.width = w
	m.height = h

	// 55% left / 45% right.
	m.leftWidth = w * 55 / 100
	if m.leftWidth < 20 {
		m.leftWidth = 20
	}
	m.rightWidth = w - m.leftWidth
	if m.rightWidth < 0 {
		m.rightWidth = 0
	}

	m.leftInnerW, m.leftInnerH = innerSize(m.leftWidth, h, m.styles.Border)
	m.rightInnerW, m.rightInnerH = innerSize(m.rightWidth, h, m.styles.Border)

	m.rulesTable.SetWidth(m.leftInnerW)
	m.rulesTable.SetHeight(m.leftInnerH)
	m.rebuildRulesColumns()

	m.previewTable.SetWidth(w - 2) // full-width minus border
	m.previewTable.SetHeight(h - 6)
	m.rebuildPreviewColumns(w - 2)

	// Resize text inputs.
	fw := m.rightInnerW
	if fw < 10 {
		fw = 10
	}
	m.patternInput.SetWidth(fw)
	m.priorityInput.SetWidth(fw)
	m.payeeInput.SetWidth(fw)
	m.accountInput.SetWidth(fw)
	m.tagsInput.SetWidth(fw)
	m.testInput.SetWidth(m.rightInnerW)

	return m
}

func (m *RulesTab) rebuildRulesColumns() {
	w := m.leftInnerW
	fixed := 3 + 15 + 20 + 15 + 4 // pri + payee + account + tags + separators
	patW := w - fixed
	if patW < 10 {
		patW = 10
	}
	m.rulesTable.SetColumns([]table.Column{
		{Title: "Pri", Width: 3},
		{Title: "Pattern", Width: patW},
		{Title: "Payee", Width: 15},
		{Title: "Account", Width: 20},
		{Title: "Tags", Width: 15},
	})
}

func (m *RulesTab) rebuildPreviewColumns(w int) {
	fixed := 3 + 20 + 20 + 15 + 4 // sel + currAcct + newAcct + newPayee + sep
	descW := w - fixed
	if descW < 10 {
		descW = 10
	}
	m.previewTable.SetColumns([]table.Column{
		{Title: "Sel", Width: 3},
		{Title: "Description", Width: descW},
		{Title: "Current Account", Width: 20},
		{Title: "New Account", Width: 20},
		{Title: "New Payee", Width: 15},
	})
}

// ─── Init ────────────────────────────────────────────────────────────────────

func (m RulesTab) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick(),
		FetchRules(m.client),
		FetchAccounts(m.client),
	)
}

// ─── Update ──────────────────────────────────────────────────────────────────

func (m RulesTab) Update(msg tea.Msg) (RulesTab, tea.Cmd) {
	switch msg := msg.(type) {
	case RulesMsg:
		if msg.Err != nil {
			m.errMsg = msg.Err.Error()
			m.loadState = stateError
		} else {
			m.rules = msg.Rules
			m.loadState = stateLoaded
			m.rebuildRulesRows()
		}
		return m, nil

	case AccountsMsg:
		if msg.Err == nil {
			m.accounts = make([]string, len(msg.Accounts))
			for i, a := range msg.Accounts {
				m.accounts[i] = a.Name
			}
		}
		return m, nil

	case AddRuleMsg:
		m.formSubmitting = false
		if msg.Err != nil {
			m.formErr = msg.Err.Error()
			return m, nil
		}
		m.exitForm()
		m.loadState = stateLoading
		return m, FetchRules(m.client)

	case UpdateRuleMsg:
		m.formSubmitting = false
		if msg.Err != nil {
			m.formErr = msg.Err.Error()
			return m, nil
		}
		m.exitForm()
		m.loadState = stateLoading
		return m, FetchRules(m.client)

	case DeleteRuleMsg:
		m.confirmDeleteID = ""
		if msg.Err != nil {
			m.errMsg = msg.Err.Error()
			return m, nil
		}
		m.loadState = stateLoading
		return m, FetchRules(m.client)

	case PreviewApplyRulesMsg:
		m.previewLoading = false
		if msg.Err != nil {
			m.previewErr = msg.Err.Error()
			return m, nil
		}
		m.previews = msg.Previews
		m.selectedFIDs = make(map[string]bool)
		for _, p := range m.previews {
			m.selectedFIDs[p.Fid] = true // select all by default
		}
		m.previewCursor = 0
		m.applyResult = ""
		m.rebuildPreviewRows()
		return m, nil

	case ApplyRulesMsg:
		m.previewLoading = false
		if msg.Err != nil {
			m.previewErr = msg.Err.Error()
			return m, nil
		}
		m.applyResult = fmt.Sprintf("Applied rules to %d transaction(s).", msg.AppliedCount)
		m.previews = nil
		m.selectedFIDs = make(map[string]bool)
		m.mode = rulesModeList
		m.loadState = stateLoading
		return m, FetchRules(m.client)

	case tea.KeyMsg:
		return m.handleKey(msg)

	default:
		// Spinner ticks.
		if cmd := m.handleSpinnerTick(msg); cmd != nil {
			return m, cmd
		}
		return m, nil
	}
}

func (m *RulesTab) handleSpinnerTick(msg tea.Msg) tea.Cmd {
	if m.loadState == stateLoading {
		if sm, ok := msg.(interface{ String() string }); ok {
			_ = sm
		}
		return m.spinner.Update(msg)
	}
	return nil
}

func (m RulesTab) handleKey(msg tea.KeyMsg) (RulesTab, tea.Cmd) {
	// ── Preview mode ────────────────────────────────────────────────────
	if m.mode == rulesModePreview {
		if m.previewLoading {
			return m, nil
		}
		switch msg.String() {
		case "esc":
			m.mode = rulesModeList
			m.previews = nil
			m.previewErr = ""
			m.applyResult = ""
			return m, nil
		case "j", "down":
			if m.previewCursor < len(m.previews)-1 {
				m.previewCursor++
				m.previewTable.SetCursor(m.previewCursor)
			}
		case "k", "up":
			if m.previewCursor > 0 {
				m.previewCursor--
				m.previewTable.SetCursor(m.previewCursor)
			}
		case " ":
			if m.previewCursor < len(m.previews) {
				fid := m.previews[m.previewCursor].Fid
				m.selectedFIDs[fid] = !m.selectedFIDs[fid]
				m.rebuildPreviewRows()
			}
		case "ctrl+a":
			// Toggle: if any are selected, deselect all; else select all.
			anySelected := false
			for _, v := range m.selectedFIDs {
				if v {
					anySelected = true
					break
				}
			}
			for k := range m.selectedFIDs {
				m.selectedFIDs[k] = !anySelected
			}
			m.rebuildPreviewRows()
		case "enter":
			fids := m.selectedFIDList()
			if len(fids) == 0 {
				return m, nil
			}
			m.previewLoading = true
			m.previewErr = ""
			return m, ApplyRulesCmd(m.client, fids)
		}
		return m, nil
	}

	// ── Form mode ───────────────────────────────────────────────────────
	if m.mode == rulesModeForm {
		if m.formSubmitting {
			return m, nil
		}
		switch msg.String() {
		case "esc":
			m.exitForm()
			return m, nil
		case "shift+enter":
			return m.submitForm()
		case "tab", "enter":
			m.formField = (m.formField + 1) % 5
			m.focusFormField()
			return m, nil
		case "shift+tab":
			m.formField = (m.formField + 4) % 5
			m.focusFormField()
			return m, nil
		}
		// Forward to focused field.
		return m.updateFormField(msg)
	}

	// ── List mode ───────────────────────────────────────────────────────

	// Delete confirmation overlay.
	if m.confirmDeleteID != "" {
		switch msg.String() {
		case "y":
			id := m.confirmDeleteID
			m.confirmDeleteID = ""
			return m, DeleteRuleCmd(m.client, id)
		case "esc", "n":
			m.confirmDeleteID = ""
		}
		return m, nil
	}

	// Test input focused.
	if m.testFocused {
		switch msg.String() {
		case "esc":
			m.testFocused = false
			m.testInput.Blur()
			m.rulesTable.Focus()
			return m, nil
		}
		var cmd tea.Cmd
		m.testInput, cmd = m.testInput.Update(msg)
		m.updateTestMatch()
		return m, cmd
	}

	switch msg.String() {
	case "r":
		m.loadState = stateLoading
		m.applyResult = ""
		return m, FetchRules(m.client)
	case "j", "down":
		m.rulesTable.MoveDown(1)
	case "k", "up":
		m.rulesTable.MoveUp(1)
	case "a":
		m.startAdd()
		return m, nil
	case "e":
		if r := m.selectedRule(); r != nil {
			m.startEdit(r)
		}
		return m, nil
	case "d":
		if r := m.selectedRule(); r != nil {
			m.confirmDeleteID = r.Id
		}
		return m, nil
	case "p":
		if len(m.rules) == 0 {
			return m, nil
		}
		m.mode = rulesModePreview
		m.previews = nil
		m.previewErr = ""
		m.applyResult = ""
		m.previewLoading = true
		return m, PreviewApplyRulesCmd(m.client)
	case "t":
		m.testFocused = true
		m.rulesTable.Blur()
		m.testInput.Focus()
		return m, nil
	}
	return m, nil
}

// ─── Form helpers ────────────────────────────────────────────────────────────

func (m *RulesTab) startAdd() {
	m.editingID = ""
	m.formField = 0
	m.formErr = ""
	m.patternInput.SetValue("")
	m.priorityInput.SetValue("0")
	m.payeeInput.SetValue("")
	m.accountInput.SetValue("")
	m.tagsInput.SetValue("")
	m.mode = rulesModeForm
	m.focusFormField()
}

func (m *RulesTab) startEdit(r *floatv1.TransactionRule) {
	m.editingID = r.Id
	m.formField = 0
	m.formErr = ""
	m.patternInput.SetValue(r.Pattern)
	m.priorityInput.SetValue(strconv.Itoa(int(r.Priority)))
	m.payeeInput.SetValue(r.Payee)
	m.accountInput.SetValue(r.Account)
	m.tagsInput.SetValue(tagsToString(r.Tags))
	m.mode = rulesModeForm
	m.focusFormField()
}

func (m *RulesTab) exitForm() {
	m.mode = rulesModeList
	m.editingID = ""
	m.formErr = ""
	m.formSubmitting = false
	m.blurFormFields()
	m.rulesTable.Focus()
}

func (m *RulesTab) focusFormField() {
	m.blurFormFields()
	switch m.formField {
	case 0:
		m.patternInput.Focus()
	case 1:
		m.priorityInput.Focus()
	case 2:
		m.payeeInput.Focus()
	case 3:
		m.accountInput.Focus()
	case 4:
		m.tagsInput.Focus()
	}
}

func (m *RulesTab) blurFormFields() {
	m.patternInput.Blur()
	m.priorityInput.Blur()
	m.payeeInput.Blur()
	m.accountInput.Blur()
	m.tagsInput.Blur()
}

func (m RulesTab) updateFormField(msg tea.KeyMsg) (RulesTab, tea.Cmd) {
	var cmd tea.Cmd
	switch m.formField {
	case 0:
		m.patternInput, cmd = m.patternInput.Update(msg)
	case 1:
		m.priorityInput, cmd = m.priorityInput.Update(msg)
	case 2:
		m.payeeInput, cmd = m.payeeInput.Update(msg)
	case 3:
		m.accountInput, cmd = m.accountInput.Update(msg)
	case 4:
		m.tagsInput, cmd = m.tagsInput.Update(msg)
	}
	return m, cmd
}

func (m RulesTab) submitForm() (RulesTab, tea.Cmd) {
	pattern := strings.TrimSpace(m.patternInput.Value())
	if pattern == "" {
		m.formErr = "Pattern is required."
		return m, nil
	}
	if _, err := regexp.Compile("(?i)" + pattern); err != nil {
		m.formErr = "Invalid regex: " + err.Error()
		return m, nil
	}
	priority := int32(0)
	if v := strings.TrimSpace(m.priorityInput.Value()); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			m.formErr = "Priority must be a number."
			return m, nil
		}
		priority = int32(n)
	}
	tags := parseTags(m.tagsInput.Value())
	m.formSubmitting = true
	m.formErr = ""

	if m.editingID == "" {
		req := &floatv1.AddRuleRequest{
			Pattern:  pattern,
			Payee:    strings.TrimSpace(m.payeeInput.Value()),
			Account:  strings.TrimSpace(m.accountInput.Value()),
			Tags:     tags,
			Priority: priority,
		}
		return m, AddRuleCmd(m.client, req)
	}
	req := &floatv1.UpdateRuleRequest{
		Id:       m.editingID,
		Pattern:  pattern,
		Payee:    strings.TrimSpace(m.payeeInput.Value()),
		Account:  strings.TrimSpace(m.accountInput.Value()),
		Tags:     tags,
		Priority: priority,
	}
	return m, UpdateRuleCmd(m.client, req)
}

// ─── Table helpers ───────────────────────────────────────────────────────────

func (m *RulesTab) rebuildRulesRows() {
	rows := make([]table.Row, 0, len(m.rules))
	sorted := make([]*floatv1.TransactionRule, len(m.rules))
	copy(sorted, m.rules)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Priority != sorted[j].Priority {
			return sorted[i].Priority < sorted[j].Priority
		}
		return sorted[i].Id < sorted[j].Id
	})
	for _, r := range sorted {
		rows = append(rows, table.Row{
			strconv.Itoa(int(r.Priority)),
			r.Pattern,
			r.Payee,
			r.Account,
			tagsToString(r.Tags),
		})
	}
	m.rulesTable.SetRows(rows)
}

func (m *RulesTab) rebuildPreviewRows() {
	rows := make([]table.Row, 0, len(m.previews))
	for _, p := range m.previews {
		sel := "[ ]"
		if m.selectedFIDs[p.Fid] {
			sel = "[x]"
		}
		rows = append(rows, table.Row{
			sel,
			p.Description,
			truncate(p.CurrentAccount, 20),
			truncate(p.NewAccount, 20),
			truncate(p.NewPayee, 15),
		})
	}
	m.previewTable.SetRows(rows)
}

func (m *RulesTab) selectedRule() *floatv1.TransactionRule {
	if len(m.rules) == 0 {
		return nil
	}
	idx := m.rulesTable.Cursor()
	// Rules are displayed sorted; we need the same sort as rebuildRulesRows.
	sorted := make([]*floatv1.TransactionRule, len(m.rules))
	copy(sorted, m.rules)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Priority != sorted[j].Priority {
			return sorted[i].Priority < sorted[j].Priority
		}
		return sorted[i].Id < sorted[j].Id
	})
	if idx < 0 || idx >= len(sorted) {
		return nil
	}
	return sorted[idx]
}

func (m *RulesTab) selectedFIDList() []string {
	out := make([]string, 0, len(m.selectedFIDs))
	for fid, sel := range m.selectedFIDs {
		if sel {
			out = append(out, fid)
		}
	}
	sort.Strings(out)
	return out
}

// ─── Pattern tester ───────────────────────────────────────────────────────────

func (m *RulesTab) updateTestMatch() {
	desc := m.testInput.Value()
	if desc == "" {
		m.testMatch = ""
		return
	}
	for _, r := range m.rules {
		re, err := regexp.Compile("(?i)" + r.Pattern)
		if err != nil {
			continue
		}
		if re.MatchString(desc) {
			m.testMatch = fmt.Sprintf("Matched rule: %s (pri %d)", r.Pattern, r.Priority)
			return
		}
	}
	m.testMatch = "No rule matched."
}

// ─── KeyMap ──────────────────────────────────────────────────────────────────

func (m RulesTab) KeyMap() help.KeyMap {
	switch m.mode {
	case rulesModeForm:
		return RulesFormKeyMap{}
	case rulesModePreview:
		return RulesPreviewKeyMap{}
	default:
		if m.confirmDeleteID != "" {
			return DeleteConfirmKeyMap{}
		}
		return RulesListKeyMap{}
	}
}

// ─── View ────────────────────────────────────────────────────────────────────

func (m RulesTab) View() string {
	// Full-screen preview mode.
	if m.mode == rulesModePreview {
		return m.viewPreview()
	}

	// 2-column layout: rules list (left) + form/tester (right).
	leftContent := m.viewLeft()
	rightContent := m.viewRight()

	var rightTitle string
	switch m.mode {
	case rulesModeForm:
		if m.editingID != "" {
			rightTitle = "Edit Rule"
		} else {
			rightTitle = "Add Rule"
		}
	default:
		rightTitle = "Pattern Tester"
	}
	leftPanel := renderCard(leftContent, "Rules", false, m.leftWidth, m.height, m.styles)
	rightPanel := renderCard(rightContent, rightTitle, false, m.rightWidth, m.height, m.styles)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
}

// viewLeft renders the rules list table (left column).
func (m RulesTab) viewLeft() string {
	if m.loadState == stateLoading {
		return lipgloss.NewStyle().
			Width(m.leftInnerW).Height(m.leftInnerH).
			Align(lipgloss.Center, lipgloss.Center).
			Render(m.spinner.View())
	}
	if m.loadState == stateError {
		return lipgloss.NewStyle().
			Width(m.leftInnerW).Height(m.leftInnerH).
			Align(lipgloss.Center, lipgloss.Center).
			Render("! " + m.errMsg + "\n\nPress r to retry")
	}

	content := m.rulesTable.View()

	// Apply-result banner
	if m.applyResult != "" {
		banner := lipgloss.NewStyle().
			Foreground(m.styles.FocusedFg).
			Width(m.leftInnerW).
			Render(m.applyResult)
		content = lipgloss.JoinVertical(lipgloss.Left, banner, content)
	}

	// Delete confirmation overlay
	if m.confirmDeleteID != "" {
		overlay := lipgloss.NewStyle().
			Width(m.leftInnerW).Height(m.leftInnerH).
			Align(lipgloss.Center, lipgloss.Center).
			Render("Delete this rule?\n\n[y] confirm  [esc] cancel")
		return overlay
	}

	if len(m.rules) == 0 {
		empty := m.styles.Help.
			Width(m.leftInnerW).Height(m.leftInnerH).
			Align(lipgloss.Center, lipgloss.Center).
			Render("No rules yet.\nPress 'a' to add one.")
		return empty
	}

	return lipgloss.NewStyle().
		Width(m.leftInnerW).Height(m.leftInnerH).
		Render(content)
}

// viewRight renders either the add/edit form or the pattern tester.
func (m RulesTab) viewRight() string {
	if m.mode == rulesModeForm {
		return m.viewForm()
	}
	return m.viewTester()
}

// viewForm renders the add/edit rule form.
func (m RulesTab) viewForm() string {
	lines := []string{
		m.fieldLabel("Pattern", 0) + m.patternInput.View(),
		m.fieldLabel("Priority", 1) + m.priorityInput.View(),
		m.fieldLabel("Payee", 2) + m.payeeInput.View(),
		m.fieldLabel("Account", 3) + m.accountInput.View(),
		m.fieldLabel("Tags", 4) + m.tagsInput.View(),
		"",
	}

	if m.formSubmitting {
		lines = append(lines, m.styles.Help.Render("Saving…"))
	} else if m.formErr != "" {
		lines = append(lines, m.styles.Error.Render("! "+m.formErr))
	} else {
		lines = append(lines, m.styles.Help.Render("shift+enter to save  esc to cancel"))
	}

	return lipgloss.NewStyle().
		Width(m.rightInnerW).Height(m.rightInnerH).
		Render(strings.Join(lines, "\n"))
}

func (m RulesTab) fieldLabel(name string, idx int) string {
	style := m.styles.Help
	if m.formField == idx {
		style = m.styles.Active
	}
	return style.Render(fmt.Sprintf("%-9s ", name))
}

// viewTester renders the pattern tester panel (right column in list mode).
func (m RulesTab) viewTester() string {
	lines := []string{
		"Press 't' to focus, type a transaction description:",
		m.testInput.View(),
		"",
	}
	if m.testInput.Value() != "" {
		if m.testMatch != "" {
			lines = append(lines, m.styles.Active.Render(m.testMatch))
		} else {
			lines = append(lines, m.styles.Help.Render("No rule matched."))
		}
	}
	lines = append(lines, "")
	for _, b := range []key.Binding{keyAdd, keyEdit, keyDelete, keyPreview, keyTest, keyRetry} {
		h := b.Help()
		lines = append(lines, m.styles.Help.Render(h.Key+"  "+h.Desc))
	}

	return lipgloss.NewStyle().
		Width(m.rightInnerW).Height(m.rightInnerH).
		Render(strings.Join(lines, "\n"))
}

// viewPreview renders the full-screen preview/apply panel.
func (m RulesTab) viewPreview() string {
	w := m.width
	h := m.height

	if m.previewLoading {
		return lipgloss.NewStyle().
			Width(w).Height(h).
			Align(lipgloss.Center, lipgloss.Center).
			Render(m.spinner.View() + " Loading preview…")
	}

	title := lipgloss.NewStyle().Bold(true).Render("Preview: Apply Rules to Transactions")

	if m.previewErr != "" {
		errLine := m.styles.Error.Render("! " + m.previewErr)
		hint := m.styles.Help.Render("esc to go back")
		body := lipgloss.JoinVertical(lipgloss.Left, title, "", errLine, "", hint)
		return lipgloss.NewStyle().Width(w).Height(h).Render(body)
	}

	if len(m.previews) == 0 {
		empty := lipgloss.JoinVertical(lipgloss.Left,
			title, "",
			m.styles.Help.Render("No transactions would be affected by the current rules."),
			"",
			m.styles.Help.Render("esc to go back"),
		)
		return lipgloss.NewStyle().Width(w).Height(h).Render(empty)
	}

	nSelected := len(m.selectedFIDList())
	statusLine := m.styles.Help.Render(fmt.Sprintf(
		"%d of %d transaction(s) selected  •  space=toggle  ctrl+a=all/none  enter=apply  esc=back",
		nSelected, len(m.previews),
	))

	tableView := m.styles.FocusedBorder.
		Width(w).
		Render(m.previewTable.View())

	return lipgloss.JoinVertical(lipgloss.Left, title, statusLine, tableView)
}

// ─── Tag helpers ─────────────────────────────────────────────────────────────

// tagsToString formats a tag map as "key=val key2=val2".
func tagsToString(tags map[string]string) string {
	if len(tags) == 0 {
		return ""
	}
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+tags[k])
	}
	return strings.Join(parts, " ")
}

// parseTags parses "key=val key2=val2" into a map.
func parseTags(s string) map[string]string {
	m := make(map[string]string)
	for _, part := range strings.Fields(s) {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			m[kv[0]] = kv[1]
		}
	}
	return m
}

// truncate shortens s to at most n runes.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return string(runes[:n-1]) + "…"
}
