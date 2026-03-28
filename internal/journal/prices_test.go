package journal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brendanv/float/internal/testgen"
)

func TestListPrices(t *testing.T) {
	t.Run("no_prices_file", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 1, NumTxns: 1})
		prices, err := ListPrices(dir)
		if err != nil {
			t.Fatalf("ListPrices: %v", err)
		}
		if len(prices) != 0 {
			t.Errorf("expected empty slice, got %d prices", len(prices))
		}
	})

	t.Run("parses_directives", func(t *testing.T) {
		dir := t.TempDir()
		content := "; float: commodity market prices\n" +
			"P 2026-01-15 AAPL 178.50 USD  ; pid:a1b2c3d4\n" +
			"P 2026-02-01 MSFT 415.00 USD  ; pid:e5f6a7b8\n" +
			"P 2026-03-01 AAPL 182.00 USD\n" // no PID
		if err := os.WriteFile(filepath.Join(dir, "prices.journal"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		prices, err := ListPrices(dir)
		if err != nil {
			t.Fatalf("ListPrices: %v", err)
		}
		if len(prices) != 3 {
			t.Fatalf("expected 3 prices, got %d", len(prices))
		}

		tests := []struct {
			idx       int
			date      string
			commodity string
			quantity  string
			currency  string
			pid       string
		}{
			{0, "2026-01-15", "AAPL", "178.50", "USD", "a1b2c3d4"},
			{1, "2026-02-01", "MSFT", "415.00", "USD", "e5f6a7b8"},
			{2, "2026-03-01", "AAPL", "182.00", "USD", ""},
		}
		for _, tt := range tests {
			p := prices[tt.idx]
			if p.Date != tt.date {
				t.Errorf("[%d] Date = %q, want %q", tt.idx, p.Date, tt.date)
			}
			if p.Commodity != tt.commodity {
				t.Errorf("[%d] Commodity = %q, want %q", tt.idx, p.Commodity, tt.commodity)
			}
			if p.Quantity != tt.quantity {
				t.Errorf("[%d] Quantity = %q, want %q", tt.idx, p.Quantity, tt.quantity)
			}
			if p.Currency != tt.currency {
				t.Errorf("[%d] Currency = %q, want %q", tt.idx, p.Currency, tt.currency)
			}
			if p.PID != tt.pid {
				t.Errorf("[%d] PID = %q, want %q", tt.idx, p.PID, tt.pid)
			}
		}
	})
}

func TestAppendPrice(t *testing.T) {
	t.Run("creates_prices_file", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 10, NumTxns: 1})

		pid, err := AppendPrice(dir, "2026-03-15", "AAPL", "178.50", "USD")
		if err != nil {
			t.Fatalf("AppendPrice: %v", err)
		}
		if len(pid) != 8 {
			t.Errorf("expected 8-char pid, got %q", pid)
		}

		prices, err := ListPrices(dir)
		if err != nil {
			t.Fatalf("ListPrices: %v", err)
		}
		if len(prices) != 1 {
			t.Fatalf("expected 1 price, got %d", len(prices))
		}
		p := prices[0]
		if p.PID != pid {
			t.Errorf("PID = %q, want %q", p.PID, pid)
		}
		if p.Commodity != "AAPL" {
			t.Errorf("Commodity = %q, want AAPL", p.Commodity)
		}
		if p.Quantity != "178.50" {
			t.Errorf("Quantity = %q, want 178.50", p.Quantity)
		}
		if p.Currency != "USD" {
			t.Errorf("Currency = %q, want USD", p.Currency)
		}
	})

	t.Run("include_prepended_in_main", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 11, NumTxns: 1})

		if _, err := AppendPrice(dir, "2026-01-01", "BTC", "45000.00", "USD"); err != nil {
			t.Fatalf("AppendPrice: %v", err)
		}

		mainData, err := os.ReadFile(filepath.Join(dir, "main.journal"))
		if err != nil {
			t.Fatalf("read main.journal: %v", err)
		}
		lines := strings.Split(string(mainData), "\n")

		// Find prices include and the first month include.
		pricesIdx, monthIdx := -1, -1
		for i, line := range lines {
			if strings.TrimSpace(line) == "include prices.journal" {
				pricesIdx = i
			}
			if strings.HasPrefix(strings.TrimSpace(line), "include 20") && monthIdx == -1 {
				monthIdx = i
			}
		}
		if pricesIdx == -1 {
			t.Fatal("include prices.journal not found in main.journal")
		}
		if monthIdx != -1 && pricesIdx > monthIdx {
			t.Errorf("prices include (line %d) should appear before month includes (line %d)", pricesIdx, monthIdx)
		}
	})

	t.Run("idempotent_main_include", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 12, NumTxns: 1})

		for i := range 3 {
			if _, err := AppendPrice(dir, "2026-01-01", "ETH", "2000.00", "USD"); err != nil {
				t.Fatalf("AppendPrice #%d: %v", i+1, err)
			}
		}

		mainData, err := os.ReadFile(filepath.Join(dir, "main.journal"))
		if err != nil {
			t.Fatalf("read main.journal: %v", err)
		}
		count := strings.Count(string(mainData), "include prices.journal")
		if count != 1 {
			t.Errorf("expected 1 prices include, found %d", count)
		}
	})
}

func TestDeletePrice(t *testing.T) {
	t.Run("removes_price", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 20, NumTxns: 1})

		pid1, err := AppendPrice(dir, "2026-01-15", "AAPL", "178.50", "USD")
		if err != nil {
			t.Fatalf("AppendPrice 1: %v", err)
		}
		pid2, err := AppendPrice(dir, "2026-02-01", "AAPL", "182.00", "USD")
		if err != nil {
			t.Fatalf("AppendPrice 2: %v", err)
		}

		if err := DeletePrice(dir, pid1); err != nil {
			t.Fatalf("DeletePrice: %v", err)
		}

		prices, err := ListPrices(dir)
		if err != nil {
			t.Fatalf("ListPrices after delete: %v", err)
		}
		if len(prices) != 1 {
			t.Fatalf("expected 1 price after delete, got %d", len(prices))
		}
		if prices[0].PID != pid2 {
			t.Errorf("remaining PID = %q, want %q", prices[0].PID, pid2)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 21, NumTxns: 1})

		if _, err := AppendPrice(dir, "2026-01-01", "AAPL", "100.00", "USD"); err != nil {
			t.Fatalf("AppendPrice: %v", err)
		}

		err := DeletePrice(dir, "00000000")
		if err == nil {
			t.Fatal("expected error for non-existent pid, got nil")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("no_prices_file", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 22, NumTxns: 1})

		err := DeletePrice(dir, "a1b2c3d4")
		if err == nil {
			t.Fatal("expected error when prices.journal absent, got nil")
		}
	})
}
