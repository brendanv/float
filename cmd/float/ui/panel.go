package ui

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type loadState int

const (
	stateLoading loadState = iota
	stateLoaded
	stateError
)

// panelBase holds the common state and behavior shared by all data panels:
// loading/error state, a spinner, and the error message. Panels embed this
// struct to get loading and error rendering for free.
type panelBase struct {
	width, height int
	state         loadState
	spinner       Spinner
	errMsg        string
}

func newPanelBase() panelBase {
	return panelBase{
		state:   stateLoading,
		spinner: NewSpinner(),
	}
}

func (p *panelBase) SetError(msg string) {
	p.errMsg = msg
	p.state = stateError
}

// handleSpinnerTick forwards spinner tick messages to the spinner and returns
// the resulting command. Returns nil for non-tick messages.
func (p *panelBase) handleSpinnerTick(msg tea.Msg) tea.Cmd {
	if sm, ok := msg.(spinner.TickMsg); ok {
		return p.spinner.Update(sm)
	}
	return nil
}

// renderLoading returns a centered spinner view sized to the panel dimensions.
func (p panelBase) renderLoading() string {
	return lipgloss.NewStyle().
		Width(p.width).Height(p.height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(p.spinner.View())
}

// renderError returns a centered error message. Pass withRetryHint=true for
// interactive panels that support 'r' to retry.
func (p panelBase) renderError(withRetryHint bool) string {
	msg := "! " + p.errMsg
	if withRetryHint {
		msg += "\n\nPress r to retry"
	}
	return lipgloss.NewStyle().
		Width(p.width).Height(p.height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(msg)
}
