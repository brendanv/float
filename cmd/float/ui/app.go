package ui

import (
	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

const (
	TabHome     = 0
	TabAccounts = 1
	TabTrends   = 2
	TabManage   = 3
	TabSettings = 4
	numTabs     = 5
)

// Model is the root Bubbletea model for the float TUI.
type Model struct {
	width     int
	height    int
	activeTab int
	hasDark   bool
	theme     Theme
	styles    Styles
	helpModel help.Model
	home      HomeTab
	manager   ManagerTab
	trends    TrendsTab
	manage    ManageTab
	settings  SettingsTab
	client    floatv1connect.LedgerServiceClient
}

// New creates the root model with the given gRPC client.
// Styles default to dark-background until BackgroundColorMsg is received.
// The saved theme is loaded from the TUI config file.
func New(client floatv1connect.LedgerServiceClient) Model {
	theme := LoadTUITheme()
	st := NewStylesWithTheme(theme, true)
	return Model{
		client:    client,
		hasDark:   true,
		theme:     theme,
		styles:    st,
		helpModel: NewHelpModel(st),
		home:     NewHomeTab(client, st),
		manager:  NewManagerTab(client, st),
		trends:   NewTrendsTab(client, st),
		manage:   NewManageTab(client, st),
		settings: NewSettingsTab(st, theme),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestBackgroundColor,
		m.home.Init(),
		m.manager.Init(),
		m.trends.Init(),
		m.manage.Init(),
		m.settings.Init(),
	)
}

// activeKeyMap returns the help.KeyMap for the currently active tab.
func (m Model) activeKeyMap() help.KeyMap {
	switch m.activeTab {
	case TabHome:
		return m.home.KeyMap()
	case TabAccounts:
		return m.manager.KeyMap()
	case TabManage:
		return m.manage.KeyMap()
	case TabSettings:
		return m.settings.KeyMap()
	default:
		return m.trends.KeyMap()
	}
}

// resizeAll recomputes the layout using the current help height and updates
// all tab sizes accordingly.
func (m *Model) resizeAll() {
	m.helpModel.SetWidth(m.width)
	helpRendered := m.helpModel.View(m.activeKeyMap())
	helpH := lipgloss.Height(helpRendered)
	layout := CalcLayout(m.width, m.height, helpH)
	m.home = m.home.SetSize(m.width, layout.ContentHeight)
	m.manager = m.manager.SetSize(m.width, layout.ContentHeight)
	m.trends = m.trends.SetSize(m.width, layout.ContentHeight)
	m.manage = m.manage.SetSize(m.width, layout.ContentHeight)
	m.settings = m.settings.SetSize(m.width, layout.ContentHeight)
}

// applyStyles rebuilds and propagates styles for the current theme/hasDark.
func (m *Model) applyStyles() {
	m.styles = NewStylesWithTheme(m.theme, m.hasDark)
	m.helpModel = NewHelpModel(m.styles)
	m.home = m.home.setStyles(m.styles)
	m.manager = m.manager.setStyles(m.styles)
	m.trends = m.trends.setStyles(m.styles)
	m.manage = m.manage.setStyles(m.styles)
	m.settings = m.settings.setStyles(m.styles)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		if dark := msg.IsDark(); dark != m.hasDark {
			m.hasDark = dark
			m.applyStyles()
			m.resizeAll() // border widths are style-independent, but triggers re-layout
		}
		return m, nil

	case ThemeSelectedMsg:
		m.theme = msg.Theme
		saveTUITheme(m.theme)
		m.applyStyles()
		m.settings = m.settings.setApplied(m.theme)
		m.resizeAll()
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeAll()
		return m, nil

	case tea.KeyMsg:
		// When the add/edit-transaction form or delete confirmation is active in
		// the home tab, let it consume all key events (including tab/shift+tab
		// for field navigation and q/ctrl+c which should not quit while editing).
		if m.activeTab == TabHome && (m.home.addTxForm.Active() || m.home.confirmDeleteTx != nil) {
			var cmd tea.Cmd
			m.home, cmd = m.home.Update(msg)
			return m, cmd
		}
		// When the manager tab has the edit form or delete confirmation active, let it
		// consume all key events (tab/shift+tab for field nav, q should not quit).
		if m.activeTab == TabAccounts && (m.manager.addTxForm.Active() || m.manager.confirmDeleteRow != nil) {
			var cmd tea.Cmd
			m.manager, cmd = m.manager.Update(msg)
			return m, cmd
		}
		// When the manage tab's active sub-tab is in a mode that needs all key events.
		if m.activeTab == TabManage && m.manage.capturesAllKeys() {
			var cmd tea.Cmd
			m.manage, cmd = m.manage.Update(msg)
			return m, cmd
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.activeTab = (m.activeTab + 1) % numTabs
		case "shift+tab":
			m.activeTab = (m.activeTab + numTabs - 1) % numTabs
			return m, nil
		case "?":
			m.helpModel.ShowAll = !m.helpModel.ShowAll
			m.resizeAll()
			return m, nil
		}
		// Forward to active tab.
		switch m.activeTab {
		case TabHome:
			var cmd tea.Cmd
			m.home, cmd = m.home.Update(msg)
			return m, cmd
		case TabAccounts:
			var cmd tea.Cmd
			m.manager, cmd = m.manager.Update(msg)
			return m, cmd
		case TabTrends:
			var cmd tea.Cmd
			m.trends, cmd = m.trends.Update(msg)
			return m, cmd
		case TabManage:
			var cmd tea.Cmd
			m.manage, cmd = m.manage.Update(msg)
			return m, cmd
		case TabSettings:
			var cmd tea.Cmd
			m.settings, cmd = m.settings.Update(msg)
			return m, cmd
		}
	default:
		var cmd1, cmd2, cmd3, cmd4, cmd5 tea.Cmd
		m.home, cmd1 = m.home.Update(msg)
		m.manager, cmd2 = m.manager.Update(msg)
		m.trends, cmd3 = m.trends.Update(msg)
		m.manage, cmd4 = m.manage.Update(msg)
		m.settings, cmd5 = m.settings.Update(msg)
		return m, tea.Batch(cmd1, cmd2, cmd3, cmd4, cmd5)
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

	tabBar := RenderTabBar(m.activeTab, m.width, m.styles)
	helpBar := m.helpModel.View(m.activeKeyMap())

	var content string
	switch m.activeTab {
	case TabHome:
		content = m.home.View()
	case TabAccounts:
		content = m.manager.View()
	case TabTrends:
		content = m.trends.View()
	case TabManage:
		content = m.manage.View()
	case TabSettings:
		content = m.settings.View()
	}

	v.Content = lipgloss.JoinVertical(lipgloss.Left, tabBar, content, helpBar)
	return v
}
