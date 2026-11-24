package ledger_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
	serverledger "github.com/brendanv/float/internal/server/ledger"
	"github.com/brendanv/float/internal/testgen"
)

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
			return []byte("hledger 1.99.6, linux-x86_64\n"), nil, nil
		}
		capturedArgs = args
		return []byte("[]"), nil, nil
	}
	c, err := hledger.NewWithRunner("hledger", "journal.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil, nil)

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

// TestListTransactionsWithQuery_SupportedVocabularyFiltersInGo verifies that
// a query entirely within txfilter's supported vocabulary (Phase 2.2) is
// served by fetching the full transaction list once and filtering in Go,
// rather than passing the query tokens to hledger.
func TestListTransactionsWithQuery_SupportedVocabularyFiltersInGo(t *testing.T) {
	var capturedArgs []string
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.99.6, linux-x86_64\n"), nil, nil
		}
		capturedArgs = args
		return []byte(printJSON), nil, nil
	}
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil, nil)

	resp, err := h.ListTransactions(t.Context(), connect.NewRequest(&floatv1.ListTransactionsRequest{
		Query: []string{"acct:checking"},
	}))
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}

	joined := strings.Join(capturedArgs, " ")
	if strings.Contains(joined, "acct:checking") {
		t.Errorf("args %v should not include the query token (expected a full-history fetch filtered in Go)", capturedArgs)
	}
	if len(resp.Msg.Transactions) != 1 {
		t.Fatalf("expected the Go filter to still match the fixture transaction, got %d", len(resp.Msg.Transactions))
	}
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

func TestListTransactions_HledgerError(t *testing.T) {
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", errorRunner(t))
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil, nil)
	_, err = h.ListTransactions(t.Context(), connect.NewRequest(&floatv1.ListTransactionsRequest{}))
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
			return []byte("hledger 1.99.6, linux-x86_64\n"), nil, nil
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
	t.Run("comment_and_tags_kept_unless_replaced", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 66, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		postings := []*floatv1.PostingInput{
			{Account: "expenses:food", Commodity: "USD", Quantity: "10.00", Comment: "split with Bob"},
			{Account: "assets:checking"},
		}
		added, err := h.AddTransaction(t.Context(), connect.NewRequest(&floatv1.AddTransactionRequest{
			Date:        "2026-01-10",
			Description: "Comment Keep",
			Comment:     "receipt in drawer",
			Tags:        map[string]string{"trip": "paris"},
			Postings:    postings,
		}))
		if err != nil {
			t.Fatalf("AddTransaction: %v", err)
		}
		fid := added.Msg.Transaction.Fid

		// What the web UI's inline edits send: no comment, no tags.
		resp, err := h.UpdateTransaction(t.Context(), connect.NewRequest(&floatv1.UpdateTransactionRequest{
			Fid: fid, Description: "Comment Keep 2", Postings: postings,
		}))
		if err != nil {
			t.Fatalf("UpdateTransaction (keep): %v", err)
		}
		got := resp.Msg.Transaction
		if !strings.Contains(got.Comment, "receipt in drawer") {
			t.Errorf("comment = %q, want it kept", got.Comment)
		}
		if got.Tags["trip"] != "paris" {
			t.Errorf("tags = %v, want trip:paris kept", got.Tags)
		}
		if c := got.Postings[0].Comment; !strings.Contains(c, "split with Bob") {
			t.Errorf("posting comment = %q, want it kept", c)
		}

		resp, err = h.UpdateTransaction(t.Context(), connect.NewRequest(&floatv1.UpdateTransactionRequest{
			Fid: fid, Description: "Comment Keep 3", Postings: postings,
			ReplaceComment: true, ReplaceTags: true,
		}))
		if err != nil {
			t.Fatalf("UpdateTransaction (clear): %v", err)
		}
		got = resp.Msg.Transaction
		if strings.Contains(got.Comment, "receipt in drawer") {
			t.Errorf("comment = %q, want it cleared", got.Comment)
		}
		if len(got.Tags) != 0 {
			t.Errorf("tags = %v, want none", got.Tags)
		}
	})

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

	t.Run("delete_rejected", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 215, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := h.BulkEditTransactions(t.Context(), connect.NewRequest(&floatv1.BulkEditTransactionsRequest{
			Fids: []string{"aa001100"},
			Operations: []*floatv1.BulkEditOperation{
				{Operation: &floatv1.BulkEditOperation_Delete{Delete: &floatv1.DeleteOperation{}}},
			},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
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

	t.Run("update_unknown_account_marks_reviewed", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 216, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}
		tx := journal.TransactionInput{
			Date:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			Description: "UPDATE UNKNOWN ACCOUNT",
			Postings: []journal.PostingInput{
				{Account: "expenses:unknown", Commodity: "USD", Quantity: "10.00"},
				{Account: "assets:checking"},
			},
		}
		fid := appendTx(t, c, dir, tx)

		resp, err := h.BulkEditTransactions(t.Context(), connect.NewRequest(&floatv1.BulkEditTransactionsRequest{
			Fids: []string{fid},
			Operations: []*floatv1.BulkEditOperation{
				{Operation: &floatv1.BulkEditOperation_UpdateUnknownAccount{UpdateUnknownAccount: &floatv1.UpdateUnknownAccountOperation{Account: "expenses:groceries"}}},
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
		if got.Postings[0].Account != "expenses:groceries" {
			t.Errorf("Postings[0].Account = %q, want %q", got.Postings[0].Account, "expenses:groceries")
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

	t.Run("clear_payee_without_payee_preserves_description", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 215, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}
		// Description has no "|", so there is no payee to clear.
		tx := baseTx("COFFEE SHOP 123")
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
		if got.Description != "COFFEE SHOP 123" {
			t.Errorf("Description = %q, want %q", got.Description, "COFFEE SHOP 123")
		}
		if got.Payee != nil {
			t.Errorf("Payee = %v, want nil", got.Payee)
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

func TestBulkDeleteTransactionsHandler(t *testing.T) {
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
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 230, NumTxns: 1, WithFIDs: true})
		h := mustRealHandler(t, dir)
		_, err := bulkDeleteTransactions(t, h, &floatv1.BulkDeleteTransactionsRequest{Fids: []string{}})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("not_found", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 231, NumTxns: 1, WithFIDs: true})
		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}
		fid := appendTx(t, c, dir, baseTx("EXISTING"))
		h := mustRealHandler(t, dir)
		_, err = bulkDeleteTransactions(t, h, &floatv1.BulkDeleteTransactionsRequest{
			Fids: []string{fid, "doesnotexist"},
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("code = %v, want NotFound", connect.CodeOf(err))
		}
	})

	t.Run("delete_multiple_transactions_single_snapshot", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 232, NumTxns: 1, WithFIDs: true})
		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}
		fid1 := appendTx(t, c, dir, baseTx("DELETE BULK 1"))
		fid2 := appendTx(t, c, dir, baseTx("DELETE BULK 2"))
		h, snap := mustRealHandlerWithSnap(t, dir)
		initialSnaps, err := snap.List(t.Context(), 20)
		if err != nil {
			t.Fatalf("List initial snapshots: %v", err)
		}

		result, err := bulkDeleteTransactions(t, h, &floatv1.BulkDeleteTransactionsRequest{
			Fids: []string{fid1, fid2},
		})
		if err != nil {
			t.Fatalf("BulkDeleteTransactions: %v", err)
		}
		if result == nil {
			t.Fatal("expected result, got nil")
		}
		if result.DeletedCount != 2 {
			t.Errorf("DeletedCount = %d, want 2", result.DeletedCount)
		}

		for _, fid := range []string{fid1, fid2} {
			txns, err := c.Transactions(t.Context(), "code:"+fid)
			if err != nil {
				t.Fatalf("lookup deleted transaction %s: %v", fid, err)
			}
			if len(txns) != 0 {
				t.Fatalf("transaction %s still exists after bulk delete", fid)
			}
		}

		afterSnaps, err := snap.List(t.Context(), 20)
		if err != nil {
			t.Fatalf("List snapshots after bulk delete: %v", err)
		}
		if len(afterSnaps) != len(initialSnaps)+1 {
			t.Errorf("snapshot count: got %d, want %d (one new commit)", len(afterSnaps), len(initialSnaps)+1)
		}
	})

	t.Run("delete_across_multiple_files", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 233, NumTxns: 1, WithFIDs: true})
		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}
		// Transactions in two different month files.
		tx1 := baseTx("MULTI FILE 1")
		tx2 := baseTx("MULTI FILE 2")
		tx2.Date = time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC)
		fid1 := appendTx(t, c, dir, tx1)
		fid2 := appendTx(t, c, dir, tx2)
		h := mustRealHandler(t, dir)

		result, err := bulkDeleteTransactions(t, h, &floatv1.BulkDeleteTransactionsRequest{
			Fids: []string{fid1, fid2},
		})
		if err != nil {
			t.Fatalf("BulkDeleteTransactions: %v", err)
		}
		if result.DeletedCount != 2 {
			t.Errorf("DeletedCount = %d, want 2", result.DeletedCount)
		}

		for _, fid := range []string{fid1, fid2} {
			txns, err := c.Transactions(t.Context(), "code:"+fid)
			if err != nil {
				t.Fatalf("lookup %s: %v", fid, err)
			}
			if len(txns) != 0 {
				t.Fatalf("transaction %s still exists after bulk delete", fid)
			}
		}
	})
}

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

func TestListTransactions_LotFields(t *testing.T) {
	h := mustLotsHandler(t)
	resp, err := h.ListTransactions(t.Context(), connect.NewRequest(&floatv1.ListTransactionsRequest{}))
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	byFID := map[string]*floatv1.Transaction{}
	for _, tx := range resp.Msg.Transactions {
		byFID[tx.Fid] = tx
	}

	tests := []struct {
		fid, account, role string
		generated          bool
		date, label        string
		costCommodity      string
		hasBasis           bool
	}{
		{fid: "aa000001", account: "assets:brokerage", role: "acquire", hasBasis: true, costCommodity: "$"},
		{fid: "aa000002", account: "assets:brokerage", role: "acquire", hasBasis: true, date: "2026-01-16", label: "mine", costCommodity: "$"},
		{fid: "aa000005", account: "assets:brokerage", role: "dispose", hasBasis: true, date: "2026-01-15", costCommodity: "$"},
		{fid: "aa000005", account: "revenues:gain", role: "gain", generated: true},
		// FIFO disposal spanning two lots: hledger reports an empty {} basis.
		{fid: "bb000003", account: "assets:ira", role: "dispose", hasBasis: true},
		{fid: "dd000001", account: "assets:ira", role: "transfer-to", hasBasis: true, date: "2026-01-15", costCommodity: "$"},
		{fid: "dd000001", account: "assets:brokerage", role: "transfer-from", hasBasis: true, date: "2026-01-15", costCommodity: "$"},
	}
	for _, tt := range tests {
		t.Run(tt.fid+"/"+tt.account, func(t *testing.T) {
			tx, ok := byFID[tt.fid]
			if !ok {
				t.Fatalf("transaction %q not found", tt.fid)
			}
			var p *floatv1.Posting
			for _, cand := range tx.Postings {
				if cand.Account == tt.account {
					p = cand
				}
			}
			if p == nil {
				t.Fatalf("posting %q not found", tt.account)
			}
			if p.LotRole != tt.role {
				t.Errorf("LotRole = %q, want %q", p.LotRole, tt.role)
			}
			if p.Generated != tt.generated {
				t.Errorf("Generated = %v, want %v", p.Generated, tt.generated)
			}
			cb := p.Amounts[0].CostBasis
			if (cb != nil) != tt.hasBasis {
				t.Fatalf("CostBasis = %v, hasBasis want %v", cb, tt.hasBasis)
			}
			if cb == nil {
				return
			}
			if cb.Date != tt.date || cb.Label != tt.label {
				t.Errorf("CostBasis date/label = %q/%q, want %q/%q", cb.Date, cb.Label, tt.date, tt.label)
			}
			if (cb.Cost != nil) != (tt.costCommodity != "") {
				t.Fatalf("CostBasis.Cost = %v, want commodity %q", cb.Cost, tt.costCommodity)
			}
			if cb.Cost != nil && cb.Cost.Commodity != tt.costCommodity {
				t.Errorf("CostBasis.Cost commodity = %q, want %q", cb.Cost.Commodity, tt.costCommodity)
			}
		})
	}
}
