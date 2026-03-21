package ui

// Layout holds computed panel dimensions for a given terminal size.
type Layout struct {
	LeftWidth     int // gross outer width for left column
	RightWidth    int // gross outer width for right column
	ContentHeight int // gross height for tab content (total minus tabbar and helpbar)
}

// CalcLayout computes panel dimensions for a terminal of size w×h.
// Left column width = clamp(30% of w, 25, 45). Right = remainder.
// ContentHeight = h - 2 (1 line for tabbar, 1 for helpbar).
func CalcLayout(w, h int) Layout {
	left := clamp(w*30/100, 25, 45)
	right := w - left
	if right < 0 {
		right = 0
	}
	content := h - 2
	if content < 0 {
		content = 0
	}
	return Layout{
		LeftWidth:     left,
		RightWidth:    right,
		ContentHeight: content,
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
