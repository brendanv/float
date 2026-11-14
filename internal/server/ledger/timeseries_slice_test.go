package ledger

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/brendanv/float/internal/hledger"
)

func usd(q float64) []hledger.Amount {
	return []hledger.Amount{{Commodity: "USD", Quantity: hledger.AmountQuantity{FloatingPoint: q}}}
}

// twoMonthBS is a full-history bs timeseries with activity in Jan and Feb 2026.
func twoMonthBS() *hledger.BalanceSheetTimeseries {
	return &hledger.BalanceSheetTimeseries{
		Periods:  []string{"2026-01-01", "2026-02-01"},
		NetWorth: [][]hledger.Amount{usd(100), usd(150)},
		Subreports: []hledger.BSSubreport{
			{Name: "Assets", Totals: [][]hledger.Amount{usd(100), usd(150)}},
		},
	}
}

func TestSliceBalanceSheetTimeseries(t *testing.T) {
	tests := []struct {
		name        string
		ts          *hledger.BalanceSheetTimeseries
		begin, end  string
		wantPeriods []string
		want        [][]hledger.Amount // NetWorth and Assets totals per period
	}{
		{
			name:        "pads before and carries forward after",
			ts:          twoMonthBS(),
			begin:       "2025-12-01",
			end:         "2026-04-01",
			wantPeriods: []string{"2025-12-01", "2026-01-01", "2026-02-01", "2026-03-01"},
			want:        [][]hledger.Amount{nil, usd(100), usd(150), usd(150)},
		},
		{
			name:        "entirely before first activity",
			ts:          twoMonthBS(),
			begin:       "2025-06-01",
			end:         "2025-08-01",
			wantPeriods: []string{"2025-06-01", "2025-07-01"},
			want:        [][]hledger.Amount{nil, nil},
		},
		{
			name:        "entirely after last activity seeds from earlier period",
			ts:          twoMonthBS(),
			begin:       "2026-05-01",
			end:         "2026-08-01",
			wantPeriods: []string{"2026-05-01", "2026-06-01", "2026-07-01"},
			want:        [][]hledger.Amount{usd(150), usd(150), usd(150)},
		},
		{
			name:        "end-only pads through end",
			ts:          twoMonthBS(),
			end:         "2026-05-01",
			wantPeriods: []string{"2026-01-01", "2026-02-01", "2026-03-01", "2026-04-01"},
			want:        [][]hledger.Amount{usd(100), usd(150), usd(150), usd(150)},
		},
		{
			name:        "end-only before first activity is a single period at end",
			ts:          twoMonthBS(),
			end:         "2025-12-01",
			wantPeriods: []string{"2025-12-01"},
			want:        [][]hledger.Amount{nil},
		},
		{
			name:        "begin-only before first activity",
			ts:          twoMonthBS(),
			begin:       "2025-12-01",
			wantPeriods: []string{"2025-12-01", "2026-01-01", "2026-02-01"},
			want:        [][]hledger.Amount{nil, usd(100), usd(150)},
		},
		{
			name:        "begin-only after last activity is a single carried-forward period",
			ts:          twoMonthBS(),
			begin:       "2026-06-01",
			wantPeriods: []string{"2026-06-01"},
			want:        [][]hledger.Amount{usd(150)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := sliceBalanceSheetTimeseries(tt.ts, tt.begin, tt.end)
			if !reflect.DeepEqual(out.Periods, tt.wantPeriods) {
				t.Fatalf("Periods = %v, want %v", out.Periods, tt.wantPeriods)
			}
			if !reflect.DeepEqual(out.NetWorth, tt.want) {
				t.Errorf("NetWorth = %v, want %v", out.NetWorth, tt.want)
			}
			if !reflect.DeepEqual(out.Subreports[0].Totals, tt.want) {
				t.Errorf("Assets totals = %v, want %v", out.Subreports[0].Totals, tt.want)
			}
		})
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

	out, ok := sliceIncomeStatementTimeseries(ts, "2025-12-01", "2026-03-01")
	if !ok {
		t.Fatal("sliceIncomeStatementTimeseries ok = false, want true (no parent rows)")
	}

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

func TestCanSliceTimeseries(t *testing.T) {
	tests := []struct {
		begin, end string
		want       bool
	}{
		{"", "", true},
		{"2026-01-01", "", true},
		{"", "2026-01-01", true},
		{"2026-01-01", "2026-03-01", true},
		{"2026-01-15", "", false},
		{"", "2026-03-02", false},
		{"2026-13-01", "", false},
		{"not-a-date", "", false},
		{"2026-03-01", "2026-03-01", false}, // empty range
		{"2026-04-01", "2026-03-01", false}, // inverted range
	}
	for _, tt := range tests {
		if got := canSliceTimeseries(tt.begin, tt.end); got != tt.want {
			t.Errorf("canSliceTimeseries(%q, %q) = %v, want %v", tt.begin, tt.end, got, tt.want)
		}
	}
}

// timeseriesJournals are fixtures for TestTimeseriesSliceMatchesHledger.
var timeseriesJournals = map[string]string{
	// Activity in Jan and Feb 2026 only; assets end at $95.
	"two-months": `
2026-01-10 Paycheck
    assets:checking     $100
    income:salary
2026-02-15 Lunch
    expenses:food:dining   $5
    assets:checking
`,
	// Activity on the first of Jan and the last of Apr, with empty months
	// between, nested expense accounts, and a liability.
	"gap": `
2026-01-01 Paycheck
    assets:checking     $1000
    income:salary
2026-01-01 Groceries
    expenses:food:groceries   $40
    liabilities:card
2026-04-30 Dinner
    expenses:food:dining   $25
    expenses:fun            $10
    assets:checking
2026-04-30 Refund
    assets:checking   $3
    income:refunds
`,
	// Prices after the last transaction extend the bs span.
	"investments": `
P 2026-04-26 AAPL 178.50 USD
2026-01-15 Buy AAPL
    assets:investments:aapl    10 AAPL @ 175.00 USD
    assets:checking           -1750.00 USD
2026-02-01 Salary
    assets:checking   3000 USD
    income:salary
`,
	"empty": "",
}

// timeseriesRanges are month-aligned [begin, end) ranges relative to the
// fixtures above: before, inside, spanning, and after their activity, with
// both bounds, begin only, end only, and neither.
var timeseriesRanges = [][2]string{
	{"", ""},
	{"2025-10-01", "2025-12-01"},
	{"2025-11-01", "2026-04-01"},
	{"2026-02-01", "2026-03-01"},
	{"2026-02-01", "2026-06-01"},
	{"2026-05-01", "2026-08-01"},
	{"2025-10-01", ""},
	{"2026-01-01", ""},
	{"2026-02-01", ""},
	{"2026-04-01", ""},
	{"2026-05-01", ""},
	{"2026-07-01", ""},
	{"", "2025-12-01"},
	{"", "2026-01-01"},
	{"", "2026-02-01"},
	{"", "2026-03-01"},
	{"", "2026-08-01"},
}

// TestTimeseriesSliceMatchesHledger asserts that slicing the full-history
// bs/is timeseries in Go yields exactly what real hledger returns when asked
// for each range directly — the contract GetNetWorthTimeseries and
// GetIncomeStatementTimeseries rely on.
func TestTimeseriesSliceMatchesHledger(t *testing.T) {
	// Guards against the income statement equivalence check passing
	// vacuously by always falling back to hledger.
	var slicedWithParents atomic.Int32
	t.Cleanup(func() {
		if slicedWithParents.Load() == 0 {
			t.Error("no sliced income statement with parent rows was checked against hledger")
		}
	})
	for name, journal := range timeseriesJournals {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "main.journal")
			if err := os.WriteFile(path, []byte(journal), 0o644); err != nil {
				t.Fatal(err)
			}
			c, err := hledger.New("hledger", path)
			if err != nil {
				t.Skipf("hledger unavailable: %v", err)
			}
			fullBS, err := c.BalanceSheetTimeseries(t.Context(), "", "")
			if err != nil {
				t.Fatalf("full bs: %v", err)
			}
			fullIS, err := c.IncomeStatementTimeseries(t.Context(), "", "")
			if err != nil {
				t.Fatalf("full is: %v", err)
			}
			for _, r := range timeseriesRanges {
				begin, end := r[0], r[1]
				t.Run(fmt.Sprintf("%s..%s", begin, end), func(t *testing.T) {
					wantBS, err := c.BalanceSheetTimeseries(t.Context(), begin, end)
					if err != nil {
						t.Fatalf("ranged bs: %v", err)
					}
					gotBS := sliceBalanceSheetTimeseries(fullBS, begin, end)
					if got, want := normBS(gotBS), normBS(wantBS); !reflect.DeepEqual(got, want) {
						t.Errorf("bs mismatch\n got: %v\nwant: %v", got, want)
					}

					wantIS, err := c.IncomeStatementTimeseries(t.Context(), begin, end)
					if err != nil {
						t.Fatalf("ranged is: %v", err)
					}
					gotIS, ok := sliceIncomeStatementTimeseries(fullIS, begin, end)
					if !ok {
						t.Log("is: ambiguous --tree elision, handler falls back to the ranged hledger query")
						return
					}
					if got, want := normIS(gotIS), normIS(wantIS); !reflect.DeepEqual(got, want) {
						t.Errorf("is mismatch\n got: %v\nwant: %v", got, want)
					}
					if hasParentRow(gotIS) {
						slicedWithParents.Add(1)
					}
				})
			}
		})
	}
}

// normAmounts renders amounts as "commodity:exact-quantity" strings so
// equivalent quantities compare equal regardless of decimal places, and
// nil/empty slices both become nil.
func normAmounts(amts []hledger.Amount) []string {
	var out []string
	for _, a := range amts {
		q, ok := new(big.Rat).SetString(a.Quantity.Format())
		if !ok {
			out = append(out, a.Commodity+":"+a.Quantity.Format())
			continue
		}
		out = append(out, a.Commodity+":"+q.RatString())
	}
	return out
}

func normPeriods(per [][]hledger.Amount) [][]string {
	out := make([][]string, len(per))
	for i, amts := range per {
		out[i] = normAmounts(amts)
	}
	return out
}

func normBS(ts *hledger.BalanceSheetTimeseries) map[string]any {
	m := map[string]any{"periods": ts.Periods, "net": normPeriods(ts.NetWorth)}
	for _, sub := range ts.Subreports {
		m["sub:"+sub.Name] = normPeriods(sub.Totals)
	}
	return m
}

func normIS(ts *hledger.IncomeStatementTimeseries) map[string]any {
	m := map[string]any{"periods": ts.Periods, "net": normPeriods(ts.NetAmounts)}
	for _, sub := range ts.Subreports {
		m["sub:"+sub.Name] = normPeriods(sub.Totals)
		var rows []string
		for _, r := range sub.Rows {
			rows = append(rows, fmt.Sprintf("%s|%s|%d|%s|%v|%v",
				r.FullName, r.DisplayName, r.Indent, r.Section,
				normPeriods(r.PerPeriodAmounts), normAmounts(r.TotalAmounts)))
		}
		m["rows:"+sub.Name] = rows
	}
	return m
}

func hasParentRow(ts *hledger.IncomeStatementTimeseries) bool {
	for _, sub := range ts.Subreports {
		for _, r := range sub.Rows {
			for _, o := range sub.Rows {
				if strings.HasPrefix(o.FullName, r.FullName+":") {
					return true
				}
			}
		}
	}
	return false
}

func TestAmbiguousTreeElision(t *testing.T) {
	tests := []struct {
		name  string
		names []string
		want  bool
	}{
		{"leaves only", []string{"expenses:food", "expenses:fun"}, false},
		{"parent with two branches", []string{"expenses", "expenses:food", "expenses:fun"}, false},
		{"parent with one branch", []string{"expenses", "expenses:food"}, true},
		{"one branch via elided child", []string{"expenses", "expenses:food:dining"}, true},
		{"two branches below elided child", []string{"expenses", "expenses:food:dining", "expenses:food:groceries"}, true},
		{"nested forks", []string{"expenses", "expenses:food", "expenses:food:dining", "expenses:food:groceries", "expenses:fun"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := make([]hledger.ISRow, len(tt.names))
			for i, n := range tt.names {
				rows[i] = hledger.ISRow{FullName: n}
			}
			if got := ambiguousTreeElision(rows); got != tt.want {
				t.Errorf("ambiguousTreeElision(%v) = %v, want %v", tt.names, got, tt.want)
			}
		})
	}
}
