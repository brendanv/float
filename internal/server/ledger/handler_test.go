package ledger_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	serverledger "github.com/brendanv/float/internal/server/ledger"

	"github.com/brendanv/float/internal/cache"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/testgen"
	"github.com/brendanv/float/internal/txlock"
)

// versionRunner returns a valid hledger version string for client construction.
func versionRunner(t *testing.T, data map[string][]byte) hledger.CommandRunner {
	t.Helper()
	return func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.52, linux-x86_64\n"), nil, nil
		}
		key := strings.Join(args, " ")
		for k, v := range data {
			if strings.Contains(key, k) {
				return v, nil, nil
			}
		}
		return []byte("[]"), nil, nil
	}
}

func mustHandler(t *testing.T, data map[string][]byte) *serverledger.Handler {
	t.Helper()
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", versionRunner(t, data))
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	return serverledger.NewHandler(c, nil, "", "", nil, nil, nil)
}

const printJSON = `[
  {
    "tcode": "aa001100",
    "tcomment": "",
    "tdate": "2026-01-05",
    "tdate2": null,
    "tdescription": "PAYROLL DIRECT DEPOSIT",
    "tindex": 1,
    "tpostings": [
      {
        "paccount": "assets:checking",
        "pamount": [{"acommodity": "$", "acost": null, "aquantity": {"decimalMantissa": 350000, "decimalPlaces": 2, "floatingPoint": 3500}}],
        "pcomment": "",
        "pdate": null,
        "pdate2": null,
        "pstatus": "Unmarked",
        "ptags": [],
        "ptransaction_": "1",
        "ptype": "RegularPosting"
      },
      {
        "paccount": "income:salary",
        "pamount": [{"acommodity": "$", "acost": null, "aquantity": {"decimalMantissa": -350000, "decimalPlaces": 2, "floatingPoint": -3500}}],
        "pcomment": "",
        "pdate": null,
        "pdate2": null,
        "pstatus": "Unmarked",
        "ptags": [],
        "ptransaction_": "1",
        "ptype": "RegularPosting"
      }
    ],
    "tprecedingcomment": "",
    "tstatus": "Unmarked",
    "ttags": [],
    "tsourcepos": [{"sourceName": "simple.journal", "sourceLine": 1, "sourceColumn": 1}, {"sourceName": "simple.journal", "sourceLine": 4, "sourceColumn": 1}]
  }
]`

const balJSON = `[[["assets:checking", "assets:checking", 0, [{"acommodity": "$", "acost": null, "aquantity": {"decimalMantissa": 700000, "decimalPlaces": 2, "floatingPoint": 7000}}]], ["income:salary", "income:salary", 0, [{"acommodity": "$", "acost": null, "aquantity": {"decimalMantissa": -700000, "decimalPlaces": 2, "floatingPoint": -7000}}]]], [{"acommodity": "$", "acost": null, "aquantity": {"decimalMantissa": 0, "decimalPlaces": 2, "floatingPoint": 0}}]]`

// bsTimeseriesJSON is a minimal hledger bs --monthly -O json fixture with 2 periods,
// Assets and Liabilities subreports, and net worth totals.
const bsTimeseriesJSON = `{
  "cbrDates": [
    [{"contents": "2026-01-01", "tag": "Exact"}, {"contents": "2026-02-01", "tag": "Exact"}],
    [{"contents": "2026-02-01", "tag": "Exact"}, {"contents": "2026-03-01", "tag": "Exact"}]
  ],
  "cbrSubreports": [
    ["Assets", {
      "prDates": [],
      "prRows": [],
      "prTotals": {
        "prrAmounts": [
          [{"acommodity": "$", "acost": null, "aquantity": {"decimalMantissa": 333500, "decimalPlaces": 2, "floatingPoint": 3335}}],
          [{"acommodity": "$", "acost": null, "aquantity": {"decimalMantissa": 682000, "decimalPlaces": 2, "floatingPoint": 6820}}]
        ],
        "prrAverage": [], "prrName": [], "prrTotal": []
      }
    }],
    ["Liabilities", {
      "prDates": [],
      "prRows": [],
      "prTotals": {"prrAmounts": [[], []], "prrAverage": [], "prrName": [], "prrTotal": []}
    }]
  ],
  "cbrTitle": "Balance Sheet",
  "cbrTotals": {
    "prrAmounts": [
      [{"acommodity": "$", "acost": null, "aquantity": {"decimalMantissa": 333500, "decimalPlaces": 2, "floatingPoint": 3335}}],
      [{"acommodity": "$", "acost": null, "aquantity": {"decimalMantissa": 682000, "decimalPlaces": 2, "floatingPoint": 6820}}]
    ],
    "prrAverage": [], "prrName": [], "prrTotal": []
  }
}`

const accountsText = `assets:checking      ; type: A
income:salary        ; type: R
`

func TestListTransactions(t *testing.T) {
	h := mustHandler(t, map[string][]byte{
		"print": []byte(printJSON),
	})

	resp, err := h.ListTransactions(t.Context(), connect.NewRequest(&floatv1.ListTransactionsRequest{}))
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}

	txns := resp.Msg.Transactions
	if len(txns) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(txns))
	}

	txn := txns[0]
	if txn.Fid != "aa001100" {
		t.Errorf("Fid = %q, want %q", txn.Fid, "aa001100")
	}
	if txn.Date != "2026-01-05" {
		t.Errorf("Date = %q, want %q", txn.Date, "2026-01-05")
	}
	if txn.Description != "PAYROLL DIRECT DEPOSIT" {
		t.Errorf("Description = %q, want %q", txn.Description, "PAYROLL DIRECT DEPOSIT")
	}
	if len(txn.Postings) != 2 {
		t.Fatalf("expected 2 postings, got %d", len(txn.Postings))
	}
	if txn.Postings[0].Account != "assets:checking" {
		t.Errorf("Posting[0].Account = %q, want %q", txn.Postings[0].Account, "assets:checking")
	}
	if len(txn.Postings[0].Amounts) != 1 {
		t.Fatalf("expected 1 amount, got %d", len(txn.Postings[0].Amounts))
	}
	amt := txn.Postings[0].Amounts[0]
	if amt.Commodity != "$" {
		t.Errorf("Commodity = %q, want %q", amt.Commodity, "$")
	}
	if amt.Quantity != "3500.00" {
		t.Errorf("Quantity = %q, want %q", amt.Quantity, "3500.00")
	}
}

func TestListTransactionsWithQuery(t *testing.T) {
	var capturedArgs []string
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.52, linux-x86_64\n"), nil, nil
		}
		capturedArgs = args
		return []byte("[]"), nil, nil
	}
	c, err := hledger.NewWithRunner("hledger", "journal.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil)

	_, err = h.ListTransactions(t.Context(), connect.NewRequest(&floatv1.ListTransactionsRequest{
		Query: []string{"assets:checking", "date:2026-01"},
	}))
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}

	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "assets:checking") {
		t.Errorf("args %v missing query token 'assets:checking'", capturedArgs)
	}
	if !strings.Contains(joined, "date:2026-01") {
		t.Errorf("args %v missing query token 'date:2026-01'", capturedArgs)
	}
}

func TestGetBalances(t *testing.T) {
	h := mustHandler(t, map[string][]byte{
		"bal": []byte(balJSON),
	})

	resp, err := h.GetBalances(t.Context(), connect.NewRequest(&floatv1.GetBalancesRequest{}))
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}

	report := resp.Msg.Report
	if report == nil {
		t.Fatal("expected non-nil report")
		return
	}
	if len(report.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(report.Rows))
	}
	row := report.Rows[0]
	if row.FullName != "assets:checking" {
		t.Errorf("FullName = %q, want %q", row.FullName, "assets:checking")
	}
	if len(row.Amounts) != 1 {
		t.Fatalf("expected 1 amount, got %d", len(row.Amounts))
	}
	if row.Amounts[0].Quantity != "7000.00" {
		t.Errorf("Quantity = %q, want %q", row.Amounts[0].Quantity, "7000.00")
	}
	if len(report.Total) != 1 {
		t.Fatalf("expected 1 total, got %d", len(report.Total))
	}
	if report.Total[0].Quantity != "0.00" {
		t.Errorf("Total Quantity = %q, want %q", report.Total[0].Quantity, "0.00")
	}
}

func TestGetNetWorthTimeseries(t *testing.T) {
	h := mustHandler(t, map[string][]byte{
		"bs": []byte(bsTimeseriesJSON),
	})

	resp, err := h.GetNetWorthTimeseries(t.Context(), connect.NewRequest(&floatv1.GetNetWorthTimeseriesRequest{}))
	if err != nil {
		t.Fatalf("GetNetWorthTimeseries: %v", err)
	}

	snapshots := resp.Msg.Snapshots
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snapshots))
	}

	s0 := snapshots[0]
	if s0.Date != "2026-01-01" {
		t.Errorf("snapshot[0].Date = %q, want %q", s0.Date, "2026-01-01")
	}
	if len(s0.Assets) != 1 {
		t.Fatalf("expected 1 asset amount in snapshot[0], got %d", len(s0.Assets))
	}
	if s0.Assets[0].Commodity != "$" {
		t.Errorf("Assets[0].Commodity = %q, want %q", s0.Assets[0].Commodity, "$")
	}
	if s0.Assets[0].Quantity != "3335.00" {
		t.Errorf("Assets[0].Quantity = %q, want %q", s0.Assets[0].Quantity, "3335.00")
	}
	if len(s0.Liabilities) != 0 {
		t.Errorf("expected 0 liability amounts in snapshot[0], got %d", len(s0.Liabilities))
	}
	if len(s0.NetWorth) != 1 {
		t.Fatalf("expected 1 net worth amount in snapshot[0], got %d", len(s0.NetWorth))
	}
	if s0.NetWorth[0].Quantity != "3335.00" {
		t.Errorf("NetWorth[0].Quantity = %q, want %q", s0.NetWorth[0].Quantity, "3335.00")
	}

	s1 := snapshots[1]
	if s1.Date != "2026-02-01" {
		t.Errorf("snapshot[1].Date = %q, want %q", s1.Date, "2026-02-01")
	}
	if len(s1.Assets) != 1 {
		t.Fatalf("expected 1 asset amount in snapshot[1], got %d", len(s1.Assets))
	}
	if s1.Assets[0].Quantity != "6820.00" {
		t.Errorf("Assets[0].Quantity = %q, want %q", s1.Assets[0].Quantity, "6820.00")
	}
}

func TestListAccounts(t *testing.T) {
	h := mustHandler(t, map[string][]byte{
		"accounts": []byte(accountsText),
	})

	resp, err := h.ListAccounts(t.Context(), connect.NewRequest(&floatv1.ListAccountsRequest{}))
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}

	accounts := resp.Msg.Accounts
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(accounts))
	}

	tests := []struct {
		idx      int
		fullName string
		typ      string
	}{
		{0, "assets:checking", "A"},
		{1, "income:salary", "R"},
	}
	for _, tt := range tests {
		a := accounts[tt.idx]
		if a.FullName != tt.fullName {
			t.Errorf("accounts[%d].FullName = %q, want %q", tt.idx, a.FullName, tt.fullName)
		}
		if a.Type != tt.typ {
			t.Errorf("accounts[%d].Type = %q, want %q", tt.idx, a.Type, tt.typ)
		}
	}
}

// mustRealHandler creates a handler backed by a real hledger client and data dir.
func mustRealHandler(t *testing.T, dir string) *serverledger.Handler {
	t.Helper()
	c, err := hledger.New("hledger", dir+"/main.journal")
	if err != nil {
		t.Skipf("hledger unavailable: %v", err)
	}
	lock := txlock.New(dir, c)
	return serverledger.NewHandler(c, lock, dir, "", nil, nil, nil)
}

func TestDeleteTransactionHandler(t *testing.T) {
	t.Run("empty_fid_returns_invalid_argument", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 20, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.DeleteTransaction(t.Context(), connect.NewRequest(&floatv1.DeleteTransactionRequest{Fid: ""}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var connectErr *connect.Error
		if !connect.IsWireError(err) {
			// check via type assertion
			_ = connectErr
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("not_found_fid_returns_not_found", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 21, NumTxns: 2, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.DeleteTransaction(t.Context(), connect.NewRequest(&floatv1.DeleteTransactionRequest{Fid: "00000000"}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("code = %v, want NotFound", connect.CodeOf(err))
		}
	})

	t.Run("deletes_transaction", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 22, NumTxns: 2, WithFIDs: true})
		h := mustRealHandler(t, dir)
		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}

		tx := journal.TransactionInput{
			Date:        time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
			Description: "HANDLER DELETE TEST",
			Postings: []journal.PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "12.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := journal.AppendTransaction(t.Context(), c, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		_, err = h.DeleteTransaction(t.Context(), connect.NewRequest(&floatv1.DeleteTransactionRequest{Fid: fid}))
		if err != nil {
			t.Fatalf("DeleteTransaction: %v", err)
		}

		// Verify gone.
		txns, err := c.Transactions(t.Context(), "code:"+fid)
		if err != nil {
			t.Fatalf("Transactions after delete: %v", err)
		}
		if len(txns) != 0 {
			t.Errorf("transaction still present after delete, got %d", len(txns))
		}
	})
}

func TestUpdateTransactionDateHandler(t *testing.T) {
	t.Run("empty_fid", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 40, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.UpdateTransactionDate(t.Context(), connect.NewRequest(&floatv1.UpdateTransactionDateRequest{
			Fid:     "",
			NewDate: "2026-03-01",
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("empty_new_date", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 41, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.UpdateTransactionDate(t.Context(), connect.NewRequest(&floatv1.UpdateTransactionDateRequest{
			Fid:     "aa001100",
			NewDate: "",
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("not_found", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 42, NumTxns: 2, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.UpdateTransactionDate(t.Context(), connect.NewRequest(&floatv1.UpdateTransactionDateRequest{
			Fid:     "00000000",
			NewDate: "2026-03-01",
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("code = %v, want NotFound", connect.CodeOf(err))
		}
	})

	t.Run("success", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 43, NumTxns: 2, WithFIDs: true})
		h := mustRealHandler(t, dir)
		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}

		tx := journal.TransactionInput{
			Date:        time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
			Description: "HANDLER UPDATE DATE TEST",
			Postings: []journal.PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "18.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := journal.AppendTransaction(t.Context(), c, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		resp, err := h.UpdateTransactionDate(t.Context(), connect.NewRequest(&floatv1.UpdateTransactionDateRequest{
			Fid:     fid,
			NewDate: "2026-02-15",
		}))
		if err != nil {
			t.Fatalf("UpdateTransactionDate: %v", err)
		}

		got := resp.Msg.Transaction
		if got.Date != "2026-02-15" {
			t.Errorf("Date = %q, want %q", got.Date, "2026-02-15")
		}
		if got.Fid != fid {
			t.Errorf("Fid = %q, want %q", got.Fid, fid)
		}
	})
}

func TestAddTransactionHandler(t *testing.T) {
	t.Run("missing_description", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 50, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.AddTransaction(t.Context(), connect.NewRequest(&floatv1.AddTransactionRequest{
			Description: "",
			Postings: []*floatv1.PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "10.00"},
				{Account: "assets:checking"},
			},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("too_few_postings", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 51, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.AddTransaction(t.Context(), connect.NewRequest(&floatv1.AddTransactionRequest{
			Description: "Test",
			Postings: []*floatv1.PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "10.00"},
			},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("invalid_date", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 52, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.AddTransaction(t.Context(), connect.NewRequest(&floatv1.AddTransactionRequest{
			Description: "Test",
			Date:        "not-a-date",
			Postings: []*floatv1.PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "10.00"},
				{Account: "assets:checking"},
			},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("empty_account_in_posting", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 55, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.AddTransaction(t.Context(), connect.NewRequest(&floatv1.AddTransactionRequest{
			Description: "Test",
			Postings: []*floatv1.PostingInput{
				{Account: "", Commodity: "USD", Quantity: "10.00"},
				{Account: "assets:checking"},
			},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("success_with_date", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 53, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}

		resp, err := h.AddTransaction(t.Context(), connect.NewRequest(&floatv1.AddTransactionRequest{
			Description: "GROCERY STORE",
			Date:        "2026-02-10",
			Postings: []*floatv1.PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "55.00"},
				{Account: "assets:checking"},
			},
		}))
		if err != nil {
			t.Fatalf("AddTransaction: %v", err)
		}

		got := resp.Msg.Transaction
		if got.Description != "GROCERY STORE" {
			t.Errorf("Description = %q, want %q", got.Description, "GROCERY STORE")
		}
		if got.Date != "2026-02-10" {
			t.Errorf("Date = %q, want %q", got.Date, "2026-02-10")
		}
		if got.Fid == "" {
			t.Error("Fid should be non-empty")
		}
		if len(got.Postings) != 2 {
			t.Fatalf("expected 2 postings, got %d", len(got.Postings))
		}

		// Verify it's in the journal.
		txns, err := c.Transactions(t.Context(), "code:"+got.Fid)
		if err != nil {
			t.Fatalf("Transactions lookup: %v", err)
		}
		if len(txns) != 1 {
			t.Fatalf("expected 1 transaction, got %d", len(txns))
		}
	})

	t.Run("success_without_date_defaults_to_today", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 54, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)

		resp, err := h.AddTransaction(t.Context(), connect.NewRequest(&floatv1.AddTransactionRequest{
			Description: "AUTO DATE TEST",
			Postings: []*floatv1.PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "20.00"},
				{Account: "assets:checking"},
			},
		}))
		if err != nil {
			t.Fatalf("AddTransaction: %v", err)
		}

		got := resp.Msg.Transaction
		today := time.Now().UTC().Format("2006-01-02")
		if got.Date != today {
			t.Errorf("Date = %q, want today %q", got.Date, today)
		}
	})
}

// errorRunner returns a runner that fails on every non-version call.
func errorRunner(t *testing.T) hledger.CommandRunner {
	t.Helper()
	return func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.52, linux-x86_64\n"), nil, nil
		}
		return nil, nil, errors.New("hledger failed")
	}
}

func mustHandlerWithCache(t *testing.T, runner hledger.CommandRunner) (*serverledger.Handler, *cache.Cache[any]) {
	t.Helper()
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	ch := cache.New[any](func() uint64 { return 0 })
	return serverledger.NewHandler(c, nil, "", "", ch, nil, nil), ch
}

func TestListTransactions_HledgerError(t *testing.T) {
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", errorRunner(t))
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil)
	_, err = h.ListTransactions(t.Context(), connect.NewRequest(&floatv1.ListTransactionsRequest{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("code = %v, want Internal", connect.CodeOf(err))
	}
}

func TestGetBalances_HledgerError(t *testing.T) {
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", errorRunner(t))
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil)
	_, err = h.GetBalances(t.Context(), connect.NewRequest(&floatv1.GetBalancesRequest{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("code = %v, want Internal", connect.CodeOf(err))
	}
}

func TestListAccounts_HledgerError(t *testing.T) {
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", errorRunner(t))
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil)
	_, err = h.ListAccounts(t.Context(), connect.NewRequest(&floatv1.ListAccountsRequest{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("code = %v, want Internal", connect.CodeOf(err))
	}
}

func TestGetNetWorthTimeseries_HledgerError(t *testing.T) {
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", errorRunner(t))
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil)
	_, err = h.GetNetWorthTimeseries(t.Context(), connect.NewRequest(&floatv1.GetNetWorthTimeseriesRequest{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("code = %v, want Internal", connect.CodeOf(err))
	}
}

func TestListTransactions_CacheHit(t *testing.T) {
	var calls atomic.Int64
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.52, linux-x86_64\n"), nil, nil
		}
		calls.Add(1)
		return []byte(printJSON), nil, nil
	}
	h, _ := mustHandlerWithCache(t, runner)

	req := connect.NewRequest(&floatv1.ListTransactionsRequest{})
	resp1, err := h.ListTransactions(t.Context(), req)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	resp2, err := h.ListTransactions(t.Context(), req)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if calls.Load() != 1 {
		t.Errorf("hledger called %d times, want 1 (cache should serve second call)", calls.Load())
	}
	if len(resp1.Msg.Transactions) != len(resp2.Msg.Transactions) {
		t.Errorf("cached result differs: got %d txns vs %d", len(resp1.Msg.Transactions), len(resp2.Msg.Transactions))
	}
}

func TestGetBalances_CacheHit(t *testing.T) {
	var calls atomic.Int64
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.52, linux-x86_64\n"), nil, nil
		}
		calls.Add(1)
		return []byte(balJSON), nil, nil
	}
	h, _ := mustHandlerWithCache(t, runner)

	req := connect.NewRequest(&floatv1.GetBalancesRequest{})
	if _, err := h.GetBalances(t.Context(), req); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := h.GetBalances(t.Context(), req); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if calls.Load() != 1 {
		t.Errorf("hledger called %d times, want 1", calls.Load())
	}
}

func TestListAccounts_CacheHit(t *testing.T) {
	var calls atomic.Int64
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.52, linux-x86_64\n"), nil, nil
		}
		calls.Add(1)
		return []byte(accountsText), nil, nil
	}
	h, _ := mustHandlerWithCache(t, runner)

	req := connect.NewRequest(&floatv1.ListAccountsRequest{})
	if _, err := h.ListAccounts(t.Context(), req); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := h.ListAccounts(t.Context(), req); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if calls.Load() != 1 {
		t.Errorf("hledger called %d times, want 1", calls.Load())
	}
}

func TestGetNetWorthTimeseries_CacheHit(t *testing.T) {
	var calls atomic.Int64
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.52, linux-x86_64\n"), nil, nil
		}
		calls.Add(1)
		return []byte(bsTimeseriesJSON), nil, nil
	}
	h, _ := mustHandlerWithCache(t, runner)

	req := connect.NewRequest(&floatv1.GetNetWorthTimeseriesRequest{})
	if _, err := h.GetNetWorthTimeseries(t.Context(), req); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := h.GetNetWorthTimeseries(t.Context(), req); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if calls.Load() != 1 {
		t.Errorf("hledger called %d times, want 1", calls.Load())
	}
}

func TestListTransactions_CacheError(t *testing.T) {
	h, _ := mustHandlerWithCache(t, errorRunner(t))
	_, err := h.ListTransactions(t.Context(), connect.NewRequest(&floatv1.ListTransactionsRequest{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("code = %v, want Internal", connect.CodeOf(err))
	}
}

func TestGetBalances_CacheError(t *testing.T) {
	h, _ := mustHandlerWithCache(t, errorRunner(t))
	_, err := h.GetBalances(t.Context(), connect.NewRequest(&floatv1.GetBalancesRequest{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("code = %v, want Internal", connect.CodeOf(err))
	}
}

func TestListAccounts_CacheError(t *testing.T) {
	h, _ := mustHandlerWithCache(t, errorRunner(t))
	_, err := h.ListAccounts(t.Context(), connect.NewRequest(&floatv1.ListAccountsRequest{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("code = %v, want Internal", connect.CodeOf(err))
	}
}

func TestGetNetWorthTimeseries_CacheError(t *testing.T) {
	h, _ := mustHandlerWithCache(t, errorRunner(t))
	_, err := h.GetNetWorthTimeseries(t.Context(), connect.NewRequest(&floatv1.GetNetWorthTimeseriesRequest{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("code = %v, want Internal", connect.CodeOf(err))
	}
}

func TestGetNetWorthTimeseries_WithDateRange(t *testing.T) {
	var capturedArgs []string
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.52, linux-x86_64\n"), nil, nil
		}
		capturedArgs = args
		return []byte(bsTimeseriesJSON), nil, nil
	}
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil)

	_, err = h.GetNetWorthTimeseries(t.Context(), connect.NewRequest(&floatv1.GetNetWorthTimeseriesRequest{
		Begin: "2026-01-01",
		End:   "2026-03-01",
	}))
	if err != nil {
		t.Fatalf("GetNetWorthTimeseries: %v", err)
	}

	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "2026-01-01") {
		t.Errorf("args %v missing begin date", capturedArgs)
	}
	if !strings.Contains(joined, "2026-03-01") {
		t.Errorf("args %v missing end date", capturedArgs)
	}
}

func TestGetBalances_WithDepthAndQuery(t *testing.T) {
	var capturedArgs []string
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.52, linux-x86_64\n"), nil, nil
		}
		capturedArgs = args
		return []byte(balJSON), nil, nil
	}
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil)

	_, err = h.GetBalances(t.Context(), connect.NewRequest(&floatv1.GetBalancesRequest{
		Depth: 2,
		Query: []string{"expenses"},
	}))
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}

	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "--depth 2") {
		t.Errorf("args %v missing --depth 2", capturedArgs)
	}
	if !strings.Contains(joined, "expenses") {
		t.Errorf("args %v missing query 'expenses'", capturedArgs)
	}
}

func TestModifyTagsHandler(t *testing.T) {
	t.Run("empty_fid_returns_invalid_argument", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 30, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.ModifyTags(t.Context(), connect.NewRequest(&floatv1.ModifyTagsRequest{
			Fid:  "",
			Tags: map[string]string{"category": "food"},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("not_found_fid_returns_not_found", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 32, NumTxns: 2, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.ModifyTags(t.Context(), connect.NewRequest(&floatv1.ModifyTagsRequest{
			Fid:  "00000000",
			Tags: map[string]string{"category": "food"},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("code = %v, want NotFound", connect.CodeOf(err))
		}
	})

	t.Run("modifies_tags", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 31, NumTxns: 2, WithFIDs: true})
		h := mustRealHandler(t, dir)
		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}

		tx := journal.TransactionInput{
			Date:        time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
			Description: "HANDLER MODIFY TAGS TEST",
			Postings: []journal.PostingInput{
				{Account: "expenses:shopping", Commodity: "USD", Quantity: "30.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := journal.AppendTransaction(t.Context(), c, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		_, err = h.ModifyTags(t.Context(), connect.NewRequest(&floatv1.ModifyTagsRequest{
			Fid:  fid,
			Tags: map[string]string{"category": "household"},
		}))
		if err != nil {
			t.Fatalf("ModifyTags: %v", err)
		}

		txns, err := c.Transactions(t.Context(), "code:"+fid)
		if err != nil {
			t.Fatalf("Transactions after modify-tags: %v", err)
		}
		if len(txns) != 1 {
			t.Fatalf("expected 1 transaction, got %d", len(txns))
		}
		tagMap := make(map[string]string)
		for _, tag := range txns[0].Tags {
			tagMap[tag[0]] = tag[1]
		}
		if tagMap["category"] != "household" {
			t.Errorf("category = %q, want %q", tagMap["category"], "household")
		}

	})
}

func TestUpdateTransactionHandler(t *testing.T) {
	t.Run("empty_fid", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 60, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.UpdateTransaction(t.Context(), connect.NewRequest(&floatv1.UpdateTransactionRequest{
			Fid:         "",
			Description: "Test",
			Postings: []*floatv1.PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "10.00"},
				{Account: "assets:checking"},
			},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("empty_description", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 61, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.UpdateTransaction(t.Context(), connect.NewRequest(&floatv1.UpdateTransactionRequest{
			Fid:         "aa001100",
			Description: "",
			Postings: []*floatv1.PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "10.00"},
				{Account: "assets:checking"},
			},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("too_few_postings", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 62, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.UpdateTransaction(t.Context(), connect.NewRequest(&floatv1.UpdateTransactionRequest{
			Fid:         "aa001100",
			Description: "Test",
			Postings: []*floatv1.PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "10.00"},
			},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("posting_missing_account", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 63, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.UpdateTransaction(t.Context(), connect.NewRequest(&floatv1.UpdateTransactionRequest{
			Fid:         "aa001100",
			Description: "Test",
			Postings: []*floatv1.PostingInput{
				{Account: "", Commodity: "USD", Quantity: "10.00"},
				{Account: "assets:checking"},
			},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("not_found", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 64, NumTxns: 2, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.UpdateTransaction(t.Context(), connect.NewRequest(&floatv1.UpdateTransactionRequest{
			Fid:         "00000000",
			Description: "Test",
			Postings: []*floatv1.PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "10.00"},
				{Account: "assets:checking"},
			},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("code = %v, want NotFound", connect.CodeOf(err))
		}
	})

	t.Run("invalid_date", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 65, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}

		fid, err := journal.AppendTransaction(t.Context(), c, dir, journal.TransactionInput{
			Date:        time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
			Description: "ORIGINAL",
			Postings: []journal.PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "10.00"},
				{Account: "assets:checking"},
			},
		})
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		_, err = h.UpdateTransaction(t.Context(), connect.NewRequest(&floatv1.UpdateTransactionRequest{
			Fid:         fid,
			Description: "UPDATED",
			Date:        "not-a-date",
			Postings: []*floatv1.PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "10.00"},
				{Account: "assets:checking"},
			},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("success_updates_fields", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 66, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}

		fid, err := journal.AppendTransaction(t.Context(), c, dir, journal.TransactionInput{
			Date:        time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
			Description: "ORIGINAL",
			Comment:     "old note",
			Postings: []journal.PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "20.00"},
				{Account: "assets:checking"},
			},
		})
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		resp, err := h.UpdateTransaction(t.Context(), connect.NewRequest(&floatv1.UpdateTransactionRequest{
			Fid:         fid,
			Description: "UPDATED",
			Date:        "2026-02-15",
			Comment:     "new note",
			Postings: []*floatv1.PostingInput{
				{Account: "expenses:shopping", Commodity: "USD", Quantity: "55.00"},
				{Account: "assets:checking"},
			},
		}))
		if err != nil {
			t.Fatalf("UpdateTransaction: %v", err)
		}

		got := resp.Msg.Transaction
		if got.Fid != fid {
			t.Errorf("Fid = %q, want %q", got.Fid, fid)
		}
		if got.Description != "UPDATED" {
			t.Errorf("Description = %q, want %q", got.Description, "UPDATED")
		}
		if got.Date != "2026-02-15" {
			t.Errorf("Date = %q, want %q", got.Date, "2026-02-15")
		}
		if !strings.Contains(got.Comment, "new note") {
			t.Errorf("Comment %q does not contain %q", got.Comment, "new note")
		}
		if len(got.Postings) != 2 {
			t.Fatalf("expected 2 postings, got %d", len(got.Postings))
		}
		if got.Postings[0].Account != "expenses:shopping" {
			t.Errorf("Posting[0].Account = %q, want %q", got.Postings[0].Account, "expenses:shopping")
		}

		// Confirm only one transaction exists with this fid.
		txns, err := c.Transactions(t.Context(), "code:"+fid)
		if err != nil {
			t.Fatalf("Transactions: %v", err)
		}
		if len(txns) != 1 {
			t.Errorf("expected 1 transaction with fid %q, got %d", fid, len(txns))
		}
	})

	t.Run("empty_date_keeps_existing", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 67, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}

		fid, err := journal.AppendTransaction(t.Context(), c, dir, journal.TransactionInput{
			Date:        time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC),
			Description: "KEEP DATE TEST",
			Postings: []journal.PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "10.00"},
				{Account: "assets:checking"},
			},
		})
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		resp, err := h.UpdateTransaction(t.Context(), connect.NewRequest(&floatv1.UpdateTransactionRequest{
			Fid:         fid,
			Description: "KEEP DATE TEST UPDATED",
			Date:        "",
			Postings: []*floatv1.PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "10.00"},
				{Account: "assets:checking"},
			},
		}))
		if err != nil {
			t.Fatalf("UpdateTransaction: %v", err)
		}

		if resp.Msg.Transaction.Date != "2026-03-12" {
			t.Errorf("Date = %q, want %q (original should be preserved)", resp.Msg.Transaction.Date, "2026-03-12")
		}
	})
}

const payeesText = "Acme Corp\nGrocery Store\n"

func TestListPayees(t *testing.T) {
	h := mustHandler(t, map[string][]byte{
		"payees": []byte(payeesText),
	})

	resp, err := h.ListPayees(t.Context(), connect.NewRequest(&floatv1.ListPayeesRequest{}))
	if err != nil {
		t.Fatalf("ListPayees: %v", err)
	}

	payees := resp.Msg.Payees
	if len(payees) != 2 {
		t.Fatalf("expected 2 payees, got %d", len(payees))
	}
	if payees[0] != "Acme Corp" {
		t.Errorf("payees[0] = %q, want %q", payees[0], "Acme Corp")
	}
	if payees[1] != "Grocery Store" {
		t.Errorf("payees[1] = %q, want %q", payees[1], "Grocery Store")
	}
}

func TestListPayees_HledgerError(t *testing.T) {
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", errorRunner(t))
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil)
	_, err = h.ListPayees(t.Context(), connect.NewRequest(&floatv1.ListPayeesRequest{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("code = %v, want Internal", connect.CodeOf(err))
	}
}

func TestListPayees_CacheHit(t *testing.T) {
	var calls atomic.Int64
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.52, linux-x86_64\n"), nil, nil
		}
		calls.Add(1)
		return []byte(payeesText), nil, nil
	}
	h, _ := mustHandlerWithCache(t, runner)

	req := connect.NewRequest(&floatv1.ListPayeesRequest{})
	if _, err := h.ListPayees(t.Context(), req); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := h.ListPayees(t.Context(), req); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if calls.Load() != 1 {
		t.Errorf("hledger called %d times, want 1", calls.Load())
	}
}

func TestListPayees_CacheError(t *testing.T) {
	h, _ := mustHandlerWithCache(t, errorRunner(t))
	_, err := h.ListPayees(t.Context(), connect.NewRequest(&floatv1.ListPayeesRequest{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("code = %v, want Internal", connect.CodeOf(err))
	}
}

func TestListTransactions_FloatMetaFiltered(t *testing.T) {
	// A transaction with both user tags and float- hidden meta tags.
	const jsonWithFloatMeta = `[{
		"tcode": "aa001100",
		"tcomment": "",
		"tdate": "2026-01-05",
		"tdate2": null,
		"tdescription": "PAYROLL",
		"tindex": 1,
		"tpostings": [],
		"tprecedingcomment": "",
		"tstatus": "Unmarked",
		"ttags": [["category","income"],["float-import-id","batch42"],["float-updated-at","2026-01-05T00:00:00Z"]],
		"tsourcepos": [{"sourceName":"","sourceLine":0,"sourceColumn":0},{"sourceName":"","sourceLine":0,"sourceColumn":0}]
	}]`

	h := mustHandler(t, map[string][]byte{"print": []byte(jsonWithFloatMeta)})
	resp, err := h.ListTransactions(t.Context(), connect.NewRequest(&floatv1.ListTransactionsRequest{}))
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(resp.Msg.Transactions) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(resp.Msg.Transactions))
	}
	tags := resp.Msg.Transactions[0].Tags

	// User tag must be present.
	if tags["category"] != "income" {
		t.Errorf("tags[category] = %q, want %q", tags["category"], "income")
	}
	// Hidden meta tags must NOT appear in the proto tags map.
	if _, ok := tags["float-import-id"]; ok {
		t.Errorf("float-import-id should be filtered from proto tags, but was present: %q", tags["float-import-id"])
	}
	if _, ok := tags["float-updated-at"]; ok {
		t.Errorf("float-updated-at should be filtered from proto tags, but was present: %q", tags["float-updated-at"])
	}
}

// aregJSON is a two-row fixture for `hledger areg assets:checking -O json`.
// Each row is the documented 6-element array:
//
//	[Transaction, Transaction, Bool, [string], [Amount], [Amount]].
const aregJSON = `[
  [
    {"tcode":"aa001100","tcomment":"","tdate":"2026-01-05","tdate2":null,"tdescription":"PAYROLL | monthly","tindex":1,"tpostings":[],"tprecedingcomment":"","tstatus":"Cleared","ttags":[],"tsourcepos":[{"sourceName":"","sourceLine":0,"sourceColumn":0},{"sourceName":"","sourceLine":0,"sourceColumn":0}]},
    {"tcode":"aa001100","tcomment":"","tdate":"2026-01-05","tdate2":null,"tdescription":"PAYROLL | monthly","tindex":1,"tpostings":[],"tprecedingcomment":"","tstatus":"Cleared","ttags":[],"tsourcepos":[{"sourceName":"","sourceLine":0,"sourceColumn":0},{"sourceName":"","sourceLine":0,"sourceColumn":0}]},
    false,
    ["income:salary"],
    [{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":350000,"decimalPlaces":2,"floatingPoint":3500}}],
    [{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":350000,"decimalPlaces":2,"floatingPoint":3500}}]
  ],
  [
    {"tcode":"bb002200","tcomment":"","tdate":"2026-01-15","tdate2":null,"tdescription":"AMAZON","tindex":2,"tpostings":[],"tprecedingcomment":"","tstatus":"Unmarked","ttags":[],"tsourcepos":[{"sourceName":"","sourceLine":0,"sourceColumn":0},{"sourceName":"","sourceLine":0,"sourceColumn":0}]},
    {"tcode":"bb002200","tcomment":"","tdate":"2026-01-15","tdate2":null,"tdescription":"AMAZON","tindex":2,"tpostings":[],"tprecedingcomment":"","tstatus":"Unmarked","ttags":[],"tsourcepos":[{"sourceName":"","sourceLine":0,"sourceColumn":0},{"sourceName":"","sourceLine":0,"sourceColumn":0}]},
    false,
    ["expenses:shopping"],
    [{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":-4500,"decimalPlaces":2,"floatingPoint":-45}}],
    [{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":345500,"decimalPlaces":2,"floatingPoint":3455}}]
  ]
]`

func TestGetAccountRegister(t *testing.T) {
	h := mustHandler(t, map[string][]byte{"areg": []byte(aregJSON)})

	resp, err := h.GetAccountRegister(t.Context(), connect.NewRequest(&floatv1.GetAccountRegisterRequest{
		Account: "assets:checking",
	}))
	if err != nil {
		t.Fatalf("GetAccountRegister: %v", err)
	}
	rows := resp.Msg.Rows
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if resp.Msg.Total != 2 {
		t.Errorf("Total = %d, want 2", resp.Msg.Total)
	}
	if resp.Msg.HasNext {
		t.Error("HasNext should be false when all rows returned")
	}

	// Row 0: payroll deposit, status normalized from "Cleared" stays "Cleared",
	// payee/note parsed from "PAYROLL | monthly".
	r0 := rows[0]
	if r0.Fid != "aa001100" {
		t.Errorf("row 0 Fid = %q, want aa001100", r0.Fid)
	}
	if r0.Date != "2026-01-05" {
		t.Errorf("row 0 Date = %q, want 2026-01-05", r0.Date)
	}
	if r0.Description != "PAYROLL | monthly" {
		t.Errorf("row 0 Description = %q", r0.Description)
	}
	if r0.Payee == nil || *r0.Payee != "PAYROLL" {
		t.Errorf("row 0 Payee = %v, want PAYROLL", r0.Payee)
	}
	if r0.Note == nil || *r0.Note != "monthly" {
		t.Errorf("row 0 Note = %v, want monthly", r0.Note)
	}
	if r0.Status != "Cleared" {
		t.Errorf("row 0 Status = %q, want Cleared", r0.Status)
	}
	if len(r0.OtherAccounts) != 1 || r0.OtherAccounts[0] != "income:salary" {
		t.Errorf("row 0 OtherAccounts = %v", r0.OtherAccounts)
	}
	if len(r0.Change) != 1 || r0.Change[0].Quantity != "3500.00" || r0.Change[0].Commodity != "$" {
		t.Errorf("row 0 Change = %+v", r0.Change)
	}
	if len(r0.RunningTotal) != 1 || r0.RunningTotal[0].Quantity != "3500.00" {
		t.Errorf("row 0 RunningTotal = %+v", r0.RunningTotal)
	}

	// Row 1: $45 expense — signed negative, running balance 3455, status
	// normalized from "Unmarked" to "", description without "|" yields nil
	// payee/note.
	r1 := rows[1]
	if r1.Status != "" {
		t.Errorf("row 1 Status = %q, want empty (Unmarked normalized)", r1.Status)
	}
	if r1.Payee != nil || r1.Note != nil {
		t.Errorf("row 1 Payee/Note should be nil, got %v/%v", r1.Payee, r1.Note)
	}
	if r1.Change[0].Quantity != "-45.00" {
		t.Errorf("row 1 Change = %q, want -45.00", r1.Change[0].Quantity)
	}
	if r1.RunningTotal[0].Quantity != "3455.00" {
		t.Errorf("row 1 RunningTotal = %q, want 3455.00", r1.RunningTotal[0].Quantity)
	}
}

func TestGetAccountRegister_EmptyAccount(t *testing.T) {
	h := mustHandler(t, map[string][]byte{"areg": []byte(aregJSON)})
	_, err := h.GetAccountRegister(t.Context(), connect.NewRequest(&floatv1.GetAccountRegisterRequest{
		Account: "   ",
	}))
	if err == nil {
		t.Fatal("expected error for empty account, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestGetAccountRegister_PassesArgsToHledger(t *testing.T) {
	var capturedArgs []string
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.52, linux-x86_64\n"), nil, nil
		}
		capturedArgs = args
		return []byte("[]"), nil, nil
	}
	c, err := hledger.NewWithRunner("hledger", "journal.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil)

	_, err = h.GetAccountRegister(t.Context(), connect.NewRequest(&floatv1.GetAccountRegisterRequest{
		Account: "assets:checking",
		Query:   []string{"date:2026-01", "status:cleared"},
	}))
	if err != nil {
		t.Fatalf("GetAccountRegister: %v", err)
	}

	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "areg") {
		t.Errorf("args %v missing areg subcommand", capturedArgs)
	}
	// Focused account must appear as a positional arg, not a query token.
	if !strings.Contains(joined, "assets:checking") {
		t.Errorf("args %v missing focused account 'assets:checking'", capturedArgs)
	}
	if !strings.Contains(joined, "date:2026-01") {
		t.Errorf("args %v missing query token 'date:2026-01'", capturedArgs)
	}
	if !strings.Contains(joined, "status:cleared") {
		t.Errorf("args %v missing query token 'status:cleared'", capturedArgs)
	}
}

func TestGetAccountRegister_Pagination(t *testing.T) {
	h := mustHandler(t, map[string][]byte{"areg": []byte(aregJSON)})

	// limit=1 → first row only, HasNext=true, Total=2.
	resp, err := h.GetAccountRegister(t.Context(), connect.NewRequest(&floatv1.GetAccountRegisterRequest{
		Account: "assets:checking",
		Limit:   1,
	}))
	if err != nil {
		t.Fatalf("GetAccountRegister: %v", err)
	}
	if len(resp.Msg.Rows) != 1 || resp.Msg.Rows[0].Fid != "aa001100" {
		t.Errorf("limit=1: unexpected rows %+v", resp.Msg.Rows)
	}
	if resp.Msg.Total != 2 {
		t.Errorf("Total = %d, want 2", resp.Msg.Total)
	}
	if !resp.Msg.HasNext {
		t.Error("HasNext should be true when limit truncates results")
	}

	// offset=1, limit=10 → second row, HasNext=false.
	resp, err = h.GetAccountRegister(t.Context(), connect.NewRequest(&floatv1.GetAccountRegisterRequest{
		Account: "assets:checking",
		Offset:  1,
		Limit:   10,
	}))
	if err != nil {
		t.Fatalf("GetAccountRegister: %v", err)
	}
	if len(resp.Msg.Rows) != 1 || resp.Msg.Rows[0].Fid != "bb002200" {
		t.Errorf("offset=1: unexpected rows %+v", resp.Msg.Rows)
	}
	if resp.Msg.HasNext {
		t.Error("HasNext should be false when fewer than limit rows remain")
	}

	// offset >= total → empty result.
	resp, err = h.GetAccountRegister(t.Context(), connect.NewRequest(&floatv1.GetAccountRegisterRequest{
		Account: "assets:checking",
		Offset:  5,
	}))
	if err != nil {
		t.Fatalf("GetAccountRegister: %v", err)
	}
	if len(resp.Msg.Rows) != 0 {
		t.Errorf("offset>total should yield empty rows, got %d", len(resp.Msg.Rows))
	}
	if resp.Msg.Total != 2 {
		t.Errorf("Total = %d, want 2 (pre-pagination total)", resp.Msg.Total)
	}
}

func TestGetAccountRegister_HledgerError(t *testing.T) {
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.52, linux-x86_64\n"), nil, nil
		}
		return nil, []byte("boom"), errors.New("exec failed")
	}
	c, err := hledger.NewWithRunner("hledger", "journal.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil)

	_, err = h.GetAccountRegister(t.Context(), connect.NewRequest(&floatv1.GetAccountRegisterRequest{
		Account: "assets:checking",
	}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("code = %v, want Internal", connect.CodeOf(err))
	}
}

func TestGetAccountRegister_CacheHit(t *testing.T) {
	var callCount atomic.Int32
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.52, linux-x86_64\n"), nil, nil
		}
		callCount.Add(1)
		return []byte(aregJSON), nil, nil
	}
	c, err := hledger.NewWithRunner("hledger", "journal.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	cch := cache.New[any](func() uint64 { return 0 })
	h := serverledger.NewHandler(c, nil, "", "", cch, nil, nil)

	req := connect.NewRequest(&floatv1.GetAccountRegisterRequest{Account: "assets:checking"})
	if _, err := h.GetAccountRegister(t.Context(), req); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := h.GetAccountRegister(t.Context(), req); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if got := callCount.Load(); got != 1 {
		t.Errorf("expected 1 hledger call (second served from cache), got %d", got)
	}
}

func TestBulkEditTransactionsHandler(t *testing.T) {
	// appendTx is a helper that adds a transaction and returns its FID.
	appendTx := func(t *testing.T, c *hledger.Client, dir string, tx journal.TransactionInput) string {
		t.Helper()
		fid, err := journal.AppendTransaction(t.Context(), c, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}
		return fid
	}

	baseTx := func(desc string) journal.TransactionInput {
		return journal.TransactionInput{
			Date:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			Description: desc,
			Postings: []journal.PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "10.00"},
				{Account: "assets:checking"},
			},
		}
	}

	t.Run("empty_fids", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 200, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.BulkEditTransactions(t.Context(), connect.NewRequest(&floatv1.BulkEditTransactionsRequest{
			Fids:       []string{},
			Operations: []*floatv1.BulkEditOperation{{Operation: &floatv1.BulkEditOperation_MarkReviewed{MarkReviewed: &floatv1.MarkReviewedOperation{Reviewed: true}}}},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("empty_operations", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 201, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.BulkEditTransactions(t.Context(), connect.NewRequest(&floatv1.BulkEditTransactionsRequest{
			Fids:       []string{"aa001100"},
			Operations: []*floatv1.BulkEditOperation{},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("add_tag_empty_key", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 202, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.BulkEditTransactions(t.Context(), connect.NewRequest(&floatv1.BulkEditTransactionsRequest{
			Fids: []string{"aa001100"},
			Operations: []*floatv1.BulkEditOperation{
				{Operation: &floatv1.BulkEditOperation_AddTag{AddTag: &floatv1.AddTagOperation{Key: "", Value: "v"}}},
			},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("add_tag_reserved_prefix", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 203, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.BulkEditTransactions(t.Context(), connect.NewRequest(&floatv1.BulkEditTransactionsRequest{
			Fids: []string{"aa001100"},
			Operations: []*floatv1.BulkEditOperation{
				{Operation: &floatv1.BulkEditOperation_AddTag{AddTag: &floatv1.AddTagOperation{Key: "float-custom", Value: "v"}}},
			},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("remove_tag_empty_key", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 204, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.BulkEditTransactions(t.Context(), connect.NewRequest(&floatv1.BulkEditTransactionsRequest{
			Fids: []string{"aa001100"},
			Operations: []*floatv1.BulkEditOperation{
				{Operation: &floatv1.BulkEditOperation_RemoveTag{RemoveTag: &floatv1.RemoveTagOperation{Key: ""}}},
			},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("set_payee_empty_payee", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 205, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.BulkEditTransactions(t.Context(), connect.NewRequest(&floatv1.BulkEditTransactionsRequest{
			Fids: []string{"aa001100"},
			Operations: []*floatv1.BulkEditOperation{
				{Operation: &floatv1.BulkEditOperation_SetPayee{SetPayee: &floatv1.SetPayeeOperation{Payee: ""}}},
			},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("unknown_fid", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 206, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.BulkEditTransactions(t.Context(), connect.NewRequest(&floatv1.BulkEditTransactionsRequest{
			Fids: []string{"00000000"},
			Operations: []*floatv1.BulkEditOperation{
				{Operation: &floatv1.BulkEditOperation_MarkReviewed{MarkReviewed: &floatv1.MarkReviewedOperation{Reviewed: true}}},
			},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("code = %v, want NotFound", connect.CodeOf(err))
		}
	})

	t.Run("mark_reviewed_true", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 207, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}
		fid1 := appendTx(t, c, dir, baseTx("MARK REVIEWED 1"))
		fid2 := appendTx(t, c, dir, baseTx("MARK REVIEWED 2"))

		resp, err := h.BulkEditTransactions(t.Context(), connect.NewRequest(&floatv1.BulkEditTransactionsRequest{
			Fids: []string{fid1, fid2},
			Operations: []*floatv1.BulkEditOperation{
				{Operation: &floatv1.BulkEditOperation_MarkReviewed{MarkReviewed: &floatv1.MarkReviewedOperation{Reviewed: true}}},
			},
		}))
		if err != nil {
			t.Fatalf("BulkEditTransactions: %v", err)
		}
		if len(resp.Msg.Transactions) != 2 {
			t.Fatalf("expected 2 transactions, got %d", len(resp.Msg.Transactions))
		}
		for _, tx := range resp.Msg.Transactions {
			if tx.Status != "Cleared" {
				t.Errorf("Status = %q, want %q", tx.Status, "Cleared")
			}
		}
		// Verify FIDs are in the same order as input.
		if resp.Msg.Transactions[0].Fid != fid1 {
			t.Errorf("Transactions[0].Fid = %q, want %q", resp.Msg.Transactions[0].Fid, fid1)
		}
		if resp.Msg.Transactions[1].Fid != fid2 {
			t.Errorf("Transactions[1].Fid = %q, want %q", resp.Msg.Transactions[1].Fid, fid2)
		}
	})

	t.Run("mark_reviewed_false", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 208, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}
		tx := baseTx("MARK UNREVIEWED")
		tx.Status = "Cleared"
		fid := appendTx(t, c, dir, tx)

		resp, err := h.BulkEditTransactions(t.Context(), connect.NewRequest(&floatv1.BulkEditTransactionsRequest{
			Fids: []string{fid},
			Operations: []*floatv1.BulkEditOperation{
				{Operation: &floatv1.BulkEditOperation_MarkReviewed{MarkReviewed: &floatv1.MarkReviewedOperation{Reviewed: false}}},
			},
		}))
		if err != nil {
			t.Fatalf("BulkEditTransactions: %v", err)
		}
		if len(resp.Msg.Transactions) != 1 {
			t.Fatalf("expected 1 transaction, got %d", len(resp.Msg.Transactions))
		}
		if resp.Msg.Transactions[0].Status != "" {
			t.Errorf("Status = %q, want %q", resp.Msg.Transactions[0].Status, "")
		}
	})

	t.Run("add_tag", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 209, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}
		fid1 := appendTx(t, c, dir, baseTx("ADD TAG 1"))
		fid2 := appendTx(t, c, dir, baseTx("ADD TAG 2"))

		resp, err := h.BulkEditTransactions(t.Context(), connect.NewRequest(&floatv1.BulkEditTransactionsRequest{
			Fids: []string{fid1, fid2},
			Operations: []*floatv1.BulkEditOperation{
				{Operation: &floatv1.BulkEditOperation_AddTag{AddTag: &floatv1.AddTagOperation{Key: "category", Value: "food"}}},
			},
		}))
		if err != nil {
			t.Fatalf("BulkEditTransactions: %v", err)
		}
		if len(resp.Msg.Transactions) != 2 {
			t.Fatalf("expected 2 transactions, got %d", len(resp.Msg.Transactions))
		}
		for i, tx := range resp.Msg.Transactions {
			if tx.Tags["category"] != "food" {
				t.Errorf("Transactions[%d].Tags[category] = %q, want %q", i, tx.Tags["category"], "food")
			}
		}
	})

	t.Run("remove_tag", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 210, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}
		tx := baseTx("REMOVE TAG")
		tx.Tags = map[string]string{"category": "food", "keep": "yes"}
		fid := appendTx(t, c, dir, tx)

		resp, err := h.BulkEditTransactions(t.Context(), connect.NewRequest(&floatv1.BulkEditTransactionsRequest{
			Fids: []string{fid},
			Operations: []*floatv1.BulkEditOperation{
				{Operation: &floatv1.BulkEditOperation_RemoveTag{RemoveTag: &floatv1.RemoveTagOperation{Key: "category"}}},
			},
		}))
		if err != nil {
			t.Fatalf("BulkEditTransactions: %v", err)
		}
		if len(resp.Msg.Transactions) != 1 {
			t.Fatalf("expected 1 transaction, got %d", len(resp.Msg.Transactions))
		}
		got := resp.Msg.Transactions[0]
		if _, ok := got.Tags["category"]; ok {
			t.Errorf("tag 'category' should have been removed, but is still present: %q", got.Tags["category"])
		}
		if got.Tags["keep"] != "yes" {
			t.Errorf("tag 'keep' = %q, want %q", got.Tags["keep"], "yes")
		}
	})

	t.Run("set_payee", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 211, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}
		// Transaction with description only (no "|") — payee should be set and description preserved as note.
		tx := baseTx("Groceries")
		fid := appendTx(t, c, dir, tx)

		resp, err := h.BulkEditTransactions(t.Context(), connect.NewRequest(&floatv1.BulkEditTransactionsRequest{
			Fids: []string{fid},
			Operations: []*floatv1.BulkEditOperation{
				{Operation: &floatv1.BulkEditOperation_SetPayee{SetPayee: &floatv1.SetPayeeOperation{Payee: "Costco"}}},
			},
		}))
		if err != nil {
			t.Fatalf("BulkEditTransactions: %v", err)
		}
		if len(resp.Msg.Transactions) != 1 {
			t.Fatalf("expected 1 transaction, got %d", len(resp.Msg.Transactions))
		}
		got := resp.Msg.Transactions[0]
		if got.Payee == nil || *got.Payee != "Costco" {
			t.Errorf("Payee = %v, want %q", got.Payee, "Costco")
		}
		// Original description "Groceries" had no "|", so it becomes the note.
		if got.Note == nil || *got.Note != "Groceries" {
			t.Errorf("Note = %v, want %q", got.Note, "Groceries")
		}
	})

	t.Run("set_payee_preserves_note", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 212, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}
		tx := baseTx("OldPayee | my note")
		fid := appendTx(t, c, dir, tx)

		resp, err := h.BulkEditTransactions(t.Context(), connect.NewRequest(&floatv1.BulkEditTransactionsRequest{
			Fids: []string{fid},
			Operations: []*floatv1.BulkEditOperation{
				{Operation: &floatv1.BulkEditOperation_SetPayee{SetPayee: &floatv1.SetPayeeOperation{Payee: "NewPayee"}}},
			},
		}))
		if err != nil {
			t.Fatalf("BulkEditTransactions: %v", err)
		}
		if len(resp.Msg.Transactions) != 1 {
			t.Fatalf("expected 1 transaction, got %d", len(resp.Msg.Transactions))
		}
		got := resp.Msg.Transactions[0]
		if got.Payee == nil || *got.Payee != "NewPayee" {
			t.Errorf("Payee = %v, want %q", got.Payee, "NewPayee")
		}
		if got.Note == nil || *got.Note != "my note" {
			t.Errorf("Note = %v, want %q", got.Note, "my note")
		}
	})

	t.Run("clear_payee", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 213, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}
		tx := baseTx("SomePayee | original note")
		fid := appendTx(t, c, dir, tx)

		resp, err := h.BulkEditTransactions(t.Context(), connect.NewRequest(&floatv1.BulkEditTransactionsRequest{
			Fids: []string{fid},
			Operations: []*floatv1.BulkEditOperation{
				{Operation: &floatv1.BulkEditOperation_ClearPayee{ClearPayee: &floatv1.ClearPayeeOperation{}}},
			},
		}))
		if err != nil {
			t.Fatalf("BulkEditTransactions: %v", err)
		}
		if len(resp.Msg.Transactions) != 1 {
			t.Fatalf("expected 1 transaction, got %d", len(resp.Msg.Transactions))
		}
		got := resp.Msg.Transactions[0]
		// After clearing payee, description becomes just the note; no "|" means no payee/note split.
		if got.Payee != nil {
			t.Errorf("Payee = %v, want nil after clear", got.Payee)
		}
	})

	t.Run("multiple_operations_applied_atomically", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 214, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}
		tx := baseTx("MULTI OP")
		fid := appendTx(t, c, dir, tx)

		resp, err := h.BulkEditTransactions(t.Context(), connect.NewRequest(&floatv1.BulkEditTransactionsRequest{
			Fids: []string{fid},
			Operations: []*floatv1.BulkEditOperation{
				{Operation: &floatv1.BulkEditOperation_MarkReviewed{MarkReviewed: &floatv1.MarkReviewedOperation{Reviewed: true}}},
				{Operation: &floatv1.BulkEditOperation_AddTag{AddTag: &floatv1.AddTagOperation{Key: "category", Value: "misc"}}},
				{Operation: &floatv1.BulkEditOperation_SetPayee{SetPayee: &floatv1.SetPayeeOperation{Payee: "Acme"}}},
			},
		}))
		if err != nil {
			t.Fatalf("BulkEditTransactions: %v", err)
		}
		if len(resp.Msg.Transactions) != 1 {
			t.Fatalf("expected 1 transaction, got %d", len(resp.Msg.Transactions))
		}
		got := resp.Msg.Transactions[0]
		if got.Status != "Cleared" {
			t.Errorf("Status = %q, want %q", got.Status, "Cleared")
		}
		if got.Tags["category"] != "misc" {
			t.Errorf("Tags[category] = %q, want %q", got.Tags["category"], "misc")
		}
		if got.Payee == nil || *got.Payee != "Acme" {
			t.Errorf("Payee = %v, want %q", got.Payee, "Acme")
		}
	})
}

func TestImportTransactionsHandler(t *testing.T) {
	t.Run("rule_with_tags_applied_without_panic", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 300, NumTxns: 1, WithFIDs: true})

		hledgerRules := "skip 1\nfields date, description, amount\naccount1 assets:checking\n\nif AMAZON\n  account2 expenses:shopping\n"
		if err := os.WriteFile(filepath.Join(dir, "bank.rules"), []byte(hledgerRules), 0o644); err != nil {
			t.Fatalf("write bank.rules: %v", err)
		}

		floatRulesJSON := `[{"id":"abcd1234","pattern":"AMAZON","payee":"","account":"","tags":{"source":"amazon"},"priority":0,"auto_reviewed":false}]`
		if err := os.WriteFile(filepath.Join(dir, "rules.json"), []byte(floatRulesJSON), 0o644); err != nil {
			t.Fatalf("write rules.json: %v", err)
		}

		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}
		lock := txlock.New(dir, c)
		cfg := &config.Config{
			BankProfiles: []config.BankProfile{
				{Name: "test-bank", RulesFile: "bank.rules"},
			},
		}
		h := serverledger.NewHandler(c, lock, dir, "", nil, nil, cfg)

		csvData := []byte("date,description,amount\n2026-01-15,AMAZON MARKETPLACE,-45.00\n")
		resp, err := h.ImportTransactions(t.Context(), connect.NewRequest(&floatv1.ImportTransactionsRequest{
			CsvData:          csvData,
			ProfileName:      "test-bank",
			CandidateIndices: []int32{0},
		}))
		if err != nil {
			t.Fatalf("ImportTransactions: %v", err)
		}
		if resp.Msg.ImportedCount != 1 {
			t.Errorf("ImportedCount = %d, want 1", resp.Msg.ImportedCount)
		}
		if len(resp.Msg.Transactions) != 1 {
			t.Fatalf("expected 1 transaction, got %d", len(resp.Msg.Transactions))
		}
		if resp.Msg.Transactions[0].Tags["source"] != "amazon" {
			t.Errorf("Tags[source] = %q, want %q", resp.Msg.Transactions[0].Tags["source"], "amazon")
		}
	})

	t.Run("import_batch_id_populated_on_imported_transactions", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 301, NumTxns: 1, WithFIDs: true})

		hledgerRules := "skip 1\nfields date, description, amount\naccount1 assets:checking\naccount2 expenses:misc\n"
		if err := os.WriteFile(filepath.Join(dir, "bank.rules"), []byte(hledgerRules), 0o644); err != nil {
			t.Fatalf("write bank.rules: %v", err)
		}

		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}
		lock := txlock.New(dir, c)
		cfg := &config.Config{
			BankProfiles: []config.BankProfile{
				{Name: "test-bank", RulesFile: "bank.rules"},
			},
		}
		h := serverledger.NewHandler(c, lock, dir, "", nil, nil, cfg)

		csvData := []byte("date,description,amount\n2026-01-15,COFFEE SHOP,-4.50\n2026-01-16,GROCERY STORE,-30.00\n")
		importResp, err := h.ImportTransactions(t.Context(), connect.NewRequest(&floatv1.ImportTransactionsRequest{
			CsvData:          csvData,
			ProfileName:      "test-bank",
			CandidateIndices: []int32{0, 1},
		}))
		if err != nil {
			t.Fatalf("ImportTransactions: %v", err)
		}
		batchID := importResp.Msg.ImportBatchId
		if batchID == "" {
			t.Fatal("response ImportBatchId is empty")
		}
		if len(importResp.Msg.Transactions) != 2 {
			t.Fatalf("expected 2 imported transactions, got %d", len(importResp.Msg.Transactions))
		}
		for i, tx := range importResp.Msg.Transactions {
			if tx.ImportBatchId == nil {
				t.Errorf("imported[%d].ImportBatchId = nil, want %q", i, batchID)
				continue
			}
			if *tx.ImportBatchId != batchID {
				t.Errorf("imported[%d].ImportBatchId = %q, want %q", i, *tx.ImportBatchId, batchID)
			}
		}

		// Round-trip verification: re-fetch via ListTransactions to ensure the
		// field survives parsing from hledger output.
		listResp, err := h.ListTransactions(t.Context(), connect.NewRequest(&floatv1.ListTransactionsRequest{}))
		if err != nil {
			t.Fatalf("ListTransactions: %v", err)
		}

		var withBatch, withoutBatch int
		for _, tx := range listResp.Msg.Transactions {
			if tx.ImportBatchId == nil {
				withoutBatch++
				continue
			}
			if *tx.ImportBatchId != batchID {
				t.Errorf("unexpected ImportBatchId %q on txn %s", *tx.ImportBatchId, tx.Fid)
				continue
			}
			withBatch++
		}
		if withBatch != 2 {
			t.Errorf("transactions with batch id = %d, want 2", withBatch)
		}
		if withoutBatch != 1 {
			t.Errorf("transactions without batch id = %d, want 1 (the seeded txn)", withoutBatch)
		}
	})
}

// printJSONWithCost has two postings: one with a UnitCost (@) and one with a
// TotalCost (@@). Used to verify toProtoAmount surfaces cost annotations.
const printJSONWithCost = `[
  {
    "tcode": "bb002200",
    "tcomment": "",
    "tdate": "2026-01-15",
    "tdate2": null,
    "tdescription": "Buy AAPL",
    "tindex": 1,
    "tpostings": [
      {
        "paccount": "assets:investments:aapl",
        "pamount": [{
          "acommodity": "AAPL",
          "acost": {"tag":"UnitCost","contents":{"acommodity":"USD","aquantity":{"decimalMantissa":17500,"decimalPlaces":2,"floatingPoint":175.00},"acost":null}},
          "aquantity": {"decimalMantissa": 10, "decimalPlaces": 0, "floatingPoint": 10}
        }],
        "pcomment": "",
        "pdate": null,
        "pdate2": null,
        "pstatus": "Unmarked",
        "ptags": [],
        "ptransaction_": "1",
        "ptype": "RegularPosting"
      },
      {
        "paccount": "assets:investments:msft",
        "pamount": [{
          "acommodity": "MSFT",
          "acost": {"tag":"TotalCost","contents":{"acommodity":"USD","aquantity":{"decimalMantissa":200000,"decimalPlaces":2,"floatingPoint":2000.00},"acost":null}},
          "aquantity": {"decimalMantissa": 5, "decimalPlaces": 0, "floatingPoint": 5}
        }],
        "pcomment": "",
        "pdate": null,
        "pdate2": null,
        "pstatus": "Unmarked",
        "ptags": [],
        "ptransaction_": "1",
        "ptype": "RegularPosting"
      },
      {
        "paccount": "assets:checking",
        "pamount": [{"acommodity":"USD","acost":null,"aquantity":{"decimalMantissa":-375000,"decimalPlaces":2,"floatingPoint":-3750.00}}],
        "pcomment": "",
        "pdate": null,
        "pdate2": null,
        "pstatus": "Unmarked",
        "ptags": [],
        "ptransaction_": "1",
        "ptype": "RegularPosting"
      }
    ],
    "tprecedingcomment": "",
    "tstatus": "Unmarked",
    "ttags": [],
    "tsourcepos": [{"sourceName": "simple.journal", "sourceLine": 1, "sourceColumn": 1}, {"sourceName": "simple.journal", "sourceLine": 5, "sourceColumn": 1}]
  }
]`

func TestListTransactions_PopulatesCost(t *testing.T) {
	h := mustHandler(t, map[string][]byte{
		"print": []byte(printJSONWithCost),
	})

	resp, err := h.ListTransactions(t.Context(), connect.NewRequest(&floatv1.ListTransactionsRequest{}))
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}

	txns := resp.Msg.Transactions
	if len(txns) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(txns))
	}
	postings := txns[0].Postings
	if len(postings) != 3 {
		t.Fatalf("expected 3 postings, got %d", len(postings))
	}

	// Posting 0: AAPL with per-unit cost.
	aapl := postings[0].Amounts[0]
	if aapl.Cost == nil {
		t.Fatalf("AAPL amount: Cost is nil, want UnitCost")
	}
	if aapl.Cost.Commodity != "USD" {
		t.Errorf("AAPL Cost.Commodity = %q, want %q", aapl.Cost.Commodity, "USD")
	}
	if aapl.Cost.Quantity != "175.00" {
		t.Errorf("AAPL Cost.Quantity = %q, want %q", aapl.Cost.Quantity, "175.00")
	}
	if aapl.Cost.IsTotal {
		t.Error("AAPL Cost.IsTotal = true, want false (UnitCost)")
	}

	// Posting 1: MSFT with total cost.
	msft := postings[1].Amounts[0]
	if msft.Cost == nil {
		t.Fatalf("MSFT amount: Cost is nil, want TotalCost")
	}
	if msft.Cost.Commodity != "USD" {
		t.Errorf("MSFT Cost.Commodity = %q, want %q", msft.Cost.Commodity, "USD")
	}
	if msft.Cost.Quantity != "2000.00" {
		t.Errorf("MSFT Cost.Quantity = %q, want %q", msft.Cost.Quantity, "2000.00")
	}
	if !msft.Cost.IsTotal {
		t.Error("MSFT Cost.IsTotal = false, want true (TotalCost)")
	}

	// Posting 2: plain USD posting with no cost.
	checking := postings[2].Amounts[0]
	if checking.Cost != nil {
		t.Errorf("checking amount: Cost = %+v, want nil", checking.Cost)
	}
}

func TestGetBalances_AmountsHaveNoCost(t *testing.T) {
	h := mustHandler(t, map[string][]byte{
		"bal": []byte(balJSON),
	})

	resp, err := h.GetBalances(t.Context(), connect.NewRequest(&floatv1.GetBalancesRequest{}))
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}
	for _, row := range resp.Msg.Report.Rows {
		for _, amt := range row.Amounts {
			if amt.Cost != nil {
				t.Errorf("balance row %q: amount %+v unexpectedly has Cost = %+v", row.FullName, amt, amt.Cost)
			}
		}
	}
}

func TestRoundTripCost(t *testing.T) {
	investAccounts := []string{
		"assets:checking",
		"assets:investments:aapl",
		"assets:investments:msft",
		"income:salary",
	}

	t.Run("add_with_unit_cost_round_trips", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 200, NumTxns: 1, WithFIDs: true, Accounts: investAccounts})
		h := mustRealHandler(t, dir)

		resp, err := h.AddTransaction(t.Context(), connect.NewRequest(&floatv1.AddTransactionRequest{
			Description: "Buy AAPL",
			Date:        "2026-04-30",
			Postings: []*floatv1.PostingInput{
				{
					Account: "assets:investments:aapl", Commodity: "AAPL", Quantity: "10",
					Cost: &floatv1.Cost{Commodity: "USD", Quantity: "175.00", IsTotal: false},
				},
				{Account: "assets:checking", Commodity: "USD", Quantity: "-1750.00"},
			},
		}))
		if err != nil {
			t.Fatalf("AddTransaction: %v", err)
		}

		got := resp.Msg.Transaction
		if len(got.Postings) < 1 || len(got.Postings[0].Amounts) < 1 {
			t.Fatalf("unexpected response shape: %+v", got)
		}
		amt := got.Postings[0].Amounts[0]
		if amt.Commodity != "AAPL" {
			t.Errorf("Commodity = %q, want %q", amt.Commodity, "AAPL")
		}
		if amt.Cost == nil {
			t.Fatalf("Cost is nil; expected per-unit cost on AAPL posting")
		}
		if amt.Cost.Commodity != "USD" || amt.Cost.Quantity != "175.00" || amt.Cost.IsTotal {
			t.Errorf("Cost = %+v, want {USD 175.00 false}", amt.Cost)
		}

		// Verify the on-disk journal contains the @ cost syntax.
		data, err := os.ReadFile(filepath.Join(dir, "2026/04.journal"))
		if err != nil {
			t.Fatalf("read journal: %v", err)
		}
		if !strings.Contains(string(data), "@ 175.00 USD") {
			t.Errorf("journal missing per-unit cost annotation; got:\n%s", data)
		}
	})

	t.Run("add_with_total_cost_round_trips", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 201, NumTxns: 1, WithFIDs: true, Accounts: investAccounts})
		h := mustRealHandler(t, dir)

		resp, err := h.AddTransaction(t.Context(), connect.NewRequest(&floatv1.AddTransactionRequest{
			Description: "Buy MSFT",
			Date:        "2026-04-30",
			Postings: []*floatv1.PostingInput{
				{
					Account: "assets:investments:msft", Commodity: "MSFT", Quantity: "5",
					Cost: &floatv1.Cost{Commodity: "USD", Quantity: "2000.00", IsTotal: true},
				},
				{Account: "assets:checking", Commodity: "USD", Quantity: "-2000.00"},
			},
		}))
		if err != nil {
			t.Fatalf("AddTransaction: %v", err)
		}

		amt := resp.Msg.Transaction.Postings[0].Amounts[0]
		if amt.Cost == nil {
			t.Fatalf("Cost is nil; expected total cost on MSFT posting")
		}
		if !amt.Cost.IsTotal {
			t.Errorf("IsTotal = false, want true")
		}
		if amt.Cost.Commodity != "USD" {
			t.Errorf("Cost.Commodity = %q, want %q", amt.Cost.Commodity, "USD")
		}
		// hledger's JSON strips trailing zeros from total-cost decimalMantissa,
		// so the returned quantity may be "2000" or "2000.00" depending on input
		// precision. Either is correct.
		if amt.Cost.Quantity != "2000" && amt.Cost.Quantity != "2000.00" {
			t.Errorf("Cost.Quantity = %q, want %q or %q", amt.Cost.Quantity, "2000", "2000.00")
		}

		data, err := os.ReadFile(filepath.Join(dir, "2026/04.journal"))
		if err != nil {
			t.Fatalf("read journal: %v", err)
		}
		s := string(data)
		if !strings.Contains(s, "@@ 2000.00 USD") && !strings.Contains(s, "@@ 2000 USD") {
			t.Errorf("journal missing total-cost annotation; got:\n%s", s)
		}
	})

	t.Run("update_overwrites_cost", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 202, NumTxns: 1, WithFIDs: true, Accounts: investAccounts})
		h := mustRealHandler(t, dir)

		// Seed a transaction with @ unit cost.
		add, err := h.AddTransaction(t.Context(), connect.NewRequest(&floatv1.AddTransactionRequest{
			Description: "Buy AAPL",
			Date:        "2026-04-30",
			Postings: []*floatv1.PostingInput{
				{
					Account: "assets:investments:aapl", Commodity: "AAPL", Quantity: "10",
					Cost: &floatv1.Cost{Commodity: "USD", Quantity: "175.00", IsTotal: false},
				},
				{Account: "assets:checking", Commodity: "USD", Quantity: "-1750.00"},
			},
		}))
		if err != nil {
			t.Fatalf("AddTransaction: %v", err)
		}
		fid := add.Msg.Transaction.Fid

		// Update: switch to @@ total cost at a different price.
		upd, err := h.UpdateTransaction(t.Context(), connect.NewRequest(&floatv1.UpdateTransactionRequest{
			Fid:         fid,
			Description: "Buy AAPL",
			Date:        "2026-04-30",
			Postings: []*floatv1.PostingInput{
				{
					Account: "assets:investments:aapl", Commodity: "AAPL", Quantity: "10",
					Cost: &floatv1.Cost{Commodity: "USD", Quantity: "1800.00", IsTotal: true},
				},
				{Account: "assets:checking", Commodity: "USD", Quantity: "-1800.00"},
			},
		}))
		if err != nil {
			t.Fatalf("UpdateTransaction: %v", err)
		}
		amt := upd.Msg.Transaction.Postings[0].Amounts[0]
		if amt.Cost == nil {
			t.Fatalf("Cost is nil after update")
		}
		if !amt.Cost.IsTotal {
			t.Errorf("IsTotal = false, want true")
		}
		// hledger may strip trailing zeros from total-cost amounts; accept either form.
		if amt.Cost.Quantity != "1800" && amt.Cost.Quantity != "1800.00" {
			t.Errorf("Cost.Quantity = %q, want %q or %q", amt.Cost.Quantity, "1800", "1800.00")
		}

		data, err := os.ReadFile(filepath.Join(dir, "2026/04.journal"))
		if err != nil {
			t.Fatalf("read journal: %v", err)
		}
		s := string(data)
		if !strings.Contains(s, "@@ 1800.00 USD") && !strings.Contains(s, "@@ 1800 USD") {
			t.Errorf("journal missing updated total-cost annotation; got:\n%s", s)
		}
		if strings.Contains(s, "@ 175.00 USD") {
			t.Errorf("journal still contains old per-unit cost annotation; got:\n%s", s)
		}
	})
}

// TestRoundTripBalanceAssertion verifies the gRPC API contract for balance
// assertions: the `=` form round-trips, non-`=` variants are hidden from
// API responses but preserved on disk, and a wrong assertion rolls the
// write back via hledger check.
func TestRoundTripBalanceAssertion(t *testing.T) {
	// emptyDataDir sets up a starting journal with no transactions so the
	// running balance after a single posting is deterministic.
	emptyDataDir := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte("; float main journal\n"), 0644); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	t.Run("add_with_assertion_round_trips", func(t *testing.T) {
		dir := emptyDataDir(t)
		h := mustRealHandler(t, dir)

		resp, err := h.AddTransaction(t.Context(), connect.NewRequest(&floatv1.AddTransactionRequest{
			Description: "Open fresh account",
			Date:        "2026-04-30",
			Postings: []*floatv1.PostingInput{
				{
					Account: "assets:fresh", Commodity: "USD", Quantity: "100.00",
					BalanceAssertion: &floatv1.BalanceAssertion{
						Amount: &floatv1.Amount{Commodity: "USD", Quantity: "100.00"},
					},
				},
				{Account: "income:salary"},
			},
		}))
		if err != nil {
			t.Fatalf("AddTransaction: %v", err)
		}

		got := resp.Msg.Transaction.Postings[0]
		if got.BalanceAssertion == nil {
			t.Fatalf("BalanceAssertion is nil in response; postings=%+v", resp.Msg.Transaction.Postings)
		}
		if got.BalanceAssertion.Amount == nil {
			t.Fatalf("BalanceAssertion.Amount is nil")
		}
		if got.BalanceAssertion.Amount.Commodity != "USD" {
			t.Errorf("Amount.Commodity = %q, want %q", got.BalanceAssertion.Amount.Commodity, "USD")
		}
		if got.BalanceAssertion.Amount.Quantity != "100.00" {
			t.Errorf("Amount.Quantity = %q, want %q", got.BalanceAssertion.Amount.Quantity, "100.00")
		}

		data, err := os.ReadFile(filepath.Join(dir, "2026/04.journal"))
		if err != nil {
			t.Fatalf("read journal: %v", err)
		}
		if !strings.Contains(string(data), "= 100.00 USD") && !strings.Contains(string(data), "= USD 100.00") {
			t.Errorf("journal missing balance assertion syntax; got:\n%s", data)
		}
	})

	t.Run("update_clears_assertion_when_omitted", func(t *testing.T) {
		dir := emptyDataDir(t)
		h := mustRealHandler(t, dir)

		add, err := h.AddTransaction(t.Context(), connect.NewRequest(&floatv1.AddTransactionRequest{
			Description: "Initial",
			Date:        "2026-04-30",
			Postings: []*floatv1.PostingInput{
				{
					Account: "assets:fresh", Commodity: "USD", Quantity: "200.00",
					BalanceAssertion: &floatv1.BalanceAssertion{
						Amount: &floatv1.Amount{Commodity: "USD", Quantity: "200.00"},
					},
				},
				{Account: "income:salary"},
			},
		}))
		if err != nil {
			t.Fatalf("AddTransaction: %v", err)
		}
		fid := add.Msg.Transaction.Fid

		// Update without specifying balance_assertion: it should be cleared.
		upd, err := h.UpdateTransaction(t.Context(), connect.NewRequest(&floatv1.UpdateTransactionRequest{
			Fid:         fid,
			Description: "Updated",
			Date:        "2026-04-30",
			Postings: []*floatv1.PostingInput{
				{Account: "assets:fresh", Commodity: "USD", Quantity: "200.00"},
				{Account: "income:salary"},
			},
		}))
		if err != nil {
			t.Fatalf("UpdateTransaction: %v", err)
		}
		if upd.Msg.Transaction.Postings[0].BalanceAssertion != nil {
			t.Errorf("expected BalanceAssertion cleared after update, got %+v",
				upd.Msg.Transaction.Postings[0].BalanceAssertion)
		}

		data, err := os.ReadFile(filepath.Join(dir, "2026/04.journal"))
		if err != nil {
			t.Fatalf("read journal: %v", err)
		}
		if strings.Contains(string(data), "= 200.00 USD") {
			t.Errorf("journal still contains old assertion; got:\n%s", data)
		}
	})

	t.Run("failed_assertion_rolls_back_write", func(t *testing.T) {
		dir := emptyDataDir(t)
		h := mustRealHandler(t, dir)

		// Capture journal contents before the failing write.
		before, err := os.ReadFile(filepath.Join(dir, "main.journal"))
		if err != nil {
			t.Fatalf("read main.journal: %v", err)
		}

		// Asserted balance ($999) does not match the actual balance ($100)
		// after the posting; hledger check inside txlock must roll this back.
		_, err = h.AddTransaction(t.Context(), connect.NewRequest(&floatv1.AddTransactionRequest{
			Description: "Bad assertion",
			Date:        "2026-04-30",
			Postings: []*floatv1.PostingInput{
				{
					Account: "assets:fresh", Commodity: "USD", Quantity: "100.00",
					BalanceAssertion: &floatv1.BalanceAssertion{
						Amount: &floatv1.Amount{Commodity: "USD", Quantity: "999.00"},
					},
				},
				{Account: "income:salary"},
			},
		}))
		if err == nil {
			t.Fatal("expected error from failed balance assertion, got nil")
		}

		after, err := os.ReadFile(filepath.Join(dir, "main.journal"))
		if err != nil {
			t.Fatalf("read main.journal after rollback: %v", err)
		}
		if string(before) != string(after) {
			t.Errorf("main.journal changed despite failed assertion:\nbefore:\n%s\nafter:\n%s", before, after)
		}
		// Month file must not have been created (or must be empty if it was).
		monthPath := filepath.Join(dir, "2026/04.journal")
		if data, err := os.ReadFile(monthPath); err == nil && strings.Contains(string(data), "Bad assertion") {
			t.Errorf("month file contains failed transaction; got:\n%s", data)
		}
	})

	t.Run("non_simple_variants_hidden_from_proto_but_preserved_on_disk", func(t *testing.T) {
		// Hand-write a journal with =* (subaccount-inclusive) and verify:
		//   1. ListTransactions response does NOT include balance_assertion for
		//      that posting (only the simple = form is exposed).
		//   2. After a non-posting mutation (UpdateTransactionStatus),
		//      the =* line is still present in the journal.
		dir := t.TempDir()
		main := "; float main journal\ninclude 2026/01.journal\n"
		journal := `2026-01-15 (cc000003) deposit
    assets:fresh                $200.00 =* $200.00
    assets:other               $-200.00
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

		listResp, err := h.ListTransactions(t.Context(), connect.NewRequest(&floatv1.ListTransactionsRequest{}))
		if err != nil {
			t.Fatalf("ListTransactions: %v", err)
		}
		if len(listResp.Msg.Transactions) != 1 {
			t.Fatalf("expected 1 transaction, got %d", len(listResp.Msg.Transactions))
		}
		if ba := listResp.Msg.Transactions[0].Postings[0].BalanceAssertion; ba != nil {
			t.Errorf("=* assertion leaked into proto response: %+v", ba)
		}

		// UpdateTransactionStatus does not touch postings — the =* line
		// should still be in the journal afterward.
		_, err = h.UpdateTransactionStatus(t.Context(), connect.NewRequest(&floatv1.UpdateTransactionStatusRequest{
			Fid:    "cc000003",
			Status: "Cleared",
		}))
		if err != nil {
			t.Fatalf("UpdateTransactionStatus: %v", err)
		}

		data, err := os.ReadFile(filepath.Join(dir, "2026/01.journal"))
		if err != nil {
			t.Fatalf("read journal after status update: %v", err)
		}
		if !strings.Contains(string(data), "=*") {
			t.Errorf("=* assertion lost after UpdateTransactionStatus; journal:\n%s", data)
		}
	})
}

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
