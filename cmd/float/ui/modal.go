package ui

import "charm.land/lipgloss/v2"

const (
	modalHorizPad = 2
	modalVertPad  = 1
	minModalWidth = 20
)

// RenderModal renders content in a centered rounded-border modal over a
// finance-character background ($¢£¥€) that fills width×height.
// title is injected into the top border via injectBorderTitle.
func RenderModal(width, height int, title, content string, st Styles) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	modalW := calcModalWidth(width)
	border := st.FocusedBorder.Padding(modalVertPad, modalHorizPad)
	innerW := modalW - border.GetHorizontalFrameSize()
	if innerW < 1 {
		innerW = 1
	}
	wrapped := st.Base.Width(innerW).Render(content)
	box := border.MaxWidth(modalW).Render(wrapped)
	box = injectBorderTitle(box, title, true, st)
	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		box,
		lipgloss.WithWhitespaceChars("$¢£¥€"),
		lipgloss.WithWhitespaceStyle(st.Base.Foreground(st.BorderFg)),
	)
}

func calcModalWidth(screenW int) int {
	offset := screenW / 6
	if offset < 4 {
		offset = 4
	}
	if offset > 40 {
		offset = 40
	}
	w := screenW - offset
	if w < minModalWidth {
		w = minModalWidth
	}
	if w > screenW {
		w = screenW
	}
	return w
}
