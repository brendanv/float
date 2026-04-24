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

type pricesMode int

const (
	pricesModeList     pricesMode = iota
	pricesModeAdd
	pricesModeBackfill
)

// PricesTab lists commodity price directives and allows adding, deleting, and backfilling prices.
type PricesTab struct {
	width, height int
	client        floatv1connect.LedgerServiceClient
	styles        Styles
	mode          pricesMode

	panelBase
	prices []*floatv1.PriceDirective
	table  table.Model

	// Add form (focused 0–3: date, commodity, quantity, currency)
	addDateInput      textinput.Model
	addCommodityInput textinput.Model
	addQuantityInput  textinput.Model
	addCurrencyInput  textinput.Model
	addFocused        int
	addErrMsg         string
	addSubmitting     bool

	// Backfill form (focused 0–3: commodity, startDate, endDate, currency)
	bfCommodityInput textinput.Model
	bfStartDateInput textinput.Model
	bfEndDateInput   textinput.Model
	bfCurrencyInput  textinput.Model
	bfFocused        int
	bfErrMsg         string
	bfResult         string
	bfSubmitting     bool

	// Delete confirmation (list mode)
	confirmDeletePID string
	deleteErrMsg     string
}

func NewPricesTab(client floatv1connect.LedgerServiceClient, st Styles) PricesTab {
	addDate := textinput.New()
	addDate.Placeholder = "YYYY-MM-DD (blank = today)"

	addCommodity := textinput.New()
	addCommodity.Placeholder = "AAPL"

	addQuantity := textinput.New()
	addQuantity.Placeholder = "178.50"

	addCurrency := textinput.New()
	addCurrency.Placeholder = "USD"
	addCurrency.SetValue("USD")

	bfCommodity := textinput.New()
	bfCommodity.Placeholder = "AAPL"

	bfStart := textinput.New()
	bfStart.Placeholder = "YYYY-MM-DD"

	bfEnd := textinput.New()
	bfEnd.Placeholder = "YYYY-MM-DD"

	bfCurrency := textinput.New()
	bfCurrency.Placeholder = "USD"
	bfCurrency.SetValue("USD")

	return PricesTab{
		client:           client,
		styles:           st,
		panelBase:        newPanelBase(),
		table:            newPricesTable(st),
		addDateInput:     addDate,
		addCommodityInput: addCommodity,
		addQuantityInput: addQuantity,
		addCurrencyInput: addCurrency,
		bfCommodityInput: bfCommodity,
		bfStartDateInput: bfStart,
		bfEndDateInput:   bfEnd,
		bfCurrencyInput:  bfCurrency,
	}
}

func (m PricesTab) setStyles(st Styles) PricesTab {
	m.styles = st
	m.table.SetStyles(styledTableStyles(st))
	return m
}

func (m PricesTab) SetSize(w, h int) PricesTab {
	m.width = w
	m.height = h
	m.panelBase.width = w
	m.panelBase.height = h
	m.table.SetWidth(w)
	m.table.SetHeight(h)
	m.resizeColumns()

	// Size form inputs to modal inner width
	modalW := calcModalWidth(w)
	border := m.styles.FocusedBorder.Padding(modalVertPad, modalHorizPad)
	innerW := modalW - border.GetHorizontalFrameSize()
	if innerW < 1 {
		innerW = 1
	}
	m.addDateInput.SetWidth(innerW)
	m.addCommodityInput.SetWidth(innerW)
	m.addQuantityInput.SetWidth(innerW)
	m.addCurrencyInput.SetWidth(innerW)
	m.bfCommodityInput.SetWidth(innerW)
	m.bfStartDateInput.SetWidth(innerW)
	m.bfEndDateInput.SetWidth(innerW)
	m.bfCurrencyInput.SetWidth(innerW)
	return m
}

func (m *PricesTab) resizeColumns() {
	dateW := 12
	commodityW := 14
	priceW := m.width - dateW - commodityW - 1
	if priceW < 10 {
		priceW = 10
	}
	m.table.SetColumns([]table.Column{
		{Title: "Date", Width: dateW},
		{Title: "Commodity", Width: commodityW},
		{Title: "Price", Width: priceW},
	})
}

func (m PricesTab) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick(), FetchPrices(m.client))
}

func (m PricesTab) capturesAllKeys() bool {
	return m.mode != pricesModeList || m.confirmDeletePID != ""
}

func (m PricesTab) KeyMap() help.KeyMap {
	if m.mode == pricesModeAdd || m.mode == pricesModeBackfill {
		return PricesFormKeyMap{}
	}
	return PricesListKeyMap{}
}

func (m PricesTab) Update(msg tea.Msg) (PricesTab, tea.Cmd) {
	switch msg := msg.(type) {
	case PricesMsg:
		if msg.Err != nil {
			m.SetError(msg.Err.Error())
		} else {
			m.setPrices(msg.Prices)
		}
		return m, nil

	case AddPriceMsg:
		m.addSubmitting = false
		if msg.Err != nil {
			m.addErrMsg = msg.Err.Error()
			return m, nil
		}
		m.mode = pricesModeList
		m.panelBase = newPanelBase()
		return m, tea.Batch(m.spinner.Tick(), FetchPrices(m.client))

	case DeletePriceMsg:
		m.confirmDeletePID = ""
		if msg.Err != nil {
			m.deleteErrMsg = msg.Err.Error()
		} else {
			m.deleteErrMsg = ""
			m.panelBase = newPanelBase()
			return m, tea.Batch(m.spinner.Tick(), FetchPrices(m.client))
		}
		return m, nil

	case BackfillPricesMsg:
		m.bfSubmitting = false
		if msg.Err != nil {
			m.bfErrMsg = msg.Err.Error()
			m.bfResult = ""
		} else {
			m.bfErrMsg = ""
			added := msg.Added
			skipped := msg.Skipped
			noun := "prices"
			if added == 1 {
				noun = "price"
			}
			m.bfResult = fmt.Sprintf("Added %d %s", added, noun)
			if skipped > 0 {
				m.bfResult += fmt.Sprintf(" (%d already existed)", skipped)
			}
			m.panelBase = newPanelBase()
			return m, tea.Batch(m.spinner.Tick(), FetchPrices(m.client))
		}
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case pricesModeAdd:
			return m.updateAddForm(msg)
		case pricesModeBackfill:
			return m.updateBackfillForm(msg)
		default:
			return m.updateList(msg)
		}

	default:
		cmd := m.handleSpinnerTick(msg)
		return m, cmd
	}
}

func (m PricesTab) updateList(msg tea.KeyMsg) (PricesTab, tea.Cmd) {
	// Delete confirmation intercepts all keys.
	if m.confirmDeletePID != "" {
		switch msg.String() {
		case "y":
			pid := m.confirmDeletePID
			m.confirmDeletePID = ""
			return m, DeletePriceCmd(m.client, pid)
		case "esc", "n":
			m.confirmDeletePID = ""
			m.deleteErrMsg = ""
		}
		return m, nil
	}

	switch msg.String() {
	case "a":
		m.mode = pricesModeAdd
		m.resetAddForm()
		return m, nil
	case "b":
		m.mode = pricesModeBackfill
		m.resetBackfillForm()
		return m, nil
	case "d":
		if p := m.selectedPrice(); p != nil {
			if p.Pid == "" {
				m.deleteErrMsg = "cannot delete: price has no id"
				return m, nil
			}
			m.confirmDeletePID = p.Pid
			m.deleteErrMsg = ""
		}
		return m, nil
	case "r":
		m.panelBase = newPanelBase()
		return m, tea.Batch(m.spinner.Tick(), FetchPrices(m.client))
	default:
		if m.state == stateLoaded {
			var cmd tea.Cmd
			m.table, cmd = m.table.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m PricesTab) updateAddForm(msg tea.KeyMsg) (PricesTab, tea.Cmd) {
	if m.addSubmitting {
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.mode = pricesModeList
		m.addErrMsg = ""
		return m, nil
	case "shift+enter":
		return m.submitAddForm()
	case "tab", "enter":
		m.addFocused = (m.addFocused + 1) % 4
		m.focusAddField()
		return m, nil
	case "shift+tab":
		m.addFocused = (m.addFocused + 3) % 4
		m.focusAddField()
		return m, nil
	}
	return m.updateAddField(msg)
}

func (m PricesTab) updateBackfillForm(msg tea.KeyMsg) (PricesTab, tea.Cmd) {
	if m.bfSubmitting {
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.mode = pricesModeList
		m.bfErrMsg = ""
		m.bfResult = ""
		return m, nil
	case "shift+enter":
		return m.submitBackfillForm()
	case "tab", "enter":
		m.bfFocused = (m.bfFocused + 1) % 4
		m.focusBfField()
		return m, nil
	case "shift+tab":
		m.bfFocused = (m.bfFocused + 3) % 4
		m.focusBfField()
		return m, nil
	}
	return m.updateBfField(msg)
}

func (m PricesTab) updateAddField(msg tea.KeyMsg) (PricesTab, tea.Cmd) {
	var cmd tea.Cmd
	switch m.addFocused {
	case 0:
		m.addDateInput, cmd = m.addDateInput.Update(msg)
	case 1:
		m.addCommodityInput, cmd = m.addCommodityInput.Update(msg)
	case 2:
		m.addQuantityInput, cmd = m.addQuantityInput.Update(msg)
	case 3:
		m.addCurrencyInput, cmd = m.addCurrencyInput.Update(msg)
	}
	return m, cmd
}

func (m PricesTab) updateBfField(msg tea.KeyMsg) (PricesTab, tea.Cmd) {
	var cmd tea.Cmd
	switch m.bfFocused {
	case 0:
		m.bfCommodityInput, cmd = m.bfCommodityInput.Update(msg)
	case 1:
		m.bfStartDateInput, cmd = m.bfStartDateInput.Update(msg)
	case 2:
		m.bfEndDateInput, cmd = m.bfEndDateInput.Update(msg)
	case 3:
		m.bfCurrencyInput, cmd = m.bfCurrencyInput.Update(msg)
	}
	return m, cmd
}

func (m *PricesTab) focusAddField() {
	inputs := []*textinput.Model{&m.addDateInput, &m.addCommodityInput, &m.addQuantityInput, &m.addCurrencyInput}
	for i, inp := range inputs {
		if i == m.addFocused {
			inp.Focus()
		} else {
			inp.Blur()
		}
	}
}

func (m *PricesTab) focusBfField() {
	inputs := []*textinput.Model{&m.bfCommodityInput, &m.bfStartDateInput, &m.bfEndDateInput, &m.bfCurrencyInput}
	for i, inp := range inputs {
		if i == m.bfFocused {
			inp.Focus()
		} else {
			inp.Blur()
		}
	}
}

func (m *PricesTab) resetAddForm() {
	m.addDateInput.SetValue("")
	m.addCommodityInput.SetValue("")
	m.addQuantityInput.SetValue("")
	m.addCurrencyInput.SetValue("USD")
	m.addFocused = 0
	m.addErrMsg = ""
	m.addSubmitting = false
	m.focusAddField()
}

func (m *PricesTab) resetBackfillForm() {
	today := time.Now().Format("2006-01-02")
	yearAgo := time.Now().AddDate(-1, 0, 0).Format("2006-01-02")
	m.bfCommodityInput.SetValue("")
	m.bfStartDateInput.SetValue(yearAgo)
	m.bfEndDateInput.SetValue(today)
	m.bfCurrencyInput.SetValue("USD")
	m.bfFocused = 0
	m.bfErrMsg = ""
	m.bfResult = ""
	m.bfSubmitting = false
	m.focusBfField()
}

func (m PricesTab) submitAddForm() (PricesTab, tea.Cmd) {
	commodity := strings.TrimSpace(m.addCommodityInput.Value())
	if commodity == "" {
		m.addErrMsg = "commodity is required"
		return m, nil
	}
	quantity := strings.TrimSpace(m.addQuantityInput.Value())
	if quantity == "" {
		m.addErrMsg = "price is required"
		return m, nil
	}
	currency := strings.TrimSpace(m.addCurrencyInput.Value())
	if currency == "" {
		m.addErrMsg = "currency is required"
		return m, nil
	}
	m.addErrMsg = ""
	m.addSubmitting = true
	return m, AddPriceCmd(m.client, &floatv1.AddPriceRequest{
		Date:      strings.TrimSpace(m.addDateInput.Value()),
		Commodity: commodity,
		Quantity:  quantity,
		Currency:  currency,
	})
}

func (m PricesTab) submitBackfillForm() (PricesTab, tea.Cmd) {
	commodity := strings.TrimSpace(m.bfCommodityInput.Value())
	if commodity == "" {
		m.bfErrMsg = "commodity is required"
		return m, nil
	}
	startDate := strings.TrimSpace(m.bfStartDateInput.Value())
	if startDate == "" {
		m.bfErrMsg = "start date is required"
		return m, nil
	}
	endDate := strings.TrimSpace(m.bfEndDateInput.Value())
	if endDate == "" {
		m.bfErrMsg = "end date is required"
		return m, nil
	}
	currency := strings.TrimSpace(m.bfCurrencyInput.Value())
	m.bfErrMsg = ""
	m.bfResult = ""
	m.bfSubmitting = true
	return m, BackfillPricesCmd(m.client, &floatv1.BackfillPricesRequest{
		Commodity: commodity,
		StartDate: startDate,
		EndDate:   endDate,
		Currency:  currency,
	})
}

func (m PricesTab) View() string {
	switch m.mode {
	case pricesModeAdd:
		return RenderModal(m.width, m.height, "Add Price", m.viewAddForm(), m.styles)
	case pricesModeBackfill:
		return RenderModal(m.width, m.height, "Backfill Prices", m.viewBackfillForm(), m.styles)
	}

	// Delete confirmation overlay.
	if m.confirmDeletePID != "" {
		content := fmt.Sprintf("Delete price %s?\n\n[y] confirm  [esc] cancel", m.confirmDeletePID)
		return RenderModal(m.width, m.height, "Confirm Delete", content, m.styles)
	}

	switch m.state {
	case stateLoading:
		return m.renderLoading()
	case stateError:
		return m.renderError(true)
	case stateLoaded:
		if len(m.prices) == 0 {
			return lipgloss.NewStyle().
				Width(m.width).Height(m.height).
				Align(lipgloss.Center, lipgloss.Center).
				Render("No prices recorded yet.\nPress 'a' to add one, 'b' to backfill.")
		}
		body := m.table.View()
		if m.deleteErrMsg != "" {
			errLine := m.styles.Error.Render("! " + m.deleteErrMsg)
			body = lipgloss.JoinVertical(lipgloss.Left, body, errLine)
		}
		return body
	}
	return ""
}

func (m PricesTab) viewAddForm() string {
	lines := []string{
		m.priceFieldLabel("Date", 0) + m.addDateInput.View(),
		m.priceFieldLabel("Commodity", 1) + m.addCommodityInput.View(),
		m.priceFieldLabel("Price", 2) + m.addQuantityInput.View(),
		m.priceFieldLabel("Currency", 3) + m.addCurrencyInput.View(),
		"",
	}
	if m.addSubmitting {
		lines = append(lines, m.styles.Help.Render("Saving…"))
	} else if m.addErrMsg != "" {
		lines = append(lines, m.styles.Error.Render("! "+m.addErrMsg))
	} else {
		lines = append(lines, m.styles.Help.Render("shift+enter to save  esc to cancel"))
	}
	return strings.Join(lines, "\n")
}

func (m PricesTab) viewBackfillForm() string {
	lines := []string{
		m.bfFieldLabel("Commodity", 0) + m.bfCommodityInput.View(),
		m.bfFieldLabel("Start Date", 1) + m.bfStartDateInput.View(),
		m.bfFieldLabel("End Date", 2) + m.bfEndDateInput.View(),
		m.bfFieldLabel("Currency", 3) + m.bfCurrencyInput.View(),
		"",
	}
	if m.bfSubmitting {
		lines = append(lines, m.styles.Help.Render("Fetching from Alpha Vantage…"))
	} else if m.bfResult != "" {
		lines = append(lines, m.styles.Active.Render(m.bfResult))
		lines = append(lines, m.styles.Help.Render("shift+enter to run again  esc to close"))
	} else if m.bfErrMsg != "" {
		lines = append(lines, m.styles.Error.Render("! "+m.bfErrMsg))
		lines = append(lines, m.styles.Help.Render("shift+enter to submit  esc to cancel"))
	} else {
		lines = append(lines, m.styles.Help.Render("shift+enter to fetch  esc to cancel"))
	}
	return strings.Join(lines, "\n")
}

func (m PricesTab) priceFieldLabel(name string, idx int) string {
	st := m.styles.Help
	if m.addFocused == idx {
		st = m.styles.Active
	}
	return st.Render(fmt.Sprintf("%-10s ", name))
}

func (m PricesTab) bfFieldLabel(name string, idx int) string {
	st := m.styles.Help
	if m.bfFocused == idx {
		st = m.styles.Active
	}
	return st.Render(fmt.Sprintf("%-10s ", name))
}

func (m *PricesTab) setPrices(prices []*floatv1.PriceDirective) {
	m.prices = prices
	m.state = stateLoaded
	// Display newest first.
	rows := make([]table.Row, len(prices))
	for i, p := range prices {
		priceStr := ""
		if p.Price != nil {
			priceStr = p.Price.Quantity + " " + p.Price.Commodity
		}
		row := len(prices) - 1 - i
		rows[row] = table.Row{p.Date, p.Commodity, priceStr}
	}
	m.table.SetRows(rows)
}

func (m PricesTab) selectedPrice() *floatv1.PriceDirective {
	if len(m.prices) == 0 {
		return nil
	}
	c := m.table.Cursor()
	// Table rows are reversed (newest first), so map back to prices slice.
	idx := len(m.prices) - 1 - c
	if idx < 0 || idx >= len(m.prices) {
		return nil
	}
	return m.prices[idx]
}

func newPricesTable(st Styles) table.Model {
	return table.New(
		table.WithColumns([]table.Column{
			{Title: "Date", Width: 12},
			{Title: "Commodity", Width: 14},
			{Title: "Price", Width: 20},
		}),
		table.WithStyles(styledTableStyles(st)),
		table.WithFocused(true),
	)
}
