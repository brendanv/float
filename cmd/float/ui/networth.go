package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	lipglossv1 "github.com/charmbracelet/lipgloss"
	"github.com/NimbleMarkets/ntcharts/canvas/runes"
	"github.com/NimbleMarkets/ntcharts/linechart/timeserieslinechart"

	floatv1 "github.com/brendanv/float/gen/float/v1"
)

const (
	dsAssets      = "assets"
	dsLiabilities = "liabilities"
	dsNetWorth    = "networth"
)

// NetWorthPanel displays a time-series line chart of assets, liabilities, and net worth.
type NetWorthPanel struct {
	panelBase
	styles    Styles
	snapshots []*floatv1.NetWorthSnapshot
}

func NewNetWorthPanel(st Styles) NetWorthPanel {
	return NetWorthPanel{
		panelBase: newPanelBase(),
		styles:    st,
	}
}

func (p *NetWorthPanel) setStyles(st Styles) {
	p.styles = st
}

func (p *NetWorthPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *NetWorthPanel) SetData(snapshots []*floatv1.NetWorthSnapshot) {
	p.snapshots = snapshots
	p.state = stateLoaded
}

func (p *NetWorthPanel) Update(msg tea.Msg) tea.Cmd {
	return p.handleSpinnerTick(msg)
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

// parseSnapshotDate parses "YYYY-MM-DD" into a time.Time.
func parseSnapshotDate(s string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", s)
	return t, err == nil
}

func (p NetWorthPanel) View() string {
	if p.height < 5 || p.width < 20 {
		return ""
	}
	switch p.state {
	case stateLoading:
		return p.renderLoading()
	case stateError:
		return p.renderError(false)
	case stateLoaded:
		if len(p.snapshots) == 0 {
			return lipgloss.NewStyle().
				Width(p.width).Height(p.height).
				Align(lipgloss.Center, lipgloss.Center).
				Render("no net worth data")
		}
		return p.renderChart()
	}
	return ""
}

func (p NetWorthPanel) renderChart() string {
	// Reserve 2 rows: 1 for title+legend, 1 for bottom padding.
	chartH := p.height - 2
	if chartH < 4 {
		chartH = 4
	}

	// Compute value range across all snapshots.
	minVal := 0.0
	maxVal := 1.0
	for _, s := range p.snapshots {
		a := signedAmountVal(s.Assets)
		l := signedAmountVal(s.Liabilities) // negative
		nw := signedAmountVal(s.NetWorth)
		if a > maxVal {
			maxVal = a
		}
		if nw > maxVal {
			maxVal = nw
		}
		if l < minVal {
			minVal = l
		}
		if nw < minVal {
			minVal = nw
		}
	}
	// Add 10% headroom.
	maxVal *= 1.1
	if minVal < 0 {
		minVal *= 1.1
	}

	// Parse time range.
	firstDate, ok1 := parseSnapshotDate(p.snapshots[0].Date)
	lastDate, ok2 := parseSnapshotDate(p.snapshots[len(p.snapshots)-1].Date)
	if !ok1 || !ok2 {
		return "date parse error"
	}
	// Extend end slightly so the last point isn't flush against the right edge.
	lastDate = lastDate.AddDate(0, 1, 0)

	// ntcharts uses github.com/charmbracelet/lipgloss (v1) for styles.
	axisStyle := lipglossv1.NewStyle().Foreground(lipglossv1.Color("#626262"))
	labelStyle := lipglossv1.NewStyle().Foreground(lipglossv1.Color("#626262"))
	assetsStyle := lipglossv1.NewStyle().Foreground(lipglossv1.Color("#7DC4E4"))   // blue
	networthStyle := lipglossv1.NewStyle().Foreground(lipglossv1.Color("#A6E3A1")) // green
	liabStyle := lipglossv1.NewStyle().Foreground(lipglossv1.Color("#F38BA8"))     // red/pink

	monthFormatter := func(_ int, v float64) string {
		t := time.Unix(int64(v), 0).UTC()
		return t.Format("Jan")
	}
	yFormatter := func(_ int, v float64) string {
		return fmtCompact(v)
	}

	chart := timeserieslinechart.New(p.width, chartH,
		timeserieslinechart.WithTimeRange(firstDate, lastDate),
		timeserieslinechart.WithYRange(minVal, maxVal),
		timeserieslinechart.WithXYSteps(4, 4),
		timeserieslinechart.WithAxesStyles(axisStyle, labelStyle),
		timeserieslinechart.WithXLabelFormatter(monthFormatter),
		timeserieslinechart.WithYLabelFormatter(yFormatter),
		// Default dataset = assets.
		timeserieslinechart.WithStyle(assetsStyle),
		timeserieslinechart.WithLineStyle(runes.ArcLineStyle),
		timeserieslinechart.WithDataSetStyle(dsNetWorth, networthStyle),
		timeserieslinechart.WithDataSetLineStyle(dsNetWorth, runes.ArcLineStyle),
		timeserieslinechart.WithDataSetStyle(dsLiabilities, liabStyle),
		timeserieslinechart.WithDataSetLineStyle(dsLiabilities, runes.ArcLineStyle),
	)

	for _, s := range p.snapshots {
		t, ok := parseSnapshotDate(s.Date)
		if !ok {
			continue
		}
		a := signedAmountVal(s.Assets)
		l := signedAmountVal(s.Liabilities)
		nw := signedAmountVal(s.NetWorth)
		chart.PushDataSet(dsAssets, timeserieslinechart.TimePoint{Time: t, Value: a})
		chart.PushDataSet(dsLiabilities, timeserieslinechart.TimePoint{Time: t, Value: l})
		chart.PushDataSet(dsNetWorth, timeserieslinechart.TimePoint{Time: t, Value: nw})
	}

	chart.DrawAll()

	// Legend line using charm.land/lipgloss/v2 for rendering with the rest of the TUI.
	assetsLegend := lipgloss.NewStyle().Foreground(lipgloss.Color("#7DC4E4")).Render("━━")
	networthLegend := lipgloss.NewStyle().Foreground(lipgloss.Color("#A6E3A1")).Render("━━")
	liabLegend := lipgloss.NewStyle().Foreground(lipgloss.Color("#F38BA8")).Render("━━")
	legend := lipgloss.NewStyle().Bold(true).Render("Net Worth") + "  " +
		assetsLegend + p.styles.Help.Render(" assets  ") +
		networthLegend + p.styles.Help.Render(" net worth  ") +
		liabLegend + p.styles.Help.Render(" liabilities")
	titleLine := lipgloss.NewStyle().Width(p.width).Render(legend)

	return lipgloss.NewStyle().Width(p.width).Height(p.height).Render(
		lipgloss.JoinVertical(lipgloss.Left, titleLine, chart.View()),
	)
}
