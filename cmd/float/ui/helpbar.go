package ui

import "charm.land/bubbles/v2/key"

// Key bindings used across contexts.
var (
	keyQuit       = key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit"))
	keyTab        = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab"))
	keyShiftTab   = key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev tab"))
	keyHelp       = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "toggle help"))
	keyNav        = key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "navigate"))
	keySwitch     = key.NewBinding(key.WithKeys("h", "l"), key.WithHelp("h/l", "switch panel"))
	keyAdd        = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add"))
	keyEdit       = key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit"))
	keyDelete     = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete"))
	keyReview     = key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "review"))
	keyFilter     = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter"))
	keySplit      = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "split"))
	keyPeriod     = key.NewBinding(key.WithKeys("[", "]"), key.WithHelp("[/]", "period"))
	keyRetry      = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "retry"))
	keyExpand     = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "expand/collapse"))
	keyEsc        = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel"))
	keyNextField  = key.NewBinding(key.WithKeys("tab", "enter"), key.WithHelp("tab/enter", "next field"))
	keyPrevField  = key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev field"))
	keyAddPosting = key.NewBinding(key.WithKeys("ctrl+a"), key.WithHelp("ctrl+a", "add posting"))
	keyDelPosting = key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "del posting"))
	keySubmit     = key.NewBinding(key.WithKeys("shift+enter"), key.WithHelp("shift+enter", "submit"))
	keyConfirm    = key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "confirm delete"))
	keySearch     = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "search"))
	keyStatusFilter = key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "cycle view"))
	keyToggleChart  = key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "toggle chart"))
)

// HomeChartKeyMap is for the home tab with the chart panel focused.
type HomeChartKeyMap struct{}

func (HomeChartKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyQuit, keyTab, keyToggleChart, keyPeriod, keyHelp}
}
func (HomeChartKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyQuit, keyTab, keyShiftTab, keyHelp},
		{keySwitch, keyToggleChart, keyPeriod, keyRetry},
	}
}

// HomeAccountsKeyMap is for the home tab with the accounts panel focused.
type HomeAccountsKeyMap struct{}

func (HomeAccountsKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyQuit, keyTab, keyNav, keyPeriod, keyHelp}
}
func (HomeAccountsKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyQuit, keyTab, keyShiftTab, keyHelp},
		{keySwitch, keyNav, keyPeriod, keyRetry},
	}
}

// HomeUnreviewedKeyMap is for the home tab with the transaction review panel focused.
type HomeUnreviewedKeyMap struct{}

func (HomeUnreviewedKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyQuit, keyNav, keyAdd, keyEdit, keyDelete, keyReview, keyHelp}
}
func (HomeUnreviewedKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyQuit, keyTab, keyShiftTab, keyHelp},
		{keySwitch, keyNav},
		{keyAdd, keyEdit, keyDelete, keyReview, keySplit},
		{keyStatusFilter, keyFilter, keyRetry},
	}
}

// HomeFormKeyMap is for the add/edit transaction form.
type HomeFormKeyMap struct{}

func (HomeFormKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyNextField, keyPrevField, keySubmit, keyEsc}
}
func (HomeFormKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyNextField, keyPrevField},
		{keyAddPosting, keyDelPosting},
		{keySubmit, keyEsc},
	}
}

// HomeDeleteKeyMap is for the delete confirmation prompt.
type HomeDeleteKeyMap struct{}

func (HomeDeleteKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyConfirm, keyEsc}
}
func (HomeDeleteKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{keyConfirm, keyEsc}}
}

// HomeFilterKeyMap is for the filter input.
type HomeFilterKeyMap struct{}

func (HomeFilterKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keySearch, keyEsc}
}
func (HomeFilterKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{keySearch, keyEsc}}
}

// ManagerKeyMap is for the manager tab.
type ManagerKeyMap struct{}

func (ManagerKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyQuit, keyTab, keyNav, keyExpand, keyHelp}
}
func (ManagerKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyQuit, keyTab, keyShiftTab, keyHelp},
		{keyNav, keyExpand, keyRetry},
	}
}

// TrendsKeyMap is for the trends tab.
type TrendsKeyMap struct{}

func (TrendsKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyQuit, keyTab, keyHelp}
}
func (TrendsKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyQuit, keyTab, keyShiftTab, keyHelp},
	}
}
