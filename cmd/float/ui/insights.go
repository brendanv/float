package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"

	floatv1 "github.com/brendanv/float/gen/float/v1"
)

var insightsBarStyle = lipgloss.NewStyle().Foreground(colorFocused)
var revenueBarStyle = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{Light: lipgloss.Color("#3D7A4A"), Dark: lipgloss.Color("#A6E3A1")})

// InsightsPanel displays a horizontal bar chart of income and expense sub-categories.
type InsightsPanel struct {
	panelBase
	expenseRows []*floatv1.BalanceRow
	revenueRows []*floatv1.BalanceRow
}

func NewInsightsPanel() InsightsPanel {
	return InsightsPanel{
		panelBase: newPanelBase(),
	}
}

func (p *InsightsPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *InsightsPanel) SetData(report *floatv1.BalanceReport) {
	p.expenseRows = nil
	p.revenueRows = nil
	p.state = stateLoaded
	if report == nil {
		return
	}
	for _, row := range report.Rows {
		lower := strings.ToLower(row.FullName)
		switch {
		case strings.HasPrefix(lower, "expense"):
			p.expenseRows = append(p.expenseRows, row)
		case strings.HasPrefix(lower, "revenue"), strings.HasPrefix(lower, "income"):
			p.revenueRows = append(p.revenueRows, row)
		}
	}
}

func (p *InsightsPanel) Update(msg tea.Msg) tea.Cmd {
	return p.handleSpinnerTick(msg)
}

// primaryValue extracts the absolute numeric value of the first amount.
func primaryValue(amounts []*floatv1.Amount) float64 {
	if len(amounts) == 0 {
		return 0
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(amounts[0].Quantity), 64)
	if err != nil {
		return 0
	}
	return math.Abs(v)
}

func maxPrimaryValue(rows []*floatv1.BalanceRow) float64 {
	var m float64
	for _, row := range rows {
		if v := primaryValue(row.Amounts); v > m {
			m = v
		}
	}
	return m
}

// shortName returns the last segment of a colon-delimited account name.
// "expenses:food" → "food", "salary" → "salary".
func shortName(displayName string) string {
	if i := strings.LastIndex(displayName, ":"); i >= 0 {
		return displayName[i+1:]
	}
	return displayName
}

func (p InsightsPanel) View() string {
	if p.height < 3 || p.width < 10 {
		return ""
	}
	switch p.state {
	case stateLoading:
		return p.renderLoading()
	case stateError:
		return p.renderError(false)
	case stateLoaded:
		if len(p.expenseRows) == 0 && len(p.revenueRows) == 0 {
			return lipgloss.NewStyle().
				Width(p.width).
				Height(p.height).
				Align(lipgloss.Center, lipgloss.Center).
				Render("no activity")
		}
		return p.renderChart()
	}
	return ""
}

func (p InsightsPanel) renderChart() string {
	allRows := append(p.revenueRows, p.expenseRows...)

	// Name column width is based on short names (last segment only).
	nameCol := 0
	for _, row := range allRows {
		if n := len([]rune(shortName(row.DisplayName))); n > nameCol {
			nameCol = n
		}
	}
	if maxNameCol := p.width / 3; nameCol > maxNameCol {
		nameCol = maxNameCol
	}
	if nameCol < 1 {
		nameCol = 1
	}

	const amountCol = 12
	const sep = 2
	barCol := p.width - nameCol - amountCol - sep*2
	if barCol < 1 {
		barCol = 1
	}

	hasBoth := len(p.revenueRows) > 0 && len(p.expenseRows) > 0

	// Section headers are shown only when both sections are present and there
	// is enough room (≥4 lines: 2 headers + 1 row per section).
	showHeaders := hasBoth && p.height >= 4
	headerLines := 0
	if showHeaders {
		headerLines = 2
	}

	// Split remaining lines proportionally between sections.
	dataLines := p.height - headerLines
	revAlloc, expAlloc := len(p.revenueRows), len(p.expenseRows)
	if hasBoth {
		total := len(p.revenueRows) + len(p.expenseRows)
		revAlloc = dataLines * len(p.revenueRows) / total
		if revAlloc < 1 {
			revAlloc = 1
		}
		expAlloc = dataLines - revAlloc
		if expAlloc < 1 {
			expAlloc = 1
		}
	}

	// Scale all bars against the global maximum so sections are comparable.
	maxVal := maxPrimaryValue(allRows)

	var lines []string

	if len(p.revenueRows) > 0 && revAlloc > 0 {
		if showHeaders {
			lines = append(lines, HelpStyle.Render("income"))
		}
		for i, row := range p.revenueRows {
			if i >= revAlloc {
				break
			}
			lines = append(lines, renderBarLine(row, maxVal, nameCol, barCol, amountCol, revenueBarStyle))
		}
	}

	if len(p.expenseRows) > 0 && expAlloc > 0 {
		if showHeaders {
			lines = append(lines, HelpStyle.Render("expenses"))
		}
		for i, row := range p.expenseRows {
			if i >= expAlloc {
				break
			}
			lines = append(lines, renderBarLine(row, maxVal, nameCol, barCol, amountCol, insightsBarStyle))
		}
	}

	return lipgloss.NewStyle().Width(p.width).Render(strings.Join(lines, "\n"))
}

func renderBarLine(row *floatv1.BalanceRow, maxVal float64, nameCol, barCol, amountCol int, barStyle lipgloss.Style) string {
	val := primaryValue(row.Amounts)

	filled := 0
	if maxVal > 0 {
		filled = int(float64(barCol) * val / maxVal)
	}
	if filled > barCol {
		filled = barCol
	}
	bar := barStyle.Render(strings.Repeat("█", filled)) +
		HelpStyle.Render(strings.Repeat("░", barCol-filled))

	name := shortName(row.DisplayName)
	nameRunes := []rune(name)
	if len(nameRunes) > nameCol {
		nameRunes = nameRunes[:nameCol]
	}
	name = fmt.Sprintf("%-*s", nameCol, string(nameRunes))

	amountStr := ""
	if len(row.Amounts) > 0 {
		amountStr = fmt.Sprintf("%.2f %s", val, row.Amounts[0].Commodity)
		if len(amountStr) > amountCol {
			amountStr = amountStr[:amountCol]
		}
	}
	amountStr = fmt.Sprintf("%*s", amountCol, amountStr)

	return name + "  " + bar + "  " + amountStr
}
