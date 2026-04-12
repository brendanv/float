package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	floatv1 "github.com/brendanv/float/gen/float/v1"
)

type chartMode int

const (
	chartModeSpending  chartMode = iota // shows InsightsPanel bar chart
	chartModeNetWorth                   // shows NetWorthPanel line chart
)

// ChartPanel wraps InsightsPanel and NetWorthPanel with a 1-line mode toggle
// header. The active panel fills the remaining (height-1) rows. Toggle between
// modes with Toggle().
type ChartPanel struct {
	panelBase
	mode     chartMode
	styles   Styles
	insights InsightsPanel
	netWorth NetWorthPanel
}

func NewChartPanel(st Styles) ChartPanel {
	return ChartPanel{
		panelBase: newPanelBase(),
		mode:      chartModeSpending,
		styles:    st,
		insights:  NewInsightsPanel(st),
		netWorth:  NewNetWorthPanel(st),
	}
}

func (p *ChartPanel) setStyles(st Styles) {
	p.styles = st
	p.insights.setStyles(st)
	p.netWorth.setStyles(st)
}

// SetSize stores dimensions and propagates to inner panels, reserving 1 row
// for the mode-toggle header.
func (p *ChartPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
	innerH := h - 1
	if innerH < 1 {
		innerH = 1
	}
	p.insights.SetSize(w, innerH)
	p.netWorth.SetSize(w, innerH)
}

func (p *ChartPanel) SetInsightsData(report *floatv1.BalanceReport) {
	p.insights.SetData(report)
}

func (p *ChartPanel) SetNetWorthData(snapshots []*floatv1.NetWorthSnapshot) {
	p.netWorth.SetData(snapshots)
}

func (p *ChartPanel) SetInsightsError(msg string) {
	p.insights.SetError(msg)
}

func (p *ChartPanel) SetNetWorthError(msg string) {
	p.netWorth.SetError(msg)
}

// Toggle switches between spending and net worth modes.
func (p *ChartPanel) Toggle() {
	if p.mode == chartModeSpending {
		p.mode = chartModeNetWorth
	} else {
		p.mode = chartModeSpending
	}
}

// Update forwards spinner ticks to both inner panels.
func (p *ChartPanel) Update(msg tea.Msg) tea.Cmd {
	cmd1 := p.insights.Update(msg)
	cmd2 := p.netWorth.Update(msg)
	return tea.Batch(cmd1, cmd2)
}

func (p ChartPanel) View() string {
	if p.height < 3 || p.width < 10 {
		return ""
	}
	header := p.renderHeader()
	var content string
	switch p.mode {
	case chartModeSpending:
		content = p.insights.View()
	case chartModeNetWorth:
		content = p.netWorth.View()
	}
	return lipgloss.NewStyle().Width(p.width).Height(p.height).Render(
		lipgloss.JoinVertical(lipgloss.Left, header, content),
	)
}

func (p ChartPanel) renderHeader() string {
	var spendingLabel, networthLabel string
	activeStyle := p.styles.Active.Bold(true)
	if p.mode == chartModeSpending {
		spendingLabel = activeStyle.Render("Spending")
		networthLabel = p.styles.Help.Render("Net Worth")
	} else {
		spendingLabel = p.styles.Help.Render("Spending")
		networthLabel = activeStyle.Render("Net Worth")
	}
	left := spendingLabel + "  " + networthLabel
	right := p.styles.Help.Render("t=toggle")
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := p.width - leftW - rightW
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}
