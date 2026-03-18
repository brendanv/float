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

func TestNew_ValidVersion(t *testing.T) {
	_, err := hledger.New("hledger", "testdata/simple.journal")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestNew_BadBinary(t *testing.T) {
	_, err := hledger.New("/nonexistent/hledger", "testdata/simple.journal")
	if err == nil {
		t.Fatal("expected error for bad binary")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error to contain 'not found', got: %v", err)
	}
}

func TestCheck_Valid(t *testing.T) {
	c := mustClient(t, "simple.journal")
	if err := c.Check(t.Context()); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestCheck_Invalid(t *testing.T) {
	c := mustClient(t, "invalid.journal")
	err := c.Check(t.Context())
	if err == nil {
		t.Fatal("expected error for invalid journal")
	}
	var checkErr *hledger.CheckError
	if !errors.As(err, &checkErr) {
		t.Errorf("expected *CheckError, got: %T", err)
	}
	if checkErr.Error() == "" {
		t.Error("expected non-empty error message")
	}
}

func TestCheck_Empty(t *testing.T) {
	c := mustClient(t, "empty.journal")
	if err := c.Check(t.Context()); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestBalances_All(t *testing.T) {
	c := mustClient(t, "simple.journal")
	report, err := c.Balances(t.Context(), 0)
	if err != nil {
		t.Fatalf("Balances: %v", err)
	}
	if len(report.Rows) < 4 {
		t.Errorf("expected at least 4 rows, got %d", len(report.Rows))
	}

	var checkingAmt, shoppingAmt, salaryAmt float64
	var foundChecking, foundShopping, foundSalary bool
	for _, row := range report.Rows {
		switch row.FullName {
		case "assets:checking":
			if len(row.Amounts) > 0 {
				checkingAmt = row.Amounts[0].Quantity.FloatingPoint
				foundChecking = true
			}
		case "expenses:shopping":
			if len(row.Amounts) > 0 {
				shoppingAmt = row.Amounts[0].Quantity.FloatingPoint
				foundShopping = true
			}
		case "income:salary":
			if len(row.Amounts) > 0 {
				salaryAmt = row.Amounts[0].Quantity.FloatingPoint
				foundSalary = true
			}
		}
	}

	if !foundChecking {
		t.Error("assets:checking not found")
	}
	if checkingAmt < 6819 || checkingAmt > 6821 {
		t.Errorf("assets:checking balance ≈ 6820, got %v", checkingAmt)
	}
	if !foundShopping {
		t.Error("expenses:shopping not found")
	}
	if shoppingAmt < 59 || shoppingAmt > 61 {
		t.Errorf("expenses:shopping balance ≈ 60, got %v", shoppingAmt)
	}
	if !foundSalary {
		t.Error("income:salary not found")
	}
	if salaryAmt >= 0 {
		t.Errorf("income:salary should be negative, got %v", salaryAmt)
	}
}

func TestBalances_Depth1(t *testing.T) {
	c := mustClient(t, "simple.journal")
	report, err := c.Balances(t.Context(), 1)
	if err != nil {
		t.Fatalf("Balances depth=1: %v", err)
	}
	for _, row := range report.Rows {
		if strings.Contains(row.FullName, ":") {
			t.Errorf("depth=1 should not have ':' in FullName, got %q", row.FullName)
		}
	}
	var expensesAmt float64
	for _, row := range report.Rows {
		if row.FullName == "expenses" && len(row.Amounts) > 0 {
			expensesAmt = row.Amounts[0].Quantity.FloatingPoint
		}
	}
	if expensesAmt < 179 || expensesAmt > 181 {
		t.Errorf("expenses balance ≈ 180, got %v", expensesAmt)
	}
}

func TestBalances_Query(t *testing.T) {
	c := mustClient(t, "simple.journal")
	report, err := c.Balances(t.Context(), 0, "expenses")
	if err != nil {
		t.Fatalf("Balances query: %v", err)
	}
	for _, row := range report.Rows {
		if !strings.HasPrefix(row.FullName, "expenses") {
			t.Errorf("expected only expenses rows, got %q", row.FullName)
		}
	}
	for _, row := range report.Rows {
		if strings.HasPrefix(row.FullName, "assets") {
			t.Errorf("expected no assets rows, got %q", row.FullName)
		}
	}
}

func TestBalances_Empty(t *testing.T) {
	c := mustClient(t, "empty.journal")
	report, err := c.Balances(t.Context(), 0)
	if err != nil {
		t.Fatalf("Balances empty: %v", err)
	}
	if len(report.Rows) != 0 {
		t.Errorf("expected no rows for empty journal, got %d", len(report.Rows))
	}
}

func TestRegister_All(t *testing.T) {
	c := mustClient(t, "simple.journal")
	rows, err := c.Register(t.Context())
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if len(rows) != 10 {
		t.Errorf("expected 10 rows (5 txns × 2 postings), got %d", len(rows))
	}
	if rows[0].Date == nil {
		t.Error("first row Date should be non-nil")
	}
	if rows[1].Date != nil {
		t.Error("second row Date should be nil")
	}
}

func TestRegister_FidQuery(t *testing.T) {
	c := mustClient(t, "simple.journal")
	rows, err := c.Register(t.Context(), "tag:fid=bb002200")
	if err != nil {
		t.Fatalf("Register fid: %v", err)
	}
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
}

func TestRegister_DateFilter(t *testing.T) {
	c := mustClient(t, "simple.journal")
	rows, err := c.Register(t.Context(), "date:2026-01")
	if err != nil {
		t.Fatalf("Register date filter: %v", err)
	}
	if len(rows) != 6 {
		t.Errorf("expected 6 rows (3 Jan txns × 2 postings), got %d", len(rows))
	}
}

func TestAccounts_Flat(t *testing.T) {
	c := mustClient(t, "simple.journal")
	nodes, err := c.Accounts(t.Context(), false)
	if err != nil {
		t.Fatalf("Accounts flat: %v", err)
	}
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
		}
	}
	if !foundChecking {
		t.Error("assets:checking not found in flat accounts")
	}
}

func TestAccounts_Tree(t *testing.T) {
	c := mustClient(t, "simple.journal")
	roots, err := c.Accounts(t.Context(), true)
	if err != nil {
		t.Fatalf("Accounts tree: %v", err)
	}
	if len(roots) != 3 {
		t.Errorf("expected 3 root nodes (assets, expenses, income), got %d", len(roots))
	}

	var expensesNode *hledger.AccountNode
	for _, r := range roots {
		if r.FullName == "expenses" {
			expensesNode = r
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
	}
}

func TestPrintCSV(t *testing.T) {
	c := mustClient(t, "simple.journal")
	txns, err := c.PrintCSV(t.Context(), "testdata/import.csv", "testdata/import.rules")
	if err != nil {
		t.Fatalf("PrintCSV: %v", err)
	}
	if len(txns) != 3 {
		t.Errorf("expected 3 transactions, got %d", len(txns))
	}

	var foundAmazon, foundPayroll bool
	for _, txn := range txns {
		if txn.Description == "AMAZON MARKETPLACE" {
			foundAmazon = true
			for _, p := range txn.Postings {
				if p.Account == "expenses:shopping" {
					if len(p.Amounts) > 0 {
						fp := p.Amounts[0].Quantity.FloatingPoint
						if fp < 44 || fp > 46 {
							t.Errorf("AMAZON shopping amount ≈ 45, got %v", fp)
						}
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
}

func TestTransactionFID(t *testing.T) {
	const versionResp = "hledger 1.51.2, linux-x86_64\n"
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
	if len(result) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(result))
	}
	if result[0].FID != "aa001100" {
		t.Errorf("expected FID aa001100, got %q", result[0].FID)
	}
	if result[1].FID != "" {
		t.Errorf("expected empty FID for transaction without fid tag, got %q", result[1].FID)
	}
}

func TestPrintCSV_BadRules(t *testing.T) {
	c := mustClient(t, "simple.journal")
	_, err := c.PrintCSV(t.Context(), "testdata/import.csv", "testdata/nonexistent.rules")
	if err == nil {
		t.Fatal("expected error for nonexistent rules file")
	}
}

func TestNewWithRunner(t *testing.T) {
	called := false
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		called = true
		// Return a valid version response for the --version check in newClient
		return []byte("hledger 1.51.2, linux-x86_64\n"), nil, nil
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
