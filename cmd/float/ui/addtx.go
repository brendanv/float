package ui

import (
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

const maxSuggestions = 6

// postingField holds the inputs for a single posting row.
type postingField struct {
	account textinput.Model
	amount  textinput.Model
}

func newPostingField() postingField {
	acc := textinput.New()
	acc.Placeholder = "account"
	amt := textinput.New()
	amt.Placeholder = "amount (blank = auto)"
	return postingField{account: acc, amount: amt}
}

// AddTxForm is the add-transaction overlay form.
type AddTxForm struct {
	active bool
	width  int
	height int
	client floatv1connect.LedgerServiceClient

	// Computed column widths (set in SetSize / rebuildWidths)
	headerInputW int
	accColW      int
	amtColW      int

	// Header inputs (focused field 0 = date, 1 = desc)
	dateInput textinput.Model
	descInput textinput.Model

	// Postings (focused field 2+2*r for account, 2+2*r+1 for amount)
	postings []postingField
	focused  int // flat field index

	// Autocomplete state (only meaningful when an account field is focused)
	allAccounts   []string
	suggestions   []string
	activeSuggIdx int // -1 = none highlighted

	submitting bool
	errMsg     string
}

func NewAddTxForm(client floatv1connect.LedgerServiceClient) AddTxForm {
	date := textinput.New()
	date.Placeholder = "YYYY-MM-DD (blank = today)"

	desc := textinput.New()
	desc.Placeholder = "description"

	f := AddTxForm{
		client:        client,
		dateInput:     date,
		descInput:     desc,
		postings:      []postingField{newPostingField(), newPostingField()},
		focused:       0,
		activeSuggIdx: -1,
	}
	return f
}

func (f *AddTxForm) SetSize(w, h int) {
	f.width = w
	f.height = h
	f.rebuildWidths()
}

func (f *AddTxForm) rebuildWidths() {
	w := f.width
	if w < 20 {
		w = 20
	}
	const labelW = 13 // len("Description: ")
	f.headerInputW = w - labelW - 2
	if f.headerInputW < 10 {
		f.headerInputW = 10
	}
	f.accColW = (w - 4) * 6 / 10
	f.amtColW = w - f.accColW - 4
	if f.amtColW < 10 {
		f.amtColW = 10
	}
	// Apply widths to all inputs
	f.dateInput.SetWidth(f.headerInputW)
	f.descInput.SetWidth(f.headerInputW)
	for i := range f.postings {
		f.postings[i].account.SetWidth(f.accColW)
		f.postings[i].amount.SetWidth(f.amtColW)
	}
}

func (f *AddTxForm) SetAccounts(accounts []*floatv1.Account) {
	f.allAccounts = make([]string, 0, len(accounts))
	for _, a := range accounts {
		f.allAccounts = append(f.allAccounts, a.FullName)
	}
	sort.Strings(f.allAccounts)
}

func (f *AddTxForm) Activate() {
	f.active = true
	f.errMsg = ""
	f.submitting = false
	f.focused = 0
	// Reset all inputs
	today := time.Now().Format("2006-01-02")
	f.dateInput.Reset()
	f.dateInput.SetValue(today)
	f.dateInput.SetWidth(f.headerInputW)
	f.descInput.Reset()
	f.descInput.SetWidth(f.headerInputW)
	p1 := newPostingField()
	p1.account.SetWidth(f.accColW)
	p1.amount.SetWidth(f.amtColW)
	p2 := newPostingField()
	p2.account.SetWidth(f.accColW)
	p2.amount.SetWidth(f.amtColW)
	f.postings = []postingField{p1, p2}
	f.suggestions = nil
	f.activeSuggIdx = -1
	f.focusField(0)
}

func (f *AddTxForm) Deactivate() {
	f.active = false
	f.blurAll()
}

func (f AddTxForm) Active() bool {
	return f.active
}

func (f *AddTxForm) totalFields() int {
	return 2 + len(f.postings)*2
}

// isAccountField returns true if the flat index corresponds to an account input.
func isAccountField(idx int) bool {
	if idx < 2 {
		return false
	}
	return (idx-2)%2 == 0
}

// postingRowForField returns the posting row index for a flat field index >= 2.
func postingRowForField(idx int) int {
	return (idx - 2) / 2
}

func (f *AddTxForm) blurAll() {
	f.dateInput.Blur()
	f.descInput.Blur()
	for i := range f.postings {
		f.postings[i].account.Blur()
		f.postings[i].amount.Blur()
	}
}

func (f *AddTxForm) focusField(idx int) {
	f.blurAll()
	f.focused = idx
	switch idx {
	case 0:
		f.dateInput.Focus()
	case 1:
		f.descInput.Focus()
	default:
		row := postingRowForField(idx)
		if row >= len(f.postings) {
			return
		}
		if (idx-2)%2 == 0 {
			f.postings[row].account.Focus()
			f.updateSuggestions()
		} else {
			f.postings[row].amount.Focus()
			f.suggestions = nil
			f.activeSuggIdx = -1
		}
	}
}

func (f *AddTxForm) updateSuggestions() {
	if !isAccountField(f.focused) {
		f.suggestions = nil
		f.activeSuggIdx = -1
		return
	}
	row := postingRowForField(f.focused)
	query := strings.ToLower(f.postings[row].account.Value())
	if query == "" {
		f.suggestions = nil
		f.activeSuggIdx = -1
		return
	}
	var results []string
	for _, a := range f.allAccounts {
		if strings.Contains(strings.ToLower(a), query) {
			results = append(results, a)
			if len(results) >= maxSuggestions {
				break
			}
		}
	}
	f.suggestions = results
	if f.activeSuggIdx >= len(f.suggestions) {
		f.activeSuggIdx = -1
	}
}

func (f *AddTxForm) advance() {
	f.suggestions = nil
	f.activeSuggIdx = -1
	next := f.focused + 1
	if next >= f.totalFields() {
		next = f.totalFields() - 1
	}
	f.focusField(next)
}

func (f *AddTxForm) retreat() {
	f.suggestions = nil
	f.activeSuggIdx = -1
	prev := f.focused - 1
	if prev < 0 {
		prev = 0
	}
	f.focusField(prev)
}

func (f *AddTxForm) addPosting() {
	p := newPostingField()
	p.account.SetWidth(f.accColW)
	p.amount.SetWidth(f.amtColW)
	f.postings = append(f.postings, p)
}

func (f *AddTxForm) deleteCurrentPosting() {
	if len(f.postings) <= 1 {
		return
	}
	if !isAccountField(f.focused) && f.focused < 2 {
		return
	}
	row := postingRowForField(f.focused)
	if row < 0 || row >= len(f.postings) {
		return
	}
	f.postings = append(f.postings[:row], f.postings[row+1:]...)
	// Reposition focus
	newFocused := f.focused
	if newFocused >= f.totalFields() {
		newFocused = f.totalFields() - 1
	}
	f.focusField(newFocused)
}

func (f *AddTxForm) buildRequest() (*floatv1.AddTransactionRequest, string) {
	desc := strings.TrimSpace(f.descInput.Value())
	if desc == "" {
		return nil, "description is required"
	}
	var postings []*floatv1.PostingInput
	for _, p := range f.postings {
		acc := strings.TrimSpace(p.account.Value())
		amt := strings.TrimSpace(p.amount.Value())
		if acc == "" && amt == "" {
			continue
		}
		postings = append(postings, &floatv1.PostingInput{
			Account: acc,
			Amount:  amt,
		})
	}
	if len(postings) == 0 {
		return nil, "at least one posting is required"
	}
	return &floatv1.AddTransactionRequest{
		Description: desc,
		Date:        strings.TrimSpace(f.dateInput.Value()),
		Postings:    postings,
	}, ""
}

func (f AddTxForm) Update(msg tea.Msg) (AddTxForm, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()

		// Global form keys
		switch key {
		case "esc":
			if len(f.suggestions) > 0 {
				// First esc closes the suggestions dropdown
				f.suggestions = nil
				f.activeSuggIdx = -1
				return f, nil
			}
			f.Deactivate()
			return f, nil

		case "shift+enter":
			req, errMsg := f.buildRequest()
			if errMsg != "" {
				f.errMsg = errMsg
				return f, nil
			}
			f.submitting = true
			f.errMsg = ""
			return f, AddTransactionCmd(f.client, req)

		case "ctrl+a":
			f.addPosting()
			// Focus the new posting's account field
			f.focusField(f.totalFields() - 2)
			return f, nil

		case "ctrl+d":
			f.deleteCurrentPosting()
			return f, nil

		case "shift+tab":
			f.retreat()
			return f, nil

		case "tab":
			f.advance()
			return f, nil

		case "up":
			if isAccountField(f.focused) && len(f.suggestions) > 0 {
				if f.activeSuggIdx > 0 {
					f.activeSuggIdx--
				} else {
					f.activeSuggIdx = len(f.suggestions) - 1
				}
				return f, nil
			}
			// Navigate to previous field like shift+tab
			f.retreat()
			return f, nil

		case "down":
			if isAccountField(f.focused) && len(f.suggestions) > 0 {
				if f.activeSuggIdx < len(f.suggestions)-1 {
					f.activeSuggIdx++
				} else {
					f.activeSuggIdx = 0
				}
				return f, nil
			}
			// Navigate to next field like tab
			f.advance()
			return f, nil

		case "enter":
			if isAccountField(f.focused) && f.activeSuggIdx >= 0 && f.activeSuggIdx < len(f.suggestions) {
				// Select the highlighted suggestion
				row := postingRowForField(f.focused)
				f.postings[row].account.SetValue(f.suggestions[f.activeSuggIdx])
				f.suggestions = nil
				f.activeSuggIdx = -1
				// Advance to amount field
				f.advance()
				return f, nil
			}
			// Confirm typed text and advance
			f.advance()
			return f, nil
		}

		// Route key to focused input
		var cmd tea.Cmd
		switch f.focused {
		case 0:
			f.dateInput, cmd = f.dateInput.Update(msg)
		case 1:
			f.descInput, cmd = f.descInput.Update(msg)
		default:
			row := postingRowForField(f.focused)
			if row >= len(f.postings) {
				return f, nil
			}
			if (f.focused-2)%2 == 0 {
				f.postings[row].account, cmd = f.postings[row].account.Update(msg)
				f.updateSuggestions()
			} else {
				f.postings[row].amount, cmd = f.postings[row].amount.Update(msg)
			}
		}
		return f, cmd
	}
	return f, nil
}

func (f AddTxForm) View() string {
	if !f.active {
		return ""
	}

	w := f.width
	if w < 20 {
		w = 20
	}

	var lines []string

	// Title
	title := lipgloss.NewStyle().Bold(true).Render("Add Transaction")
	lines = append(lines, title)

	// Separator
	sep := strings.Repeat("─", w)
	lines = append(lines, HelpStyle.Render(sep))
	lines = append(lines, "")

	// Date field
	lines = append(lines, "Date:        "+f.dateInput.View())

	// Description field
	lines = append(lines, "Description: "+f.descInput.View())
	lines = append(lines, "")

	// Postings header
	lines = append(lines, lipgloss.NewStyle().Bold(true).Render("Postings"))

	// Column header
	colHeader := HelpStyle.Render(
		padRight("  Account", f.accColW+2) + "  Amount",
	)
	lines = append(lines, colHeader)

	// Posting rows with optional suggestion dropdown
	for i, p := range f.postings {
		accountFocusIdx := 2 + i*2

		// Build the posting row line
		postingLine := "  " + p.account.View() + "  " + p.amount.View()
		lines = append(lines, postingLine)

		// Show autocomplete dropdown below the account input if this row is focused
		if f.focused == accountFocusIdx && len(f.suggestions) > 0 {
			dropdownLines := renderDropdown(f.suggestions, f.activeSuggIdx, f.accColW+2)
			lines = append(lines, dropdownLines...)
		}
	}

	lines = append(lines, "")

	// Help hint
	hint := HelpStyle.Render("  ctrl+a add posting  ctrl+d del posting  shift+enter submit  esc cancel")
	lines = append(lines, hint)

	// Error message
	if f.errMsg != "" {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555"))
		lines = append(lines, errStyle.Render("  Error: "+f.errMsg))
	}

	// Submitting indicator
	if f.submitting {
		lines = append(lines, HelpStyle.Render("  Submitting..."))
	}

	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		Width(w).
		Height(f.height).
		Render(content)
}

// renderDropdown renders a suggestion list as a small bordered box.
func renderDropdown(suggestions []string, activeIdx int, maxW int) []string {
	borderW := maxW
	if borderW > 50 {
		borderW = 50
	}
	innerW := borderW - 2
	if innerW < 5 {
		innerW = 5
	}

	top := "  ┌" + strings.Repeat("─", innerW) + "┐"
	bottom := "  └" + strings.Repeat("─", innerW) + "┘"

	lines := []string{top}
	for i, s := range suggestions {
		truncated := truncateString(s, innerW)
		padded := padRight(truncated, innerW)
		var row string
		if i == activeIdx {
			row = "  │" + lipgloss.NewStyle().Foreground(colorFocused).Render(padded) + "│"
		} else {
			row = "  │" + padded + "│"
		}
		lines = append(lines, row)
	}
	lines = append(lines, bottom)
	return lines
}

func padRight(s string, n int) string {
	l := utf8.RuneCountInString(s)
	if l >= n {
		return s
	}
	return s + strings.Repeat(" ", n-l)
}

func truncateString(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n > 1 {
		return string(runes[:n-1]) + "…"
	}
	return string(runes[:n])
}
