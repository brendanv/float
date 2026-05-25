package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

const (
	isPeriodColW = 10 // chars per period column (right-aligned amount)
	isTotalColW  = 10 // chars for the grand-total column
)

// MonthlyTab shows the income statement timeseries: revenues and expenses
// broken down by account across monthly periods for a selected year.
type MonthlyTab struct {
	width, height int
	year          int
	client        floatv1connect.LedgerServiceClient
	panel         incomeStatementPanel
}

func NewMonthlyTab(client floatv1connect.LedgerServiceClient, st Styles) MonthlyTab {
	return MonthlyTab{
		client: client,
		year:   time.Now().Year(),
		panel:  newIncomeStatementPanel(st),
	}
}

func (m MonthlyTab) setStyles(st Styles) MonthlyTab {
	m.panel.styles = st
	return m
}

func (m MonthlyTab) SetSize(w, h int) MonthlyTab {
	m.width = w
	m.height = h
	m.panel.SetSize(w, h)
	return m
}

func (m MonthlyTab) Init() tea.Cmd {
	return tea.Batch(
		m.panel.spinner.Tick(),
		m.fetchForYear(),
	)
}

func (m MonthlyTab) fetchForYear() tea.Cmd {
	begin := fmt.Sprintf("%d-01-01", m.year)
	end := fmt.Sprintf("%d-01-01", m.year+1)
	return FetchIncomeStatement(m.client, m.year, begin, end)
}

func (m MonthlyTab) Update(msg tea.Msg) (MonthlyTab, tea.Cmd) {
	switch msg := msg.(type) {
	case IncomeStatementMsg:
		if msg.Year != m.year {
			return m, nil // stale response from a previous year navigation
		}
		if msg.Err != nil {
			m.panel.SetError(msg.Err.Error())
		} else {
			m.panel.setData(msg.Periods, msg.Rows, msg.NetAmounts)
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "[":
			m.year--
			m.panel.panelBase = newPanelBase()
			m.panel.hOffset = 0
			m.panel.vOffset = 0
			return m, tea.Batch(m.panel.spinner.Tick(), m.fetchForYear())
		case "]":
			m.year++
			m.panel.panelBase = newPanelBase()
			m.panel.hOffset = 0
			m.panel.vOffset = 0
			return m, tea.Batch(m.panel.spinner.Tick(), m.fetchForYear())
		case "r":
			m.panel.panelBase = newPanelBase()
			m.panel.hOffset = 0
			m.panel.vOffset = 0
			return m, tea.Batch(m.panel.spinner.Tick(), m.fetchForYear())
		default:
			var cmd tea.Cmd
			m.panel, cmd = m.panel.Update(msg)
			return m, cmd
		}
	default:
		cmd := m.panel.handleSpinnerTick(msg)
		return m, cmd
	}
}

func (m MonthlyTab) KeyMap() help.KeyMap {
	return MonthlyKeyMap{}
}

func (m MonthlyTab) View() string {
	return m.panel.ViewWithYear(m.year)
}

// ---- incomeStatementPanel ----

// isRowKind tags each logical row in the scrollable region.
type isRowKind int

const (
	isRowKindSection isRowKind = iota // bold section header ("REVENUES" / "EXPENSES")
	isRowKindAccount                  // regular or total account row
)

type isRowDesc struct {
	kind        isRowKind
	sectionName string
	row         *floatv1.IncomeStatementRow
}

// incomeStatementPanel holds and renders the income statement table.
type incomeStatementPanel struct {
	panelBase
	styles     Styles
	periods    []string
	rows       []*floatv1.IncomeStatementRow
	netAmounts []*floatv1.AmountList
	hOffset    int // first visible period column index
	vOffset    int // first visible data-row index (in the scrollable region)
}

func newIncomeStatementPanel(st Styles) incomeStatementPanel {
	return incomeStatementPanel{
		panelBase: newPanelBase(),
		styles:    st,
	}
}

func (p *incomeStatementPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *incomeStatementPanel) setData(periods []string, rows []*floatv1.IncomeStatementRow, net []*floatv1.AmountList) {
	p.periods = periods
	p.rows = rows
	p.netAmounts = net
	p.state = stateLoaded
}

// calcLayout returns the account column width and max visible period columns.
func (p incomeStatementPanel) calcLayout() (acctW, maxVis int) {
	w := p.width
	if w < 40 {
		w = 40
	}
	acctW = w * 25 / 100
	if acctW < 18 {
		acctW = 18
	}
	if acctW > 32 {
		acctW = 32
	}
	avail := w - acctW - isTotalColW
	maxVis = avail / isPeriodColW
	if maxVis < 1 {
		maxVis = 1
	}
	if maxVis > len(p.periods) {
		maxVis = len(p.periods)
	}
	return
}

// buildRowDescs constructs the ordered list of logical rows for scrolling.
func (p incomeStatementPanel) buildRowDescs() []isRowDesc {
	var descs []isRowDesc
	currentSection := ""
	for _, row := range p.rows {
		if row.Section != currentSection {
			if currentSection != "" {
				// blank separator row between sections
				descs = append(descs, isRowDesc{kind: isRowKindSection, sectionName: ""})
			}
			currentSection = row.Section
			descs = append(descs, isRowDesc{kind: isRowKindSection, sectionName: currentSection})
		}
		descs = append(descs, isRowDesc{kind: isRowKindAccount, row: row})
	}
	return descs
}

func (p incomeStatementPanel) Update(msg tea.KeyMsg) (incomeStatementPanel, tea.Cmd) {
	if p.state != stateLoaded {
		return p, nil
	}
	_, maxVis := p.calcLayout()
	descs := p.buildRowDescs()

	// Fixed lines: year header(1) + sep(1) + col headers(1) + sep(1) + net-sep(1) + net row(1) = 6
	const fixedLines = 6
	availH := p.height - fixedLines
	if availH < 1 {
		availH = 1
	}

	switch msg.String() {
	case "left", "h":
		if p.hOffset > 0 {
			p.hOffset--
		}
	case "right", "l":
		maxHOffset := len(p.periods) - maxVis
		if maxHOffset < 0 {
			maxHOffset = 0
		}
		if p.hOffset < maxHOffset {
			p.hOffset++
		}
	case "up", "k":
		if p.vOffset > 0 {
			p.vOffset--
		}
	case "down", "j":
		maxV := len(descs) - availH
		if maxV < 0 {
			maxV = 0
		}
		if p.vOffset < maxV {
			p.vOffset++
		}
	}
	return p, nil
}

func (p incomeStatementPanel) ViewWithYear(year int) string {
	if p.height < 5 {
		return ""
	}
	switch p.state {
	case stateLoading:
		return p.renderLoading()
	case stateError:
		return p.renderError(true)
	case stateLoaded:
		return p.renderTable(year)
	}
	return ""
}

func (p incomeStatementPanel) renderTable(year int) string {
	acctW, maxVis := p.calcLayout()

	// Clamp hOffset
	hOffset := p.hOffset
	if len(p.periods) > 0 {
		maxHOff := len(p.periods) - maxVis
		if maxHOff < 0 {
			maxHOff = 0
		}
		if hOffset > maxHOff {
			hOffset = maxHOff
		}
	}

	// Determine visible period slice
	end := hOffset + maxVis
	if end > len(p.periods) {
		end = len(p.periods)
	}
	visibleIdxs := make([]int, 0, maxVis)
	for i := hOffset; i < end; i++ {
		visibleIdxs = append(visibleIdxs, i)
	}

	// Separator line spans the used columns only
	usedW := acctW + len(visibleIdxs)*isPeriodColW + isTotalColW
	if usedW > p.width {
		usedW = p.width
	}
	sep := strings.Repeat("─", usedW)

	// Year / scroll-hint header
	yearStr := fmt.Sprintf("%d", year)
	if len(p.periods) > maxVis && len(p.periods) > 0 {
		yearStr += fmt.Sprintf("  (months %d-%d of %d  ←/→ to scroll)", hOffset+1, hOffset+len(visibleIdxs), len(p.periods))
	}
	yearLine := p.styles.Base.Bold(true).Render("Income Statement: " + yearStr)

	// Column header row
	colHeader := isRenderAcctCell("Account", acctW)
	for _, idx := range visibleIdxs {
		colHeader += isRenderPeriodHeader(p.periods[idx])
	}
	colHeader += fmt.Sprintf("%*s", isTotalColW, "Total")

	// Build all logical data rows
	descs := p.buildRowDescs()

	const fixedLines = 6
	availH := p.height - fixedLines
	if availH < 1 {
		availH = 1
	}

	// Clamp vOffset
	vOffset := p.vOffset
	maxVOff := len(descs) - availH
	if maxVOff < 0 {
		maxVOff = 0
	}
	if vOffset > maxVOff {
		vOffset = maxVOff
	}

	// Render visible rows
	endIdx := vOffset + availH
	if endIdx > len(descs) {
		endIdx = len(descs)
	}
	visible := descs[vOffset:endIdx]

	var renderedRows []string
	for _, d := range visible {
		switch d.kind {
		case isRowKindSection:
			if d.sectionName == "" {
				renderedRows = append(renderedRows, "") // blank separator
			} else {
				renderedRows = append(renderedRows,
					p.styles.Active.Bold(true).Render(strings.ToUpper(d.sectionName)))
			}
		case isRowKindAccount:
			renderedRows = append(renderedRows, p.renderAccountRow(d.row, acctW, visibleIdxs))
		}
	}

	// Net income row (always shown at bottom, not scrolled)
	netRow := p.renderNetRow(acctW, visibleIdxs)

	// Assemble
	lines := []string{
		yearLine,
		p.styles.Help.Render(sep),
		p.styles.Base.Bold(true).Render(colHeader),
		p.styles.Help.Render(sep),
	}
	lines = append(lines, renderedRows...)
	lines = append(lines,
		p.styles.Help.Render(sep),
		p.styles.Base.Bold(true).Render(netRow),
	)

	return lipgloss.NewStyle().Width(p.width).Height(p.height).Render(
		strings.Join(lines, "\n"),
	)
}

func (p incomeStatementPanel) renderAccountRow(row *floatv1.IncomeStatementRow, acctW int, periodIdxs []int) string {
	indent := int(row.Indent)
	if row.IsTotal {
		indent = 0
	}
	prefix := strings.Repeat("  ", indent)
	name := row.DisplayName
	maxNameW := acctW - len(prefix)
	if maxNameW < 3 {
		maxNameW = 3
	}
	if len(name) > maxNameW {
		name = name[:maxNameW-1] + "…"
	}
	acctCell := isRenderAcctCell(prefix+name, acctW)

	var sb strings.Builder
	sb.WriteString(acctCell)
	for _, idx := range periodIdxs {
		var al *floatv1.AmountList
		if idx < len(row.PerPeriodAmounts) {
			al = row.PerPeriodAmounts[idx]
		}
		sb.WriteString(isFormatAmountCell(al, isPeriodColW))
	}
	// Total column: use row's total, not sum of visible periods
	var totalStr string
	if len(row.TotalAmounts) > 0 {
		totalStr = isFormatQty(row.TotalAmounts[0].Quantity)
	} else {
		totalStr = "—"
	}
	fmt.Fprintf(&sb, "%*s", isTotalColW, totalStr)

	line := sb.String()
	if row.IsTotal {
		return p.styles.Base.Bold(true).Render(line)
	}
	return line
}

func (p incomeStatementPanel) renderNetRow(acctW int, periodIdxs []int) string {
	acctCell := isRenderAcctCell("Net Income", acctW)

	var sb strings.Builder
	sb.WriteString(acctCell)
	for _, idx := range periodIdxs {
		var al *floatv1.AmountList
		if idx < len(p.netAmounts) {
			al = p.netAmounts[idx]
		}
		sb.WriteString(isFormatAmountCell(al, isPeriodColW))
	}

	// Grand total: sum all net periods
	var total float64
	for _, al := range p.netAmounts {
		if al != nil && len(al.Amounts) > 0 {
			if f, err := strconv.ParseFloat(strings.TrimSpace(al.Amounts[0].Quantity), 64); err == nil {
				total += f
			}
		}
	}
	fmt.Fprintf(&sb, "%*s", isTotalColW, isFormatFloat(total))

	return sb.String()
}

// ---- rendering helpers ----

// isRenderAcctCell left-aligns name in a fixed-width cell.
func isRenderAcctCell(name string, w int) string {
	if len(name) > w {
		return name[:w-1] + "…"
	}
	return name + strings.Repeat(" ", w-len(name))
}

// isRenderPeriodHeader formats "2026-01-01" → "Jan'26" right-aligned in isPeriodColW.
func isRenderPeriodHeader(period string) string {
	return fmt.Sprintf("%*s", isPeriodColW, formatISPeriod(period))
}

// formatISPeriod converts "2026-01-01" to "Jan'26".
func formatISPeriod(period string) string {
	if len(period) < 7 {
		return period
	}
	parts := strings.SplitN(period, "-", 3)
	if len(parts) < 2 {
		return period
	}
	monthNum, err := strconv.Atoi(parts[1])
	if err != nil || monthNum < 1 || monthNum > 12 {
		return period
	}
	months := [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	year := parts[0]
	if len(year) >= 4 {
		year = year[2:]
	}
	return months[monthNum-1] + "'" + year
}

// isFormatAmountCell extracts the first amount from al and right-aligns it in w chars.
func isFormatAmountCell(al *floatv1.AmountList, w int) string {
	if al == nil || len(al.Amounts) == 0 {
		return fmt.Sprintf("%*s", w, "—")
	}
	return fmt.Sprintf("%*s", w, isFormatQty(al.Amounts[0].Quantity))
}

// isFormatQty formats a quantity string as a compact integer with commas.
func isFormatQty(quantity string) string {
	f, err := strconv.ParseFloat(strings.TrimSpace(quantity), 64)
	if err != nil {
		return "?"
	}
	return isFormatFloat(f)
}

// isFormatFloat formats a float as an integer with comma thousands separator.
// Values near zero become "—".
func isFormatFloat(f float64) string {
	if math.Abs(f) < 0.005 {
		return "—"
	}
	prefix := ""
	if f < 0 {
		prefix = "-"
		f = -f
	}
	n := int64(math.Round(f))
	if n == 0 {
		return "—"
	}
	s := strconv.FormatInt(n, 10)
	var out strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out.WriteRune(',')
		}
		out.WriteRune(c)
	}
	return prefix + out.String()
}
