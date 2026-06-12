package ui

import (
	"context"
	"errors"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"connectrpc.com/connect"
)

// gateState tracks the AuthGate lifecycle.
type gateState int

const (
	gateProbing gateState = iota
	gatePrompting
	gateDone
)

type probeResultMsg struct{ err error }

type loginResultMsg struct {
	token string
	err   error
}

// AuthGate is a root Bubble Tea model that gates the float TUI behind a
// passphrase prompt when the server requires authentication. It probes the
// server with a cheap RPC; on CodeUnauthenticated it prompts for the
// passphrase, exchanges it for a session token via login, and only then
// initializes the wrapped Model. Probe failures other than unauthenticated
// (e.g. connection refused) fall through to the inner UI, which surfaces
// them per-tab as before.
type AuthGate struct {
	inner   Model
	state   gateState
	probe   func(ctx context.Context) error                            // nil skips the probe and prompts immediately
	login   func(ctx context.Context, passphrase string) (string, error)
	onToken func(token string)
	input   textinput.Model
	errMsg  string
	busy    bool
	width   int
	height  int
}

// NewAuthGate wraps inner behind a passphrase prompt. probe checks whether
// the current credentials (if any) are accepted; a nil probe goes straight to
// the prompt. login exchanges a passphrase for a session token. onToken is
// called with the token before the inner model is initialized.
func NewAuthGate(
	inner Model,
	probe func(ctx context.Context) error,
	login func(ctx context.Context, passphrase string) (string, error),
	onToken func(token string),
) AuthGate {
	in := textinput.New()
	in.Placeholder = "passphrase"
	in.EchoMode = textinput.EchoPassword
	in.SetWidth(40)
	state := gateProbing
	if probe == nil {
		state = gatePrompting
	}
	return AuthGate{
		inner:   inner,
		state:   state,
		probe:   probe,
		login:   login,
		onToken: onToken,
		input:   in,
	}
}

func (g AuthGate) Init() tea.Cmd {
	switch g.state {
	case gateProbing:
		probe := g.probe
		return func() tea.Msg {
			return probeResultMsg{err: probe(context.Background())}
		}
	case gatePrompting:
		return g.input.Focus()
	default:
		return g.inner.Init()
	}
}

func (g AuthGate) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if g.state == gateDone {
		inner, cmd := g.inner.Update(msg)
		g.inner = inner.(Model)
		return g, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		g.width = msg.Width
		g.height = msg.Height
		// Keep the inner model's layout current so it renders correctly the
		// moment the gate resolves.
		inner, _ := g.inner.Update(msg)
		g.inner = inner.(Model)
		return g, nil

	case tea.BackgroundColorMsg:
		inner, _ := g.inner.Update(msg)
		g.inner = inner.(Model)
		return g, nil

	case probeResultMsg:
		var connectErr *connect.Error
		if errors.As(msg.err, &connectErr) && connectErr.Code() == connect.CodeUnauthenticated {
			g.state = gatePrompting
			return g, g.input.Focus()
		}
		// Success or a non-auth error (e.g. server down): enter the UI.
		g.state = gateDone
		return g, g.inner.Init()

	case loginResultMsg:
		g.busy = false
		if msg.err != nil {
			g.errMsg = msg.err.Error()
			g.input.SetValue("")
			return g, nil
		}
		if g.onToken != nil {
			g.onToken(msg.token)
		}
		g.state = gateDone
		return g, g.inner.Init()

	case tea.KeyMsg:
		if g.state != gatePrompting || g.busy {
			if msg.String() == "ctrl+c" {
				return g, tea.Quit
			}
			return g, nil
		}
		switch msg.String() {
		case "ctrl+c", "esc":
			return g, tea.Quit
		case "enter":
			passphrase := g.input.Value()
			if passphrase == "" {
				return g, nil
			}
			g.busy = true
			g.errMsg = ""
			login := g.login
			return g, func() tea.Msg {
				token, err := login(context.Background(), passphrase)
				return loginResultMsg{token: token, err: err}
			}
		}
		var cmd tea.Cmd
		g.input, cmd = g.input.Update(msg)
		return g, cmd
	}

	if g.state == gatePrompting {
		var cmd tea.Cmd
		g.input, cmd = g.input.Update(msg)
		return g, cmd
	}
	return g, nil
}

func (g AuthGate) View() tea.View {
	if g.state == gateDone {
		return g.inner.View()
	}

	var v tea.View
	v.AltScreen = true

	var body string
	switch g.state {
	case gateProbing:
		body = "Connecting…"
	default:
		title := "float — passphrase required"
		prompt := g.input.View()
		hint := "enter to submit · esc to quit"
		if g.busy {
			hint = "checking…"
		}
		lines := []string{title, "", prompt, "", hint}
		if g.errMsg != "" {
			lines = append(lines, "", g.errMsg)
		}
		body = lipgloss.JoinVertical(lipgloss.Left, lines...)
	}

	v.Content = lipgloss.NewStyle().
		Width(g.width).
		Height(g.height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(body)
	return v
}
