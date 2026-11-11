package hledger_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/brendanv/float/internal/hledger"
)

func mustClient(t *testing.T, journal string) *hledger.Client {
	t.Helper()
	c, err := hledger.New("hledger", filepath.Join("testdata", journal))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestNew(t *testing.T) {
	tests := []struct {
		name        string
		binary      string
		wantErr     bool
		errContains string
	}{
		{name: "valid binary", binary: "hledger"},
		{name: "bad binary", binary: "/nonexistent/hledger", wantErr: true, errContains: "not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := hledger.New(tt.binary, "testdata/simple.journal")
			if (err != nil) != tt.wantErr {
				t.Fatalf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	tests := []struct {
		name       string
		journal    string
		wantOutput string // non-empty means Check must fail with a *CheckError containing this
	}{
		{name: "valid journal", journal: "simple.journal"},
		{name: "empty journal", journal: "empty.journal"},
		{name: "assertions", journal: "assertions.journal"},
		{name: "investments", journal: "investments.journal"},
		{name: "lots", journal: "lots.journal"},
		{name: "closed accounts", journal: "closed.journal"},
		{name: "hidden meta", journal: "hiddenmeta.journal"},
		{name: "unbalanced transaction", journal: "invalid.journal", wantOutput: "UNBALANCED TRANSACTION"},
		{name: "failed balance assertion", journal: "badassertion.journal", wantOutput: "Balance assertion failed"},
		{name: "oversold lot", journal: "badlot.journal", wantOutput: "Insufficient lots"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := mustClient(t, tt.journal)
			err := c.Check(t.Context())
			if tt.wantOutput == "" {
				if err != nil {
					t.Fatalf("Check() = %v, want nil", err)
				}
				return
			}
			var checkErr *hledger.CheckError
			if !errors.As(err, &checkErr) {
				t.Fatalf("Check() = %v (%T), want *CheckError", err, err)
			}
			if !strings.Contains(checkErr.Output, tt.wantOutput) {
				t.Errorf("CheckError.Output = %q, want it to contain %q", checkErr.Output, tt.wantOutput)
			}
		})
	}
}

func TestBalances(t *testing.T) {
	tests := []struct {
		name    string
		journal string
		depth   int
		query   []string
		check   func(t *testing.T, report *hledger.BalanceReport)
	}{
		{
			name:    "all accounts",
			journal: "simple.journal",
			depth:   0,
			check: func(t *testing.T, report *hledger.BalanceReport) {
				if len(report.Rows) < 4 {
					t.Errorf("expected at least 4 rows, got %d", len(report.Rows))
				}
				amts := map[string]float64{}
				found := map[string]bool{}
				for _, row := range report.Rows {
					if len(row.Amounts) > 0 {
						amts[row.FullName] = row.Amounts[0].Quantity.FloatingPoint
						found[row.FullName] = true
					}
				}
				if !found["assets:checking"] {
					t.Error("assets:checking not found")
				} else if v := amts["assets:checking"]; v < 6819 || v > 6821 {
					t.Errorf("assets:checking balance ≈ 6820, got %v", v)
				}
				if !found["expenses:shopping"] {
					t.Error("expenses:shopping not found")
				} else if v := amts["expenses:shopping"]; v < 59 || v > 61 {
					t.Errorf("expenses:shopping balance ≈ 60, got %v", v)
				}
				if !found["income:salary"] {
					t.Error("income:salary not found")
				} else if amts["income:salary"] >= 0 {
					t.Errorf("income:salary should be negative, got %v", amts["income:salary"])
				}
			},
		},
		{
			name:    "depth 1 collapses sub-accounts",
			journal: "simple.journal",
			depth:   1,
			check: func(t *testing.T, report *hledger.BalanceReport) {
				for _, row := range report.Rows {
					if strings.Contains(row.FullName, ":") {
						t.Errorf("depth=1 should not have ':' in FullName, got %q", row.FullName)
					}
				}
				for _, row := range report.Rows {
					if row.FullName == "expenses" && len(row.Amounts) > 0 {
						v := row.Amounts[0].Quantity.FloatingPoint
						if v < 179 || v > 181 {
							t.Errorf("expenses balance ≈ 180, got %v", v)
						}
					}
				}
			},
		},
		{
			name:    "expense query filters accounts",
			journal: "simple.journal",
			depth:   0,
			query:   []string{"expenses"},
			check: func(t *testing.T, report *hledger.BalanceReport) {
				for _, row := range report.Rows {
					if !strings.HasPrefix(row.FullName, "expenses") {
						t.Errorf("expected only expenses rows, got %q", row.FullName)
					}
				}
			},
		},
		{
			name:    "empty journal has no rows",
			journal: "empty.journal",
			depth:   0,
			check: func(t *testing.T, report *hledger.BalanceReport) {
				if len(report.Rows) != 0 {
					t.Errorf("expected no rows, got %d", len(report.Rows))
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := mustClient(t, tt.journal)
			report, err := c.Balances(t.Context(), tt.depth, tt.query...)
			if err != nil {
				t.Fatalf("Balances: %v", err)
			}
			tt.check(t, report)
		})
	}
}

// amountsByAccount flattens a balance report to account -> "QTY COMMODITY" strings.
func amountsByAccount(report *hledger.BalanceReport) map[string][]string {
	out := map[string][]string{}
	for _, row := range report.Rows {
		for _, a := range row.Amounts {
			out[row.FullName] = append(out[row.FullName], a.Quantity.Format()+" "+a.Commodity)
		}
	}
	return out
}

func TestBalancesValued(t *testing.T) {
	c := mustClient(t, "investments.journal")

	raw, err := c.Balances(t.Context(), 0, "assets:investments")
	if err != nil {
		t.Fatalf("Balances: %v", err)
	}
	wantRaw := map[string][]string{
		"assets:investments:aapl": {"10 AAPL"},
		"assets:investments:msft": {"5 MSFT"},
	}
	if got := amountsByAccount(raw); !reflect.DeepEqual(got, wantRaw) {
		t.Errorf("raw balances = %v, want %v", got, wantRaw)
	}

	// The 2026-04-26 P directives are the latest prices, so now,USD is
	// deterministic for any run date after them: 10 × 178.50 and 5 × 398.75.
	valued, err := c.BalancesValued(t.Context(), "now,USD", 0, "assets:investments")
	if err != nil {
		t.Fatalf("BalancesValued: %v", err)
	}
	wantValued := map[string][]string{
		"assets:investments:aapl": {"1785 USD"},
		"assets:investments:msft": {"1993.75 USD"},
	}
	if got := amountsByAccount(valued); !reflect.DeepEqual(got, wantValued) {
		t.Errorf("valued balances = %v, want %v", got, wantValued)
	}
}

func TestBalancesCost(t *testing.T) {
	c := mustClient(t, "investments.journal")

	cost, err := c.BalancesCost(t.Context(), 0, "assets:investments")
	if err != nil {
		t.Fatalf("BalancesCost: %v", err)
	}
	want := map[string][]string{
		"assets:investments:aapl": {"1750 USD"},
		"assets:investments:msft": {"2000 USD"},
	}
	if got := amountsByAccount(cost); !reflect.DeepEqual(got, want) {
		t.Errorf("cost balances = %v, want %v", got, want)
	}
}

// assetsTotals returns the Assets subreport totals as "QTY COMMODITY" per period.
func assetsTotals(t *testing.T, ts *hledger.BalanceSheetTimeseries) []string {
	t.Helper()
	for _, sub := range ts.Subreports {
		if sub.Name != "Assets" {
			continue
		}
		out := make([]string, len(sub.Totals))
		for i, amts := range sub.Totals {
			var parts []string
			for _, a := range amts {
				parts = append(parts, a.Quantity.Format()+" "+a.Commodity)
			}
			out[i] = strings.Join(parts, ", ")
		}
		return out
	}
	t.Fatal("Assets subreport not found")
	return nil
}

func TestPortfolioTimeseries(t *testing.T) {
	c := mustClient(t, "investments.journal")

	tests := []struct {
		name        string
		begin       string
		wantPeriods []string
		wantTotals  []string
	}{
		{
			// Jan–Mar are valued at the market prices inferred from the @
			// costs; April picks up the P directives.
			name:        "full history",
			wantPeriods: []string{"2026-01-01", "2026-02-01", "2026-03-01", "2026-04-01"},
			wantTotals:  []string{"1750 USD", "3750 USD", "3750 USD", "3778.75 USD"},
		},
		{
			name:        "begin is historical",
			begin:       "2026-02-01",
			wantPeriods: []string{"2026-02-01", "2026-03-01", "2026-04-01"},
			wantTotals:  []string{"3750 USD", "3750 USD", "3778.75 USD"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, err := c.PortfolioTimeseries(t.Context(), []string{"assets:investments"}, tt.begin)
			if err != nil {
				t.Fatalf("PortfolioTimeseries: %v", err)
			}
			if !reflect.DeepEqual(ts.Periods, tt.wantPeriods) {
				t.Errorf("Periods = %v, want %v", ts.Periods, tt.wantPeriods)
			}
			if got := assetsTotals(t, ts); !reflect.DeepEqual(got, tt.wantTotals) {
				t.Errorf("Assets totals = %v, want %v", got, tt.wantTotals)
			}
		})
	}
}

func TestPortfolioCostBasisTimeseries(t *testing.T) {
	tests := []struct {
		name        string
		journal     string
		accounts    []string
		wantPeriods []string
		wantTotals  []string
	}{
		{
			name:        "@ unit costs",
			journal:     "investments.journal",
			accounts:    []string{"assets:investments"},
			wantPeriods: []string{"2026-01-01", "2026-02-01"},
			wantTotals:  []string{"1750 USD", "3750 USD"},
		},
		{
			// Lot acquires carry only a {} basis; hledger infers the @ cost
			// from it, so --cost still yields the cost basis. February nets
			// out the disposed and transferred-away lots.
			name:        "lot cost basis",
			journal:     "lots.journal",
			accounts:    []string{"assets:brokerage"},
			wantPeriods: []string{"2026-01-01", "2026-02-01"},
			wantTotals:  []string{"2204.567 $", "1724.567 $"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := mustClient(t, tt.journal)
			ts, err := c.PortfolioCostBasisTimeseries(t.Context(), tt.accounts, "")
			if err != nil {
				t.Fatalf("PortfolioCostBasisTimeseries: %v", err)
			}
			if !reflect.DeepEqual(ts.Periods, tt.wantPeriods) {
				t.Errorf("Periods = %v, want %v", ts.Periods, tt.wantPeriods)
			}
			if got := assetsTotals(t, ts); !reflect.DeepEqual(got, tt.wantTotals) {
				t.Errorf("Assets totals = %v, want %v", got, tt.wantTotals)
			}
		})
	}
}

func TestRegister(t *testing.T) {
	tests := []struct {
		name  string
		query []string
		check func(t *testing.T, rows []hledger.RegisterRow)
	}{
		{
			name: "all transactions",
			check: func(t *testing.T, rows []hledger.RegisterRow) {
				if len(rows) != 10 {
					t.Errorf("expected 10 rows (5 txns × 2 postings), got %d", len(rows))
				}
				if rows[0].Date == nil {
					t.Error("first row Date should be non-nil")
				}
				if rows[1].Date != nil {
					t.Error("second row Date should be nil")
				}
			},
		},
		{
			name:  "fid tag query",
			query: []string{"code:bb002200"},
			check: func(t *testing.T, rows []hledger.RegisterRow) {
				if len(rows) != 2 {
					t.Errorf("expected 2 rows, got %d", len(rows))
				}
				if rows[0].Description == nil || *rows[0].Description != "AMAZON MARKETPLACE" {
					t.Errorf("expected description AMAZON MARKETPLACE, got %v", rows[0].Description)
				}
				var foundShopping bool
				for _, row := range rows {
					if row.Posting.Account == "expenses:shopping" {
						foundShopping = true
						if len(row.Posting.Amounts) > 0 {
							fp := row.Posting.Amounts[0].Quantity.FloatingPoint
							if fp < 44 || fp > 46 {
								t.Errorf("expenses:shopping amount ≈ 45, got %v", fp)
							}
						}
					}
				}
				if !foundShopping {
					t.Error("expenses:shopping posting not found")
				}
			},
		},
		{
			name:  "date filter",
			query: []string{"date:2026-01"},
			check: func(t *testing.T, rows []hledger.RegisterRow) {
				if len(rows) != 6 {
					t.Errorf("expected 6 rows (3 Jan txns × 2 postings), got %d", len(rows))
				}
			},
		},
	}
	c := mustClient(t, "simple.journal")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, err := c.Register(t.Context(), tt.query...)
			if err != nil {
				t.Fatalf("Register: %v", err)
			}
			tt.check(t, rows)
		})
	}
}

func TestAregister(t *testing.T) {
	type expected struct {
		fid           string
		change        float64
		balance       float64
		otherAccounts []string
	}
	tests := []struct {
		name    string
		account string
		query   []string
		want    []expected
	}{
		{
			name:    "assets:checking full history",
			account: "assets:checking",
			want: []expected{
				{fid: "aa001100", change: 3500, balance: 3500, otherAccounts: []string{"income:salary"}},
				{fid: "bb002200", change: -45, balance: 3455, otherAccounts: []string{"expenses:shopping"}},
				{fid: "cc003300", change: -120, balance: 3335, otherAccounts: []string{"expenses:food"}},
				{fid: "dd004400", change: -15, balance: 3320, otherAccounts: []string{"expenses:shopping"}},
				{fid: "ee005500", change: 3500, balance: 6820, otherAccounts: []string{"income:salary"}},
			},
		},
		{
			name:    "assets subtree matches checking",
			account: "assets",
			want: []expected{
				{fid: "aa001100", change: 3500, balance: 3500, otherAccounts: []string{"income:salary"}},
				{fid: "bb002200", change: -45, balance: 3455, otherAccounts: []string{"expenses:shopping"}},
				{fid: "cc003300", change: -120, balance: 3335, otherAccounts: []string{"expenses:food"}},
				{fid: "dd004400", change: -15, balance: 3320, otherAccounts: []string{"expenses:shopping"}},
				{fid: "ee005500", change: 3500, balance: 6820, otherAccounts: []string{"income:salary"}},
			},
		},
		{
			name:    "date query filters to January",
			account: "assets:checking",
			query:   []string{"date:2026-01"},
			want: []expected{
				{fid: "aa001100", change: 3500, balance: 3500, otherAccounts: []string{"income:salary"}},
				{fid: "bb002200", change: -45, balance: 3455, otherAccounts: []string{"expenses:shopping"}},
				{fid: "cc003300", change: -120, balance: 3335, otherAccounts: []string{"expenses:food"}},
			},
		},
	}
	c := mustClient(t, "simple.journal")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, err := c.Aregister(t.Context(), tt.account, tt.query...)
			if err != nil {
				t.Fatalf("Aregister: %v", err)
			}
			if len(rows) != len(tt.want) {
				t.Fatalf("got %d rows, want %d", len(rows), len(tt.want))
			}
			for i, want := range tt.want {
				got := rows[i]
				if got.Transaction.FID != want.fid {
					t.Errorf("row %d FID = %q, want %q", i, got.Transaction.FID, want.fid)
				}
				if len(got.Change) != 1 {
					t.Fatalf("row %d Change: expected 1 amount, got %d", i, len(got.Change))
				}
				if fp := got.Change[0].Quantity.FloatingPoint; fp < want.change-0.5 || fp > want.change+0.5 {
					t.Errorf("row %d Change ≈ %v, got %v", i, want.change, fp)
				}
				if len(got.Balance) != 1 {
					t.Fatalf("row %d Balance: expected 1 amount, got %d", i, len(got.Balance))
				}
				if fp := got.Balance[0].Quantity.FloatingPoint; fp < want.balance-0.5 || fp > want.balance+0.5 {
					t.Errorf("row %d Balance ≈ %v, got %v", i, want.balance, fp)
				}
				if len(got.OtherAccounts) != len(want.otherAccounts) {
					t.Errorf("row %d OtherAccounts = %v, want %v", i, got.OtherAccounts, want.otherAccounts)
				} else {
					for j, a := range want.otherAccounts {
						if got.OtherAccounts[j] != a {
							t.Errorf("row %d OtherAccounts[%d] = %q, want %q", i, j, got.OtherAccounts[j], a)
						}
					}
				}
			}
		})
	}
}

func TestAccounts(t *testing.T) {
	tests := []struct {
		name  string
		tree  bool
		check func(t *testing.T, nodes []*hledger.AccountNode)
	}{
		{
			name: "flat list",
			tree: false,
			check: func(t *testing.T, nodes []*hledger.AccountNode) {
				if len(nodes) != 4 {
					t.Errorf("expected 4 nodes, got %d", len(nodes))
				}
				for _, n := range nodes {
					if n.Children != nil {
						t.Errorf("flat node %q should have nil Children", n.FullName)
					}
				}
				var foundChecking bool
				for _, n := range nodes {
					if n.FullName == "assets:checking" {
						foundChecking = true
						if n.Type != hledger.AccountTypeCash {
							t.Errorf("assets:checking expected type C, got %q", n.Type)
						}
					}
					if strings.HasPrefix(n.FullName, "expenses:") && n.Type != hledger.AccountTypeExpense {
						t.Errorf("%q expected type X, got %q", n.FullName, n.Type)
					}
				}
				if !foundChecking {
					t.Error("assets:checking not found in flat accounts")
				}
			},
		},
		{
			name: "tree with children",
			tree: true,
			check: func(t *testing.T, roots []*hledger.AccountNode) {
				if len(roots) != 3 {
					t.Errorf("expected 3 root nodes (assets, expenses, income), got %d", len(roots))
				}
				var expensesNode *hledger.AccountNode
				for _, r := range roots {
					if r.FullName == "expenses" {
						expensesNode = r
						if r.Type != hledger.AccountTypeExpense {
							t.Errorf("expenses root expected type X, got %q", r.Type)
						}
					}
					if r.FullName == "income" && r.Type != hledger.AccountTypeRevenue {
						t.Errorf("income root expected type R, got %q", r.Type)
					}
				}
				if expensesNode == nil {
					t.Fatal("expenses root node not found")
					return
				}
				if len(expensesNode.Children) != 2 {
					t.Errorf("expected expenses to have 2 children, got %d", len(expensesNode.Children))
				}
				for _, child := range expensesNode.Children {
					if !strings.HasPrefix(child.FullName, "expenses:") {
						t.Errorf("child FullName should start with 'expenses:', got %q", child.FullName)
					}
					if child.Type != hledger.AccountTypeExpense {
						t.Errorf("child %q expected type X, got %q", child.FullName, child.Type)
					}
				}
			},
		},
	}
	c := mustClient(t, "simple.journal")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes, err := c.Accounts(t.Context(), tt.tree)
			if err != nil {
				t.Fatalf("Accounts: %v", err)
			}
			tt.check(t, nodes)
		})
	}
}

func TestPrintCSV(t *testing.T) {
	tests := []struct {
		name      string
		csvFile   string
		rulesFile string
		wantErr   bool
		check     func(t *testing.T, txns []hledger.Transaction)
	}{
		{
			name:      "valid import",
			csvFile:   "testdata/import.csv",
			rulesFile: "testdata/import.rules",
			check: func(t *testing.T, txns []hledger.Transaction) {
				if len(txns) != 3 {
					t.Errorf("expected 3 transactions, got %d", len(txns))
				}
				var foundAmazon, foundPayroll bool
				for _, txn := range txns {
					if txn.Description == "AMAZON MARKETPLACE" {
						foundAmazon = true
						for _, p := range txn.Postings {
							if p.Account == "expenses:shopping" && len(p.Amounts) > 0 {
								fp := p.Amounts[0].Quantity.FloatingPoint
								if fp < 44 || fp > 46 {
									t.Errorf("AMAZON shopping amount ≈ 45, got %v", fp)
								}
							}
						}
					}
					if txn.Description == "PAYROLL DIRECT DEPOSIT" {
						foundPayroll = true
						var hasSalary bool
						for _, p := range txn.Postings {
							if p.Account == "income:salary" {
								hasSalary = true
							}
						}
						if !hasSalary {
							t.Error("PAYROLL transaction missing income:salary posting")
						}
					}
				}
				if !foundAmazon {
					t.Error("AMAZON MARKETPLACE transaction not found")
				}
				if !foundPayroll {
					t.Error("PAYROLL DIRECT DEPOSIT transaction not found")
				}
			},
		},
		{
			// Quoted field with a comma, thousands separator, an integer
			// amount, unmatched rows falling back to expenses/income:unknown,
			// and out-of-order rows (hledger sorts by date).
			name:      "edge cases",
			csvFile:   "testdata/import_edge.csv",
			rulesFile: "testdata/import.rules",
			check: func(t *testing.T, txns []hledger.Transaction) {
				type row struct{ date, desc, account, checking string }
				want := []row{
					{"2026-01-05", "PAYROLL DIRECT DEPOSIT", "income:salary", "3500.00"},
					{"2026-01-15", "AMAZON MARKETPLACE, SEATTLE WA", "expenses:shopping", "-45.00"},
					{"2026-01-20", "WHOLE FOODS", "expenses:food", "-120"},
					{"2026-01-22", "UNKNOWN MERCHANT", "expenses:unknown", "-12.34"},
					{"2026-01-23", "REFUND FROM SOMEWHERE", "income:unknown", "8.00"},
				}
				var got []row
				for _, tx := range txns {
					if len(tx.Postings) != 2 || tx.Postings[0].Account != "assets:checking" {
						t.Errorf("%s: unexpected postings %+v", tx.Description, tx.Postings)
						continue
					}
					got = append(got, row{tx.Date, tx.Description, tx.Postings[1].Account, tx.Postings[0].Amounts[0].Quantity.Format()})
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("got  %v\nwant %v", got, want)
				}
			},
		},
		{
			name:      "nonexistent rules file",
			csvFile:   "testdata/import.csv",
			rulesFile: "testdata/nonexistent.rules",
			wantErr:   true,
		},
	}
	c := mustClient(t, "simple.journal")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			txns, err := c.PrintCSV(t.Context(), tt.csvFile, tt.rulesFile)
			if (err != nil) != tt.wantErr {
				t.Fatalf("PrintCSV() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil {
				tt.check(t, txns)
			}
		})
	}
}

func TestTransactionFID(t *testing.T) {
	const versionResp = "hledger 1.99.6, linux-x86_64\n"
	const printJSON = `[{"tindex":1,"tdate":"2026-01-05","tdate2":null,"tdescription":"PAYROLL","tcode":"aa001100","tcomment":"","ttags":[],"tpostings":[],"tstatus":"","tprecedingcomment":""},{"tindex":2,"tdate":"2026-01-15","tdate2":null,"tdescription":"AMAZON","tcode":"","tcomment":"","ttags":[],"tpostings":[],"tstatus":"","tprecedingcomment":""}]`

	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) > 0 && args[0] == "--version" {
			return []byte(versionResp), nil, nil
		}
		return []byte(printJSON), nil, nil
	}
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	result, err := c.PrintCSV(t.Context(), "testdata/import.csv", "testdata/import.rules")
	if err != nil {
		t.Fatalf("PrintCSV via stub: %v", err)
	}

	tests := []struct {
		name    string
		idx     int
		wantFID string
	}{
		{name: "transaction with fid tag", idx: 0, wantFID: "aa001100"},
		{name: "transaction without fid tag", idx: 1, wantFID: ""},
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(result))
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if result[tt.idx].FID != tt.wantFID {
				t.Errorf("FID = %q, want %q", result[tt.idx].FID, tt.wantFID)
			}
		})
	}
}

func TestTransactionPayeeNote(t *testing.T) {
	const versionResp = "hledger 1.99.6, linux-x86_64\n"
	const printJSON = `[
		{"tindex":1,"tdate":"2026-01-05","tdate2":null,"tdescription":"Acme Corp | Monthly invoice","tcode":"aa001100","tcomment":"","ttags":[],"tpostings":[],"tstatus":"","tprecedingcomment":"","tsourcepos":[{"sourceName":"","sourceLine":0,"sourceColumn":0},{"sourceName":"","sourceLine":0,"sourceColumn":0}]},
		{"tindex":2,"tdate":"2026-01-15","tdate2":null,"tdescription":"AMAZON","tcode":"","tcomment":"","ttags":[],"tpostings":[],"tstatus":"","tprecedingcomment":"","tsourcepos":[{"sourceName":"","sourceLine":0,"sourceColumn":0},{"sourceName":"","sourceLine":0,"sourceColumn":0}]},
		{"tindex":3,"tdate":"2026-01-20","tdate2":null,"tdescription":"Shop | ","tcode":"","tcomment":"","ttags":[],"tpostings":[],"tstatus":"","tprecedingcomment":"","tsourcepos":[{"sourceName":"","sourceLine":0,"sourceColumn":0},{"sourceName":"","sourceLine":0,"sourceColumn":0}]}
	]`

	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) > 0 && args[0] == "--version" {
			return []byte(versionResp), nil, nil
		}
		return []byte(printJSON), nil, nil
	}
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	result, err := c.Transactions(t.Context())
	if err != nil {
		t.Fatalf("Transactions: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 transactions, got %d", len(result))
	}

	ptr := func(s string) *string { return &s }

	tests := []struct {
		name      string
		idx       int
		wantPayee *string
		wantNote  *string
	}{
		{name: "with pipe", idx: 0, wantPayee: ptr("Acme Corp"), wantNote: ptr("Monthly invoice")},
		{name: "no pipe", idx: 1, wantPayee: nil, wantNote: nil},
		{name: "pipe with empty note", idx: 2, wantPayee: ptr("Shop"), wantNote: ptr("")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := result[tt.idx]
			if (got.Payee == nil) != (tt.wantPayee == nil) || (got.Payee != nil && *got.Payee != *tt.wantPayee) {
				t.Errorf("Payee = %v, want %v", got.Payee, tt.wantPayee)
			}
			if (got.Note == nil) != (tt.wantNote == nil) || (got.Note != nil && *got.Note != *tt.wantNote) {
				t.Errorf("Note = %v, want %v", got.Note, tt.wantNote)
			}
		})
	}
}

func TestBalanceSheetTimeseries(t *testing.T) {
	c := mustClient(t, "simple.journal")
	ts, err := c.BalanceSheetTimeseries(t.Context(), "", "")
	if err != nil {
		t.Fatalf("BalanceSheetTimeseries: %v", err)
	}

	// simple.journal has transactions in 2026-01 and 2026-02.
	if len(ts.Periods) != 2 {
		t.Fatalf("expected 2 periods, got %d", len(ts.Periods))
	}
	if ts.Periods[0] != "2026-01-01" {
		t.Errorf("period 0 date = %q, want 2026-01-01", ts.Periods[0])
	}
	if ts.Periods[1] != "2026-02-01" {
		t.Errorf("period 1 date = %q, want 2026-02-01", ts.Periods[1])
	}

	// Net worth length must match periods.
	if len(ts.NetWorth) != 2 {
		t.Fatalf("expected 2 net worth entries, got %d", len(ts.NetWorth))
	}

	// Find Assets and Liabilities subreports.
	var assetsSub, liabSub *hledger.BSSubreport
	for i := range ts.Subreports {
		switch ts.Subreports[i].Name {
		case "Assets":
			assetsSub = &ts.Subreports[i]
		case "Liabilities":
			liabSub = &ts.Subreports[i]
		}
	}
	if assetsSub == nil {
		t.Fatal("Assets subreport not found")
		return
	}
	if liabSub == nil {
		t.Fatal("Liabilities subreport not found")
		return
	}

	// simple.journal has assets:checking with $3335 in Jan (historical).
	if len(assetsSub.Totals[0]) == 0 {
		t.Error("expected non-empty assets totals for period 0")
	} else {
		v := assetsSub.Totals[0][0].Quantity.FloatingPoint
		if v < 3334 || v > 3336 {
			t.Errorf("assets period 0 ≈ 3335, got %v", v)
		}
	}

	// simple.journal has no liability accounts, so liabilities should be empty.
	if len(liabSub.Totals[0]) != 0 {
		t.Errorf("expected empty liabilities for period 0, got %v", liabSub.Totals[0])
	}

	// Net worth in period 0 should equal assets (no liabilities).
	if len(ts.NetWorth[0]) == 0 {
		t.Error("expected non-empty net worth for period 0")
	} else {
		v := ts.NetWorth[0][0].Quantity.FloatingPoint
		if v < 3334 || v > 3336 {
			t.Errorf("net worth period 0 ≈ 3335, got %v", v)
		}
	}
}

func TestNewWithRunner(t *testing.T) {
	called := false
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		called = true
		return []byte("hledger 1.99.6, linux-x86_64\n"), nil, nil
	}
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	if !called {
		t.Error("expected runner to be called during construction")
	}
	if c == nil {
		t.Error("expected non-nil client")
	}
}

func TestTags(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []string
	}{
		{
			name:   "filters empty lines",
			output: "category\nnotes\n\n",
			want:   []string{"category", "notes"},
		},
		{
			name:   "empty output",
			output: "",
			want:   nil,
		},
		{
			name:   "filters hledger internal tags",
			output: "_generated-posting\n_ptype\nlots\n",
			want:   []string{"lots"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tagsOutput := tt.output
			runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
				if len(args) > 0 && args[0] == "--version" {
					return []byte("hledger 1.99.6, linux-x86_64\n"), nil, nil
				}
				return []byte(tagsOutput), nil, nil
			}
			c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", runner)
			if err != nil {
				t.Fatalf("NewWithRunner: %v", err)
			}
			got, err := c.Tags(t.Context())
			if err != nil {
				t.Fatalf("Tags: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i, w := range tt.want {
				if got[i] != w {
					t.Errorf("tags[%d] = %q, want %q", i, got[i], w)
				}
			}
		})
	}

	integration := []struct {
		journal string
		want    []string
	}{
		{journal: "simple.journal", want: nil},
		// float-* hidden meta is filtered by the server layer, not here.
		{journal: "hiddenmeta.journal", want: []string{"category", "float-import-id", "float-updated-at"}},
		// hledger adds _ptype/_generated-posting to lot postings and inherits
		// the commodity's lots: tag; only the latter is user-visible.
		{journal: "lots.journal", want: []string{"lots"}},
	}
	for _, tt := range integration {
		t.Run("integration "+tt.journal, func(t *testing.T) {
			c, err := hledger.New("hledger", filepath.Join("testdata", tt.journal))
			if err != nil {
				t.Skip("hledger binary not available:", err)
			}
			got, err := c.Tags(t.Context())
			if err != nil {
				t.Fatalf("Tags: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Tags = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTransactionFloatMeta(t *testing.T) {
	t.Run("unit_parses_hidden_meta_from_tags", func(t *testing.T) {
		const versionResp = "hledger 1.99.6, linux-x86_64\n"
		// Simulate hledger JSON output where one transaction has float- tags mixed with a user tag.
		const printJSON = `[
			{"tindex":1,"tdate":"2026-01-15","tdate2":null,"tdescription":"Amazon","tcode":"aa001100","tcomment":"","ttags":[["category","food"],["float-import-id","batch123"],["float-updated-at","2026-01-15T10:00:00Z"]],"tpostings":[],"tstatus":"","tprecedingcomment":"","tsourcepos":[{"sourceName":"","sourceLine":0,"sourceColumn":0},{"sourceName":"","sourceLine":0,"sourceColumn":0}]},
			{"tindex":2,"tdate":"2026-01-20","tdate2":null,"tdescription":"Whole Foods","tcode":"bb002200","tcomment":"","ttags":[],"tpostings":[],"tstatus":"","tprecedingcomment":"","tsourcepos":[{"sourceName":"","sourceLine":0,"sourceColumn":0},{"sourceName":"","sourceLine":0,"sourceColumn":0}]}
		]`

		runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
			if len(args) > 0 && args[0] == "--version" {
				return []byte(versionResp), nil, nil
			}
			return []byte(printJSON), nil, nil
		}
		c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", runner)
		if err != nil {
			t.Fatalf("NewWithRunner: %v", err)
		}
		txns, err := c.Transactions(t.Context())
		if err != nil {
			t.Fatalf("Transactions: %v", err)
		}
		if len(txns) != 2 {
			t.Fatalf("expected 2 transactions, got %d", len(txns))
		}

		// First transaction: should have hidden meta and still have all tags.
		tx := txns[0]
		if tx.FloatMeta == nil {
			t.Fatal("FloatMeta is nil, expected non-nil")
		}
		if tx.FloatMeta["float-import-id"] != "batch123" {
			t.Errorf("float-import-id = %q, want %q", tx.FloatMeta["float-import-id"], "batch123")
		}
		if tx.FloatMeta["float-updated-at"] != "2026-01-15T10:00:00Z" {
			t.Errorf("float-updated-at = %q, want %q", tx.FloatMeta["float-updated-at"], "2026-01-15T10:00:00Z")
		}
		// FloatMeta should NOT include user tag.
		if _, ok := tx.FloatMeta["category"]; ok {
			t.Errorf("category should not be in FloatMeta")
		}
		// Raw Tags should still include all tags (including float- ones).
		tagMap := make(map[string]string)
		for _, kv := range tx.Tags {
			tagMap[kv[0]] = kv[1]
		}
		if tagMap["category"] != "food" {
			t.Errorf("category tag in Tags = %q, want %q", tagMap["category"], "food")
		}

		// Second transaction: no hidden meta.
		if txns[1].FloatMeta != nil {
			t.Errorf("expected nil FloatMeta for transaction without float- tags, got %v", txns[1].FloatMeta)
		}
	})

	t.Run("integration_parses_hidden_meta_from_fixture", func(t *testing.T) {
		c, err := hledger.New("hledger", "testdata/hiddenmeta.journal")
		if err != nil {
			t.Skip("hledger binary not available:", err)
		}
		txns, err := c.Transactions(t.Context())
		if err != nil {
			t.Fatalf("Transactions: %v", err)
		}
		if len(txns) != 2 {
			t.Fatalf("expected 2 transactions, got %d", len(txns))
		}
		tx := txns[0]
		if tx.FloatMeta == nil {
			t.Fatal("FloatMeta is nil for first transaction")
		}
		if tx.FloatMeta["float-import-id"] != "batch123" {
			t.Errorf("float-import-id = %q, want %q", tx.FloatMeta["float-import-id"], "batch123")
		}
		if tx.FloatMeta["float-updated-at"] != "2026-01-15T10:00:00Z" {
			t.Errorf("float-updated-at = %q, want %q", tx.FloatMeta["float-updated-at"], "2026-01-15T10:00:00Z")
		}
		if txns[1].FloatMeta != nil {
			t.Errorf("second transaction should have no hidden meta, got %v", txns[1].FloatMeta)
		}
	})
}

func TestTransactionBalanceAssertion(t *testing.T) {
	c, err := hledger.New("hledger", "testdata/assertions.journal")
	if err != nil {
		t.Skip("hledger binary not available:", err)
	}
	txns, err := c.Transactions(t.Context())
	if err != nil {
		t.Fatalf("Transactions: %v", err)
	}
	if len(txns) != 4 {
		t.Fatalf("expected 4 transactions, got %d", len(txns))
	}

	tests := []struct {
		name          string
		fid           string
		postingIdx    int
		wantNil       bool
		wantQuantity  float64
		wantInclusive bool
		wantTotal     bool
	}{
		{name: "simple = on first posting", fid: "aa000001", postingIdx: 0, wantQuantity: 1000.00},
		{name: "auto-balance posting has no assertion", fid: "aa000001", postingIdx: 1, wantNil: true},
		{name: "= after running balance update", fid: "bb000002", postingIdx: 0, wantQuantity: 1500.00},
		{name: "=* inclusive variant", fid: "cc000003", postingIdx: 0, wantQuantity: 200.00, wantInclusive: true},
		{name: "== total variant", fid: "dd000004", postingIdx: 1, wantQuantity: 1250.00, wantTotal: true},
	}

	byFID := map[string]hledger.Transaction{}
	for _, tx := range txns {
		byFID[tx.FID] = tx
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, ok := byFID[tt.fid]
			if !ok {
				t.Fatalf("no transaction with fid %q", tt.fid)
			}
			if tt.postingIdx >= len(tx.Postings) {
				t.Fatalf("posting index %d out of range (got %d postings)", tt.postingIdx, len(tx.Postings))
			}
			ba := tx.Postings[tt.postingIdx].BalanceAssertion
			if tt.wantNil {
				if ba != nil {
					t.Errorf("BalanceAssertion = %+v, want nil", ba)
				}
				return
			}
			if ba == nil {
				t.Fatalf("BalanceAssertion is nil, want non-nil")
			}
			if ba.Inclusive != tt.wantInclusive {
				t.Errorf("Inclusive = %v, want %v", ba.Inclusive, tt.wantInclusive)
			}
			if ba.Total != tt.wantTotal {
				t.Errorf("Total = %v, want %v", ba.Total, tt.wantTotal)
			}
			if ba.Amount.Quantity.FloatingPoint != tt.wantQuantity {
				t.Errorf("Quantity = %v, want %v", ba.Amount.Quantity.FloatingPoint, tt.wantQuantity)
			}
			if ba.Amount.Commodity != "$" {
				t.Errorf("Commodity = %q, want %q", ba.Amount.Commodity, "$")
			}
		})
	}
}

func TestPayees(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []string
	}{
		{
			name:   "filters empty lines",
			output: "Acme Corp\nGrocery Store\n\n",
			want:   []string{"Acme Corp", "Grocery Store"},
		},
		{
			name:   "empty output",
			output: "",
			want:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payeesOutput := tt.output
			runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
				if len(args) > 0 && args[0] == "--version" {
					return []byte("hledger 1.99.6, linux-x86_64\n"), nil, nil
				}
				return []byte(payeesOutput), nil, nil
			}
			c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", runner)
			if err != nil {
				t.Fatalf("NewWithRunner: %v", err)
			}
			got, err := c.Payees(t.Context())
			if err != nil {
				t.Fatalf("Payees: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i, w := range tt.want {
				if got[i] != w {
					t.Errorf("payees[%d] = %q, want %q", i, got[i], w)
				}
			}
		})
	}

	// Integration smoke test: Payees() against the real journal runs without error.
	t.Run("integration", func(t *testing.T) {
		c, err := hledger.New("hledger", "testdata/simple.journal")
		if err != nil {
			t.Skip("hledger binary not available:", err)
		}
		_, err = c.Payees(t.Context())
		if err != nil {
			t.Fatalf("Payees: %v", err)
		}
	})
}

func TestIncomeStatementTimeseries(t *testing.T) {
	// Unit test: parse inline JSON constant without real hledger binary.
	t.Run("unit_parses_rows", func(t *testing.T) {
		// Minimal representative JSON from `hledger is --monthly --tree -O json`.
		// prrName is the full account name; amounts and totals are trimmed to essentials.
		const isJSON = `{
"cbrDates":[
  [{"contents":"2026-01-01","tag":"Exact"},{"contents":"2026-02-01","tag":"Exact"}],
  [{"contents":"2026-02-01","tag":"Exact"},{"contents":"2026-03-01","tag":"Exact"}]
],
"cbrSubreports":[
  ["Revenues",{
    "prRows":[{"prrAmounts":[[{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":350000,"decimalPlaces":2,"floatingPoint":3500}}],[{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":350000,"decimalPlaces":2,"floatingPoint":3500}}]],"prrAverage":[],"prrName":"income:salary","prrTotal":[{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":700000,"decimalPlaces":2,"floatingPoint":7000}}]}],
    "prTotals":{"prrAmounts":[[{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":350000,"decimalPlaces":2,"floatingPoint":3500}}],[{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":350000,"decimalPlaces":2,"floatingPoint":3500}}]],"prrAverage":[],"prrName":[],"prrTotal":[]}
  },true],
  ["Expenses",{
    "prRows":[
      {"prrAmounts":[[{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":16500,"decimalPlaces":2,"floatingPoint":165}}],[{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":1500,"decimalPlaces":2,"floatingPoint":15}}]],"prrAverage":[],"prrName":"expenses","prrTotal":[{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":18000,"decimalPlaces":2,"floatingPoint":180}}]},
      {"prrAmounts":[[{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":4500,"decimalPlaces":2,"floatingPoint":45}}],[{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":1500,"decimalPlaces":2,"floatingPoint":15}}]],"prrAverage":[],"prrName":"expenses:shopping","prrTotal":[{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":6000,"decimalPlaces":2,"floatingPoint":60}}]},
      {"prrAmounts":[[{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":12000,"decimalPlaces":2,"floatingPoint":120}}],[]],"prrAverage":[],"prrName":"expenses:food","prrTotal":[{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":12000,"decimalPlaces":2,"floatingPoint":120}}]}
    ],
    "prTotals":{"prrAmounts":[[{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":16500,"decimalPlaces":2,"floatingPoint":165}}],[{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":1500,"decimalPlaces":2,"floatingPoint":15}}]],"prrAverage":[],"prrName":[],"prrTotal":[]}
  },false]
],
"cbrTotals":{"prrAmounts":[[{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":333500,"decimalPlaces":2,"floatingPoint":3335}}],[{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":348500,"decimalPlaces":2,"floatingPoint":3485}}]],"prrAverage":[],"prrName":[],"prrTotal":[]}
}`
		runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
			if len(args) > 0 && args[0] == "--version" {
				return []byte("hledger 1.99.6, linux-x86_64\n"), nil, nil
			}
			return []byte(isJSON), nil, nil
		}
		c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", runner)
		if err != nil {
			t.Fatalf("NewWithRunner: %v", err)
		}
		ts, err := c.IncomeStatementTimeseries(t.Context(), "", "")
		if err != nil {
			t.Fatalf("IncomeStatementTimeseries: %v", err)
		}

		if len(ts.Periods) != 2 {
			t.Fatalf("expected 2 periods, got %d", len(ts.Periods))
		}
		if ts.Periods[0] != "2026-01-01" {
			t.Errorf("periods[0] = %q, want %q", ts.Periods[0], "2026-01-01")
		}
		if len(ts.Subreports) != 2 {
			t.Fatalf("expected 2 subreports, got %d", len(ts.Subreports))
		}

		revSub := ts.Subreports[0]
		if revSub.Name != "Revenues" {
			t.Errorf("subreport[0].Name = %q, want %q", revSub.Name, "Revenues")
		}
		if len(revSub.Rows) != 1 {
			t.Fatalf("expected 1 revenue row, got %d", len(revSub.Rows))
		}
		row := revSub.Rows[0]
		if row.FullName != "income:salary" {
			t.Errorf("row.FullName = %q, want %q", row.FullName, "income:salary")
		}
		if row.DisplayName != "salary" {
			t.Errorf("row.DisplayName = %q, want %q", row.DisplayName, "salary")
		}
		if row.Indent != 1 {
			t.Errorf("row.Indent = %d, want 1", row.Indent)
		}
		if row.Section != "Revenues" {
			t.Errorf("row.Section = %q, want %q", row.Section, "Revenues")
		}
		if len(row.PerPeriodAmounts) != 2 {
			t.Fatalf("expected 2 per-period amounts, got %d", len(row.PerPeriodAmounts))
		}
		if len(row.TotalAmounts) != 1 || row.TotalAmounts[0].Quantity.FloatingPoint != 7000 {
			t.Errorf("unexpected TotalAmounts: %v", row.TotalAmounts)
		}

		expSub := ts.Subreports[1]
		if expSub.Name != "Expenses" {
			t.Errorf("subreport[1].Name = %q, want %q", expSub.Name, "Expenses")
		}
		if len(expSub.Rows) != 3 {
			t.Fatalf("expected 3 expense rows (parent + 2 children), got %d", len(expSub.Rows))
		}
		// Parent row has indent 0
		if expSub.Rows[0].Indent != 0 || expSub.Rows[0].FullName != "expenses" {
			t.Errorf("expenses parent row: %+v", expSub.Rows[0])
		}
		// Child rows have indent 1
		if expSub.Rows[1].Indent != 1 {
			t.Errorf("expenses child indent = %d, want 1", expSub.Rows[1].Indent)
		}

		// Net amounts: 2 periods
		if len(ts.NetAmounts) != 2 {
			t.Fatalf("expected 2 net amount periods, got %d", len(ts.NetAmounts))
		}
		if len(ts.NetAmounts[0]) != 1 || ts.NetAmounts[0][0].Quantity.FloatingPoint != 3335 {
			t.Errorf("unexpected NetAmounts[0]: %v", ts.NetAmounts[0])
		}
	})

	// Integration test: runs the real hledger binary against simple.journal.
	t.Run("integration", func(t *testing.T) {
		c, err := hledger.New("hledger", "testdata/simple.journal")
		if err != nil {
			t.Skip("hledger binary not available:", err)
		}
		ts, err := c.IncomeStatementTimeseries(t.Context(), "", "")
		if err != nil {
			t.Fatalf("IncomeStatementTimeseries: %v", err)
		}
		if len(ts.Periods) != 2 {
			t.Fatalf("expected 2 periods, got %d: %v", len(ts.Periods), ts.Periods)
		}
		if ts.Periods[0] != "2026-01-01" {
			t.Errorf("periods[0] = %q, want 2026-01-01", ts.Periods[0])
		}
		if len(ts.Subreports) < 2 {
			t.Fatalf("expected at least 2 subreports, got %d", len(ts.Subreports))
		}
		// Revenues subreport should have income:salary
		revSub := ts.Subreports[0]
		if revSub.Name != "Revenues" {
			t.Errorf("subreport[0].Name = %q, want Revenues", revSub.Name)
		}
		var foundSalary bool
		for _, r := range revSub.Rows {
			if r.FullName == "income:salary" {
				foundSalary = true
				if len(r.PerPeriodAmounts) == 0 {
					t.Error("income:salary has no per-period amounts")
				}
			}
		}
		if !foundSalary {
			t.Error("income:salary not found in Revenues subreport")
		}
		// Expenses subreport should have expenses:shopping and expenses:food
		expSub := ts.Subreports[1]
		if expSub.Name != "Expenses" {
			t.Errorf("subreport[1].Name = %q, want Expenses", expSub.Name)
		}
		foundAccounts := make(map[string]bool)
		for _, r := range expSub.Rows {
			foundAccounts[r.FullName] = true
		}
		for _, want := range []string{"expenses:shopping", "expenses:food"} {
			if !foundAccounts[want] {
				t.Errorf("account %q not found in Expenses subreport", want)
			}
		}
		if len(ts.NetAmounts) != 2 {
			t.Fatalf("expected 2 net amount periods, got %d", len(ts.NetAmounts))
		}
	})
}

func TestSetConcurrencyLimitsInFlightInvocations(t *testing.T) {
	const versionResp = "hledger 1.99.6, linux-x86_64\n"

	var inFlight, maxInFlight atomic.Int64
	release := make(chan struct{})
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) > 0 && args[0] == "--version" {
			return []byte(versionResp), nil, nil
		}
		cur := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			old := maxInFlight.Load()
			if cur <= old || maxInFlight.CompareAndSwap(old, cur) {
				break
			}
		}
		<-release
		return []byte(`[]`), nil, nil
	}

	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	c.SetConcurrency(2)

	const n = 5
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Transactions(t.Context())
		}()
	}

	// Give every goroutine a chance to attempt its acquire before releasing.
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := maxInFlight.Load(); got > 2 {
		t.Errorf("max concurrent hledger invocations = %d, want <= 2", got)
	}
	// n Transactions() calls + 1 --version call from NewWithRunner.
	if got := c.Invocations(); got != n+1 {
		t.Errorf("Invocations() = %d, want %d", got, n+1)
	}
}

// TestSetConcurrencyMidFlightDoesNotPanic confirms an in-flight invocation
// releases the same semaphore instance it acquired, not whatever c.sem has
// been swapped to by a concurrent SetConcurrency call. Run with -race.
func TestSetConcurrencyMidFlightDoesNotPanic(t *testing.T) {
	const versionResp = "hledger 1.99.6, linux-x86_64\n"
	inFlight := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once

	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) > 0 && args[0] == "--version" {
			return []byte(versionResp), nil, nil
		}
		once.Do(func() { close(inFlight) })
		<-release
		return []byte(`[]`), nil, nil
	}

	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	c.SetConcurrency(1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = c.Transactions(t.Context())
	}()

	<-inFlight
	c.SetConcurrency(4) // swap c.sem while the goroutine above still holds a slot on the old one
	close(release)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Transactions did not complete")
	}

	// A second call should acquire cleanly against the new semaphore.
	if _, err := c.Transactions(t.Context()); err != nil {
		t.Errorf("Transactions after SetConcurrency: %v", err)
	}
}

// TestWaitUncontendedBlocksWhileSlotsBusy confirms that WaitUncontended
// doesn't return while the concurrency semaphore is fully occupied, and
// returns promptly once a slot frees up — without itself ever holding a
// slot, so it can't strand it or race a normal acquire for the released one.
func TestWaitUncontendedBlocksWhileSlotsBusy(t *testing.T) {
	const versionResp = "hledger 1.99.6, linux-x86_64\n"
	busyRelease := make(chan struct{})

	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) > 0 && args[0] == "--version" {
			return []byte(versionResp), nil, nil
		}
		<-busyRelease
		return []byte(`[]`), nil, nil
	}

	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	c.SetConcurrency(1)

	busyDone := make(chan struct{})
	go func() {
		_, _ = c.Transactions(t.Context())
		close(busyDone)
	}()
	time.Sleep(50 * time.Millisecond)

	waitDone := make(chan struct{})
	go func() {
		_ = c.WaitUncontended(t.Context())
		close(waitDone)
	}()

	select {
	case <-waitDone:
		t.Fatal("WaitUncontended returned while the only slot was busy")
	case <-time.After(100 * time.Millisecond):
	}

	close(busyRelease)
	<-busyDone

	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
		t.Fatal("WaitUncontended did not return after the slot freed up")
	}
}

func TestUnusedAndUndeclaredAccounts(t *testing.T) {
	tests := []struct {
		name           string
		journal        string
		wantUnused     []string
		wantUndeclared []string
	}{
		{name: "all declared and used", journal: "simple.journal"},
		{name: "declared but unused", journal: "empty.journal", wantUnused: []string{"assets:checking"}},
		{name: "undeclared balancing account", journal: "assertions.journal", wantUndeclared: []string{"equity:opening"}},
		// A declared parent whose only postings are on an undeclared child
		// counts as unused.
		{name: "used only via child", journal: "closed.journal", wantUnused: []string{"assets:old"}, wantUndeclared: []string{"assets:old:savings"}},
		// floatd auto-declares UndeclaredAccounts at startup, so lot
		// subaccounts like assets:brokerage:{2026-01-15, $50} must not appear.
		{name: "lot subaccounts are not undeclared", journal: "lots.journal", wantUndeclared: []string{
			"assets:brokerage", "assets:cash", "assets:crypto", "assets:ira", "revenues:gain",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := mustClient(t, tt.journal)
			unused, err := c.UnusedAccounts(t.Context())
			if err != nil {
				t.Fatalf("UnusedAccounts: %v", err)
			}
			if !reflect.DeepEqual(unused, tt.wantUnused) {
				t.Errorf("UnusedAccounts = %q, want %q", unused, tt.wantUnused)
			}
			undeclared, err := c.UndeclaredAccounts(t.Context())
			if err != nil {
				t.Fatalf("UndeclaredAccounts: %v", err)
			}
			if !reflect.DeepEqual(undeclared, tt.wantUndeclared) {
				t.Errorf("UndeclaredAccounts = %q, want %q", undeclared, tt.wantUndeclared)
			}
		})
	}
}

func TestClosedAccounts(t *testing.T) {
	tests := []struct {
		name    string
		journal string
		want    []string
	}{
		{name: "closed parent and inherited child", journal: "closed.journal", want: []string{"assets:old", "assets:old:savings"}},
		{name: "no closed accounts", journal: "simple.journal", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := mustClient(t, tt.journal)
			got, err := c.ClosedAccounts(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("ClosedAccounts() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransactionsLots(t *testing.T) {
	c := mustClient(t, "lots.journal")
	txns, err := c.Transactions(t.Context())
	if err != nil {
		t.Fatalf("Transactions: %v", err)
	}

	byFID := map[string]hledger.Transaction{}
	for _, tx := range txns {
		byFID[tx.FID] = tx
	}

	findPosting := func(t *testing.T, fid, account string) hledger.Posting {
		t.Helper()
		tx, ok := byFID[fid]
		if !ok {
			t.Fatalf("transaction %q not found", fid)
		}
		for _, p := range tx.Postings {
			if p.Account == account {
				return p
			}
		}
		t.Fatalf("posting %q not found in %q", account, fid)
		return hledger.Posting{}
	}

	tests := []struct {
		name          string
		fid           string
		account       string
		wantRole      string
		wantGenerated bool
		wantBasis     bool
		wantDate      string
		wantLabel     string
		wantCostQty   string // "" with wantBasis means an empty {} basis
		wantAmount    string // posting quantity; checked when non-empty
	}{
		{name: "explicit acquire", fid: "aa000001", account: "assets:brokerage", wantRole: "acquire", wantBasis: true, wantCostQty: "50", wantAmount: "10"},
		{name: "labelled acquire", fid: "aa000002", account: "assets:brokerage", wantRole: "acquire", wantBasis: true, wantDate: "2026-01-16", wantLabel: "mine", wantCostQty: "40"},
		{name: "fractional cost", fid: "aa000004", account: "assets:brokerage", wantRole: "acquire", wantBasis: true, wantCostQty: "1234.567"},
		{name: "specid disposal", fid: "aa000005", account: "assets:brokerage", wantRole: "dispose", wantBasis: true, wantDate: "2026-01-15", wantCostQty: "50", wantAmount: "-5"},
		{name: "cash leg", fid: "aa000005", account: "assets:cash", wantAmount: "350"},
		{name: "inferred gain", fid: "aa000005", account: "revenues:gain", wantRole: "gain", wantGenerated: true, wantAmount: "-100"},
		{name: "label-only selector inferred", fid: "cc000002", account: "assets:brokerage", wantRole: "dispose", wantBasis: true, wantDate: "2026-01-17", wantLabel: "short", wantCostQty: "65"},
		{name: "inferred loss", fid: "cc000002", account: "revenues:gain", wantRole: "gain", wantGenerated: true, wantAmount: "20"},
		{name: "fifo acquire", fid: "bb000001", account: "assets:ira", wantRole: "acquire", wantBasis: true, wantCostQty: "100"},
		{name: "fifo disposal across lots has empty basis", fid: "bb000003", account: "assets:ira", wantRole: "dispose", wantBasis: true, wantAmount: "-3"},
		{name: "fifo gain spans lots", fid: "bb000003", account: "revenues:gain", wantRole: "gain", wantGenerated: true, wantAmount: "-50"},
		{name: "transfer to", fid: "dd000001", account: "assets:ira", wantRole: "transfer-to", wantBasis: true, wantDate: "2026-01-15", wantCostQty: "50", wantAmount: "2"},
		{name: "transfer from", fid: "dd000001", account: "assets:brokerage", wantRole: "transfer-from", wantBasis: true, wantDate: "2026-01-15", wantCostQty: "50", wantAmount: "-2"},
		{name: "fractional units acquire", fid: "ee000001", account: "assets:crypto", wantRole: "acquire", wantBasis: true, wantCostQty: "40000", wantAmount: "0.5"},
		{name: "selectorless disposal of sole lot inferred", fid: "ee000002", account: "assets:crypto", wantRole: "dispose", wantBasis: true, wantDate: "2026-02-15", wantCostQty: "40000", wantAmount: "-0.125"},
		{name: "fractional gain", fid: "ee000002", account: "revenues:gain", wantRole: "gain", wantGenerated: true, wantAmount: "-1000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := findPosting(t, tt.fid, tt.account)
			if got := p.LotRole(); got != tt.wantRole {
				t.Errorf("LotRole = %q, want %q", got, tt.wantRole)
			}
			if got := p.Generated(); got != tt.wantGenerated {
				t.Errorf("Generated = %v, want %v", got, tt.wantGenerated)
			}
			if len(p.Amounts) != 1 {
				t.Fatalf("got %d amounts, want 1: %+v", len(p.Amounts), p.Amounts)
			}
			if tt.wantAmount != "" && p.Amounts[0].Quantity.Format() != tt.wantAmount {
				t.Errorf("amount = %s, want %s", p.Amounts[0].Quantity.Format(), tt.wantAmount)
			}
			cb := p.Amounts[0].CostBasis
			if !tt.wantBasis {
				if cb != nil {
					t.Errorf("CostBasis = %+v, want nil", cb)
				}
				return
			}
			if cb == nil {
				t.Fatal("CostBasis = nil, want non-nil")
			}
			if tt.wantCostQty == "" {
				if cb.Cost != nil || cb.Date != nil || cb.Label != nil {
					t.Errorf("CostBasis = %+v, want empty", cb)
				}
				return
			}
			if cb.Cost == nil {
				t.Fatalf("CostBasis = %+v, want non-nil cost", cb)
			}
			if tt.wantDate == "" && cb.Date != nil {
				t.Errorf("CostBasis.Date = %q, want nil", *cb.Date)
			}
			if tt.wantDate != "" && (cb.Date == nil || *cb.Date != tt.wantDate) {
				t.Errorf("CostBasis.Date = %v, want %q", cb.Date, tt.wantDate)
			}
			if tt.wantLabel == "" && cb.Label != nil {
				t.Errorf("CostBasis.Label = %q, want nil", *cb.Label)
			}
			if tt.wantLabel != "" && (cb.Label == nil || *cb.Label != tt.wantLabel) {
				t.Errorf("CostBasis.Label = %v, want %q", cb.Label, tt.wantLabel)
			}
			if cb.Cost.Commodity != "$" || cb.Cost.Quantity.Format() != tt.wantCostQty {
				t.Errorf("CostBasis.Cost = %s %s, want $ %s", cb.Cost.Commodity, cb.Cost.Quantity.Format(), tt.wantCostQty)
			}
		})
	}
}

func TestLots(t *testing.T) {
	c := mustClient(t, "lots.journal")

	tests := []struct {
		name    string
		query   []string
		want    map[string]struct{ date, label, units, unitCost, totalCost string }
		wantErr error
	}{
		{
			name: "all lots",
			want: map[string]struct{ date, label, units, unitCost, totalCost string }{
				// 10 bought, 5 sold, 2 transferred to the IRA.
				"assets:brokerage:{2026-01-15, $50}":         {"2026-01-15", "", "3", "50", "150"},
				`assets:brokerage:{2026-01-16, "mine", $40}`: {"2026-01-16", "mine", "4", "40", "160"},
				"assets:brokerage:{2026-01-20, $60}":         {"2026-01-20", "", "3", "60", "180"},
				"assets:brokerage:{2026-01-25, $1,234.567}":  {"2026-01-25", "", "1", "1234.567", "1234.567"},
				// The fully disposed "short" lot is omitted.
				// The same lot under a second base account after a transfer.
				"assets:ira:{2026-01-15, $50}": {"2026-01-15", "", "2", "50", "100"},
				// FIFO: the 2026-01-10 lot is fully consumed and omitted.
				"assets:ira:{2026-01-11, $110}":       {"2026-01-11", "", "2", "110", "220"},
				"assets:crypto:{2026-02-15, $40,000}": {"2026-02-15", "", "0.375", "40000", "15000"},
			},
		},
		{
			name:  "query filters lots",
			query: []string{"assets:brokerage:.2026-01-20"},
			want: map[string]struct{ date, label, units, unitCost, totalCost string }{
				"assets:brokerage:{2026-01-20, $60}": {"2026-01-20", "", "3", "60", "180"},
			},
		},
		{
			// Known limitation: hledger applies cur: after -B converts to cost,
			// so the cost report is empty and TotalCost is missing. Pinned here
			// so an hledger upgrade that changes this is noticed.
			name:  "commodity query drops total cost",
			query: []string{"cur:VTI"},
			want: map[string]struct{ date, label, units, unitCost, totalCost string }{
				"assets:ira:{2026-01-11, $110}": {"2026-01-11", "", "2", "110", ""},
			},
		},
		{
			name:  "date query before disposals",
			query: []string{"date:-2026-02-01"},
			want: map[string]struct{ date, label, units, unitCost, totalCost string }{
				"assets:brokerage:{2026-01-15, $50}":          {"2026-01-15", "", "10", "50", "500"},
				`assets:brokerage:{2026-01-16, "mine", $40}`:  {"2026-01-16", "mine", "4", "40", "160"},
				`assets:brokerage:{2026-01-17, "short", $65}`: {"2026-01-17", "short", "2", "65", "130"},
				"assets:brokerage:{2026-01-20, $60}":          {"2026-01-20", "", "3", "60", "180"},
				"assets:brokerage:{2026-01-25, $1,234.567}":   {"2026-01-25", "", "1", "1234.567", "1234.567"},
				"assets:ira:{2026-01-10, $100}":               {"2026-01-10", "", "2", "100", "200"},
				"assets:ira:{2026-01-11, $110}":               {"2026-01-11", "", "3", "110", "330"},
			},
		},
		{
			name:  "non-lot account yields no lots",
			query: []string{"assets:cash"},
			want:  map[string]struct{ date, label, units, unitCost, totalCost string }{},
		},
		{name: "unsafe query rejected", query: []string{"--output-file=/tmp/x"}, wantErr: hledger.ErrUnsafeQuery},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lots, err := c.Lots(t.Context(), tt.query...)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Lots: %v", err)
			}
			if len(lots) != len(tt.want) {
				t.Fatalf("got %d lots, want %d: %+v", len(lots), len(tt.want), lots)
			}
			for _, l := range lots {
				full := l.Account + ":" + l.LotName
				w, ok := tt.want[full]
				if !ok {
					t.Errorf("unexpected lot %q", full)
					continue
				}
				wantCommodity := "AAPL"
				switch {
				case strings.HasPrefix(l.LotName, "{2026-01-10") || strings.HasPrefix(l.LotName, "{2026-01-11"):
					wantCommodity = "VTI"
				case l.Account == "assets:crypto":
					wantCommodity = "BTC"
				}
				if l.Date != w.date || l.Label != w.label {
					t.Errorf("%s: date/label = %q/%q, want %q/%q", l.LotName, l.Date, l.Label, w.date, w.label)
				}
				if l.Units.Commodity != wantCommodity || l.Units.Quantity.Format() != w.units {
					t.Errorf("%s: Units = %s %s, want %s %s", full, l.Units.Commodity, l.Units.Quantity.Format(), wantCommodity, w.units)
				}
				if l.UnitCost == nil || l.UnitCost.Commodity != "$" || l.UnitCost.Quantity.Format() != w.unitCost {
					t.Errorf("%s: UnitCost = %+v, want $ %s", l.LotName, l.UnitCost, w.unitCost)
				}
				if w.totalCost == "" {
					if len(l.TotalCost) != 0 {
						t.Errorf("%s: TotalCost = %+v, want empty", full, l.TotalCost)
					}
					continue
				}
				if len(l.TotalCost) != 1 || l.TotalCost[0].Commodity != "$" || l.TotalCost[0].Quantity.Format() != w.totalCost {
					t.Errorf("%s: TotalCost = %+v, want $ %s", l.LotName, l.TotalCost, w.totalCost)
				}
			}
		})
	}
}

// TestIncludedJournal runs against a multi-file fixture shaped like a float
// data directory, where main.journal only holds include directives.
func TestIncludedJournal(t *testing.T) {
	c := mustClient(t, filepath.Join("includes", "main.journal"))

	t.Run("source positions point at included files", func(t *testing.T) {
		txns, err := c.Transactions(t.Context())
		if err != nil {
			t.Fatalf("Transactions: %v", err)
		}
		want := map[string]struct {
			file        string
			start, next int
		}{
			"aa000001": {"2026/01.journal", 1, 4},
			"bb000002": {"2026/01.journal", 5, 8},
			"cc000003": {"2026/02.journal", 1, 4},
			"dd000004": {"2026/02.journal", 5, 8},
		}
		if len(txns) != len(want) {
			t.Fatalf("got %d transactions, want %d", len(txns), len(want))
		}
		for _, tx := range txns {
			w, ok := want[tx.FID]
			if !ok {
				t.Errorf("unexpected transaction %q", tx.FID)
				continue
			}
			for i, pos := range tx.SourcePos {
				if !strings.HasSuffix(filepath.ToSlash(pos.File), "includes/"+w.file) {
					t.Errorf("%s: SourcePos[%d].File = %q, want .../includes/%s", tx.FID, i, pos.File, w.file)
				}
			}
			if tx.SourcePos[0].Line != w.start || tx.SourcePos[1].Line != w.next {
				t.Errorf("%s: lines = %d..%d, want %d..%d", tx.FID, tx.SourcePos[0].Line, tx.SourcePos[1].Line, w.start, w.next)
			}
		}
	})

	t.Run("account types from included declarations", func(t *testing.T) {
		nodes, err := c.Accounts(t.Context(), false)
		if err != nil {
			t.Fatalf("Accounts: %v", err)
		}
		got := map[string]hledger.AccountType{}
		for _, n := range nodes {
			got[n.FullName] = n.Type
		}
		want := map[string]hledger.AccountType{
			"assets:checking":  hledger.AccountTypeCash,
			"assets:brokerage": hledger.AccountTypeAsset,
			"equity:opening":   hledger.AccountTypeEquity,
			"expenses:food":    hledger.AccountTypeExpense,
			"income:salary":    hledger.AccountTypeRevenue,
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("account types = %v, want %v", got, want)
		}
	})

	t.Run("lots directive and prices from included files", func(t *testing.T) {
		lots, err := c.Lots(t.Context())
		if err != nil {
			t.Fatalf("Lots: %v", err)
		}
		if len(lots) != 1 || lots[0].LotName != "{2026-01-15, $60}" || lots[0].Units.Quantity.Format() != "5" {
			t.Errorf("Lots = %+v, want one 5 AAPL {2026-01-15, $60} lot", lots)
		}
		valued, err := c.BalancesValued(t.Context(), "now,$", 0, "assets:brokerage")
		if err != nil {
			t.Fatalf("BalancesValued: %v", err)
		}
		if got, want := amountsByAccount(valued), map[string][]string{"assets:brokerage": {"400 $"}}; !reflect.DeepEqual(got, want) {
			t.Errorf("valued = %v, want %v", got, want)
		}
	})

	t.Run("PrintJournal inlines included transactions", func(t *testing.T) {
		out, err := c.PrintJournal(t.Context())
		if err != nil {
			t.Fatalf("PrintJournal: %v", err)
		}
		for _, code := range []string{"(aa000001)", "(bb000002)", "(cc000003)", "(dd000004)"} {
			if !strings.Contains(out, code) {
				t.Errorf("PrintJournal output missing %s:\n%s", code, out)
			}
		}
		if strings.Contains(out, "include ") {
			t.Errorf("PrintJournal output still has include directives:\n%s", out)
		}
	})
}

func TestChart(t *testing.T) {
	tests := []struct {
		name      string
		journal   string
		spec      hledger.ChartSpec
		want      []string
		notWant   []string
		wantEmpty bool
		wantErr   error
	}{
		{
			name:    "expense bars sorted by amount",
			journal: "simple.journal",
			spec:    hledger.ChartSpec{Kind: hledger.ChartBalance, Depth: 2, SortByAmount: true, Query: []string{"expenses"}},
			want:    []string{`class="hledger-chart"`, "<title>expenses:food · Balance · $120.00</title>", "<title>expenses:shopping · Balance · $60.00</title>"},
		},
		{
			name:    "inverted revenue is positive",
			journal: "simple.journal",
			spec:    hledger.ChartSpec{Kind: hledger.ChartBalance, Invert: true, Query: []string{"income"}},
			want:    []string{"<title>income:salary · Balance · $7000.00</title>"},
		},
		{
			name:    "dark styles rescoped to selector",
			journal: "simple.journal",
			spec:    hledger.ChartSpec{Kind: hledger.ChartBalance, DarkSelector: ".dark", Query: []string{"expenses"}},
			want:    []string{":where(.dark) .hledger-chart .bg{"},
			notWant: []string{"prefers-color-scheme"},
		},
		{
			name:      "no matching postings is empty",
			journal:   "simple.journal",
			spec:      hledger.ChartSpec{Kind: hledger.ChartBalance, Query: []string{"date:2030"}},
			wantEmpty: true,
		},
		{
			name:      "empty journal",
			journal:   "empty.journal",
			spec:      hledger.ChartSpec{Kind: hledger.ChartBalance},
			wantEmpty: true,
		},
		{
			name:    "flag-shaped query rejected",
			journal: "simple.journal",
			spec:    hledger.ChartSpec{Kind: hledger.ChartBalance, Query: []string{"-o/tmp/x.svg"}},
			wantErr: hledger.ErrUnsafeQuery,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := mustClient(t, tt.journal)
			got, err := c.Chart(t.Context(), tt.spec)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Chart() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Chart() error = %v", err)
			}
			if !strings.HasPrefix(got.SVG, `<svg xmlns="http://www.w3.org/2000/svg"`) {
				t.Errorf("SVG does not start with an <svg> root: %.80s", got.SVG)
			}
			for _, s := range tt.want {
				if !strings.Contains(got.SVG, s) {
					t.Errorf("SVG missing %q", s)
				}
			}
			for _, s := range tt.notWant {
				if strings.Contains(got.SVG, s) {
					t.Errorf("SVG unexpectedly contains %q", s)
				}
			}
			if got.Empty != tt.wantEmpty {
				t.Errorf("Empty = %v, want %v", got.Empty, tt.wantEmpty)
			}
		})
	}
}

func TestChartUnknownKind(t *testing.T) {
	c := mustClient(t, "simple.journal")
	if _, err := c.Chart(t.Context(), hledger.ChartSpec{}); err == nil {
		t.Fatal("Chart() with zero Kind: want error")
	}
}
