package ui

import (
	"strings"
	"testing"

	floatv1 "github.com/brendanv/float/gen/float/v1"
)

func TestSummaryPanel_NetWorth(t *testing.T) {
	p := NewSummaryPanel(NewStyles(true))
	p.SetSize(40, 12)
	p.SetData(&floatv1.BalanceReport{
		Rows: []*floatv1.BalanceRow{
			{FullName: "assets", Amounts: []*floatv1.Amount{{Quantity: "1500.00", Commodity: "USD"}}},
			{FullName: "liabilities", Amounts: []*floatv1.Amount{{Quantity: "-300.00", Commodity: "USD"}}},
		},
	})
	view := p.View()
	if !strings.Contains(view, "1200.00") {
		t.Errorf("expected net worth 1200.00 in view, got: %q", view)
	}
}

func TestSummaryPanel_NetIncome(t *testing.T) {
	p := NewSummaryPanel(NewStyles(true))
	p.SetSize(40, 12)
	p.SetData(&floatv1.BalanceReport{
		Rows: []*floatv1.BalanceRow{
			// In hledger convention: revenue is stored negative
			{FullName: "revenue", Amounts: []*floatv1.Amount{{Quantity: "-3000.00", Commodity: "USD"}}},
			{FullName: "expenses", Amounts: []*floatv1.Amount{{Quantity: "200.00", Commodity: "USD"}}},
		},
	})
	view := p.View()
	// net income = -((-3000) + 200) = 2800
	if !strings.Contains(view, "2800.00") {
		t.Errorf("expected net income 2800.00 in view, got: %q", view)
	}
	// income displayed as positive (negated)
	if !strings.Contains(view, "3000.00") {
		t.Errorf("expected income 3000.00 (negated revenue) in view, got: %q", view)
	}
}

func TestSummaryPanel_NilReport(t *testing.T) {
	p := NewSummaryPanel(NewStyles(true))
	p.SetSize(40, 12)
	// Should not panic
	p.SetData(nil)
	view := p.View()
	if view == "" {
		t.Error("expected non-empty view after SetData(nil)")
	}
}

func TestSummaryPanel_EmptyRows(t *testing.T) {
	p := NewSummaryPanel(NewStyles(true))
	p.SetSize(40, 12)
	p.SetData(&floatv1.BalanceReport{Rows: []*floatv1.BalanceRow{}})
	view := p.View()
	// Should render without crash; all values are zero
	if view == "" {
		t.Error("expected non-empty view for empty rows")
	}
	if !strings.Contains(view, "0.00") {
		t.Errorf("expected 0.00 values in view, got: %q", view)
	}
}

func TestSummaryPanel_CaseInsensitive(t *testing.T) {
	p := NewSummaryPanel(NewStyles(true))
	p.SetSize(40, 12)
	p.SetData(&floatv1.BalanceReport{
		Rows: []*floatv1.BalanceRow{
			{FullName: "Assets", Amounts: []*floatv1.Amount{{Quantity: "500.00", Commodity: "USD"}}},
			{FullName: "LIABILITIES", Amounts: []*floatv1.Amount{{Quantity: "-100.00", Commodity: "USD"}}},
		},
	})
	view := p.View()
	// Net worth = 500 + (-100) = 400
	if !strings.Contains(view, "400.00") {
		t.Errorf("expected net worth 400.00, got: %q", view)
	}
}

func TestSummaryPanel_ViewLabels(t *testing.T) {
	p := NewSummaryPanel(NewStyles(true))
	p.SetSize(40, 12)
	p.SetData(&floatv1.BalanceReport{Rows: nil})
	view := p.View()
	for _, label := range []string{"Net Worth", "Net Income", "Assets", "Liabilities"} {
		if !strings.Contains(view, label) {
			t.Errorf("expected label %q in view, got: %q", label, view)
		}
	}
}

func TestSummaryPanel_Loading(t *testing.T) {
	p := NewSummaryPanel(NewStyles(true))
	p.SetSize(40, 12)
	view := p.View()
	if view == "" {
		t.Error("expected non-empty loading view")
	}
}

func TestSummaryPanel_Error(t *testing.T) {
	p := NewSummaryPanel(NewStyles(true))
	p.SetSize(40, 12)
	p.SetError("server unavailable")
	view := p.View()
	if !strings.Contains(view, "!") {
		t.Errorf("expected ! in error view, got: %q", view)
	}
	if !strings.Contains(view, "server unavailable") {
		t.Errorf("expected error message in view, got: %q", view)
	}
}

func TestSummaryPanel_TooSmall(t *testing.T) {
	p := NewSummaryPanel(NewStyles(true))
	p.SetSize(40, 2)
	view := p.View()
	if view != "" {
		t.Errorf("expected empty view for height < 3, got: %q", view)
	}
}

func TestSumFirstCommodity_Empty(t *testing.T) {
	v, com := sumFirstCommodity(nil)
	if v != 0 || com != "" {
		t.Errorf("expected (0, \"\"), got (%v, %q)", v, com)
	}
}

func TestSumFirstCommodity_Single(t *testing.T) {
	v, com := sumFirstCommodity([]*floatv1.Amount{{Quantity: "42.50", Commodity: "USD"}})
	if v != 42.50 || com != "USD" {
		t.Errorf("expected (42.50, USD), got (%v, %q)", v, com)
	}
}

func TestSumFirstCommodity_Multiple(t *testing.T) {
	v, com := sumFirstCommodity([]*floatv1.Amount{
		{Quantity: "100.00", Commodity: "USD"},
		{Quantity: "50.00", Commodity: "USD"},
	})
	if v != 150.00 || com != "USD" {
		t.Errorf("expected (150.00, USD), got (%v, %q)", v, com)
	}
}

func TestSumFirstCommodity_Negative(t *testing.T) {
	v, com := sumFirstCommodity([]*floatv1.Amount{{Quantity: "-300.00", Commodity: "USD"}})
	if v != -300.00 || com != "USD" {
		t.Errorf("expected (-300.00, USD), got (%v, %q)", v, com)
	}
}
