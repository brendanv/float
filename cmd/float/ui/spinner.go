package ui

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

type Spinner struct {
	s spinner.Model
}

func NewSpinner() Spinner {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Spinner{s: s}
}

func (s *Spinner) Update(msg tea.Msg) tea.Cmd {
	if _, ok := msg.(spinner.TickMsg); !ok {
		return nil
	}
	var cmd tea.Cmd
	s.s, cmd = s.s.Update(msg)
	return cmd
}

func (s Spinner) View() string {
	return s.s.View()
}

func (s Spinner) Tick() tea.Cmd {
	return s.s.Tick
}
