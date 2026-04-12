package ui

import (
	"strings"
	"testing"

	floatv1 "github.com/brendanv/float/gen/float/v1"
)

func TestGroupAccounts(t *testing.T) {
	accounts := []*floatv1.Account{
		{Name: "food", FullName: "expenses:food", Type: "X"},
		{Name: "checking", FullName: "assets:checking", Type: "A"},
		{Name: "visa", FullName: "liabilities:visa", Type: "L"},
		{Name: "salary", FullName: "revenue:salary", Type: "R"},
		{Name: "opening", FullName: "equity:opening", Type: "E"},
	}
	rows := groupedRows(accounts)

	var order []string
	for _, r := range rows {
		if r.isHeader {
			order = append(order, "H:"+r.label)
		} else {
			order = append(order, r.account.Type)
		}
	}

	want := []string{"H:Assets", "A", "H:Liabilities", "L", "H:Revenue", "R", "H:Expenses", "X", "H:Equity", "E"}
	if len(order) != len(want) {
		t.Fatalf("expected %v, got %v", want, order)
	}
	for i, v := range want {
		if order[i] != v {
			t.Errorf("position %d: expected %q, got %q", i, v, order[i])
		}
	}
}

func TestAccountsPanel_RebuildRows(t *testing.T) {
	p := NewAccountsPanel(NewStyles(true))
	p.SetSize(60, 20)
	p.SetAccounts([]*floatv1.Account{
		{Name: "checking", FullName: "assets:checking", Type: "A"},
		{Name: "visa", FullName: "liabilities:visa", Type: "L"},
	})
	p.SetBalances(&floatv1.BalanceReport{
		Rows: []*floatv1.BalanceRow{
			{FullName: "assets:checking", Amounts: []*floatv1.Amount{{Quantity: "1000.00", Commodity: "USD"}}},
			{FullName: "liabilities:visa", Amounts: []*floatv1.Amount{{Quantity: "-200.00", Commodity: "USD"}}},
		},
	})

	rows := p.table.Rows()

	if len(rows) != 4 {
		t.Fatalf("expected 4 rows (2 headers + 2 accounts), got %d", len(rows))
	}

	if rows[0][0] != "Assets" || rows[0][1] != "" {
		t.Errorf("expected Assets header row, got %v", rows[0])
	}
	if !strings.HasPrefix(rows[1][0], "  ") {
		t.Errorf("expected indented account name, got %q", rows[1][0])
	}
	if !strings.Contains(rows[1][0], "checking") {
		t.Errorf("expected account name in row, got %q", rows[1][0])
	}
	if rows[1][1] != "1000.00 USD" {
		t.Errorf("expected balance %q, got %q", "1000.00 USD", rows[1][1])
	}
	if rows[2][0] != "Liabilities" {
		t.Errorf("expected Liabilities header, got %q", rows[2][0])
	}
	if rows[3][1] != "-200.00 USD" {
		t.Errorf("expected balance %q, got %q", "-200.00 USD", rows[3][1])
	}
}

func TestAccountsPanel_RebuildRows_NoBalances(t *testing.T) {
	p := NewAccountsPanel(NewStyles(true))
	p.SetSize(60, 20)
	p.SetAccounts([]*floatv1.Account{
		{Name: "checking", FullName: "assets:checking", Type: "A"},
	})

	rows := p.table.Rows()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[1][1] != "" {
		t.Errorf("expected empty balance, got %q", rows[1][1])
	}
}

func TestAccountsPanel_View_Loading(t *testing.T) {
	p := NewAccountsPanel(NewStyles(true))
	p.SetSize(40, 20)
	view := p.View()
	if view == "" {
		t.Fatal("expected non-empty view for loading state")
	}
}

func TestAccountsPanel_View_Loaded(t *testing.T) {
	p := NewAccountsPanel(NewStyles(true))
	p.SetSize(60, 20)
	p.SetAccounts([]*floatv1.Account{
		{Name: "checking", FullName: "assets:checking", Type: "A"},
	})
	p.SetBalances(&floatv1.BalanceReport{
		Rows: []*floatv1.BalanceRow{
			{FullName: "assets:checking", Amounts: []*floatv1.Amount{{Quantity: "1000.00", Commodity: "USD"}}},
		},
	})
	view := p.View()
	if !strings.Contains(view, "checking") {
		t.Errorf("expected account name in view, got: %q", view)
	}
	if !strings.Contains(view, "1000.00") {
		t.Errorf("expected amount in view, got: %q", view)
	}
}

func TestAccountsPanel_View_Error(t *testing.T) {
	p := NewAccountsPanel(NewStyles(true))
	p.SetSize(60, 20)
	p.SetError("connection refused")
	view := p.View()
	if !strings.Contains(view, "!") {
		t.Errorf("expected ! in error view, got: %q", view)
	}
	if !strings.Contains(view, "connection refused") {
		t.Errorf("expected error text in view, got: %q", view)
	}
}

func TestAccountsPanel_View_TooSmall(t *testing.T) {
	p := NewAccountsPanel(NewStyles(true))
	p.SetSize(40, 2)
	view := p.View()
	if view != "" {
		t.Errorf("expected empty view for height < 3, got: %q", view)
	}
}
