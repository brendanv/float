package ledger_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	serverledger "github.com/brendanv/float/internal/server/ledger"

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
    "tcode": "",
    "tcomment": "fid:aa001100\n",
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
    "ttags": [["fid", "aa001100"]],
    "tsourcepos": [{"sourceName": "simple.journal", "sourceLine": 1, "sourceColumn": 1}, {"sourceName": "simple.journal", "sourceLine": 4, "sourceColumn": 1}]
  }
]`

const balJSON = `[[["assets:checking", "assets:checking", 0, [{"acommodity": "$", "acost": null, "aquantity": {"decimalMantissa": 700000, "decimalPlaces": 2, "floatingPoint": 7000}}]], ["income:salary", "income:salary", 0, [{"acommodity": "$", "acost": null, "aquantity": {"decimalMantissa": -700000, "decimalPlaces": 2, "floatingPoint": -7000}}]]], [{"acommodity": "$", "acost": null, "aquantity": {"decimalMantissa": 0, "decimalPlaces": 2, "floatingPoint": 0}}]]`

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
		txns, err := c.Transactions(t.Context(), "tag:fid="+fid)
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

		txns, err := c.Transactions(t.Context(), "tag:fid="+fid)
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
		if tagMap["fid"] != fid {
			t.Errorf("fid = %q, want %q", tagMap["fid"], fid)
		}
	})
}
