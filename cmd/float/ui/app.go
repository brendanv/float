package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

const (
	TabHome    = 0
	TabManager = 1
)

// Model is the root Bubbletea model for the float TUI.
type Model struct {
	width     int
	height    int
	activeTab int
	home      HomeTab
	manager   ManagerTab
	client    floatv1connect.LedgerServiceClient
}

// New creates the root model with the given gRPC client.
func New(client floatv1connect.LedgerServiceClient) Model {
	return Model{
		client:  client,
		home:    NewHomeTab(client),
		manager: NewManagerTab(),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.home.Init(), m.manager.Init())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		layout := CalcLayout(m.width, m.height)
		m.home = m.home.SetSize(m.width, layout.ContentHeight)
		m.manager = m.manager.SetSize(m.width, layout.ContentHeight)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab", "shift+tab":
			m.activeTab = (m.activeTab + 1) % 2
			return m, nil
		}
		// Forward to active tab.
		switch m.activeTab {
		case TabHome:
			var cmd tea.Cmd
			m.home, cmd = m.home.Update(msg)
			return m, cmd
		case TabManager:
			var cmd tea.Cmd
			m.manager, cmd = m.manager.Update(msg)
			return m, cmd
		}
	default:
		var cmd tea.Cmd
		m.home, cmd = m.home.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) View() tea.View {
	var v tea.View
	v.AltScreen = true

	if m.width < 60 || m.height < 15 {
		v.Content = lipgloss.NewStyle().
			Width(m.width).
			Height(m.height).
			Align(lipgloss.Center, lipgloss.Center).
			Render("Terminal too small.\nNeed at least 60×15.")
		return v
	}

	tabBar := RenderTabBar(m.activeTab, m.width)

	var helpCtx HelpContext
	switch m.activeTab {
	case TabHome:
		helpCtx = m.home.HelpContext()
	case TabManager:
		helpCtx = m.manager.HelpContext()
	}
	helpBar := RenderHelpBar(helpCtx, m.width)

	var content string
	switch m.activeTab {
	case TabHome:
		content = m.home.View()
	case TabManager:
		content = m.manager.View()
	}

	v.Content = lipgloss.JoinVertical(lipgloss.Left, tabBar, content, helpBar)
	return v
}
