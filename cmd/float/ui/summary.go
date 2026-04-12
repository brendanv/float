package ui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	floatv1 "github.com/brendanv/float/gen/float/v1"
)

// SummaryPanel shows net worth and net income computed from depth-1 balance rows.
type SummaryPanel struct {
	panelBase
	styles         Styles
	assetsAmt      []*floatv1.Amount
	liabilitiesAmt []*floatv1.Amount
	revenueAmt     []*floatv1.Amount
	expensesAmt    []*floatv1.Amount
}

func NewSummaryPanel(st Styles) SummaryPanel {
	return SummaryPanel{panelBase: newPanelBase(), styles: st}
}

func (p *SummaryPanel) setStyles(st Styles) {
	p.styles = st
}

func (p *SummaryPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *SummaryPanel) Update(msg tea.Msg) tea.Cmd {
	return p.handleSpinnerTick(msg)
}

// SetData parses a depth-1 balance report and extracts per-type amounts.
func (p *SummaryPanel) SetData(report *floatv1.BalanceReport) {
	p.assetsAmt = nil
	p.liabilitiesAmt = nil
	p.revenueAmt = nil
	p.expensesAmt = nil
	p.state = stateLoaded
	if report == nil {
		return
	}
	for _, row := range report.Rows {
		lower := strings.ToLower(row.FullName)
		switch {
		case strings.HasPrefix(lower, "asset"):
			p.assetsAmt = row.Amounts
		case strings.HasPrefix(lower, "liabilit"):
			p.liabilitiesAmt = row.Amounts
		case strings.HasPrefix(lower, "revenue") || strings.HasPrefix(lower, "income"):
			p.revenueAmt = row.Amounts
		case strings.HasPrefix(lower, "expense"):
			p.expensesAmt = row.Amounts
		}
	}
}

// sumFirstCommodity sums the first-commodity quantity across a list of amounts.
// Returns the total float64 and the commodity string of the first amount.
func sumFirstCommodity(amounts []*floatv1.Amount) (float64, string) {
	if len(amounts) == 0 {
		return 0, ""
	}
	var total float64
	commodity := amounts[0].Commodity
	for _, a := range amounts {
		v, err := strconv.ParseFloat(strings.TrimSpace(a.Quantity), 64)
		if err == nil {
			total += v
		}
	}
	return total, commodity
}

func (p SummaryPanel) View() string {
	if p.height < 3 {
		return ""
	}
	switch p.state {
	case stateLoading:
		return p.renderLoading()
	case stateError:
		return p.renderError(false)
	case stateLoaded:
		return p.renderSummary()
	}
	return ""
}

func (p SummaryPanel) renderSummary() string {
	assets, aCom := sumFirstCommodity(p.assetsAmt)
	liab, lCom := sumFirstCommodity(p.liabilitiesAmt)
	rev, rCom := sumFirstCommodity(p.revenueAmt)
	exp, eCom := sumFirstCommodity(p.expensesAmt)

	// Net worth = assets + liabilities (liabilities are already negative in hledger).
	netWorth := assets + liab
	nwCom := aCom
	if nwCom == "" {
		nwCom = lCom
	}

	// Net income: revenue is stored negative in hledger (credit side).
	// net income = -(revenue + expenses) to get human-positive profit.
	netIncome := -(rev + exp)
	niCom := rCom
	if niCom == "" {
		niCom = eCom
	}

	const labelW = 14
	valW := p.width - labelW - 1
	if valW < 1 {
		valW = 1
	}

	bold := lipgloss.NewStyle().Bold(true)
	sep := p.styles.Help.Render(strings.Repeat("─", p.width))

	formatVal := func(v float64, com string) string {
		if com == "" {
			return fmt.Sprintf("%.2f", v)
		}
		return fmt.Sprintf("%.2f %s", v, com)
	}

	row := func(label, val string, emphasize bool) string {
		l := fmt.Sprintf("%-*s", labelW, label)
		v := fmt.Sprintf("%*s", valW, val)
		if emphasize {
			return bold.Render(l + v)
		}
		return l + v
	}

	lines := []string{
		row("Assets", formatVal(assets, aCom), false),
		row("Liabilities", formatVal(liab, lCom), false),
		sep,
		row("Net Worth", formatVal(netWorth, nwCom), true),
		"",
		// Display income as positive (negate the hledger-negative revenue amount).
		row("Income", formatVal(-rev, rCom), false),
		row("Expenses", formatVal(exp, eCom), false),
		sep,
		row("Net Income", formatVal(netIncome, niCom), true),
	}

	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		Width(p.width).
		Height(p.height).
		Render(content)
}
