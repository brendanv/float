package ui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
)

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
	keyAddTag     = key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "add tag"))
	keyAddPosting = key.NewBinding(key.WithKeys("ctrl+a"), key.WithHelp("ctrl+a", "add posting"))
	keyDelRow     = key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "del row"))
	keySubmit     = key.NewBinding(key.WithKeys("shift+enter"), key.WithHelp("shift+enter", "submit"))
	keyConfirm    = key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "confirm delete"))
	keySearch     = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "search"))
	keyStatusFilter    = key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "cycle view"))
	keyToggleChart     = key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "toggle chart"))
	keyManageSection   = key.NewBinding(key.WithKeys("[", "]"), key.WithHelp("[/]", "switch section"))
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
	return []key.Binding{keyQuit, keyTab, keyNav, keyEdit, keyReview, keyHelp}
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
		{keyAddTag, keyAddPosting, keyDelRow},
		{keySubmit, keyEsc},
	}
}

// DeleteConfirmKeyMap is for delete confirmation prompts (shared across tabs).
type DeleteConfirmKeyMap struct{}

func (DeleteConfirmKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyConfirm, keyEsc}
}
func (DeleteConfirmKeyMap) FullHelp() [][]key.Binding {
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

// ManagerKeyMap is for the manager tab in tree mode.
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

var keyOpenRegister = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open register"))

// ManagerTreeKeyMap is for the manager tab in tree mode with register navigation.
type ManagerTreeKeyMap struct{}

func (ManagerTreeKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyQuit, keyTab, keyNav, keyOpenRegister, keyHelp}
}
func (ManagerTreeKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyQuit, keyTab, keyShiftTab, keyHelp},
		{keyNav, keyExpand, keyOpenRegister, keyRetry},
	}
}

// ManagerRegisterKeyMap is for the manager tab in account register view mode.
type ManagerRegisterKeyMap struct{}

func (ManagerRegisterKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyQuit, keyNav, keyEdit, keyDelete, keyReview, keyEsc, keyHelp}
}
func (ManagerRegisterKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyQuit, keyTab, keyShiftTab, keyHelp},
		{keyNav, keyEsc, keyRetry},
		{keyEdit, keyDelete, keyReview},
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

var (
	keyPreview = key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "preview apply"))
	keyApply   = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "apply selected"))
	keyToggle  = key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "toggle select"))
	keySelAll  = key.NewBinding(key.WithKeys("ctrl+a"), key.WithHelp("ctrl+a", "select all"))
	keyTest    = key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "test pattern"))
)

// RulesListKeyMap is for the rules tab in list mode.
type RulesListKeyMap struct{}

func (RulesListKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyQuit, keyNav, keyAdd, keyEdit, keyDelete, keyPreview, keyHelp}
}
func (RulesListKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyQuit, keyTab, keyShiftTab, keyHelp},
		{keyNav, keyAdd, keyEdit, keyDelete},
		{keyPreview, keyTest, keyRetry},
	}
}

// RulesFormKeyMap is for the rules tab in add/edit form mode.
type RulesFormKeyMap struct{}

func (RulesFormKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyNextField, keyPrevField, keySubmit, keyEsc}
}
func (RulesFormKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyNextField, keyPrevField},
		{keySubmit, keyEsc},
	}
}

// RulesTesterKeyMap is for the rules tab pattern tester modal.
type RulesTesterKeyMap struct{}

func (RulesTesterKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyEsc}
}
func (RulesTesterKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{keyEsc}}
}

// RulesPreviewKeyMap is for the rules tab in preview/apply mode.
type RulesPreviewKeyMap struct{}

func (RulesPreviewKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyNav, keyToggle, keySelAll, keyApply, keyEsc}
}
func (RulesPreviewKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyNav, keyToggle, keySelAll},
		{keyApply, keyEsc},
	}
}


// ImportsListKeyMap is for the imports tab in list mode.
type ImportsListKeyMap struct{}

func (ImportsListKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyQuit, keyTab, keyNav, keyExpand, keyHelp}
}
func (ImportsListKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyQuit, keyTab, keyShiftTab, keyHelp},
		{keyNav, keyExpand, keyRetry},
	}
}

var keyBack = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))

// ImportsDetailKeyMap is for the imports tab in batch detail mode.
type ImportsDetailKeyMap struct{}

func (ImportsDetailKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyQuit, keyNav, keyEdit, keyDelete, keyReview, keyBack, keyHelp}
}
func (ImportsDetailKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyQuit, keyTab, keyShiftTab, keyHelp},
		{keyNav, keyBack, keyRetry},
		{keyEdit, keyDelete, keyReview, keySplit},
	}
}

// TagsListKeyMap is for the tags tab in list mode.
type TagsListKeyMap struct{}

func (TagsListKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyQuit, keyTab, keyNav, keyExpand, keyHelp}
}
func (TagsListKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyQuit, keyTab, keyShiftTab, keyHelp},
		{keyNav, keyExpand, keyRetry},
	}
}

// TagsDetailKeyMap is for the tags tab when viewing transactions for a tag.
type TagsDetailKeyMap struct{}

func (TagsDetailKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyQuit, keyNav, keySplit, keyBack, keyHelp}
}
func (TagsDetailKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyQuit, keyTab, keyShiftTab, keyHelp},
		{keyNav, keyBack, keySplit, keyRetry},
	}
}

// ManageKeyMap wraps an inner key map (from the active sub-tab) and prepends
// the [/] section-switching binding so it appears in the help bar.
type ManageKeyMap struct {
	inner help.KeyMap
}

func (m ManageKeyMap) ShortHelp() []key.Binding {
	return append([]key.Binding{keyManageSection}, m.inner.ShortHelp()...)
}
func (m ManageKeyMap) FullHelp() [][]key.Binding {
	full := m.inner.FullHelp()
	if len(full) > 0 {
		full[0] = append([]key.Binding{keyManageSection}, full[0]...)
	}
	return full
}

var keyRestore = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "restore"))

// SnapshotsListKeyMap is for the snapshots tab in list mode.
type SnapshotsListKeyMap struct{}

func (SnapshotsListKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyQuit, keyTab, keyNav, keyRestore, keyHelp}
}
func (SnapshotsListKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyQuit, keyTab, keyShiftTab, keyHelp},
		{keyNav, keyRestore, keyRetry},
	}
}

// RestoreConfirmKeyMap is for the restore confirmation dialog.
type RestoreConfirmKeyMap struct{}

func (RestoreConfirmKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyConfirm, keyEsc}
}
func (RestoreConfirmKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{keyConfirm, keyEsc}}
}

var keyBackfill = key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "backfill"))

// PricesListKeyMap is for the prices tab in list mode.
type PricesListKeyMap struct{}

func (PricesListKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyQuit, keyTab, keyNav, keyAdd, keyDelete, keyBackfill, keyHelp}
}
func (PricesListKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyQuit, keyTab, keyShiftTab, keyHelp},
		{keyNav, keyAdd, keyDelete, keyBackfill, keyRetry},
	}
}

// PricesFormKeyMap is for the prices tab add/backfill form modes.
type PricesFormKeyMap struct{}

func (PricesFormKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keySubmit, keyEsc, keyNextField}
}
func (PricesFormKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{keySubmit, keyEsc, keyNextField, keyPrevField}}
}

var keySetPayee = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "set payee"))

// PayeesListKeyMap is for the payees tab when browsing payees or descriptions.
type PayeesListKeyMap struct{}

func (PayeesListKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyQuit, keyTab, keyNav, keySwitch, keyExpand, keyHelp}
}
func (PayeesListKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyQuit, keyTab, keyShiftTab, keyHelp},
		{keyNav, keySwitch, keyExpand, keyRetry},
	}
}

// PayeesTxKeyMap is for the payees tab when viewing transactions for a payee.
type PayeesTxKeyMap struct{}

func (PayeesTxKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyQuit, keyNav, keySplit, keyBack, keyHelp}
}
func (PayeesTxKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyQuit, keyTab, keyShiftTab, keyHelp},
		{keyNav, keyBack, keySplit, keyRetry},
	}
}

// PayeesAssignKeyMap is for the payees tab assign-payee form.
type PayeesAssignKeyMap struct{}

func (PayeesAssignKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keySetPayee, keyEsc}
}
func (PayeesAssignKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{keySetPayee, keyEsc}}
}

// SettingsKeyMap is for the settings tab.
type SettingsKeyMap struct{}

func (SettingsKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keyQuit, keyTab, keyNav, keyHelp}
}
func (SettingsKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyQuit, keyTab, keyShiftTab, keyHelp},
		{keyNav, keyExpand},
	}
}
