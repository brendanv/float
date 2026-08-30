package ledger

import (
	"reflect"
	"testing"

	"github.com/brendanv/float/internal/hledger"
)

func usd(q float64) []hledger.Amount {
	return []hledger.Amount{{Commodity: "USD", Quantity: hledger.AmountQuantity{FloatingPoint: q}}}
}

func TestSliceBalanceSheetTimeseriesPadsMissingMonths(t *testing.T) {
	ts := &hledger.BalanceSheetTimeseries{
		Periods:  []string{"2026-01-01", "2026-02-01"},
		NetWorth: [][]hledger.Amount{usd(100), usd(150)},
		Subreports: []hledger.BSSubreport{
			{Name: "Assets", Totals: [][]hledger.Amount{usd(100), usd(150)}},
		},
	}

	// Request a range that extends a month before and two months after the
	// fetched periods.
	out := sliceBalanceSheetTimeseries(ts, "2025-12-01", "2026-04-01")

	wantPeriods := []string{"2025-12-01", "2026-01-01", "2026-02-01", "2026-03-01"}
	if !reflect.DeepEqual(out.Periods, wantPeriods) {
		t.Fatalf("Periods = %v, want %v", out.Periods, wantPeriods)
	}
	// Before first real period: zero (nil/empty amounts).
	if len(out.NetWorth[0]) != 0 {
		t.Errorf("NetWorth[2025-12] = %v, want empty (zero)", out.NetWorth[0])
	}
	// Real periods pass through unchanged.
	if !reflect.DeepEqual(out.NetWorth[1], usd(100)) {
		t.Errorf("NetWorth[2026-01] = %v, want %v", out.NetWorth[1], usd(100))
	}
	if !reflect.DeepEqual(out.NetWorth[2], usd(150)) {
		t.Errorf("NetWorth[2026-02] = %v, want %v", out.NetWorth[2], usd(150))
	}
	// After last real period: carries the last cumulative value forward.
	if !reflect.DeepEqual(out.NetWorth[3], usd(150)) {
		t.Errorf("NetWorth[2026-03] = %v, want carried-forward %v", out.NetWorth[3], usd(150))
	}
	if !reflect.DeepEqual(out.Subreports[0].Totals[3], usd(150)) {
		t.Errorf("Subreports[Assets].Totals[2026-03] = %v, want carried-forward %v", out.Subreports[0].Totals[3], usd(150))
	}
}

func TestSliceBalanceSheetTimeseriesAllBeforeFirstActivity(t *testing.T) {
	ts := &hledger.BalanceSheetTimeseries{
		Periods:    []string{"2026-01-01", "2026-02-01"},
		NetWorth:   [][]hledger.Amount{usd(100), usd(150)},
		Subreports: []hledger.BSSubreport{{Name: "Assets", Totals: [][]hledger.Amount{usd(100), usd(150)}}},
	}
	// Requested range entirely before the ledger's first activity.
	out := sliceBalanceSheetTimeseries(ts, "2025-06-01", "2025-08-01")
	if len(out.Periods) != 2 {
		t.Fatalf("Periods = %v, want 2 zero-valued periods", out.Periods)
	}
	for i, amts := range out.NetWorth {
		if len(amts) != 0 {
			t.Errorf("NetWorth[%d] = %v, want empty (zero)", i, amts)
		}
	}
}

func TestSliceIncomeStatementTimeseriesPadsAndDropsEmptyRows(t *testing.T) {
	ts := &hledger.IncomeStatementTimeseries{
		Periods:    []string{"2026-01-01", "2026-02-01"},
		NetAmounts: [][]hledger.Amount{usd(-50), usd(-60)},
		Subreports: []hledger.ISSubreport{
			{
				Name: "Expenses",
				Rows: []hledger.ISRow{
					{
						DisplayName:      "groceries",
						FullName:         "expenses:groceries",
						PerPeriodAmounts: [][]hledger.Amount{usd(50), usd(60)},
					},
					{
						DisplayName:      "moving",
						FullName:         "expenses:moving",
						PerPeriodAmounts: [][]hledger.Amount{nil, nil}, // no activity in these periods
					},
				},
				Totals: [][]hledger.Amount{usd(50), usd(60)},
			},
		},
	}

	out := sliceIncomeStatementTimeseries(ts, "2025-12-01", "2026-03-01")

	wantPeriods := []string{"2025-12-01", "2026-01-01", "2026-02-01"}
	if !reflect.DeepEqual(out.Periods, wantPeriods) {
		t.Fatalf("Periods = %v, want %v", out.Periods, wantPeriods)
	}
	if len(out.NetAmounts[0]) != 0 {
		t.Errorf("NetAmounts[2025-12] = %v, want empty (no flow)", out.NetAmounts[0])
	}

	rows := out.Subreports[0].Rows
	if len(rows) != 1 {
		t.Fatalf("Rows = %v, want exactly the groceries row (moving has zero activity in range)", rows)
	}
	if rows[0].FullName != "expenses:groceries" {
		t.Errorf("surviving row = %q, want expenses:groceries", rows[0].FullName)
	}
	if len(rows[0].PerPeriodAmounts[0]) != 0 {
		t.Errorf("groceries PerPeriodAmounts[2025-12] = %v, want empty (padded)", rows[0].PerPeriodAmounts[0])
	}
	if !reflect.DeepEqual(rows[0].PerPeriodAmounts[1], usd(50)) {
		t.Errorf("groceries PerPeriodAmounts[2026-01] = %v, want %v", rows[0].PerPeriodAmounts[1], usd(50))
	}
}
