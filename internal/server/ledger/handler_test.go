package ledger_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	serverledger "github.com/brendanv/float/internal/server/ledger"

	"github.com/brendanv/float/internal/cache"
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
	return serverledger.NewHandler(c, nil, "", nil)
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
	h := serverledger.NewHandler(c, nil, "", nil)

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
	return serverledger.NewHandler(c, lock, dir, nil)
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
				{Account: "expenses:food", Amount: "$12.00"},
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
				{Account: "expenses:food", Amount: "$18.00"},
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
				{Account: "expenses:food", Amount: "$10.00"},
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
				{Account: "expenses:food", Amount: "$10.00"},
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
				{Account: "expenses:food", Amount: "$10.00"},
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
				{Account: "", Amount: "$10.00"},
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
				{Account: "expenses:food", Amount: "$55.00"},
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
				{Account: "expenses:food", Amount: "$20.00"},
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
	return serverledger.NewHandler(c, nil, "", ch), ch
}

func TestListTransactions_HledgerError(t *testing.T) {
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", errorRunner(t))
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", nil)
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
	h := serverledger.NewHandler(c, nil, "", nil)
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
	h := serverledger.NewHandler(c, nil, "", nil)
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
	h := serverledger.NewHandler(c, nil, "", nil)
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
	h := serverledger.NewHandler(c, nil, "", nil)

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
	h := serverledger.NewHandler(c, nil, "", nil)

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
				{Account: "expenses:shopping", Amount: "$30.00"},
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
				{Account: "expenses:food", Amount: "$10.00"},
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
				{Account: "expenses:food", Amount: "$10.00"},
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
				{Account: "expenses:food", Amount: "$10.00"},
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
				{Account: "", Amount: "$10.00"},
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
				{Account: "expenses:food", Amount: "$10.00"},
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
				{Account: "expenses:food", Amount: "$10.00"},
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
				{Account: "expenses:food", Amount: "$10.00"},
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
				{Account: "expenses:food", Amount: "$20.00"},
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
				{Account: "expenses:shopping", Amount: "$55.00"},
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
				{Account: "expenses:food", Amount: "$10.00"},
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
				{Account: "expenses:food", Amount: "$10.00"},
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
	h := serverledger.NewHandler(c, nil, "", nil)
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

