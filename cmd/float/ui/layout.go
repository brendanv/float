package ui

// Layout holds computed panel dimensions for a given terminal size.
type Layout struct {
	ContentHeight int // gross height for tab content (total minus tabbar and helpbar)
}

// CalcLayout computes panel dimensions for a terminal of size h with the given helpbar height.
// ContentHeight = h - 1 (tabbar) - helpHeight. Each tab is responsible for its own column split.
func CalcLayout(h, helpHeight int) Layout {
	content := h - 1 - helpHeight
	if content < 0 {
		content = 0
	}
	return Layout{
		ContentHeight: content,
	}
}

