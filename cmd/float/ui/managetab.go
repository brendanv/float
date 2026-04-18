package ui

import (
	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

const (
	manageSubTabRules   = 0
	manageSubTabImports = 1
	numManageSubTabs    = 2
)

// ManageTab combines the Rules and Imports sub-tabs into a single top-level tab.
type ManageTab struct {
	width        int
	height       int
	styles       Styles
	activeSubTab int
	rules        RulesTab
	imports      ImportsTab
}

func NewManageTab(client floatv1connect.LedgerServiceClient, st Styles) ManageTab {
	return ManageTab{
		styles:       st,
		activeSubTab: manageSubTabRules,
		rules:        NewRulesTab(client, st),
		imports:      NewImportsTab(client, st),
	}
}

func (m ManageTab) setStyles(st Styles) ManageTab {
	m.styles = st
	m.rules = m.rules.setStyles(st)
	m.imports = m.imports.setStyles(st)
	return m
}

func (m ManageTab) SetSize(w, h int) ManageTab {
	m.width = w
	m.height = h
	subBarH := 1
	m.rules = m.rules.SetSize(w, h-subBarH)
	m.imports = m.imports.SetSize(w, h-subBarH)
	return m
}

func (m ManageTab) Init() tea.Cmd {
	return tea.Batch(m.rules.Init(), m.imports.Init())
}

// capturesAllKeys reports whether the active sub-tab is in a mode that should
// receive all key events (form entry, preview, delete confirmation).
func (m ManageTab) capturesAllKeys() bool {
	switch m.activeSubTab {
	case manageSubTabRules:
		return m.rules.mode == rulesModeForm || m.rules.mode == rulesModePreview
	case manageSubTabImports:
		return m.imports.addTxForm.Active() || m.imports.confirmDeleteTx != nil
	}
	return false
}

func (m ManageTab) KeyMap() help.KeyMap {
	var inner help.KeyMap
	switch m.activeSubTab {
	case manageSubTabRules:
		inner = m.rules.KeyMap()
	case manageSubTabImports:
		inner = m.imports.KeyMap()
	default:
		inner = m.rules.KeyMap()
	}
	if m.capturesAllKeys() {
		return inner
	}
	return ManageKeyMap{inner: inner}
}

func (m ManageTab) Update(msg tea.Msg) (ManageTab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if !m.capturesAllKeys() {
			switch msg.String() {
			case "[":
				m.activeSubTab = (m.activeSubTab + numManageSubTabs - 1) % numManageSubTabs
				return m, nil
			case "]":
				m.activeSubTab = (m.activeSubTab + 1) % numManageSubTabs
				return m, nil
			}
		}
		switch m.activeSubTab {
		case manageSubTabRules:
			var cmd tea.Cmd
			m.rules, cmd = m.rules.Update(msg)
			return m, cmd
		case manageSubTabImports:
			var cmd tea.Cmd
			m.imports, cmd = m.imports.Update(msg)
			return m, cmd
		}
	default:
		var cmd1, cmd2 tea.Cmd
		m.rules, cmd1 = m.rules.Update(msg)
		m.imports, cmd2 = m.imports.Update(msg)
		return m, tea.Batch(cmd1, cmd2)
	}
	return m, nil
}

func (m ManageTab) View() string {
	subBar := m.renderSubBar()
	var content string
	switch m.activeSubTab {
	case manageSubTabRules:
		content = m.rules.View()
	case manageSubTabImports:
		content = m.imports.View()
	}
	return lipgloss.JoinVertical(lipgloss.Left, subBar, content)
}

func (m ManageTab) renderSubBar() string {
	labels := []struct {
		label  string
		active bool
	}{
		{"Rules", m.activeSubTab == manageSubTabRules},
		{"Imports", m.activeSubTab == manageSubTabImports},
	}

	var rendered string
	for i, tab := range labels {
		if i > 0 {
			rendered += "  "
		}
		if tab.active {
			rendered += m.styles.TabActive.Render("[ " + tab.label + " ]")
		} else {
			rendered += m.styles.TabInactive.Render(tab.label)
		}
	}
	return lipgloss.NewStyle().Width(m.width).Render(rendered)
}
