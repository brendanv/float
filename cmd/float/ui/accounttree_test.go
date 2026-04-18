package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	floatv1 "github.com/brendanv/float/gen/float/v1"
)

// -- Tree building tests --

func TestBuildTree_FlatAccounts(t *testing.T) {
	tree := NewAccountTree()
	tree.SetAccounts([]*floatv1.Account{
		{Name: "checking", FullName: "assets:checking", Type: "A"},
		{Name: "savings", FullName: "assets:savings", Type: "A"},
	})
	// Expect one root "assets" with two children.
	if len(tree.roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(tree.roots))
	}
	assets := tree.roots[0]
	if assets.segment != "assets" {
		t.Errorf("root segment = %q, want assets", assets.segment)
	}
	if assets.depth != 0 {
		t.Errorf("root depth = %d, want 0", assets.depth)
	}
	if len(assets.children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(assets.children))
	}
}

func TestBuildTree_DeepPath(t *testing.T) {
	tree := NewAccountTree()
	tree.SetAccounts([]*floatv1.Account{
		{Name: "checking", FullName: "assets:bank:checking", Type: "A"},
		{Name: "savings", FullName: "assets:bank:savings", Type: "A"},
	})
	if len(tree.roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(tree.roots))
	}
	assets := tree.roots[0]
	if assets.segment != "assets" {
		t.Errorf("root segment = %q, want assets", assets.segment)
	}
	if len(assets.children) != 1 {
		t.Fatalf("expected 1 child of assets (bank), got %d", len(assets.children))
	}
	bank := assets.children[0]
	if bank.segment != "bank" {
		t.Errorf("child segment = %q, want bank", bank.segment)
	}
	if bank.depth != 1 {
		t.Errorf("bank depth = %d, want 1", bank.depth)
	}
	if len(bank.children) != 2 {
		t.Fatalf("expected 2 grandchildren, got %d", len(bank.children))
	}
}

func TestBuildTree_SiblingsMergeUnderParent(t *testing.T) {
	tree := NewAccountTree()
	tree.SetAccounts([]*floatv1.Account{
		{Name: "food", FullName: "expenses:food", Type: "X"},
		{Name: "rent", FullName: "expenses:rent", Type: "X"},
		{Name: "coffee", FullName: "expenses:food:coffee", Type: "X"},
	})
	if len(tree.roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(tree.roots))
	}
	expenses := tree.roots[0]
	if expenses.segment != "expenses" {
		t.Errorf("root = %q, want expenses", expenses.segment)
	}
	// food and rent should both be children of expenses
	if len(expenses.children) != 2 {
		t.Fatalf("expected 2 children (food, rent), got %d", len(expenses.children))
	}
	// food should have coffee as child
	var food *treeNode
	for _, c := range expenses.children {
		if c.segment == "food" {
			food = c
		}
	}
	if food == nil {
		t.Fatal("food node not found under expenses")
		return
	}
	if len(food.children) != 1 || food.children[0].segment != "coffee" {
		t.Errorf("expected coffee under food, got %v", food.children)
	}
}

func TestBuildTree_TypeOrder(t *testing.T) {
	tree := NewAccountTree()
	tree.SetAccounts([]*floatv1.Account{
		{Name: "food", FullName: "expenses:food", Type: "X"},
		{Name: "checking", FullName: "assets:checking", Type: "A"},
		{Name: "visa", FullName: "liabilities:visa", Type: "L"},
		{Name: "salary", FullName: "revenue:salary", Type: "R"},
		{Name: "opening", FullName: "equity:opening", Type: "E"},
	})
	// Roots should appear in accountTypeOrder: A, L, R, X, E
	if len(tree.roots) != 5 {
		t.Fatalf("expected 5 roots, got %d", len(tree.roots))
	}
	wantSegments := []string{"assets", "liabilities", "revenue", "expenses", "equity"}
	for i, nd := range tree.roots {
		if nd.segment != wantSegments[i] {
			t.Errorf("root[%d] = %q, want %q", i, nd.segment, wantSegments[i])
		}
	}
}

func TestBuildTree_TopLevelNodesStartExpanded(t *testing.T) {
	tree := NewAccountTree()
	tree.SetAccounts([]*floatv1.Account{
		{Name: "checking", FullName: "assets:checking", Type: "A"},
	})
	if !tree.roots[0].expanded {
		t.Error("top-level node should start expanded")
	}
}

func TestBuildTree_ChildrenSortedAlphabetically(t *testing.T) {
	tree := NewAccountTree()
	tree.SetAccounts([]*floatv1.Account{
		{Name: "rent", FullName: "expenses:rent", Type: "X"},
		{Name: "food", FullName: "expenses:food", Type: "X"},
		{Name: "auto", FullName: "expenses:auto", Type: "X"},
	})
	expenses := tree.roots[0]
	segs := make([]string, len(expenses.children))
	for i, c := range expenses.children {
		segs[i] = c.segment
	}
	want := []string{"auto", "food", "rent"}
	for i, s := range want {
		if segs[i] != s {
			t.Errorf("child[%d] = %q, want %q", i, segs[i], s)
		}
	}
}

// -- Navigation tests --

func TestAccountTree_Navigate(t *testing.T) {
	tree := NewAccountTree()
	tree.SetSize(40, 20)
	tree.SetAccounts([]*floatv1.Account{
		{Name: "checking", FullName: "assets:checking", Type: "A"},
		{Name: "savings", FullName: "assets:savings", Type: "A"},
	})
	// flat should have: assets, checking, savings (assets expanded by default)
	if tree.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", tree.cursor)
	}

	// j moves cursor down
	tree.Update(tea.KeyPressMsg{Code: 'j'})
	if tree.cursor != 1 {
		t.Errorf("after j, cursor = %d, want 1", tree.cursor)
	}

	// k moves cursor up
	tree.Update(tea.KeyPressMsg{Code: 'k'})
	if tree.cursor != 0 {
		t.Errorf("after k, cursor = %d, want 0", tree.cursor)
	}

	// k at top doesn't go negative
	tree.Update(tea.KeyPressMsg{Code: 'k'})
	if tree.cursor != 0 {
		t.Errorf("k at top: cursor = %d, want 0", tree.cursor)
	}
}

func TestAccountTree_ExpandCollapse(t *testing.T) {
	tree := NewAccountTree()
	tree.SetSize(40, 20)
	tree.SetAccounts([]*floatv1.Account{
		{Name: "checking", FullName: "assets:checking", Type: "A"},
		{Name: "savings", FullName: "assets:savings", Type: "A"},
	})
	// Initially expanded: assets + checking + savings = 3
	if len(tree.flat) != 3 {
		t.Fatalf("initial flat len = %d, want 3", len(tree.flat))
	}

	// Enter at cursor=0 (assets) collapses it
	tree.cursor = 0
	tree.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(tree.flat) != 1 {
		t.Errorf("after collapse, flat len = %d, want 1", len(tree.flat))
	}
	if tree.roots[0].expanded {
		t.Error("assets should be collapsed after Enter")
	}

	// Enter again expands
	tree.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(tree.flat) != 3 {
		t.Errorf("after expand, flat len = %d, want 3", len(tree.flat))
	}
}

func TestAccountTree_CollapseHidesDescendants(t *testing.T) {
	tree := NewAccountTree()
	tree.SetSize(40, 20)
	tree.SetAccounts([]*floatv1.Account{
		{Name: "food", FullName: "expenses:food", Type: "X"},
		{Name: "coffee", FullName: "expenses:food:coffee", Type: "X"},
	})
	// Initially: expenses, food, coffee = 3
	if len(tree.flat) != 3 {
		t.Fatalf("initial flat len = %d, want 3", len(tree.flat))
	}

	// Collapse expenses (cursor=0)
	tree.cursor = 0
	tree.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(tree.flat) != 1 {
		t.Errorf("after collapse, flat len = %d, want 1 (only expenses)", len(tree.flat))
	}
}

func TestAccountTree_EnterOnLeafNoOp(t *testing.T) {
	tree := NewAccountTree()
	tree.SetSize(40, 20)
	tree.SetAccounts([]*floatv1.Account{
		{Name: "checking", FullName: "assets:checking", Type: "A"},
	})
	// flat: assets, checking (checking is leaf)
	before := len(tree.flat)
	// Navigate to checking (index 1)
	tree.cursor = 1
	tree.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(tree.flat) != before {
		t.Errorf("Enter on leaf changed flat len from %d to %d", before, len(tree.flat))
	}
}

// -- Viewport tests --

func TestAccountTree_ViewportScrollsDown(t *testing.T) {
	tree := NewAccountTree()
	tree.SetSize(40, 3) // only 3 rows visible
	accounts := []*floatv1.Account{
		{Name: "a1", FullName: "assets:a1", Type: "A"},
		{Name: "a2", FullName: "assets:a2", Type: "A"},
		{Name: "a3", FullName: "assets:a3", Type: "A"},
		{Name: "a4", FullName: "assets:a4", Type: "A"},
	}
	tree.SetAccounts(accounts)
	// flat: assets, a1, a2, a3, a4 = 5 rows, viewport = 3

	// Move cursor to row 4 (index 4)
	for i := 0; i < 4; i++ {
		tree.Update(tea.KeyPressMsg{Code: 'j'})
	}
	if tree.cursor != 4 {
		t.Fatalf("cursor = %d, want 4", tree.cursor)
	}
	if tree.offset == 0 {
		t.Error("offset should have scrolled down but is still 0")
	}
}

// -- Rendering tests --

func TestAccountTree_TruncateName(t *testing.T) {
	tree := NewAccountTree()
	tree.SetSize(20, 10) // very narrow
	tree.SetAccounts([]*floatv1.Account{
		{Name: "verylongnamethatexceedslimit", FullName: "assets:verylongnamethatexceedslimit", Type: "A"},
	})
	view := tree.View()
	// View must not panic and must be non-empty
	if view == "" {
		t.Error("expected non-empty view")
	}
	// Should not contain the full long name if width is too narrow
	if strings.Contains(view, "verylongnamethatexceedslimit") {
		t.Error("expected long name to be truncated")
	}
}

func TestAccountTree_EmptyAccounts(t *testing.T) {
	tree := NewAccountTree()
	tree.SetSize(40, 10)
	tree.SetAccounts([]*floatv1.Account{})
	view := tree.View()
	if !strings.Contains(view, "no accounts") {
		t.Errorf("expected 'no accounts' in view, got: %q", view)
	}
}

func TestAccountTree_LoadingView(t *testing.T) {
	tree := NewAccountTree()
	tree.SetSize(40, 10)
	view := tree.View()
	if view == "" {
		t.Error("expected non-empty loading view")
	}
}

func TestAccountTree_ErrorView(t *testing.T) {
	tree := NewAccountTree()
	tree.SetSize(40, 10)
	tree.SetError("connection refused")
	view := tree.View()
	if !strings.Contains(view, "!") {
		t.Errorf("expected ! in error view, got: %q", view)
	}
	if !strings.Contains(view, "connection refused") {
		t.Errorf("expected error message in view, got: %q", view)
	}
}

func TestAccountTree_TooSmall(t *testing.T) {
	tree := NewAccountTree()
	tree.SetSize(40, 2)
	view := tree.View()
	if view != "" {
		t.Errorf("expected empty view for height < 3, got: %q", view)
	}
}

func TestAccountTree_BalancesShownInView(t *testing.T) {
	tree := NewAccountTree()
	tree.SetSize(60, 10)
	tree.SetAccounts([]*floatv1.Account{
		{Name: "checking", FullName: "assets:checking", Type: "A"},
	})
	tree.SetBalances(&floatv1.BalanceReport{
		Rows: []*floatv1.BalanceRow{
			{FullName: "assets:checking", Amounts: []*floatv1.Amount{{Quantity: "1000.00", Commodity: "USD"}}},
		},
	})
	view := tree.View()
	if !strings.Contains(view, "1000.00") {
		t.Errorf("expected balance in view, got: %q", view)
	}
}
