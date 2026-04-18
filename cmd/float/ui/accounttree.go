package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	floatv1 "github.com/brendanv/float/gen/float/v1"
)

// treeNode represents one node in the hierarchical account tree.
type treeNode struct {
	segment  string // last path segment displayed (e.g. "checking")
	fullName string // full colon path (e.g. "assets:checking")
	depth    int    // nesting level (0 = top-level account type root)
	children []*treeNode
	expanded bool // collapsed or expanded; top-level nodes start expanded
}

// isLeaf returns true when the node has no children.
func (n *treeNode) isLeaf() bool { return len(n.children) == 0 }

// childMap builds a segment → node map from a node's existing children.
func childMap(n *treeNode) map[string]*treeNode {
	m := make(map[string]*treeNode, len(n.children))
	for _, c := range n.children {
		m[c.segment] = c
	}
	return m
}

// sortChildren recursively sorts a node's children alphabetically by segment.
func sortChildren(n *treeNode) {
	sort.Slice(n.children, func(i, j int) bool {
		return n.children[i].segment < n.children[j].segment
	})
	for _, c := range n.children {
		sortChildren(c)
	}
}

// AccountTree is the interactive hierarchical account panel.
type AccountTree struct {
	panelBase
	roots    []*treeNode               // top-level nodes in accountTypeOrder
	balances map[string][]*floatv1.Amount // keyed by full account name
	flat     []*treeNode               // flattened visible nodes (rebuilt on toggle)
	cursor   int                       // index into flat
	offset   int                       // first visible row (viewport scroll)
}

func NewAccountTree() AccountTree {
	return AccountTree{
		panelBase: newPanelBase(),
		balances:  make(map[string][]*floatv1.Amount),
	}
}

func (t *AccountTree) SetSize(w, h int) {
	t.width = w
	t.height = h
	t.clampOffset()
}

// SetAccounts builds the tree from a flat account list.
func (t *AccountTree) SetAccounts(accounts []*floatv1.Account) {
	// Per-type root maps: typeCode → (segment → *treeNode).
	// Each type gets its own independent tree rooted at the first path segment.
	type nodeMapT map[string]*treeNode
	rootMaps := make(map[string]nodeMapT)
	for _, tc := range accountTypeOrder {
		rootMaps[tc] = make(nodeMapT)
	}

	for _, a := range accounts {
		segments := strings.Split(a.FullName, ":")
		if len(segments) == 0 {
			continue
		}
		typeCode := a.Type
		if _, ok := rootMaps[typeCode]; !ok {
			rootMaps[typeCode] = make(nodeMapT)
		}

		// Walk segments, creating or reusing nodes at each level.
		currentMap := rootMaps[typeCode]
		var parent *treeNode
		for depth, seg := range segments {
			fullSoFar := strings.Join(segments[:depth+1], ":")
			nd, exists := currentMap[seg]
			if !exists {
				nd = &treeNode{
					segment:  seg,
					fullName: fullSoFar,
					depth:    depth,
					expanded: true, // all nodes start expanded
				}
				currentMap[seg] = nd
				if parent != nil {
					parent.children = append(parent.children, nd)
				}
			}
			parent = nd
			currentMap = childMap(nd)
		}
	}

	// Linearise roots by accountTypeOrder and sort children.
	t.roots = nil
	for _, tc := range accountTypeOrder {
		nm := rootMaps[tc]
		segs := make([]string, 0, len(nm))
		for seg := range nm {
			segs = append(segs, seg)
		}
		sort.Strings(segs)
		for _, seg := range segs {
			nd := nm[seg]
			sortChildren(nd)
			t.roots = append(t.roots, nd)
		}
	}

	if t.state != stateError {
		t.state = stateLoaded
	}
	t.rebuildFlat()
}

// SetBalances stores balance amounts keyed by full account name for display in the tree.
func (t *AccountTree) SetBalances(report *floatv1.BalanceReport) {
	if report == nil {
		return
	}
	for _, row := range report.Rows {
		t.balances[row.FullName] = row.Amounts
	}
}

// rebuildFlat regenerates the flattened visible node list after any structural change.
func (t *AccountTree) rebuildFlat() {
	t.flat = nil
	for _, root := range t.roots {
		collectVisible(root, &t.flat)
	}
	// Clamp cursor.
	if t.cursor >= len(t.flat) {
		t.cursor = len(t.flat) - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
	t.clampOffset()
}

func collectVisible(n *treeNode, out *[]*treeNode) {
	*out = append(*out, n)
	if n.expanded {
		for _, child := range n.children {
			collectVisible(child, out)
		}
	}
}

// clampOffset adjusts the scroll offset so the cursor remains in the viewport.
func (t *AccountTree) clampOffset() {
	visible := t.height
	if visible <= 0 {
		visible = 1
	}
	if t.cursor >= t.offset+visible {
		t.offset = t.cursor - visible + 1
	}
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.offset < 0 {
		t.offset = 0
	}
}

// SelectedAccount returns the full account name of the currently highlighted node,
// or "" if the tree is empty or not loaded.
func (t *AccountTree) SelectedAccount() string {
	if t.state != stateLoaded || t.cursor >= len(t.flat) {
		return ""
	}
	return t.flat[t.cursor].fullName
}

// Update handles key events and spinner ticks.
func (t *AccountTree) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if t.state != stateLoaded {
			return nil
		}
		switch msg.String() {
		case "j", "down":
			if t.cursor < len(t.flat)-1 {
				t.cursor++
				t.clampOffset()
			}
		case "k", "up":
			if t.cursor > 0 {
				t.cursor--
				t.clampOffset()
			}
		case "enter", "space":
			if t.cursor < len(t.flat) {
				nd := t.flat[t.cursor]
				if !nd.isLeaf() {
					nd.expanded = !nd.expanded
					t.rebuildFlat()
				}
			}
		}
		return nil
	}
	return t.handleSpinnerTick(msg)
}

func (t AccountTree) View() string {
	if t.height < 3 {
		return ""
	}
	switch t.state {
	case stateLoading:
		return t.renderLoading()
	case stateError:
		return t.renderError(true)
	case stateLoaded:
		return t.renderTree()
	}
	return ""
}

func (t AccountTree) renderTree() string {
	if len(t.flat) == 0 {
		return lipgloss.NewStyle().
			Width(t.width).Height(t.height).
			Align(lipgloss.Center, lipgloss.Center).
			Render("no accounts")
	}

	end := t.offset + t.height
	if end > len(t.flat) {
		end = len(t.flat)
	}
	visible := t.flat[t.offset:end]

	const amtWidth = 14
	const prefixWidth = 2
	const gapWidth = 2

	var lines []string
	for i, nd := range visible {
		idx := t.offset + i
		indent := strings.Repeat("  ", nd.depth)

		var prefix string
		switch {
		case nd.isLeaf():
			prefix = "  "
		case nd.expanded:
			prefix = "v "
		default:
			prefix = "> "
		}

		indentLen := nd.depth * 2
		nameAvail := t.width - indentLen - prefixWidth - amtWidth - gapWidth
		if nameAvail < 1 {
			nameAvail = 1
		}

		name := nd.segment
		nameRunes := []rune(name)
		if len(nameRunes) > nameAvail {
			nameRunes = nameRunes[:nameAvail]
			name = string(nameRunes)
		}
		name = padRight(name, nameAvail)

		amtStr := formatBalance(t.balances[nd.fullName])
		amtStr = fmt.Sprintf("%*s", amtWidth, amtStr)

		line := indent + prefix + name + "  " + amtStr

		if idx == t.cursor {
			line = lipgloss.NewStyle().Reverse(true).Render(line)
		}
		lines = append(lines, line)
	}

	return lipgloss.NewStyle().
		Width(t.width).
		Height(t.height).
		Render(strings.Join(lines, "\n"))
}
