package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PeriodChangedMsg is emitted when the user navigates to a new period.
type PeriodChangedMsg struct{}

// PeriodSelector is a 1-line month/year navigator rendered as "<<< March 2026 >>>".
type PeriodSelector struct {
	year  int
	month time.Month
	width int
}

// NewPeriodSelector initialises the selector to the current calendar month.
func NewPeriodSelector() PeriodSelector {
	now := time.Now()
	return PeriodSelector{year: now.Year(), month: now.Month()}
}

// Query returns an hledger date filter for the current period, e.g. "date:2026-03".
func (p PeriodSelector) Query() string {
	return fmt.Sprintf("date:%d-%02d", p.year, int(p.month))
}

// SetWidth stores the available width for rendering.
func (p *PeriodSelector) SetWidth(w int) { p.width = w }

// Update handles [ and ] key presses. Returns a PeriodChangedMsg cmd on change.
func (p PeriodSelector) Update(msg tea.KeyMsg) (PeriodSelector, tea.Cmd) {
	switch msg.String() {
	case "[":
		p.month--
		if p.month < time.January {
			p.month = time.December
			p.year--
		}
		return p, func() tea.Msg { return PeriodChangedMsg{} }
	case "]":
		p.month++
		if p.month > time.December {
			p.month = time.January
			p.year++
		}
		return p, func() tea.Msg { return PeriodChangedMsg{} }
	}
	return p, nil
}

// View renders the period selector as a centred 1-line string.
func (p PeriodSelector) View() string {
	label := fmt.Sprintf("<<< %s %d >>>", p.month.String(), p.year)
	return lipgloss.NewStyle().Width(p.width).Align(lipgloss.Center).Render(label)
}
