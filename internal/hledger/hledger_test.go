package hledger_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

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
		name         string
		journal      string
		wantErr      bool
		wantCheckErr bool
	}{
		{name: "valid journal", journal: "simple.journal"},
		{name: "invalid journal", journal: "invalid.journal", wantErr: true, wantCheckErr: true},
		{name: "empty journal", journal: "empty.journal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := mustClient(t, tt.journal)
			err := c.Check(t.Context())
			if (err != nil) != tt.wantErr {
				t.Fatalf("Check() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantCheckErr {
				var checkErr *hledger.CheckError
				if !errors.As(err, &checkErr) {
					t.Errorf("expected *CheckError, got: %T", err)
				}
				if checkErr.Error() == "" {
					t.Error("expected non-empty error message")
				}
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
			query: []string{"tag:fid=bb002200"},
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
	const versionResp = "hledger 1.52, linux-x86_64\n"
	const printJSON = `[{"tindex":1,"tdate":"2026-01-05","tdate2":null,"tdescription":"PAYROLL","tcode":"","tcomment":"fid:aa001100\n","ttags":[["fid","aa001100"]],"tpostings":[],"tstatus":"","tprecedingcomment":""},{"tindex":2,"tdate":"2026-01-15","tdate2":null,"tdescription":"AMAZON","tcode":"","tcomment":"","ttags":[],"tpostings":[],"tstatus":"","tprecedingcomment":""}]`

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
	}
	if liabSub == nil {
		t.Fatal("Liabilities subreport not found")
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
		return []byte("hledger 1.52, linux-x86_64\n"), nil, nil
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
			name:   "filters fid and empty lines",
			output: "category\nfid\nnotes\n\n",
			want:   []string{"category", "notes"},
		},
		{
			name:   "empty output",
			output: "",
			want:   nil,
		},
		{
			name:   "only fid returns empty",
			output: "fid\n",
			want:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tagsOutput := tt.output
			runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
				if len(args) > 0 && args[0] == "--version" {
					return []byte("hledger 1.52, linux-x86_64\n"), nil, nil
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

	// Integration smoke test: Tags() against the real journal runs without error.
	t.Run("integration", func(t *testing.T) {
		c, err := hledger.New("hledger", "testdata/simple.journal")
		if err != nil {
			t.Skip("hledger binary not available:", err)
		}
		tags, err := c.Tags(t.Context())
		if err != nil {
			t.Fatalf("Tags: %v", err)
		}
		for _, tag := range tags {
			if tag == "fid" {
				t.Error("fid should be filtered from Tags() output")
			}
		}
	})
}
