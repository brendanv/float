package ui

import (
	"fmt"
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

// tagField holds the inputs for a single tag row.
type tagField struct {
	key   textinput.Model
	value textinput.Model
}

func newTagField() tagField {
	k := textinput.New()
	k.Placeholder = "key"
	v := textinput.New()
	v.Placeholder = "value"
	return tagField{key: k, value: v}
}

// AddTxForm is the add/edit-transaction overlay form.
// When editFID is non-empty the form operates in edit mode:
//   - the title changes to "Edit Transaction"
//   - fields are pre-populated from the existing transaction
//   - submission calls UpdateTransaction instead of AddTransaction
type AddTxForm struct {
	active  bool
	editFID string // non-empty = edit mode
	width   int
	height  int
	styles  Styles
	client  floatv1connect.LedgerServiceClient

	// Computed column widths (set in SetSize / rebuildWidths)
	headerInputW int
	accColW      int
	amtColW      int

	// Header inputs
	// focused field 0 = date, 1 = desc
	dateInput textinput.Model
	descInput textinput.Model

	// Tags (focused field 2..2+2*len(tags)-1)
	tags []tagField

	// Postings (focused field postingBase()+2*r for account, postingBase()+2*r+1 for amount)
	postings []postingField
	focused  int // flat field index

	// Autocomplete state (only meaningful when an account field is focused)
	allAccounts   []string
	suggestions   []string
	activeSuggIdx int // -1 = none highlighted

	submitting bool
	errMsg     string
}

func NewAddTxForm(client floatv1connect.LedgerServiceClient, st Styles) AddTxForm {
	date := textinput.New()
	date.Placeholder = "YYYY-MM-DD (blank = today)"

	desc := textinput.New()
	desc.Placeholder = "description"

	f := AddTxForm{
		client:        client,
		styles:        st,
		dateInput:     date,
		descInput:     desc,
		postings:      []postingField{newPostingField(), newPostingField()},
		focused:       0,
		activeSuggIdx: -1,
	}
	return f
}

func (f *AddTxForm) setStyles(st Styles) {
	f.styles = st
}

func (f *AddTxForm) SetSize(w, h int) {
	f.width = w
	f.height = h
	f.rebuildWidths()
}

func (f *AddTxForm) rebuildWidths() {
	modalW := calcModalWidth(f.width)
	border := f.styles.FocusedBorder.Padding(modalVertPad, modalHorizPad)
	w := modalW - border.GetHorizontalFrameSize()
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
	for i := range f.tags {
		f.tags[i].key.SetWidth(f.accColW)
		f.tags[i].value.SetWidth(f.amtColW)
	}
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

// postingBase returns the flat field index of the first posting field.
func (f *AddTxForm) postingBase() int {
	return 2 + 2*len(f.tags)
}

// Activate opens the form in add mode with today's date pre-filled.
func (f *AddTxForm) Activate() {
	f.active = true
	f.editFID = ""
	f.errMsg = ""
	f.submitting = false
	f.focused = 0
	today := time.Now().Format("2006-01-02")
	f.dateInput.Reset()
	f.dateInput.SetValue(today)
	f.dateInput.SetWidth(f.headerInputW)
	f.descInput.Reset()
	f.descInput.SetWidth(f.headerInputW)
	f.tags = nil
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

// ActivateEdit opens the form in edit mode pre-populated from an existing transaction.
func (f *AddTxForm) ActivateEdit(tx *floatv1.Transaction) {
	if tx == nil || tx.Fid == "" {
		return
	}
	f.active = true
	f.editFID = tx.Fid
	f.errMsg = ""
	f.submitting = false
	f.focused = 0

	f.dateInput.Reset()
	f.dateInput.SetValue(tx.Date)
	f.dateInput.SetWidth(f.headerInputW)

	f.descInput.Reset()
	f.descInput.SetValue(tx.Description)
	f.descInput.SetWidth(f.headerInputW)

	// Populate tags from tx.Tags (sorted for stable display order).
	f.tags = nil
	keys := make([]string, 0, len(tx.Tags))
	for k := range tx.Tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		tf := newTagField()
		tf.key.SetWidth(f.accColW)
		tf.value.SetWidth(f.amtColW)
		tf.key.SetValue(k)
		tf.value.SetValue(tx.Tags[k])
		f.tags = append(f.tags, tf)
	}

	// Pre-populate postings.
	f.postings = nil
	for _, p := range tx.Postings {
		pf := newPostingField()
		pf.account.SetWidth(f.accColW)
		pf.amount.SetWidth(f.amtColW)
		pf.account.SetValue(p.Account)
		if len(p.Amounts) > 0 {
			a := p.Amounts[0]
			pf.amount.SetValue(fmt.Sprintf("%s%s", a.Commodity, a.Quantity))
		}
		f.postings = append(f.postings, pf)
	}
	for len(f.postings) < 2 {
		pf := newPostingField()
		pf.account.SetWidth(f.accColW)
		pf.amount.SetWidth(f.amtColW)
		f.postings = append(f.postings, pf)
	}

	f.suggestions = nil
	f.activeSuggIdx = -1
	f.focusField(0)
}

func (f *AddTxForm) Deactivate() {
	f.active = false
	f.editFID = ""
	f.blurAll()
}

func (f AddTxForm) Active() bool {
	return f.active
}

func (f AddTxForm) EditMode() bool {
	return f.editFID != ""
}

func (f *AddTxForm) totalFields() int {
	return 2 + len(f.tags)*2 + len(f.postings)*2
}

// isTagKeyField returns true if idx is a tag key field.
func isTagKeyField(idx, postingBase int) bool {
	return idx >= 2 && idx < postingBase && (idx-2)%2 == 0
}

// tagRowForField returns the tag row index for a flat field index in the tag range.
func tagRowForField(idx int) int {
	return (idx - 2) / 2
}

// isAccountField returns true if the flat index corresponds to a posting account input.
func isAccountField(idx, postingBase int) bool {
	if idx < postingBase {
		return false
	}
	return (idx-postingBase)%2 == 0
}

// postingRowForField returns the posting row index for a flat field index in the posting range.
func postingRowForField(idx, postingBase int) int {
	return (idx - postingBase) / 2
}

func (f *AddTxForm) blurAll() {
	f.dateInput.Blur()
	f.descInput.Blur()
	for i := range f.tags {
		f.tags[i].key.Blur()
		f.tags[i].value.Blur()
	}
	for i := range f.postings {
		f.postings[i].account.Blur()
		f.postings[i].amount.Blur()
	}
}

func (f *AddTxForm) focusField(idx int) {
	f.blurAll()
	f.focused = idx
	pb := f.postingBase()
	switch {
	case idx == 0:
		f.dateInput.Focus()
	case idx == 1:
		f.descInput.Focus()
	case idx >= 2 && idx < pb:
		row := tagRowForField(idx)
		if row >= len(f.tags) {
			return
		}
		if isTagKeyField(idx, pb) {
			f.tags[row].key.Focus()
		} else {
			f.tags[row].value.Focus()
		}
	default:
		row := postingRowForField(idx, pb)
		if row >= len(f.postings) {
			return
		}
		if isAccountField(idx, pb) {
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
	pb := f.postingBase()
	if !isAccountField(f.focused, pb) {
		f.suggestions = nil
		f.activeSuggIdx = -1
		return
	}
	row := postingRowForField(f.focused, pb)
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

func (f *AddTxForm) addTag() {
	t := newTagField()
	t.key.SetWidth(f.accColW)
	t.value.SetWidth(f.amtColW)
	f.tags = append(f.tags, t)
}

func (f *AddTxForm) addPosting() {
	p := newPostingField()
	p.account.SetWidth(f.accColW)
	p.amount.SetWidth(f.amtColW)
	f.postings = append(f.postings, p)
}

func (f *AddTxForm) deleteCurrentTag() {
	pb := f.postingBase()
	if f.focused < 2 || f.focused >= pb {
		return
	}
	row := tagRowForField(f.focused)
	if row < 0 || row >= len(f.tags) {
		return
	}
	f.tags = append(f.tags[:row], f.tags[row+1:]...)
	newFocused := f.focused
	if newFocused >= f.totalFields() {
		newFocused = f.totalFields() - 1
	}
	f.focusField(newFocused)
}

func (f *AddTxForm) deleteCurrentPosting() {
	pb := f.postingBase()
	if len(f.postings) <= 1 {
		return
	}
	if f.focused < pb {
		return
	}
	row := postingRowForField(f.focused, pb)
	if row < 0 || row >= len(f.postings) {
		return
	}
	f.postings = append(f.postings[:row], f.postings[row+1:]...)
	newFocused := f.focused
	if newFocused >= f.totalFields() {
		newFocused = f.totalFields() - 1
	}
	f.focusField(newFocused)
}

// buildTags collects non-empty, non-float-prefixed tag rows into a map.
func (f *AddTxForm) buildTags() map[string]string {
	tags := make(map[string]string)
	for _, t := range f.tags {
		k := strings.TrimSpace(t.key.Value())
		if k == "" || strings.HasPrefix(k, "float-") {
			continue
		}
		tags[k] = strings.TrimSpace(t.value.Value())
	}
	return tags
}

// buildPostings collects non-empty posting rows into PostingInput protos.
// Returns an error string if validation fails.
func (f *AddTxForm) buildPostings() ([]*floatv1.PostingInput, string) {
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
	return postings, ""
}

func (f *AddTxForm) buildAddRequest() (*floatv1.AddTransactionRequest, string) {
	desc := strings.TrimSpace(f.descInput.Value())
	if desc == "" {
		return nil, "description is required"
	}
	postings, errMsg := f.buildPostings()
	if errMsg != "" {
		return nil, errMsg
	}
	tags := f.buildTags()
	var tagsArg map[string]string
	if len(tags) > 0 {
		tagsArg = tags
	}
	return &floatv1.AddTransactionRequest{
		Description: desc,
		Date:        strings.TrimSpace(f.dateInput.Value()),
		Tags:        tagsArg,
		Postings:    postings,
	}, ""
}

func (f *AddTxForm) buildUpdateRequest() (*floatv1.UpdateTransactionRequest, string) {
	desc := strings.TrimSpace(f.descInput.Value())
	if desc == "" {
		return nil, "description is required"
	}
	postings, errMsg := f.buildPostings()
	if errMsg != "" {
		return nil, errMsg
	}
	return &floatv1.UpdateTransactionRequest{
		Fid:         f.editFID,
		Description: desc,
		Date:        strings.TrimSpace(f.dateInput.Value()),
		Tags:        f.buildTags(),
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
				f.suggestions = nil
				f.activeSuggIdx = -1
				return f, nil
			}
			f.Deactivate()
			return f, nil

		case "shift+enter":
			if f.editFID != "" {
				req, errMsg := f.buildUpdateRequest()
				if errMsg != "" {
					f.errMsg = errMsg
					return f, nil
				}
				f.submitting = true
				f.errMsg = ""
				return f, UpdateTransactionCmd(f.client, req)
			}
			req, errMsg := f.buildAddRequest()
			if errMsg != "" {
				f.errMsg = errMsg
				return f, nil
			}
			f.submitting = true
			f.errMsg = ""
			return f, AddTransactionCmd(f.client, req)

		case "ctrl+t":
			f.addTag()
			// Focus the new tag's key field.
			f.focusField(2 + (len(f.tags)-1)*2)
			return f, nil

		case "ctrl+a":
			f.addPosting()
			// Focus the new posting's account field.
			f.focusField(f.totalFields() - 2)
			return f, nil

		case "ctrl+d":
			pb := f.postingBase()
			if f.focused >= 2 && f.focused < pb {
				f.deleteCurrentTag()
			} else {
				f.deleteCurrentPosting()
			}
			return f, nil

		case "shift+tab":
			f.retreat()
			return f, nil

		case "tab":
			f.advance()
			return f, nil

		case "up":
			pb := f.postingBase()
			if isAccountField(f.focused, pb) && len(f.suggestions) > 0 {
				if f.activeSuggIdx > 0 {
					f.activeSuggIdx--
				} else {
					f.activeSuggIdx = len(f.suggestions) - 1
				}
				return f, nil
			}
			f.retreat()
			return f, nil

		case "down":
			pb := f.postingBase()
			if isAccountField(f.focused, pb) && len(f.suggestions) > 0 {
				if f.activeSuggIdx < len(f.suggestions)-1 {
					f.activeSuggIdx++
				} else {
					f.activeSuggIdx = 0
				}
				return f, nil
			}
			f.advance()
			return f, nil

		case "enter":
			pb := f.postingBase()
			if isAccountField(f.focused, pb) && f.activeSuggIdx >= 0 && f.activeSuggIdx < len(f.suggestions) {
				row := postingRowForField(f.focused, pb)
				f.postings[row].account.SetValue(f.suggestions[f.activeSuggIdx])
				f.suggestions = nil
				f.activeSuggIdx = -1
				f.advance()
				return f, nil
			}
			f.advance()
			return f, nil
		}

		// Route key to focused input.
		var cmd tea.Cmd
		pb := f.postingBase()
		switch {
		case f.focused == 0:
			f.dateInput, cmd = f.dateInput.Update(msg)
		case f.focused == 1:
			f.descInput, cmd = f.descInput.Update(msg)
		case f.focused >= 2 && f.focused < pb:
			row := tagRowForField(f.focused)
			if row >= len(f.tags) {
				return f, nil
			}
			if isTagKeyField(f.focused, pb) {
				f.tags[row].key, cmd = f.tags[row].key.Update(msg)
			} else {
				f.tags[row].value, cmd = f.tags[row].value.Update(msg)
			}
		default:
			row := postingRowForField(f.focused, pb)
			if row >= len(f.postings) {
				return f, nil
			}
			if isAccountField(f.focused, pb) {
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

	var lines []string

	// Date field
	lines = append(lines, "Date:        "+f.dateInput.View())

	// Description field
	lines = append(lines, "Description: "+f.descInput.View())
	lines = append(lines, "")

	// Tags section
	lines = append(lines, lipgloss.NewStyle().Bold(true).Render("Tags"))
	if len(f.tags) == 0 {
		lines = append(lines, f.styles.Help.Render("  (none — ctrl+t to add)"))
	} else {
		colHeader := f.styles.Help.Render(
			padRight("  Key", f.accColW+2)+"  Value",
		)
		lines = append(lines, colHeader)
		for _, t := range f.tags {
			tagLine := "  " + t.key.View() + "  " + t.value.View()
			lines = append(lines, tagLine)
		}
	}
	lines = append(lines, "")

	// Postings section
	lines = append(lines, lipgloss.NewStyle().Bold(true).Render("Postings"))

	colHeader := f.styles.Help.Render(
		padRight("  Account", f.accColW+2)+"  Amount",
	)
	lines = append(lines, colHeader)

	pb := f.postingBase()
	for i, p := range f.postings {
		accountFocusIdx := pb + i*2

		postingLine := "  " + p.account.View() + "  " + p.amount.View()
		lines = append(lines, postingLine)

		if f.focused == accountFocusIdx && len(f.suggestions) > 0 {
			dropdownLines := renderDropdown(f.suggestions, f.activeSuggIdx, f.accColW+2, f.styles)
			lines = append(lines, dropdownLines...)
		}
	}

	lines = append(lines, "")

	hint := f.styles.Help.Render("  ctrl+t add tag  ctrl+a add posting  ctrl+d del row  shift+enter submit  esc cancel")
	lines = append(lines, hint)

	if f.errMsg != "" {
		lines = append(lines, f.styles.Error.Render("  Error: "+f.errMsg))
	}

	if f.submitting {
		lines = append(lines, f.styles.Help.Render("  Submitting..."))
	}

	content := strings.Join(lines, "\n")
	title := "Add Transaction"
	if f.editFID != "" {
		title = "Edit Transaction"
	}
	return RenderModal(f.width, f.height, title, content, f.styles)
}

// renderDropdown renders a suggestion list as a small bordered box.
func renderDropdown(suggestions []string, activeIdx int, maxW int, st Styles) []string {
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
			row = "  │" + st.Active.Render(padded) + "│"
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
