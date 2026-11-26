package ledger_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/hledger"
	serverledger "github.com/brendanv/float/internal/server/ledger"
	"github.com/brendanv/float/internal/txlock"
)

func TestGetPortfolioHoldings_AggregatesLots(t *testing.T) {
	dir := t.TempDir()

	main := `; float main journal
account assets:investments:aapl
account assets:investments:msft
account assets:checking

include 2026/01.journal
`
	journal := `2026-01-05 Buy AAPL lot 1
    assets:investments:aapl    10 AAPL @ 150.00 USD
    assets:checking           -1500.00 USD

2026-01-10 Buy AAPL lot 2
    assets:investments:aapl     5 AAPL @ 160.00 USD
    assets:checking            -800.00 USD

2026-01-15 Buy AAPL lot 3
    assets:investments:aapl     3 AAPL @ 170.00 USD
    assets:checking            -510.00 USD

2026-01-20 Buy MSFT
    assets:investments:msft     4 MSFT @ 400.00 USD
    assets:checking           -1600.00 USD
`

	if err := os.MkdirAll(filepath.Join(dir, "2026"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte(main), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026/01.journal"), []byte(journal), 0644); err != nil {
		t.Fatal(err)
	}

	h := mustRealHandler(t, dir)

	resp, err := h.GetPortfolioHoldings(t.Context(), connect.NewRequest(&floatv1.GetPortfolioHoldingsRequest{}))
	if err != nil {
		t.Fatalf("GetPortfolioHoldings: %v", err)
	}

	holdings := resp.Msg.Holdings

	bySymbol := make(map[string]*floatv1.Holding)
	for _, h := range holdings {
		if prev, dup := bySymbol[h.Symbol]; dup {
			t.Errorf("symbol %q appears more than once (accounts %q and %q); lots were not aggregated",
				h.Symbol, prev.Account, h.Account)
		}
		bySymbol[h.Symbol] = h
	}

	aapl, ok := bySymbol["AAPL"]
	if !ok {
		t.Fatalf("AAPL not found in holdings; got %v", holdings)
	}
	if aapl.Quantity != "18" {
		t.Errorf("AAPL quantity = %q, want %q (10+5+3 lots aggregated)", aapl.Quantity, "18")
	}

	msft, ok := bySymbol["MSFT"]
	if !ok {
		t.Fatalf("MSFT not found in holdings; got %v", holdings)
	}
	if msft.Quantity != "4" {
		t.Errorf("MSFT quantity = %q, want %q", msft.Quantity, "4")
	}
}

func TestGetPortfolioHoldings_PerCommodityValues(t *testing.T) {
	dir := t.TempDir()

	main := `; float main journal
account assets:investments:aapl
account assets:checking

include prices.journal
include 2026/01.journal
`
	prices := `P 2026-01-25 AAPL 200.00 USD
`
	txns := `2026-01-05 Buy AAPL lot 1
    assets:investments:aapl    10 AAPL @ 150.00 USD
    assets:checking           -1500.00 USD

2026-01-10 Buy AAPL lot 2
    assets:investments:aapl     8 AAPL @ 125.00 USD
    assets:checking           -1000.00 USD
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

	resp, err := h.GetPortfolioHoldings(t.Context(), connect.NewRequest(&floatv1.GetPortfolioHoldingsRequest{}))
	if err != nil {
		t.Fatalf("GetPortfolioHoldings: %v", err)
	}

	var aapl *floatv1.Holding
	for _, holding := range resp.Msg.Holdings {
		if holding.Symbol == "AAPL" {
			aapl = holding
		}
	}
	if aapl == nil {
		t.Fatalf("AAPL not found in holdings")
	}

	// 18 shares total (10 + 8)
	if aapl.Quantity != "18" {
		t.Errorf("Quantity = %q, want %q", aapl.Quantity, "18")
	}
	// CurrentValue = 18 × $200 = $3600
	if aapl.CurrentValue == nil || aapl.CurrentValue.Quantity != "3600.00" {
		t.Errorf("CurrentValue = %v, want 3600.00 USD", aapl.CurrentValue)
	}
	// BookValue = (10 × $150) + (8 × $125) = $1500 + $1000 = $2500
	if aapl.BookValue == nil || aapl.BookValue.Quantity != "2500.00" {
		t.Errorf("BookValue = %v, want 2500.00 USD", aapl.BookValue)
	}
	// UnrealizedGain = $3600 - $2500 = $1100
	if aapl.UnrealizedGain == nil || aapl.UnrealizedGain.Quantity != "1100.00" {
		t.Errorf("UnrealizedGain = %v, want 1100.00 USD", aapl.UnrealizedGain)
	}
}

func TestGetPortfolioHoldings_MultiCommoditySameAccount(t *testing.T) {
	dir := t.TempDir()

	// Both AAPL and GOOG are held in the same account. The old code keyed
	// cost and valued-balance data by account name only, so both holdings
	// would receive the combined account total instead of per-commodity values.
	main := `; float main journal
account assets:investments
account assets:checking

include prices.journal
include 2026/01.journal
`
	prices := `P 2026-01-25 AAPL 200.00 USD
P 2026-01-25 GOOG 120.00 USD
`
	txns := `2026-01-05 Buy AAPL
    assets:investments    10 AAPL @ 150.00 USD
    assets:checking      -1500.00 USD

2026-01-06 Buy GOOG
    assets:investments     5 GOOG @ 100.00 USD
    assets:checking       -500.00 USD
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

	resp, err := h.GetPortfolioHoldings(t.Context(), connect.NewRequest(&floatv1.GetPortfolioHoldingsRequest{}))
	if err != nil {
		t.Fatalf("GetPortfolioHoldings: %v", err)
	}

	bySymbol := make(map[string]*floatv1.Holding)
	for _, holding := range resp.Msg.Holdings {
		bySymbol[holding.Symbol] = holding
	}

	aapl, ok := bySymbol["AAPL"]
	if !ok {
		t.Fatalf("AAPL not found in holdings")
	}
	goog, ok := bySymbol["GOOG"]
	if !ok {
		t.Fatalf("GOOG not found in holdings")
	}

	// AAPL: 10 shares @ $150 cost, current $200
	// CurrentValue = 10 × $200 = $2000
	if aapl.CurrentValue == nil || aapl.CurrentValue.Quantity != "2000.00" {
		t.Errorf("AAPL CurrentValue = %v, want 2000.00 USD", aapl.CurrentValue)
	}
	// BookValue = 10 × $150 = $1500
	if aapl.BookValue == nil || aapl.BookValue.Quantity != "1500.00" {
		t.Errorf("AAPL BookValue = %v, want 1500.00 USD", aapl.BookValue)
	}
	// UnrealizedGain = $2000 - $1500 = $500
	if aapl.UnrealizedGain == nil || aapl.UnrealizedGain.Quantity != "500.00" {
		t.Errorf("AAPL UnrealizedGain = %v, want 500.00 USD", aapl.UnrealizedGain)
	}

	// GOOG: 5 shares @ $100 cost, current $120
	// CurrentValue = 5 × $120 = $600
	if goog.CurrentValue == nil || goog.CurrentValue.Quantity != "600.00" {
		t.Errorf("GOOG CurrentValue = %v, want 600.00 USD", goog.CurrentValue)
	}
	// BookValue = 5 × $100 = $500
	if goog.BookValue == nil || goog.BookValue.Quantity != "500.00" {
		t.Errorf("GOOG BookValue = %v, want 500.00 USD", goog.BookValue)
	}
	// UnrealizedGain = $600 - $500 = $100
	if goog.UnrealizedGain == nil || goog.UnrealizedGain.Quantity != "100.00" {
		t.Errorf("GOOG UnrealizedGain = %v, want 100.00 USD", goog.UnrealizedGain)
	}
}

func TestGetPortfolioHoldings_PriceDateFallsBackToLastTxDate(t *testing.T) {
	dir := t.TempDir()

	main := `; float main journal
account assets:investments:vtsax
account assets:checking

include 2026/01.journal
`
	// No prices.journal — simulates a 401k fund with no market price data.
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

	resp, err := h.GetPortfolioHoldings(t.Context(), connect.NewRequest(&floatv1.GetPortfolioHoldingsRequest{}))
	if err != nil {
		t.Fatalf("GetPortfolioHoldings: %v", err)
	}

	var vtsax *floatv1.Holding
	for _, holding := range resp.Msg.Holdings {
		if holding.Symbol == "VTSAX" {
			vtsax = holding
		}
	}
	if vtsax == nil {
		t.Fatalf("VTSAX not found in holdings")
	}

	// No P directive exists; PriceDate should be the last transaction date.
	if vtsax.PriceDate != "2026-05-15" {
		t.Errorf("PriceDate = %q, want %q (last transaction date)", vtsax.PriceDate, "2026-05-15")
	}
}

func TestGetPortfolioHoldings_PriceDatePrefersNewerPriceDirective(t *testing.T) {
	dir := t.TempDir()

	main := `; float main journal
account assets:investments:aapl
account assets:checking

include prices.journal
include 2026/01.journal
`
	// Price directive is newer than last transaction.
	prices := `P 2026-02-01 AAPL 210.00 USD
`
	txns := `2026-01-05 Buy AAPL
    assets:investments:aapl    10 AAPL @ 150.00 USD
    assets:checking           -1500.00 USD
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

	resp, err := h.GetPortfolioHoldings(t.Context(), connect.NewRequest(&floatv1.GetPortfolioHoldingsRequest{}))
	if err != nil {
		t.Fatalf("GetPortfolioHoldings: %v", err)
	}

	var aapl *floatv1.Holding
	for _, holding := range resp.Msg.Holdings {
		if holding.Symbol == "AAPL" {
			aapl = holding
		}
	}
	if aapl == nil {
		t.Fatalf("AAPL not found in holdings")
	}

	// Price directive (2026-02-01) is newer than last transaction (2026-01-05).
	if aapl.PriceDate != "2026-02-01" {
		t.Errorf("PriceDate = %q, want %q (price directive date)", aapl.PriceDate, "2026-02-01")
	}
}

func TestGetPortfolioHoldings_MultiCurrencyTotals(t *testing.T) {
	dir := t.TempDir()

	// AAPL is priced in USD; VEUR is priced in EUR. Mixing their values into a
	// single total would produce a nonsensical number, so TotalValue must be nil
	// and CurrencyTotals must carry separate per-currency subtotals.
	main := `; float main journal
account assets:investments:aapl
account assets:investments:veur
account assets:checking

include prices.journal
include 2026/01.journal
`
	prices := `P 2026-01-25 AAPL 200.00 USD
P 2026-01-25 VEUR 120.00 EUR
`
	txns := `2026-01-05 Buy AAPL
    assets:investments:aapl    10 AAPL @ 150.00 USD
    assets:checking           -1500.00 USD

2026-01-06 Buy VEUR
    assets:investments:veur     5 VEUR @ 110.00 EUR
    assets:checking            -550.00 EUR
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

	resp, err := h.GetPortfolioHoldings(t.Context(), connect.NewRequest(&floatv1.GetPortfolioHoldingsRequest{}))
	if err != nil {
		t.Fatalf("GetPortfolioHoldings: %v", err)
	}
	msg := resp.Msg

	// TotalValue must be nil — currencies cannot be summed.
	if msg.TotalValue != nil {
		t.Errorf("TotalValue = %v, want nil (mixed currencies must not be summed)", msg.TotalValue)
	}

	// Must have exactly two per-currency subtotals.
	if len(msg.CurrencyTotals) != 2 {
		t.Fatalf("len(CurrencyTotals) = %d, want 2; got %v", len(msg.CurrencyTotals), msg.CurrencyTotals)
	}

	totalsByCurrency := make(map[string]string)
	for _, ct := range msg.CurrencyTotals {
		totalsByCurrency[ct.Commodity] = ct.Quantity
	}
	// AAPL: 10 shares × $200 = $2000
	if totalsByCurrency["USD"] != "2000.00" {
		t.Errorf("USD total = %q, want %q", totalsByCurrency["USD"], "2000.00")
	}
	// VEUR: 5 shares × €120 = €600
	if totalsByCurrency["EUR"] != "600.00" {
		t.Errorf("EUR total = %q, want %q", totalsByCurrency["EUR"], "600.00")
	}

	// PortfolioPct must sum to ~100% within each currency, not across all holdings.
	pctByCurrency := make(map[string]float64)
	for _, holding := range msg.Holdings {
		if holding.CurrentValue != nil {
			pctByCurrency[holding.CurrentValue.Commodity] += holding.PortfolioPct
		}
	}
	for curr, sum := range pctByCurrency {
		if sum < 99.9 || sum > 100.1 {
			t.Errorf("PortfolioPct sum for %s = %.4f, want ~100", curr, sum)
		}
	}
}

// TestGetPortfolioHoldings_AccountPrefixScoping verifies that AccountPrefix
// still isolates holdings correctly now that the handler fetches the full
// unfiltered balance/transaction sets once and filters by prefix in Go
// (Phase 2.4), instead of asking hledger for a prefix-scoped query.
func TestGetPortfolioHoldings_AccountPrefixScoping(t *testing.T) {
	dir := t.TempDir()

	main := `; float main journal
account assets:investments:aapl
account assets:retirement:vwo
account assets:checking

include 2026/01.journal
`
	txns := `2026-01-05 Buy AAPL
    assets:investments:aapl    10 AAPL @ 150.00 USD
    assets:checking           -1500.00 USD

2026-01-10 Buy VWO
    assets:retirement:vwo      5 VWO @ 80.00 USD
    assets:checking           -400.00 USD
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

	resp, err := h.GetPortfolioHoldings(t.Context(), connect.NewRequest(&floatv1.GetPortfolioHoldingsRequest{
		AccountPrefix: "assets:investments",
	}))
	if err != nil {
		t.Fatalf("GetPortfolioHoldings: %v", err)
	}

	if len(resp.Msg.Holdings) != 1 || resp.Msg.Holdings[0].Symbol != "AAPL" {
		t.Fatalf("expected only AAPL under assets:investments, got %+v", resp.Msg.Holdings)
	}
	if resp.Msg.Holdings[0].Account != "assets:investments:aapl" {
		t.Errorf("Account = %q, want %q", resp.Msg.Holdings[0].Account, "assets:investments:aapl")
	}
}

func writePortfolioJournal(t *testing.T, main, prices, txns string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "2026"), 0755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"main.journal": main, "2026/01.journal": txns}
	if prices != "" {
		files["prices.journal"] = prices
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestGetPortfolioTimeseries_ZeroAssertionPosting(t *testing.T) {
	main := `; float main journal
account assets:ira
account assets:checking

include prices.journal
include 2026/01.journal
`
	prices := `P 2026-01-31 AAPL 200.00 USD
P 2026-01-31 GSFTX 12.00 USD
`
	txns := `2026-01-05 Buy AAPL
    assets:ira           10 AAPL @@ 1500.00 USD
    assets:checking   -1500.00 USD

2026-01-06 Buy GSFTX
    assets:ira          100 GSFTX @@ 1000.00 USD
    assets:checking   -1000.00 USD

2026-02-17 Statement balances
    assets:ira            0 GSFTX = 100 GSFTX
    assets:ira            0 AAPL = 10 AAPL
`
	h := mustRealHandler(t, writePortfolioJournal(t, main, prices, txns))

	resp, err := h.GetPortfolioTimeseries(t.Context(), connect.NewRequest(&floatv1.GetPortfolioTimeseriesRequest{}))
	if err != nil {
		t.Fatalf("GetPortfolioTimeseries: %v", err)
	}
	snaps := resp.Msg.Snapshots
	if len(snaps) < 2 {
		t.Fatalf("got %d snapshots, want at least 2: %v", len(snaps), snaps)
	}
	for _, s := range snaps {
		if s.CostBasis == nil || s.CostBasis.Commodity != "USD" || s.CostBasis.Quantity == "0.00" {
			t.Errorf("%s: CostBasis = %v, want non-zero USD", s.Date, s.CostBasis)
		}
		if s.TotalValue == nil || s.TotalValue.Commodity != "USD" {
			t.Errorf("%s: TotalValue = %v, want USD", s.Date, s.TotalValue)
		}
	}
	if last := snaps[len(snaps)-1]; last.CostBasis.GetQuantity() != "2500.00" {
		t.Errorf("last CostBasis = %v, want 2500.00 USD", last.CostBasis)
	}
}

func TestGetPortfolioTimeseries_CostBasisCarriedPastLastTxn(t *testing.T) {
	main := `; float main journal
account assets:investments
account assets:checking

include prices.journal
include 2026/01.journal
`
	prices := `P 2026-01-31 AAPL 160.00 USD
P 2026-03-26 AAPL 180.00 USD
`
	txns := `2026-01-05 Buy AAPL
    assets:investments    10 AAPL @@ 1500.00 USD
    assets:checking    -1500.00 USD

2026-02-10 Buy AAPL
    assets:investments     5 AAPL @@ 800.00 USD
    assets:checking     -800.00 USD
`
	h := mustRealHandler(t, writePortfolioJournal(t, main, prices, txns))

	resp, err := h.GetPortfolioTimeseries(t.Context(), connect.NewRequest(&floatv1.GetPortfolioTimeseriesRequest{Begin: "2026-01-03"}))
	if err != nil {
		t.Fatalf("GetPortfolioTimeseries: %v", err)
	}
	snaps := resp.Msg.Snapshots
	if len(snaps) < 3 {
		t.Fatalf("got %d snapshots, want at least 3: %v", len(snaps), snaps)
	}
	last, prev := snaps[len(snaps)-1], snaps[len(snaps)-2]
	if last.CostBasis == nil {
		t.Fatalf("last snapshot %s has no CostBasis; snapshots: %v", last.Date, snaps)
	}
	if last.CostBasis.Quantity != prev.CostBasis.GetQuantity() || last.CostBasis.Quantity != "2300.00" {
		t.Errorf("last CostBasis = %v, prev = %v, want both 2300.00 USD", last.CostBasis, prev.CostBasis)
	}
}

func TestGetPortfolioTimeseries_CostBasisCarriedIntoGaps(t *testing.T) {
	main := `; float main journal
account assets:investments
account assets:checking

include prices.journal
include 2026/01.journal
`
	prices := `P 2026-01-31 AAPL 160.00 USD
P 2026-04-30 AAPL 180.00 USD
`
	txns := `2026-01-05 Buy AAPL
    assets:investments    10 AAPL @@ 1500.00 USD
    assets:checking    -1500.00 USD

2026-02-10 Buy AAPL
    assets:investments     5 AAPL @@ 800.00 USD
    assets:checking     -800.00 USD

2026-04-10 Buy AAPL
    assets:investments     2 AAPL @@ 340.00 USD
    assets:checking     -340.00 USD
`
	dir := writePortfolioJournal(t, main, prices, txns)
	quarterlyCost := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if slices.Contains(args, "--cost") {
			args = slices.Clone(args)
			args[slices.Index(args, "--monthly")] = "--quarterly"
		}
		return hledger.ExecRunner(ctx, name, args...)
	}
	c, err := hledger.NewWithRunner("hledger", dir+"/main.journal", quarterlyCost)
	if err != nil {
		t.Skipf("hledger unavailable: %v", err)
	}
	h := serverledger.NewHandler(c, txlock.New(dir, c), dir, "", nil, nil, nil, nil)

	resp, err := h.GetPortfolioTimeseries(t.Context(), connect.NewRequest(&floatv1.GetPortfolioTimeseriesRequest{}))
	if err != nil {
		t.Fatalf("GetPortfolioTimeseries: %v", err)
	}
	got := map[string]string{}
	for _, s := range resp.Msg.Snapshots {
		got[s.Date] = s.CostBasis.GetQuantity()
	}
	want := map[string]string{
		"2026-01-01": "2300.00",
		"2026-02-01": "2300.00",
		"2026-03-01": "2300.00",
		"2026-04-01": "2640.00",
	}
	for date, q := range want {
		if got[date] != q {
			t.Errorf("%s: CostBasis = %q, want %q; all: %v", date, got[date], q, got)
		}
	}
}
