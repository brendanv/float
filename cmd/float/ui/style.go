package ui

import (
	"image/color"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Theme identifies which color palette to use.
type Theme int

const (
	ThemeDefault    Theme = iota // auto dark/light detection
	ThemeDracula                 // Dracula color scheme
	ThemeCatppuccin              // Catppuccin Mocha
	ThemeNord                    // Nord
	ThemeEverforest              // Everforest Dark
)

// ThemeNames returns display names for all themes in Theme order.
func ThemeNames() []string {
	return []string{"Default", "Dracula", "Catppuccin", "Nord", "Everforest"}
}

// Styles holds all lipgloss styles for the TUI, built from a specific
// dark-background determination. Create one with NewStyles(hasDark).
// All styles — including transient ones — should be derived from this struct
// rather than constructed directly with lipgloss.NewStyle().
type Styles struct {
	// Base is an empty style. Use it instead of lipgloss.NewStyle() when
	// you need to build a transient colored style inline.
	Base lipgloss.Style

	Border        lipgloss.Style
	FocusedBorder lipgloss.Style
	TabActive     lipgloss.Style
	TabInactive   lipgloss.Style
	Help          lipgloss.Style
	RevenueBar    lipgloss.Style
	InsightsBar   lipgloss.Style

	// Active is the focused/highlighted foreground style (no bold).
	Active lipgloss.Style
	// Error is for error message text.
	Error lipgloss.Style

	// Chart colors for the net worth line chart legend.
	ChartAssets   lipgloss.Style
	ChartNetWorth lipgloss.Style
	ChartLiab     lipgloss.Style

	// Raw colors for cases that need a color.Color value directly.
	BorderFg  color.Color
	FocusedFg color.Color
}

// NewStyles creates a Styles set tuned for dark or light terminal backgrounds.
// Used for ThemeDefault; for other themes call NewStylesWithTheme.
func NewStyles(hasDark bool) Styles {
	return NewStylesWithTheme(ThemeDefault, hasDark)
}

// NewStylesWithTheme creates a Styles set for the given theme.
// hasDark is only used for ThemeDefault (auto dark/light detection); all other
// themes define their own fixed palette.
func NewStylesWithTheme(theme Theme, hasDark bool) Styles {
	switch theme {
	case ThemeDracula:
		return newDraculaStyles()
	case ThemeCatppuccin:
		return newCatppuccinStyles()
	case ThemeNord:
		return newNordStyles()
	case ThemeEverforest:
		return newEverforestStyles()
	default:
		return newDefaultStyles(hasDark)
	}
}

func newDefaultStyles(hasDark bool) Styles {
	ld := lipgloss.LightDark(hasDark)

	borderFg := ld(lipgloss.Color("#A49FA5"), lipgloss.Color("#555555"))
	focusedFg := ld(lipgloss.Color("#6C91BF"), lipgloss.Color("#7DC4E4"))
	tabActiveFg := ld(lipgloss.Color("#1C1C1C"), lipgloss.Color("#EEEEEE"))
	helpFg := ld(lipgloss.Color("#9B9B9B"), lipgloss.Color("#626262"))
	errFg := ld(lipgloss.Color("#CC3333"), lipgloss.Color("#FF5555"))
	chartAssetsFg := ld(lipgloss.Color("#2980B9"), lipgloss.Color("#7DC4E4"))
	chartNwFg := ld(lipgloss.Color("#27AE60"), lipgloss.Color("#A6E3A1"))
	chartLiabFg := ld(lipgloss.Color("#C0392B"), lipgloss.Color("#F38BA8"))

	return Styles{
		Base:          lipgloss.NewStyle(),
		Border:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(borderFg),
		FocusedBorder: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(focusedFg),
		TabActive:     lipgloss.NewStyle().Bold(true).Foreground(tabActiveFg),
		TabInactive:   lipgloss.NewStyle().Foreground(helpFg),
		Help:          lipgloss.NewStyle().Foreground(helpFg),
		RevenueBar:    lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#3D7A4A"), lipgloss.Color("#A6E3A1"))),
		InsightsBar:   lipgloss.NewStyle().Foreground(focusedFg),
		Active:        lipgloss.NewStyle().Foreground(focusedFg),
		Error:         lipgloss.NewStyle().Foreground(errFg),
		ChartAssets:   lipgloss.NewStyle().Foreground(chartAssetsFg),
		ChartNetWorth: lipgloss.NewStyle().Foreground(chartNwFg),
		ChartLiab:     lipgloss.NewStyle().Foreground(chartLiabFg),
		BorderFg:      borderFg,
		FocusedFg:     focusedFg,
	}
}

// newDraculaStyles returns the Dracula color palette.
func newDraculaStyles() Styles {
	borderFg := lipgloss.Color("#6272A4") // comment
	focusedFg := lipgloss.Color("#BD93F9") // purple

	return Styles{
		Base:          lipgloss.NewStyle(),
		Border:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(borderFg),
		FocusedBorder: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(focusedFg),
		TabActive:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F8F8F2")),
		TabInactive:   lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4")),
		Help:          lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4")),
		RevenueBar:    lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")),
		InsightsBar:   lipgloss.NewStyle().Foreground(focusedFg),
		Active:        lipgloss.NewStyle().Foreground(focusedFg),
		Error:         lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")),
		ChartAssets:   lipgloss.NewStyle().Foreground(lipgloss.Color("#8BE9FD")),
		ChartNetWorth: lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")),
		ChartLiab:     lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")),
		BorderFg:      borderFg,
		FocusedFg:     focusedFg,
	}
}

// newCatppuccinStyles returns the Catppuccin Mocha color palette.
func newCatppuccinStyles() Styles {
	borderFg := lipgloss.Color("#6C7086") // overlay0
	focusedFg := lipgloss.Color("#89B4FA") // blue

	return Styles{
		Base:          lipgloss.NewStyle(),
		Border:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(borderFg),
		FocusedBorder: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(focusedFg),
		TabActive:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#CDD6F4")),
		TabInactive:   lipgloss.NewStyle().Foreground(lipgloss.Color("#6C7086")),
		Help:          lipgloss.NewStyle().Foreground(lipgloss.Color("#6C7086")),
		RevenueBar:    lipgloss.NewStyle().Foreground(lipgloss.Color("#A6E3A1")),
		InsightsBar:   lipgloss.NewStyle().Foreground(focusedFg),
		Active:        lipgloss.NewStyle().Foreground(focusedFg),
		Error:         lipgloss.NewStyle().Foreground(lipgloss.Color("#F38BA8")),
		ChartAssets:   lipgloss.NewStyle().Foreground(lipgloss.Color("#89B4FA")),
		ChartNetWorth: lipgloss.NewStyle().Foreground(lipgloss.Color("#A6E3A1")),
		ChartLiab:     lipgloss.NewStyle().Foreground(lipgloss.Color("#F38BA8")),
		BorderFg:      borderFg,
		FocusedFg:     focusedFg,
	}
}

// newNordStyles returns the Nord color palette.
func newNordStyles() Styles {
	borderFg := lipgloss.Color("#4C566A") // nord3
	focusedFg := lipgloss.Color("#88C0D0") // nord8

	return Styles{
		Base:          lipgloss.NewStyle(),
		Border:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(borderFg),
		FocusedBorder: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(focusedFg),
		TabActive:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ECEFF4")),
		TabInactive:   lipgloss.NewStyle().Foreground(lipgloss.Color("#4C566A")),
		Help:          lipgloss.NewStyle().Foreground(lipgloss.Color("#4C566A")),
		RevenueBar:    lipgloss.NewStyle().Foreground(lipgloss.Color("#A3BE8C")),
		InsightsBar:   lipgloss.NewStyle().Foreground(focusedFg),
		Active:        lipgloss.NewStyle().Foreground(focusedFg),
		Error:         lipgloss.NewStyle().Foreground(lipgloss.Color("#BF616A")),
		ChartAssets:   lipgloss.NewStyle().Foreground(lipgloss.Color("#88C0D0")),
		ChartNetWorth: lipgloss.NewStyle().Foreground(lipgloss.Color("#A3BE8C")),
		ChartLiab:     lipgloss.NewStyle().Foreground(lipgloss.Color("#BF616A")),
		BorderFg:      borderFg,
		FocusedFg:     focusedFg,
	}
}

// newEverforestStyles returns the Everforest Dark color palette.
func newEverforestStyles() Styles {
	borderFg := lipgloss.Color("#5C6A72")
	focusedFg := lipgloss.Color("#7FBBB3") // blue

	return Styles{
		Base:          lipgloss.NewStyle(),
		Border:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(borderFg),
		FocusedBorder: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(focusedFg),
		TabActive:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D3C6AA")),
		TabInactive:   lipgloss.NewStyle().Foreground(lipgloss.Color("#7A8478")),
		Help:          lipgloss.NewStyle().Foreground(lipgloss.Color("#7A8478")),
		RevenueBar:    lipgloss.NewStyle().Foreground(lipgloss.Color("#A7C080")),
		InsightsBar:   lipgloss.NewStyle().Foreground(focusedFg),
		Active:        lipgloss.NewStyle().Foreground(focusedFg),
		Error:         lipgloss.NewStyle().Foreground(lipgloss.Color("#E67E80")),
		ChartAssets:   lipgloss.NewStyle().Foreground(lipgloss.Color("#7FBBB3")),
		ChartNetWorth: lipgloss.NewStyle().Foreground(lipgloss.Color("#A7C080")),
		ChartLiab:     lipgloss.NewStyle().Foreground(lipgloss.Color("#E67E80")),
		BorderFg:      borderFg,
		FocusedFg:     focusedFg,
	}
}

// injectBorderTitle post-processes a rendered panel string (with a rounded
// border) to embed a title in the top border. The title color matches the
// border color (focused or unfocused).
func injectBorderTitle(rendered, title string, focused bool, st Styles) string {
	if title == "" {
		return rendered
	}
	lines := strings.Split(rendered, "\n")
	if len(lines) == 0 {
		return rendered
	}
	topLine := lines[0]
	totalWidth := ansi.StringWidth(topLine)
	if totalWidth < 4 {
		return rendered
	}

	// Build: ╭─ Title ──────────────╮
	titlePart := "─ " + title + " "
	titleWidth := ansi.StringWidth(titlePart)
	remaining := totalWidth - 2 - titleWidth // -2 for ╭ and ╮
	if remaining < 1 {
		return rendered // title doesn't fit, keep as-is
	}

	newTop := "╭" + titlePart + strings.Repeat("─", remaining) + "╮"

	var fg color.Color
	if focused {
		fg = st.FocusedFg
	} else {
		fg = st.BorderFg
	}
	lines[0] = st.Base.Foreground(fg).Render(newTop)
	return strings.Join(lines, "\n")
}

// NewHelpModel returns a help.Model styled with the app's help color palette.
func NewHelpModel(st Styles) help.Model {
	h := help.New()
	s := h.Styles
	s.ShortKey = s.ShortKey.Foreground(st.Help.GetForeground())
	s.ShortDesc = s.ShortDesc.Foreground(st.Help.GetForeground())
	s.ShortSeparator = s.ShortSeparator.Foreground(st.Help.GetForeground())
	s.FullKey = s.FullKey.Foreground(st.Help.GetForeground())
	s.FullDesc = s.FullDesc.Foreground(st.Help.GetForeground())
	s.FullSeparator = s.FullSeparator.Foreground(st.Help.GetForeground())
	s.Ellipsis = s.Ellipsis.Foreground(st.Help.GetForeground())
	h.Styles = s
	return h
}
