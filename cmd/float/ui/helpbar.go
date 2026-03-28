package ui

// HelpContext carries the state needed to render context-sensitive help.
type HelpContext struct {
	ActiveTab           int
	HomeFocused         int  // 0 = accounts (left), 1 = transactions (right)
	FilterActive        bool
	AddTxActive         bool
	EditTxActive        bool
	ConfirmDeleteActive bool
}

// RenderHelpBar returns a 1-line help string appropriate for the current context.
func RenderHelpBar(ctx HelpContext, width int) string {
	var help string
	switch {
	case ctx.ActiveTab == TabHome && (ctx.AddTxActive || ctx.EditTxActive):
		help = "  tab/enter next  shift+tab prev  ctrl+a add posting  ctrl+d del posting  shift+enter submit  esc cancel"
	case ctx.ActiveTab == TabHome && ctx.ConfirmDeleteActive:
		help = "  y confirm delete  esc cancel"
	case ctx.ActiveTab == TabHome && ctx.FilterActive:
		help = "  enter search  esc cancel"
	case ctx.ActiveTab == TabHome && ctx.HomeFocused == 1:
		help = "  q quit  tab tabs  h/l switch  j/k navigate  a add  e edit  d delete  / filter  s split  [/] period  r retry"
	case ctx.ActiveTab == TabHome:
		help = "  q quit  tab tabs  h/l switch  j/k navigate  [/] period  r retry"
	default:
		help = "  q quit  tab tabs"
	}
	return HelpStyle.Width(width).Render(help)
}
