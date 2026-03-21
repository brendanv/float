package ui

import (
	"context"
	"strings"
	"testing"

	connect "connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
)

func amount(qty, commodity string) *floatv1.Amount {
	return &floatv1.Amount{Quantity: qty, Commodity: commodity}
}

func expenseRow(name string, amounts ...*floatv1.Amount) *floatv1.BalanceRow {
	return &floatv1.BalanceRow{DisplayName: "expenses:" + name, FullName: "expenses:" + name, Amounts: amounts}
}

func revenueRow(name string, amounts ...*floatv1.Amount) *floatv1.BalanceRow {
	return &floatv1.BalanceRow{DisplayName: "income:" + name, FullName: "income:" + name, Amounts: amounts}
}

func TestShortName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"expenses:food", "food"},
		{"expenses:food:restaurants", "restaurants"},
		{"income:salary", "salary"},
		{"salary", "salary"},
		{"expenses", "expenses"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := shortName(tc.in); got != tc.want {
				t.Errorf("shortName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestInsightsPanel_EmptyRows_ShowsNoActivity(t *testing.T) {
	p := NewInsightsPanel()
	p.SetSize(60, 10)
	p.SetData(&floatv1.BalanceReport{Rows: nil})
	view := p.View()
	if !strings.Contains(view, "no activity") {
		t.Errorf("expected 'no activity', got %q", view)
	}
}

func TestInsightsPanel_NilReport_ShowsNoActivity(t *testing.T) {
	p := NewInsightsPanel()
	p.SetSize(60, 10)
	p.SetData(nil)
	view := p.View()
	if !strings.Contains(view, "no activity") {
		t.Errorf("expected 'no activity' for nil report, got %q", view)
	}
}

func TestInsightsPanel_UnrelatedAccountsIgnored(t *testing.T) {
	p := NewInsightsPanel()
	p.SetSize(60, 10)
	p.SetData(&floatv1.BalanceReport{
		Rows: []*floatv1.BalanceRow{
			{DisplayName: "assets:checking", FullName: "assets:checking", Amounts: []*floatv1.Amount{amount("5000.00", "USD")}},
			{DisplayName: "liabilities:visa", FullName: "liabilities:visa", Amounts: []*floatv1.Amount{amount("-200.00", "USD")}},
		},
	})
	view := p.View()
	if !strings.Contains(view, "no activity") {
		t.Errorf("expected 'no activity' when only assets/liabilities, got %q", view)
	}
}

func TestInsightsPanel_ErrorState(t *testing.T) {
	p := NewInsightsPanel()
	p.SetSize(60, 10)
	p.SetError("connection refused")
	view := p.View()
	if !strings.Contains(view, "connection refused") {
		t.Errorf("expected error message in view, got %q", view)
	}
}

func TestInsightsPanel_TooSmall_ReturnsEmpty(t *testing.T) {
	tests := []struct {
		name string
		w, h int
	}{
		{"zero height", 60, 0},
		{"tiny height", 60, 2},
		{"tiny width", 5, 10},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := NewInsightsPanel()
			p.SetSize(tc.w, tc.h)
			p.SetData(&floatv1.BalanceReport{
				Rows: []*floatv1.BalanceRow{expenseRow("food", amount("100.00", "USD"))},
			})
			view := p.View()
			if view != "" {
				t.Errorf("expected empty view for small terminal, got %q", view)
			}
		})
	}
}

func TestInsightsPanel_ExpensesOnly_RendersShortName(t *testing.T) {
	p := NewInsightsPanel()
	p.SetSize(60, 5)
	p.SetData(&floatv1.BalanceReport{
		Rows: []*floatv1.BalanceRow{
			expenseRow("food", amount("100.00", "USD")),
		},
	})
	view := p.View()
	if !strings.Contains(view, "food") {
		t.Error("expected short name 'food' in view")
	}
	// Full prefix must NOT appear.
	if strings.Contains(view, "expenses:food") {
		t.Error("full name 'expenses:food' should not appear in view")
	}
	// No section header when only one type.
	if strings.Contains(view, "expenses") {
		t.Error("expected no 'expenses' header when only expense rows")
	}
}

func TestInsightsPanel_RevenueOnly_RendersShortName(t *testing.T) {
	p := NewInsightsPanel()
	p.SetSize(60, 5)
	p.SetData(&floatv1.BalanceReport{
		Rows: []*floatv1.BalanceRow{
			revenueRow("salary", amount("-3000.00", "USD")),
		},
	})
	view := p.View()
	if !strings.Contains(view, "salary") {
		t.Error("expected short name 'salary' in view")
	}
	if strings.Contains(view, "income:salary") {
		t.Error("full name 'income:salary' should not appear in view")
	}
	if strings.Contains(view, "income") {
		t.Error("expected no 'income' header when only revenue rows")
	}
}

func TestInsightsPanel_BothSections_ShowsHeaders(t *testing.T) {
	p := NewInsightsPanel()
	p.SetSize(60, 10)
	p.SetData(&floatv1.BalanceReport{
		Rows: []*floatv1.BalanceRow{
			revenueRow("salary", amount("-3000.00", "USD")),
			expenseRow("food", amount("100.00", "USD")),
		},
	})
	view := p.View()
	if !strings.Contains(view, "income") {
		t.Error("expected 'income' header when both sections present")
	}
	if !strings.Contains(view, "expenses") {
		t.Error("expected 'expenses' header when both sections present")
	}
}

func TestInsightsPanel_BothSections_BothGetRows_SmallHeight(t *testing.T) {
	p := NewInsightsPanel()
	p.SetSize(80, 4) // tight: 2 headers + 1 rev + 1 exp
	p.SetData(&floatv1.BalanceReport{
		Rows: []*floatv1.BalanceRow{
			revenueRow("salary", amount("-3000.00", "USD")),
			expenseRow("groceries", amount("80.00", "USD")),
			expenseRow("restaurants", amount("60.00", "USD")),
		},
	})
	view := p.View()
	if !strings.Contains(view, "salary") {
		t.Error("expected revenue row to appear even in tight height")
	}
	if !strings.Contains(view, "groceries") {
		t.Error("expected at least one expense row to appear even in tight height")
	}
}

func TestInsightsPanel_MultipleExpenses_ProportionalBars(t *testing.T) {
	p := NewInsightsPanel()
	p.SetSize(80, 10)
	p.SetData(&floatv1.BalanceReport{
		Rows: []*floatv1.BalanceRow{
			expenseRow("food", amount("200.00", "USD")),
			expenseRow("transport", amount("100.00", "USD")),
			expenseRow("entertainment", amount("50.00", "USD")),
		},
	})
	view := p.View()
	lines := strings.Split(strings.TrimRight(view, " \n"), "\n")
	if len(lines) < 3 {
		t.Errorf("expected at least 3 lines, got %d: %q", len(lines), view)
	}
	foodFilled := strings.Count(lines[0], "█")
	transportFilled := strings.Count(lines[1], "█")
	if foodFilled <= transportFilled {
		t.Errorf("expected food bar (%d) > transport bar (%d)", foodFilled, transportFilled)
	}
}

func TestInsightsPanel_GlobalScale_IncomeDominatesExpenses(t *testing.T) {
	p := NewInsightsPanel()
	p.SetSize(80, 10)
	// Income is 5× larger than largest expense — income bar should be full,
	// expense bars should be much shorter.
	p.SetData(&floatv1.BalanceReport{
		Rows: []*floatv1.BalanceRow{
			revenueRow("salary", amount("-5000.00", "USD")),
			expenseRow("groceries", amount("800.00", "USD")),
			expenseRow("transport", amount("200.00", "USD")),
		},
	})
	view := p.View()
	lines := strings.Split(strings.TrimRight(view, " \n"), "\n")

	// Find lines containing each category.
	var salaryLine, groceriesLine string
	for _, l := range lines {
		if strings.Contains(l, "salary") {
			salaryLine = l
		}
		if strings.Contains(l, "groceries") {
			groceriesLine = l
		}
	}
	if salaryLine == "" || groceriesLine == "" {
		t.Fatalf("could not find expected lines in view:\n%s", view)
	}

	salaryFilled := strings.Count(salaryLine, "█")
	groceriesFilled := strings.Count(groceriesLine, "█")
	if salaryFilled <= groceriesFilled {
		t.Errorf("salary bar (%d) should be larger than groceries bar (%d) with global scaling", salaryFilled, groceriesFilled)
	}
}

func TestInsightsPanel_IncomeAccountPrefix(t *testing.T) {
	p := NewInsightsPanel()
	p.SetSize(60, 10)
	p.SetData(&floatv1.BalanceReport{
		Rows: []*floatv1.BalanceRow{
			{DisplayName: "income:salary", FullName: "income:salary", Amounts: []*floatv1.Amount{amount("-3000.00", "USD")}},
		},
	})
	view := p.View()
	if !strings.Contains(view, "salary") {
		t.Error("expected 'income:*' accounts to be shown with short name")
	}
}

func TestPrimaryValue(t *testing.T) {
	tests := []struct {
		name    string
		amounts []*floatv1.Amount
		want    float64
	}{
		{"empty", nil, 0},
		{"positive", []*floatv1.Amount{amount("100.50", "USD")}, 100.50},
		{"negative", []*floatv1.Amount{amount("-75.25", "USD")}, 75.25},
		{"zero", []*floatv1.Amount{amount("0", "EUR")}, 0},
		{"invalid", []*floatv1.Amount{amount("abc", "USD")}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := primaryValue(tc.amounts)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFetchInsights_PropagatesQueryAndDepth(t *testing.T) {
	var gotDepth int32
	var gotQuery []string
	client := &mockLedgerClient{
		getBalancesFn: func(_ context.Context, req *connect.Request[floatv1.GetBalancesRequest]) (*connect.Response[floatv1.GetBalancesResponse], error) {
			gotDepth = req.Msg.Depth
			gotQuery = req.Msg.Query
			return connect.NewResponse(&floatv1.GetBalancesResponse{
				Report: &floatv1.BalanceReport{},
			}), nil
		},
	}
	cmd := FetchInsights(client, "date:2026-03")
	msg := cmd().(InsightsMsg)
	if msg.Err != nil {
		t.Fatalf("unexpected error: %v", msg.Err)
	}
	if gotDepth != 2 {
		t.Errorf("expected depth 2, got %d", gotDepth)
	}
	if len(gotQuery) != 1 || gotQuery[0] != "date:2026-03" {
		t.Errorf("expected [date:2026-03], got %v", gotQuery)
	}
}
