package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	lipglossv1 "github.com/charmbracelet/lipgloss"
	"github.com/NimbleMarkets/ntcharts/canvas/runes"
	"github.com/NimbleMarkets/ntcharts/linechart/timeserieslinechart"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

const (
	dsPortfolioValue = "value"
	dsPortfolioCost  = "cost"
)

// PortfolioTab shows investment holdings and a portfolio value timeseries chart.
type PortfolioTab struct {
	width, height int
	client        floatv1connect.LedgerServiceClient
	holdings      holdingsPanel
	chart         portfolioChartPanel
}

func NewPortfolioTab(client floatv1connect.LedgerServiceClient, st Styles) PortfolioTab {
	return PortfolioTab{
		client:   client,
		holdings: newHoldingsPanel(st),
		chart:    newPortfolioChartPanel(st),
	}
}

func (m PortfolioTab) setStyles(st Styles) PortfolioTab {
	m.holdings.setStyles(st)
	m.chart.setStyles(st)
	return m
}

func (m PortfolioTab) SetSize(w, h int) PortfolioTab {
	m.width = w
	m.height = h
	// Split: holdings gets ~55% of height, chart gets the rest.
	holdH := h * 55 / 100
	chartH := h - holdH
	m.holdings.SetSize(w, holdH)
	m.chart.SetSize(w, chartH)
	return m
}

func (m PortfolioTab) Init() tea.Cmd {
	return tea.Batch(
		m.holdings.spinner.Tick(),
		m.chart.spinner.Tick(),
		FetchPortfolioHoldings(m.client),
		FetchPortfolioTimeseries(m.client),
	)
}

func (m PortfolioTab) Update(msg tea.Msg) (PortfolioTab, tea.Cmd) {
	switch msg := msg.(type) {
	case PortfolioHoldingsMsg:
		if msg.Err != nil {
			m.holdings.SetError(msg.Err.Error())
		} else {
			m.holdings.setData(msg.Holdings, msg.TotalValue, msg.AsOfDate)
		}
		return m, nil
	case PortfolioTimeseriesMsg:
		if msg.Err != nil {
			m.chart.SetError(msg.Err.Error())
		} else {
			m.chart.setData(msg.Snapshots)
		}
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "r" {
			m.holdings.panelBase = newPanelBase()
			m.chart.panelBase = newPanelBase()
			return m, tea.Batch(
				m.holdings.spinner.Tick(),
				m.chart.spinner.Tick(),
				FetchPortfolioHoldings(m.client),
				FetchPortfolioTimeseries(m.client),
			)
		}
		var cmd tea.Cmd
		m.holdings, cmd = m.holdings.Update(msg)
		return m, cmd
	default:
		cmd1 := m.holdings.Update2(msg)
		cmd2 := m.chart.Update(msg)
		return m, tea.Batch(cmd1, cmd2)
	}
}

func (m PortfolioTab) KeyMap() help.KeyMap {
	return PortfolioKeyMap{}
}

func (m PortfolioTab) View() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		m.holdings.View(),
		m.chart.View(),
	)
}

// ---- holdingsPanel ----

type holdingsPanel struct {
	panelBase
	styles     Styles
	holdings   []*floatv1.Holding
	totalValue *floatv1.Amount
	asOfDate   string
	table      table.Model
}

func newHoldingsPanel(st Styles) holdingsPanel {
	return holdingsPanel{
		panelBase: newPanelBase(),
		styles:    st,
		table:     newHoldingsTable(st),
	}
}

func (p *holdingsPanel) setStyles(st Styles) {
	p.styles = st
	p.table.SetStyles(styledTableStyles(st))
}

func (p *holdingsPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
	p.panelBase.width = w
	p.panelBase.height = h
	p.table.SetWidth(w)
	p.table.SetHeight(h - 2) // -2 for summary line + spacing
	p.resizeColumns()
}

func (p *holdingsPanel) resizeColumns() {
	w := p.width
	if w < 40 {
		w = 40
	}
	// Fixed widths for key columns; account gets remaining space.
	symbolW := 8
	qtyW := 12
	priceW := 14
	valueW := 14
	gainW := 13
	gainPctW := 8
	accountW := w - symbolW - qtyW - priceW - valueW - gainW - gainPctW - 1
	if accountW < 10 {
		accountW = 10
	}
	p.table.SetColumns([]table.Column{
		{Title: "Account", Width: accountW},
		{Title: "Symbol", Width: symbolW},
		{Title: "Qty", Width: qtyW},
		{Title: "Price", Width: priceW},
		{Title: "Value", Width: valueW},
		{Title: "Gain/Loss", Width: gainW},
		{Title: "Gain%", Width: gainPctW},
	})
}

func (p *holdingsPanel) setData(holdings []*floatv1.Holding, total *floatv1.Amount, asOfDate string) {
	p.holdings = holdings
	p.totalValue = total
	p.asOfDate = asOfDate
	p.state = stateLoaded

	rows := make([]table.Row, len(holdings))
	for i, h := range holdings {
		account := truncateAccount(h.Account, p.table.Columns()[0].Width)
		price := formatOptionalAmount(h.LatestPrice)
		value := formatOptionalAmount(h.CurrentValue)
		gain := formatGain(h.UnrealizedGain)
		gainPct := formatGainPct(h.UnrealizedGainPct, h.UnrealizedGain)
		rows[i] = table.Row{account, h.Symbol, h.Quantity, price, value, gain, gainPct}
	}
	p.table.SetRows(rows)
}

// Update handles key messages (table navigation).
func (p holdingsPanel) Update(msg tea.KeyMsg) (holdingsPanel, tea.Cmd) {
	if p.state == stateLoaded {
		var cmd tea.Cmd
		p.table, cmd = p.table.Update(msg)
		return p, cmd
	}
	return p, nil
}

// Update2 handles non-key messages (spinner ticks).
func (p *holdingsPanel) Update2(msg tea.Msg) tea.Cmd {
	return p.handleSpinnerTick(msg)
}

func (p holdingsPanel) View() string {
	switch p.state {
	case stateLoading:
		return p.renderLoading()
	case stateError:
		return p.renderError(true)
	case stateLoaded:
		if len(p.holdings) == 0 {
			return lipgloss.NewStyle().
				Width(p.width).Height(p.height).
				Align(lipgloss.Center, lipgloss.Center).
				Render("No investment holdings found.\nAdd prices for your investment accounts to see holdings.")
		}
		summary := p.renderSummary()
		tableView := p.table.View()
		return lipgloss.NewStyle().Width(p.width).
			Render(lipgloss.JoinVertical(lipgloss.Left, tableView, summary))
	}
	return ""
}

func (p holdingsPanel) renderSummary() string {
	parts := []string{}
	if p.totalValue != nil {
		parts = append(parts, "Total: "+p.totalValue.Quantity+" "+p.totalValue.Commodity)
	}
	if p.asOfDate != "" {
		parts = append(parts, "as of "+p.asOfDate)
	}
	line := strings.Join(parts, "  •  ")
	return p.styles.Help.Width(p.width).Render(line)
}

// ---- portfolioChartPanel ----

type portfolioChartPanel struct {
	panelBase
	styles    Styles
	snapshots []*floatv1.PortfolioTimeseriesSnapshot
}

func newPortfolioChartPanel(st Styles) portfolioChartPanel {
	return portfolioChartPanel{
		panelBase: newPanelBase(),
		styles:    st,
	}
}

func (p *portfolioChartPanel) setStyles(st Styles) {
	p.styles = st
}

func (p *portfolioChartPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
	p.panelBase.width = w
	p.panelBase.height = h
}

func (p *portfolioChartPanel) setData(snapshots []*floatv1.PortfolioTimeseriesSnapshot) {
	p.snapshots = snapshots
	p.state = stateLoaded
}

func (p *portfolioChartPanel) Update(msg tea.Msg) tea.Cmd {
	return p.handleSpinnerTick(msg)
}

func (p portfolioChartPanel) View() string {
	if p.height < 5 || p.width < 20 {
		return ""
	}
	switch p.state {
	case stateLoading:
		return p.renderLoading()
	case stateError:
		return p.renderError(false)
	case stateLoaded:
		usable := filterPortfolioSnapshots(p.snapshots)
		if len(usable) == 0 {
			return lipgloss.NewStyle().
				Width(p.width).Height(p.height).
				Align(lipgloss.Center, lipgloss.Center).
				Render("No portfolio history yet")
		}
		return p.renderChart(usable)
	}
	return ""
}

func filterPortfolioSnapshots(all []*floatv1.PortfolioTimeseriesSnapshot) []*floatv1.PortfolioTimeseriesSnapshot {
	var out []*floatv1.PortfolioTimeseriesSnapshot
	for _, s := range all {
		if s.TotalValue != nil {
			out = append(out, s)
		}
	}
	return out
}

func (p portfolioChartPanel) renderChart(snapshots []*floatv1.PortfolioTimeseriesSnapshot) string {
	chartH := p.height - 2
	if chartH < 4 {
		chartH = 4
	}

	minVal := 0.0
	maxVal := 1.0
	for _, s := range snapshots {
		v := signedAmountVal(amountSlice(s.TotalValue))
		if v > maxVal {
			maxVal = v
		}
		if s.CostBasis != nil {
			cb := signedAmountVal(amountSlice(s.CostBasis))
			if cb > maxVal {
				maxVal = cb
			}
			if cb < minVal {
				minVal = cb
			}
		}
	}
	maxVal *= 1.1
	if minVal < 0 {
		minVal *= 1.1
	}

	firstDate, ok1 := parseSnapshotDate(snapshots[0].Date)
	lastDate, ok2 := parseSnapshotDate(snapshots[len(snapshots)-1].Date)
	if !ok1 || !ok2 {
		return "date parse error"
	}
	lastDate = lastDate.AddDate(0, 1, 0)

	axisStyle := lipglossv1.NewStyle().Foreground(lipglossv1.Color("#626262"))
	labelStyle := lipglossv1.NewStyle().Foreground(lipglossv1.Color("#626262"))
	valueStyle := lipglossv1.NewStyle().Foreground(lipglossv1.Color("#A6E3A1"))  // green
	costStyle := lipglossv1.NewStyle().Foreground(lipglossv1.Color("#7DC4E4"))   // blue

	monthFormatter := func(_ int, v float64) string {
		return time.Unix(int64(v), 0).UTC().Format("Jan")
	}
	yFormatter := func(_ int, v float64) string {
		return fmtCompact(v)
	}

	hasCostBasis := false
	for _, s := range snapshots {
		if s.CostBasis != nil {
			hasCostBasis = true
			break
		}
	}

	opts := []timeserieslinechart.Option{
		timeserieslinechart.WithTimeRange(firstDate, lastDate),
		timeserieslinechart.WithYRange(minVal, maxVal),
		timeserieslinechart.WithXYSteps(4, 4),
		timeserieslinechart.WithAxesStyles(axisStyle, labelStyle),
		timeserieslinechart.WithXLabelFormatter(monthFormatter),
		timeserieslinechart.WithYLabelFormatter(yFormatter),
		timeserieslinechart.WithStyle(valueStyle),
		timeserieslinechart.WithLineStyle(runes.ArcLineStyle),
	}
	if hasCostBasis {
		opts = append(opts,
			timeserieslinechart.WithDataSetStyle(dsPortfolioCost, costStyle),
			timeserieslinechart.WithDataSetLineStyle(dsPortfolioCost, runes.ArcLineStyle),
		)
	}

	chart := timeserieslinechart.New(p.width, chartH, opts...)

	for _, s := range snapshots {
		t, ok := parseSnapshotDate(s.Date)
		if !ok {
			continue
		}
		v := signedAmountVal(amountSlice(s.TotalValue))
		chart.PushDataSet(dsPortfolioValue, timeserieslinechart.TimePoint{Time: t, Value: v})
		if hasCostBasis && s.CostBasis != nil {
			cb := signedAmountVal(amountSlice(s.CostBasis))
			chart.PushDataSet(dsPortfolioCost, timeserieslinechart.TimePoint{Time: t, Value: cb})
		}
	}

	chart.DrawAll()

	valueLegend := p.styles.ChartNetWorth.Render("━━")
	legend := p.styles.Base.Bold(true).Render("Portfolio") + "  " +
		valueLegend + p.styles.Help.Render(" value")
	if hasCostBasis {
		costLegend := p.styles.ChartAssets.Render("━━")
		legend += "  " + costLegend + p.styles.Help.Render(" cost basis")
	}
	titleLine := lipgloss.NewStyle().Width(p.width).Render(legend)

	return lipgloss.NewStyle().Width(p.width).Height(p.height).Render(
		lipgloss.JoinVertical(lipgloss.Left, titleLine, chart.View()),
	)
}

// ---- helpers ----

func amountSlice(a *floatv1.Amount) []*floatv1.Amount {
	if a == nil {
		return nil
	}
	return []*floatv1.Amount{a}
}

func formatOptionalAmount(a *floatv1.Amount) string {
	if a == nil {
		return "—"
	}
	return a.Quantity + " " + a.Commodity
}

func formatGain(a *floatv1.Amount) string {
	if a == nil {
		return "—"
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(a.Quantity), 64)
	if err != nil {
		return a.Quantity + " " + a.Commodity
	}
	prefix := ""
	if v > 0 {
		prefix = "+"
	}
	return fmt.Sprintf("%s%.2f %s", prefix, v, a.Commodity)
}

func formatGainPct(pct float64, gain *floatv1.Amount) string {
	if gain == nil || pct == 0 {
		return "—"
	}
	prefix := ""
	if pct > 0 {
		prefix = "+"
	} else if pct < 0 {
		prefix = "-"
	}
	return fmt.Sprintf("%s%.1f%%", prefix, math.Abs(pct))
}

func truncateAccount(account string, maxW int) string {
	if maxW <= 0 {
		return account
	}
	// Show the last N chars of the account path (most specific part).
	if len(account) <= maxW {
		return account
	}
	return "…" + account[len(account)-(maxW-1):]
}

func newHoldingsTable(st Styles) table.Model {
	return table.New(
		table.WithColumns([]table.Column{
			{Title: "Account", Width: 20},
			{Title: "Symbol", Width: 8},
			{Title: "Qty", Width: 12},
			{Title: "Price", Width: 14},
			{Title: "Value", Width: 14},
			{Title: "Gain/Loss", Width: 13},
			{Title: "Gain%", Width: 8},
		}),
		table.WithStyles(styledTableStyles(st)),
		table.WithFocused(true),
	)
}
