package ui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

type FilterInput struct {
	input  textinput.Model
	active bool
	client floatv1connect.LedgerServiceClient
}

func NewFilterInput(client floatv1connect.LedgerServiceClient) FilterInput {
	ti := textinput.New()
	ti.Placeholder = "hledger query..."
	return FilterInput{
		input:  ti,
		client: client,
	}
}

func (f *FilterInput) Activate() {
	f.active = true
	f.input.Focus()
}

func (f *FilterInput) Deactivate() tea.Cmd {
	f.active = false
	f.input.Reset()
	f.input.Blur()
	return FetchTransactions(f.client, nil)
}

func (f FilterInput) Active() bool {
	return f.active
}

func (f FilterInput) Query() []string {
	v := strings.TrimSpace(f.input.Value())
	if v == "" {
		return nil
	}
	return strings.Fields(v)
}

func (f FilterInput) Update(msg tea.Msg) (FilterInput, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			cmd := f.Deactivate()
			return f, cmd
		case "enter":
			return f, FetchTransactions(f.client, f.Query())
		default:
			var cmd tea.Cmd
			f.input, cmd = f.input.Update(msg)
			return f, cmd
		}
	default:
		var cmd tea.Cmd
		f.input, cmd = f.input.Update(msg)
		return f, cmd
	}
}

func (f FilterInput) View() string {
	if !f.active {
		return ""
	}
	return "/ " + f.input.View()
}
