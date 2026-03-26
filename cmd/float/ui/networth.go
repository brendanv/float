package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"

	floatv1 "github.com/brendanv/float/gen/float/v1"
)

var (
	networthPositiveStyle = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{
		Light: lipgloss.Color("#3D7A4A"),
		Dark:  lipgloss.Color("#A6E3A1"),
	})
	networthLiabilityStyle = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{
		Light: lipgloss.Color("#C0392B"),
		Dark:  lipgloss.Color("#F38BA8"),
	})
	networthAssetsStyle = lipgloss.NewStyle().Foreground(colorFocused)
)

// NetWorthPanel displays a stacked bar chart of net worth, assets, and liabilities over time.
type NetWorthPanel struct {
	width, height int
	state         loadState
	spinner       Spinner
	snapshots     []*floatv1.NetWorthSnapshot
	errMsg        string
}

func NewNetWorthPanel() NetWorthPanel {
	return NetWorthPanel{
		state:   stateLoading,
		spinner: NewSpinner(),
	}
}

func (p *NetWorthPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *NetWorthPanel) SetData(snapshots []*floatv1.NetWorthSnapshot) {
	p.snapshots = snapshots
	p.state = stateLoaded
}

func (p *NetWorthPanel) SetError(msg string) {
	p.errMsg = msg
	p.state = stateError
}

func (p *NetWorthPanel) Update(msg tea.Msg) tea.Cmd {
	if sm, ok := msg.(spinner.TickMsg); ok {
		return p.spinner.Update(sm)
	}
	return nil
}

// signedAmountVal returns the raw (possibly negative) value of the first amount.
func signedAmountVal(amounts []*floatv1.Amount) float64 {
	if len(amounts) == 0 {
		return 0
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(amounts[0].Quantity), 64)
	if err != nil {
		return 0
	}
	return v
}

// fmtCompact formats a float as a compact string suitable for y-axis labels.
func fmtCompact(v float64) string {
	abs := math.Abs(v)
	prefix := ""
	if v < 0 {
		prefix = "-"
	}
	switch {
	case abs >= 1_000_000:
		return fmt.Sprintf("%s%.1fM", prefix, abs/1_000_000)
	case abs >= 1_000:
		return fmt.Sprintf("%s%.0fK", prefix, abs/1_000)
	default:
		return fmt.Sprintf("%s%.0f", prefix, abs)
	}
}

func (p NetWorthPanel) View() string {
	if p.height < 5 || p.width < 20 {
		return ""
	}
	switch p.state {
	case stateLoading:
		return lipgloss.NewStyle().
			Width(p.width).
			Height(p.height).
			Align(lipgloss.Center, lipgloss.Center).
			Render(p.spinner.View())
	case stateError:
		return lipgloss.NewStyle().
			Width(p.width).
			Height(p.height).
			Render(HelpStyle.Render("! " + p.errMsg))
	case stateLoaded:
		if len(p.snapshots) == 0 {
			return lipgloss.NewStyle().
				Width(p.width).
				Height(p.height).
				Align(lipgloss.Center, lipgloss.Center).
				Render("no net worth data")
		}
		return p.renderChart()
	}
	return ""
}

func (p NetWorthPanel) renderChart() string {
	const yAxisW = 9 // label width (8) + separator (1)
	const colW = 4   // chars per month column: 3 bar + 1 gap
	const barW = 3   // bar width in chars

	chartW := p.width - yAxisW
	if chartW < colW {
		chartW = colW
	}
	maxMonths := chartW / colW
	if maxMonths < 1 {
		maxMonths = 1
	}

	// Height: 1 title + chartH bar rows + 1 x-axis line + 1 month labels
	chartH := p.height - 3
	if chartH < 2 {
		chartH = 2
	}

	// Take the most recent months that fit.
	snaps := p.snapshots
	if len(snaps) > maxMonths {
		snaps = snaps[len(snaps)-maxMonths:]
	}

	// Compute value range.
	maxVal := 0.0
	minVal := 0.0
	for _, s := range snaps {
		a := signedAmountVal(s.Assets)
		nw := signedAmountVal(s.NetWorth)
		if a > maxVal {
			maxVal = a
		}
		if nw > maxVal {
			maxVal = nw
		}
		if nw < minVal {
			minVal = nw
		}
	}
	if maxVal == minVal {
		if maxVal == 0 {
			maxVal = 1
		} else {
			maxVal += math.Abs(maxVal) * 0.1
		}
	}
	totalRange := maxVal - minVal

	// rowMidVal returns the value at the midpoint of row r.
	rowMidVal := func(r int) float64 {
		return maxVal - (float64(r)+0.5)*totalRange/float64(chartH)
	}

	// valToRow maps a value to the nearest chart row.
	valToRow := func(v float64) int {
		return int(math.Round(float64(chartH-1) * (maxVal - v) / totalRange))
	}

	zeroInRange := minVal < 0 && maxVal > 0
	zeroRow := 0
	if zeroInRange {
		zeroRow = valToRow(0)
	}

	// Y-axis label positions.
	yLabels := map[int]string{
		0:          fmtCompact(maxVal),
		chartH / 2: fmtCompact((maxVal + minVal) / 2),
		chartH - 1: fmtCompact(minVal),
	}
	if zeroInRange {
		yLabels[zeroRow] = "0"
	}

	var lines []string

	// Title + legend.
	legend := networthPositiveStyle.Render("█") + HelpStyle.Render(" net worth  ") +
		networthLiabilityStyle.Render("█") + HelpStyle.Render(" liabilities  ") +
		networthAssetsStyle.Render("█") + HelpStyle.Render(" assets (neg NW)")
	lines = append(lines, lipgloss.NewStyle().Width(p.width).Render(
		lipgloss.NewStyle().Bold(true).Render("Net Worth")+"  "+legend,
	))

	// Chart rows.
	for row := 0; row < chartH; row++ {
		mid := rowMidVal(row)

		label := ""
		if lbl, ok := yLabels[row]; ok {
			label = lbl
		}
		axisChar := "│"
		if zeroInRange && row == zeroRow {
			axisChar = "┼"
		}

		var sb strings.Builder
		sb.WriteString(HelpStyle.Render(fmt.Sprintf("%*s%s", yAxisW-1, label, axisChar)))

		for _, s := range snaps {
			assets := signedAmountVal(s.Assets)
			nw := signedAmountVal(s.NetWorth)

			var cellStr string
			bar := strings.Repeat("█", barW)

			if zeroInRange && row == zeroRow {
				// Draw zero baseline through this row.
				cellStr = HelpStyle.Render(strings.Repeat("─", barW))
			} else if nw >= 0 {
				// Positive net worth: bottom = net worth (green), top = liabilities (red).
				switch {
				case mid >= 0 && mid <= nw:
					cellStr = networthPositiveStyle.Render(bar)
				case mid > nw && mid <= assets:
					cellStr = networthLiabilityStyle.Render(bar)
				default:
					cellStr = strings.Repeat(" ", barW)
				}
			} else {
				// Negative net worth: assets above zero (blue), negative portion below (red).
				switch {
				case mid >= 0 && mid <= assets:
					cellStr = networthAssetsStyle.Render(bar)
				case mid >= nw && mid < 0:
					cellStr = networthLiabilityStyle.Render(bar)
				default:
					cellStr = strings.Repeat(" ", barW)
				}
			}

			sb.WriteString(cellStr)
			sb.WriteString(" ") // gap between months
		}

		lines = append(lines, sb.String())
	}

	// X-axis line.
	xLine := fmt.Sprintf("%*s└%s", yAxisW-1, "", strings.Repeat("─", p.width-yAxisW))
	lines = append(lines, HelpStyle.Render(xLine))

	// Month labels.
	monthNames := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	var mlSB strings.Builder
	mlSB.WriteString(strings.Repeat(" ", yAxisW))
	for _, s := range snaps {
		label := "   "
		if len(s.Date) >= 7 {
			parts := strings.SplitN(s.Date[:7], "-", 2)
			if len(parts) == 2 {
				if m, err := strconv.Atoi(parts[1]); err == nil && m >= 1 && m <= 12 {
					label = fmt.Sprintf("%-3s", monthNames[m-1])
				}
			}
		}
		mlSB.WriteString(label)
		mlSB.WriteString(" ")
	}
	lines = append(lines, HelpStyle.Render(mlSB.String()))

	return lipgloss.NewStyle().Width(p.width).Height(p.height).Render(strings.Join(lines, "\n"))
}
