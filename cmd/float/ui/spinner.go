package ui

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
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
