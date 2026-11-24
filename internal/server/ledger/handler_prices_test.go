package ledger_test

import (
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
)

func TestGeneratePricesFromCost(t *testing.T) {
	dir := t.TempDir()

	main := `; float main journal
account assets:investments:vtsax
account assets:checking

include 2026/01.journal
`
	txns := `2026-05-01 Biweekly contribution
    assets:investments:vtsax    2 VTSAX @ 100.00 USD
    assets:checking            -200.00 USD

2026-05-15 Biweekly contribution
    assets:investments:vtsax    2 VTSAX @ 102.00 USD
    assets:checking            -204.00 USD
`

	if err := os.MkdirAll(filepath.Join(dir, "2026"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte(main), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026/01.journal"), []byte(txns), 0644); err != nil {
		t.Fatal(err)
	}

	h := mustRealHandler(t, dir)

	genResp, err := h.GeneratePricesFromCost(t.Context(), connect.NewRequest(&floatv1.GeneratePricesFromCostRequest{}))
	if err != nil {
		t.Fatalf("GeneratePricesFromCost: %v", err)
	}

	if got := len(genResp.Msg.AddedPrices); got != 2 {
		t.Fatalf("AddedPrices count = %d, want 2", got)
	}
	if genResp.Msg.SkippedCommodities != 0 {
		t.Errorf("SkippedCommodities = %d, want 0", genResp.Msg.SkippedCommodities)
	}

	byDate := make(map[string]*floatv1.PriceDirective)
	for _, p := range genResp.Msg.AddedPrices {
		if p.Commodity != "VTSAX" {
			t.Errorf("unexpected commodity %q in added prices", p.Commodity)
		}
		byDate[p.Date] = p
	}
	if p := byDate["2026-05-01"]; p == nil || p.Price == nil || p.Price.Quantity != "100.00" {
		t.Errorf("2026-05-01 price = %v, want 100.00 USD", byDate["2026-05-01"])
	}
	if p := byDate["2026-05-15"]; p == nil || p.Price == nil || p.Price.Quantity != "102.00" {
		t.Errorf("2026-05-15 price = %v, want 102.00 USD", byDate["2026-05-15"])
	}

	holdingsResp, err := h.GetPortfolioHoldings(t.Context(), connect.NewRequest(&floatv1.GetPortfolioHoldingsRequest{}))
	if err != nil {
		t.Fatalf("GetPortfolioHoldings: %v", err)
	}
	var vtsax *floatv1.Holding
	for _, holding := range holdingsResp.Msg.Holdings {
		if holding.Symbol == "VTSAX" {
			vtsax = holding
		}
	}
	if vtsax == nil {
		t.Fatalf("VTSAX not found in holdings")
	}
	if vtsax.CurrentValue == nil {
		t.Fatal("CurrentValue is nil after generating prices, want non-nil")
	}
	if vtsax.CurrentValue.Quantity != "408.00" {
		t.Errorf("CurrentValue = %s, want 408.00", vtsax.CurrentValue.Quantity)
	}
	if holdingsResp.Msg.TotalValue == nil || holdingsResp.Msg.TotalValue.Quantity != "408.00" {
		t.Errorf("TotalValue = %v, want 408.00 USD", holdingsResp.Msg.TotalValue)
	}
}

func TestGeneratePricesFromCost_GeneratesForNewerPurchases(t *testing.T) {
	dir := t.TempDir()

	main := `; float main journal
account assets:investments:vtsax
account assets:checking

include prices.journal
include 2026/01.journal
`
	prices := `P 2026-05-01 VTSAX 99.00 USD
`
	txns := `2026-05-01 Biweekly contribution
    assets:investments:vtsax    2 VTSAX @ 100.00 USD
    assets:checking            -200.00 USD

2026-05-15 Biweekly contribution
    assets:investments:vtsax    2 VTSAX @ 102.00 USD
    assets:checking            -204.00 USD
`

	if err := os.MkdirAll(filepath.Join(dir, "2026"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte(main), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prices.journal"), []byte(prices), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026/01.journal"), []byte(txns), 0644); err != nil {
		t.Fatal(err)
	}

	h := mustRealHandler(t, dir)

	resp, err := h.GeneratePricesFromCost(t.Context(), connect.NewRequest(&floatv1.GeneratePricesFromCostRequest{}))
	if err != nil {
		t.Fatalf("GeneratePricesFromCost: %v", err)
	}
	// 2026-05-01 already has a price; only 2026-05-15 is newer and should be generated.
	if len(resp.Msg.AddedPrices) != 1 {
		t.Fatalf("AddedPrices = %d, want 1", len(resp.Msg.AddedPrices))
	}
	if resp.Msg.AddedPrices[0].Date != "2026-05-15" {
		t.Errorf("added price date = %q, want 2026-05-15", resp.Msg.AddedPrices[0].Date)
	}
	if resp.Msg.AddedPrices[0].Price == nil || resp.Msg.AddedPrices[0].Price.Quantity != "102.00" {
		t.Errorf("added price quantity = %v, want 102.00", resp.Msg.AddedPrices[0].Price)
	}
	// VTSAX had prices and generated a new entry — not skipped.
	if resp.Msg.SkippedCommodities != 0 {
		t.Errorf("SkippedCommodities = %d, want 0", resp.Msg.SkippedCommodities)
	}
}

func TestGeneratePricesFromCost_SkipsUpToDatePriced(t *testing.T) {
	dir := t.TempDir()

	main := `; float main journal
account assets:investments:vtsax
account assets:checking

include prices.journal
include 2026/01.journal
`
	prices := `P 2026-05-20 VTSAX 105.00 USD
`
	txns := `2026-05-15 Biweekly contribution
    assets:investments:vtsax    2 VTSAX @ 102.00 USD
    assets:checking            -204.00 USD
`

	if err := os.MkdirAll(filepath.Join(dir, "2026"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte(main), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prices.journal"), []byte(prices), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026/01.journal"), []byte(txns), 0644); err != nil {
		t.Fatal(err)
	}

	h := mustRealHandler(t, dir)

	resp, err := h.GeneratePricesFromCost(t.Context(), connect.NewRequest(&floatv1.GeneratePricesFromCostRequest{}))
	if err != nil {
		t.Fatalf("GeneratePricesFromCost: %v", err)
	}
	// Latest price (2026-05-20) is newer than the purchase (2026-05-15) — nothing to generate.
	if len(resp.Msg.AddedPrices) != 0 {
		t.Errorf("AddedPrices = %d, want 0 (price already newer than purchases)", len(resp.Msg.AddedPrices))
	}
	if resp.Msg.SkippedCommodities != 1 {
		t.Errorf("SkippedCommodities = %d, want 1", resp.Msg.SkippedCommodities)
	}
}
