package journal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/brendanv/float/internal/hledger"
)

// setupWriteDir creates a minimal data directory with main.journal for WriteTransaction tests.
func setupWriteDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte("; float main journal\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestWriteTransaction_New(t *testing.T) {
	t.Run("mints_fid_when_empty", func(t *testing.T) {
		dir := setupWriteDir(t)
		c := mustHledgerClient(t, dir)
		tx := TransactionInput{
			Date:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			Description: "MINT FID TEST",
			Postings: []PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "10.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := WriteTransaction(t.Context(), c, dir, tx, nil)
		if err != nil {
			t.Fatalf("WriteTransaction: %v", err)
		}
		if len(fid) != hledger.FIDLen {
			t.Errorf("fid length = %d, want %d; got %q", len(fid), hledger.FIDLen, fid)
		}
	})

	t.Run("preserves_provided_fid", func(t *testing.T) {
		dir := setupWriteDir(t)
		c := mustHledgerClient(t, dir)
		tx := TransactionInput{
			Date:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			Description: "PRESET FID TEST",
			FID:         "aabbccdd",
			Postings: []PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "10.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := WriteTransaction(t.Context(), c, dir, tx, nil)
		if err != nil {
			t.Fatalf("WriteTransaction: %v", err)
		}
		if fid != "aabbccdd" {
			t.Errorf("fid = %q, want %q", fid, "aabbccdd")
		}
	})

	t.Run("appends_to_correct_month_file", func(t *testing.T) {
		dir := setupWriteDir(t)
		c := mustHledgerClient(t, dir)
		tx := TransactionInput{
			Date:        time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
			Description: "MARCH TXN",
			Postings: []PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "20.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := WriteTransaction(t.Context(), c, dir, tx, nil)
		if err != nil {
			t.Fatalf("WriteTransaction: %v", err)
		}

		data, err := os.ReadFile(filepath.Join(dir, "2026/03.journal"))
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "("+fid+")") {
			t.Errorf("fid not found in 2026/03.journal:\n%s", content)
		}
		if !strings.Contains(content, "MARCH TXN") {
			t.Errorf("description not found in 2026/03.journal:\n%s", content)
		}
	})

	t.Run("updates_main_journal_includes", func(t *testing.T) {
		dir := setupWriteDir(t)
		c := mustHledgerClient(t, dir)
		tx := TransactionInput{
			Date:        time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
			Description: "MAY TXN",
			Postings: []PostingInput{
				{Account: "expenses:misc", Commodity: "USD", Quantity: "1.00"},
				{Account: "assets:checking"},
			},
		}
		if _, err := WriteTransaction(t.Context(), c, dir, tx, nil); err != nil {
			t.Fatalf("WriteTransaction: %v", err)
		}
		main, err := os.ReadFile(filepath.Join(dir, "main.journal"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(main), "include 2026/05.journal") {
			t.Errorf("main.journal missing include directive:\n%s", main)
		}
	})

	t.Run("stamps_updated_at", func(t *testing.T) {
		dir := setupWriteDir(t)
		c := mustHledgerClient(t, dir)
		tx := TransactionInput{
			Date:        time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
			Description: "TIMESTAMP TEST",
			Postings: []PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "5.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := WriteTransaction(t.Context(), c, dir, tx, nil)
		if err != nil {
			t.Fatalf("WriteTransaction: %v", err)
		}
		txns, err := c.Transactions(t.Context(), "code:"+fid)
		if err != nil {
			t.Fatalf("Transactions: %v", err)
		}
		if len(txns) != 1 {
			t.Fatalf("expected 1 transaction, got %d", len(txns))
		}
		if txns[0].FloatMeta["float-updated-at"] == "" {
			t.Error("float-updated-at not stamped on new transaction")
		}
	})

	t.Run("writes_tags", func(t *testing.T) {
		dir := setupWriteDir(t)
		c := mustHledgerClient(t, dir)
		tx := TransactionInput{
			Date:        time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			Description: "TAGS TEST",
			Tags:        map[string]string{"category": "food", "source": "manual"},
			Postings: []PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "15.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := WriteTransaction(t.Context(), c, dir, tx, nil)
		if err != nil {
			t.Fatalf("WriteTransaction: %v", err)
		}
		txns, err := c.Transactions(t.Context(), "code:"+fid)
		if err != nil {
			t.Fatalf("Transactions: %v", err)
		}
		if len(txns) != 1 {
			t.Fatalf("expected 1 transaction, got %d", len(txns))
		}
		tagMap := make(map[string]string)
		for _, tag := range txns[0].Tags {
			tagMap[tag[0]] = tag[1]
		}
		if tagMap["category"] != "food" {
			t.Errorf("category = %q, want %q", tagMap["category"], "food")
		}
		if tagMap["source"] != "manual" {
			t.Errorf("source = %q, want %q", tagMap["source"], "manual")
		}
	})
}

func TestWriteTransaction_Replace(t *testing.T) {
	t.Run("replace_same_month", func(t *testing.T) {
		dir := setupWriteDir(t)
		c := mustHledgerClient(t, dir)

		// Write original transaction.
		original := TransactionInput{
			Date:        time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
			Description: "ORIGINAL",
			Postings: []PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "10.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := WriteTransaction(t.Context(), c, dir, original, nil)
		if err != nil {
			t.Fatalf("WriteTransaction new: %v", err)
		}

		// Look up source location.
		txns, err := c.Transactions(t.Context(), "code:"+fid)
		if err != nil {
			t.Fatalf("Transactions lookup: %v", err)
		}
		if len(txns) != 1 {
			t.Fatalf("expected 1 transaction, got %d", len(txns))
		}
		src := &SourceLocation{
			File: txns[0].SourcePos[0].File,
			Line: txns[0].SourcePos[0].Line,
		}

		// Replace with updated description, same month.
		updated := TransactionInput{
			Date:        time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
			Description: "UPDATED SAME MONTH",
			FID:         fid,
			Postings: []PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "99.00"},
				{Account: "assets:checking"},
			},
		}
		if _, err := WriteTransaction(t.Context(), c, dir, updated, src); err != nil {
			t.Fatalf("WriteTransaction replace: %v", err)
		}

		// Verify old description is gone and new is present.
		result, err := c.Transactions(t.Context(), "code:"+fid)
		if err != nil {
			t.Fatalf("Transactions after replace: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("expected 1 transaction after replace, got %d", len(result))
		}
		if result[0].Description != "UPDATED SAME MONTH" {
			t.Errorf("description = %q, want %q", result[0].Description, "UPDATED SAME MONTH")
		}
		// Original description must not appear in the file.
		data, err := os.ReadFile(src.File)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "ORIGINAL") {
			t.Errorf("old description still present in file:\n%s", data)
		}
	})

	t.Run("replace_cross_month", func(t *testing.T) {
		dir := setupWriteDir(t)
		c := mustHledgerClient(t, dir)

		// Write in January.
		original := TransactionInput{
			Date:        time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC),
			Description: "JANUARY TXN",
			Postings: []PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "10.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := WriteTransaction(t.Context(), c, dir, original, nil)
		if err != nil {
			t.Fatalf("WriteTransaction new: %v", err)
		}

		// Look up source location (will be in 2026/01.journal).
		txns, err := c.Transactions(t.Context(), "code:"+fid)
		if err != nil {
			t.Fatalf("Transactions: %v", err)
		}
		src := &SourceLocation{
			File: txns[0].SourcePos[0].File,
			Line: txns[0].SourcePos[0].Line,
		}

		// Replace with February date → should move to 2026/02.journal.
		moved := TransactionInput{
			Date:        time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
			Description: "MOVED TO FEBRUARY",
			FID:         fid,
			Postings: []PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "10.00"},
				{Account: "assets:checking"},
			},
		}
		if _, err := WriteTransaction(t.Context(), c, dir, moved, src); err != nil {
			t.Fatalf("WriteTransaction replace cross-month: %v", err)
		}

		if err := c.Check(t.Context()); err != nil {
			t.Fatalf("hledger check: %v", err)
		}

		// Transaction should now be in February.
		result, err := c.Transactions(t.Context(), "code:"+fid)
		if err != nil {
			t.Fatalf("Transactions after cross-month replace: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("expected 1 transaction, got %d", len(result))
		}
		if result[0].Date != "2026-02-05" {
			t.Errorf("date = %q, want %q", result[0].Date, "2026-02-05")
		}
		if !strings.HasSuffix(result[0].SourcePos[0].File, "2026/02.journal") {
			t.Errorf("source file = %q, want to end with 2026/02.journal", result[0].SourcePos[0].File)
		}

		// January file should not contain the transaction.
		jan, err := os.ReadFile(filepath.Join(dir, "2026/01.journal"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(jan), "("+fid+")") {
			t.Errorf("fid still found in January file after cross-month move:\n%s", jan)
		}
	})

	t.Run("tags_roundtrip", func(t *testing.T) {
		dir := setupWriteDir(t)
		c := mustHledgerClient(t, dir)

		// Write with tags.
		tx := TransactionInput{
			Date:        time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC),
			Description: "TAG ROUNDTRIP",
			Tags:        map[string]string{"category": "food"},
			FloatMeta:   map[string]string{"float-import-id": "batch1"},
			Postings: []PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "25.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := WriteTransaction(t.Context(), c, dir, tx, nil)
		if err != nil {
			t.Fatalf("WriteTransaction new: %v", err)
		}

		// Look up source and replace with different tags.
		txns, err := c.Transactions(t.Context(), "code:"+fid)
		if err != nil {
			t.Fatalf("Transactions: %v", err)
		}
		src := &SourceLocation{
			File: txns[0].SourcePos[0].File,
			Line: txns[0].SourcePos[0].Line,
		}

		updated := TransactionInput{
			Date:        time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC),
			Description: "TAG ROUNDTRIP",
			FID:         fid,
			Tags:        map[string]string{"category": "groceries"},
			FloatMeta:   map[string]string{"float-import-id": "batch1"},
			Postings: []PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "25.00"},
				{Account: "assets:checking"},
			},
		}
		if _, err := WriteTransaction(t.Context(), c, dir, updated, src); err != nil {
			t.Fatalf("WriteTransaction replace: %v", err)
		}

		result, err := c.Transactions(t.Context(), "code:"+fid)
		if err != nil {
			t.Fatalf("Transactions after replace: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("expected 1 transaction, got %d", len(result))
		}
		tagMap := make(map[string]string)
		for _, tag := range result[0].Tags {
			tagMap[tag[0]] = tag[1]
		}
		if tagMap["category"] != "groceries" {
			t.Errorf("category = %q, want %q", tagMap["category"], "groceries")
		}
		if tagMap["float-import-id"] != "batch1" {
			t.Errorf("float-import-id = %q, want %q", tagMap["float-import-id"], "batch1")
		}
	})
}

func TestInputFromTransaction(t *testing.T) {
	dir := setupWriteDir(t)
	c := mustHledgerClient(t, dir)

	// Write a full transaction and round-trip through InputFromTransaction.
	tx := TransactionInput{
		Date:        time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC),
		Description: "ROUNDTRIP TEST",
		Comment:     "free text note",
		Tags:        map[string]string{"category": "misc"},
		Status:      "Pending",
		FloatMeta:   map[string]string{"float-import-id": "batchX"},
		Postings: []PostingInput{
			{Account: "expenses:misc", Commodity: "USD", Quantity: "7.50"},
			{Account: "assets:checking"},
		},
	}
	fid, err := WriteTransaction(t.Context(), c, dir, tx, nil)
	if err != nil {
		t.Fatalf("WriteTransaction: %v", err)
	}

	txns, err := c.Transactions(t.Context(), "code:"+fid)
	if err != nil {
		t.Fatalf("Transactions: %v", err)
	}
	if len(txns) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(txns))
	}

	input, err := InputFromTransaction(txns[0])
	if err != nil {
		t.Fatalf("InputFromTransaction: %v", err)
	}

	if input.Description != "ROUNDTRIP TEST" {
		t.Errorf("description = %q, want %q", input.Description, "ROUNDTRIP TEST")
	}
	if input.Comment != "free text note" {
		t.Errorf("comment = %q, want %q", input.Comment, "free text note")
	}
	if input.Status != "Pending" {
		t.Errorf("status = %q, want %q", input.Status, "Pending")
	}
	if input.Tags["category"] != "misc" {
		t.Errorf("tags[category] = %q, want %q", input.Tags["category"], "misc")
	}
	if input.FloatMeta["float-import-id"] != "batchX" {
		t.Errorf("FloatMeta[float-import-id] = %q, want %q", input.FloatMeta["float-import-id"], "batchX")
	}
	if input.FID != fid {
		t.Errorf("FID = %q, want %q", input.FID, fid)
	}
}

func TestWriteTransaction_BalanceAssertion(t *testing.T) {
	t.Run("simple_=_round_trip", func(t *testing.T) {
		dir := setupWriteDir(t)
		c := mustHledgerClient(t, dir)
		// Empty journal: posting $100 to assets:checking yields a balance of $100,
		// so assert that exact value to satisfy hledger check.
		tx := TransactionInput{
			Date:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			Description: "ASSERT TEST",
			Postings: []PostingInput{
				{
					Account: "assets:checking", Commodity: "$", Quantity: "100.00",
					BalanceAssertion: &BalanceAssertionInput{Commodity: "$", Quantity: "100.00"},
				},
				{Account: "income:salary"},
			},
		}
		fid, err := WriteTransaction(t.Context(), c, dir, tx, nil)
		if err != nil {
			t.Fatalf("WriteTransaction: %v", err)
		}
		txns, err := c.Transactions(t.Context(), "code:"+fid)
		if err != nil || len(txns) != 1 {
			t.Fatalf("re-fetch: txns=%d err=%v", len(txns), err)
		}
		ba := txns[0].Postings[0].BalanceAssertion
		if ba == nil {
			t.Fatal("BalanceAssertion is nil after round-trip")
		}
		if ba.Inclusive || ba.Total {
			t.Errorf("flags Inclusive=%v Total=%v, want both false", ba.Inclusive, ba.Total)
		}
		if ba.Amount.Quantity.FloatingPoint != 100.00 {
			t.Errorf("Quantity = %v, want 100.00", ba.Amount.Quantity.FloatingPoint)
		}
	})

	t.Run("inclusive_=*_preserved_via_input_roundtrip", func(t *testing.T) {
		// Simulate the flow used by ModifyTags / UpdateTransactionStatus etc.:
		// write a transaction with =*, fetch it, run InputFromTransaction,
		// then re-write it. The =* must survive untouched.
		dir := setupWriteDir(t)
		c := mustHledgerClient(t, dir)
		tx := TransactionInput{
			Date:        time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC),
			Description: "INCLUSIVE ASSERT",
			Postings: []PostingInput{
				{
					Account: "assets:savings", Commodity: "$", Quantity: "200.00",
					BalanceAssertion: &BalanceAssertionInput{Commodity: "$", Quantity: "200.00", Inclusive: true},
				},
				{Account: "assets:checking"},
			},
		}
		fid, err := WriteTransaction(t.Context(), c, dir, tx, nil)
		if err != nil {
			t.Fatalf("WriteTransaction: %v", err)
		}
		txns, err := c.Transactions(t.Context(), "code:"+fid)
		if err != nil || len(txns) != 1 {
			t.Fatalf("re-fetch: txns=%d err=%v", len(txns), err)
		}
		input, err := InputFromTransaction(txns[0])
		if err != nil {
			t.Fatalf("InputFromTransaction: %v", err)
		}
		ba := input.Postings[0].BalanceAssertion
		if ba == nil {
			t.Fatal("BalanceAssertion lost in InputFromTransaction")
		}
		if !ba.Inclusive {
			t.Errorf("Inclusive flag lost: got %+v", ba)
		}
	})

	t.Run("replace_with_later_transaction_keeps_assertion_valid", func(t *testing.T) {
		// Regression: replacing a transaction that has a balance assertion and is
		// NOT the last transaction in the file used to move it to the end of the
		// file, changing the running balance at that position and causing hledger
		// to reject the journal with "Balance assertion failed".
		dir := setupWriteDir(t)
		c := mustHledgerClient(t, dir)

		// First transaction: $100 deposit, assert running balance = $100.
		txFirst := TransactionInput{
			Date:        time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
			Description: "DEPOSIT",
			Postings: []PostingInput{
				{
					Account: "assets:checking", Commodity: "$", Quantity: "100.00",
					BalanceAssertion: &BalanceAssertionInput{Commodity: "$", Quantity: "100.00"},
				},
				{Account: "income:salary"},
			},
		}
		fidFirst, err := WriteTransaction(t.Context(), c, dir, txFirst, nil)
		if err != nil {
			t.Fatalf("WriteTransaction first: %v", err)
		}

		// Second transaction: $50 expense after the deposit.
		txSecond := TransactionInput{
			Date:        time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC),
			Description: "EXPENSE",
			Postings: []PostingInput{
				{Account: "expenses:food", Commodity: "$", Quantity: "50.00"},
				{Account: "assets:checking"},
			},
		}
		if _, err := WriteTransaction(t.Context(), c, dir, txSecond, nil); err != nil {
			t.Fatalf("WriteTransaction second: %v", err)
		}

		// Now simulate UpdateTransactionStatus on the first transaction (it has a
		// balance assertion and is followed by another transaction in the file).
		txns, err := c.Transactions(t.Context(), "code:"+fidFirst)
		if err != nil || len(txns) != 1 {
			t.Fatalf("re-fetch first: txns=%d err=%v", len(txns), err)
		}
		input, err := InputFromTransaction(txns[0])
		if err != nil {
			t.Fatalf("InputFromTransaction: %v", err)
		}
		input.Status = "Cleared"
		src := &SourceLocation{File: txns[0].SourcePos[0].File, Line: txns[0].SourcePos[0].Line}

		// This must not fail with "Balance assertion failed".
		if _, err := WriteTransaction(t.Context(), c, dir, input, src); err != nil {
			t.Fatalf("WriteTransaction replace: %v", err)
		}

		// Sanity-check: hledger can still read the journal without errors.
		result, err := c.Transactions(t.Context(), "")
		if err != nil {
			t.Fatalf("Transactions after replace: %v", err)
		}
		if len(result) != 2 {
			t.Errorf("expected 2 transactions after replace, got %d", len(result))
		}
	})
}

func TestTransactionEndIndex(t *testing.T) {
	tests := []struct {
		name      string
		lines     []string
		headerIdx int
		want      int
	}{
		{
			name: "block_with_trailing_blank",
			lines: []string{
				"2026-01-15 (aaaaaaaa) ONE",
				"    expenses:food  $10.00",
				"    assets:checking",
				"",
				"2026-01-16 (bbbbbbbb) TWO",
			},
			headerIdx: 0,
			want:      4,
		},
		{
			name: "adjacent_transaction_without_blank_separator",
			lines: []string{
				"2026-01-15 (aaaaaaaa) ONE",
				"    expenses:food  $10.00",
				"    assets:checking",
				"2026-01-16 (bbbbbbbb) TWO",
				"    expenses:rent  $20.00",
				"    assets:checking",
			},
			headerIdx: 0,
			want:      3,
		},
		{
			name: "adjacent_directive_without_blank_separator",
			lines: []string{
				"2026-01-15 (aaaaaaaa) ONE",
				"    expenses:food  $10.00",
				"    assets:checking",
				"P 2026-01-16 AAPL $150.00",
			},
			headerIdx: 0,
			want:      3,
		},
		{
			name: "block_at_end_of_file",
			lines: []string{
				"2026-01-15 (aaaaaaaa) ONE",
				"    expenses:food  $10.00",
				"    assets:checking",
			},
			headerIdx: 0,
			want:      3,
		},
		{
			name: "indented_comment_belongs_to_block",
			lines: []string{
				"2026-01-15 (aaaaaaaa) ONE",
				"    ; note: indented comment",
				"    expenses:food  $10.00",
				"    assets:checking",
				"",
			},
			headerIdx: 0,
			want:      5,
		},
		{
			name: "top_level_comment_not_consumed",
			lines: []string{
				"2026-01-15 (aaaaaaaa) ONE",
				"    expenses:food  $10.00",
				"    assets:checking",
				"; top-level file comment",
			},
			headerIdx: 0,
			want:      3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := transactionEndIndex(tt.lines, tt.headerIdx); got != tt.want {
				t.Errorf("transactionEndIndex = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestRemoveTransactionAtLine_AdjacentTransactions guards against the
// boundary-scan bug where deleting a transaction that is not followed by a
// blank line also consumed the next transaction (or directive).
func TestRemoveTransactionAtLine_AdjacentTransactions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "01.journal")
	content := "2026-01-15 (aaaaaaaa) FIRST\n" +
		"    expenses:food  $10.00\n" +
		"    assets:checking\n" +
		"2026-01-16 (bbbbbbbb) SECOND\n" +
		"    expenses:rent  $20.00\n" +
		"    assets:checking\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	if err := removeTransactionAtLine(path, 1, "aaaaaaaa"); err != nil {
		t.Fatalf("removeTransactionAtLine: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.Contains(got, "FIRST") {
		t.Errorf("removed transaction still present:\n%s", got)
	}
	if !strings.Contains(got, "SECOND") || !strings.Contains(got, "expenses:rent") {
		t.Errorf("adjacent transaction was destroyed by the removal:\n%s", got)
	}
}

func TestReplaceTransactionAtLine_AdjacentTransactions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "01.journal")
	content := "2026-01-15 (aaaaaaaa) FIRST\n" +
		"    expenses:food  $10.00\n" +
		"    assets:checking\n" +
		"2026-01-16 (bbbbbbbb) SECOND\n" +
		"    expenses:rent  $20.00\n" +
		"    assets:checking\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	newText := "2026-01-15 (aaaaaaaa) FIRST EDITED\n" +
		"    expenses:food  $15.00\n" +
		"    assets:checking\n\n"
	if err := replaceTransactionAtLine(path, 1, "aaaaaaaa", newText); err != nil {
		t.Fatalf("replaceTransactionAtLine: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "FIRST EDITED") {
		t.Errorf("replacement text missing:\n%s", got)
	}
	if !strings.Contains(got, "SECOND") || !strings.Contains(got, "expenses:rent") {
		t.Errorf("adjacent transaction was destroyed by the replacement:\n%s", got)
	}
}

func TestBatchHelpers_AdjacentTransactions(t *testing.T) {
	mkFile := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		path := filepath.Join(dir, "01.journal")
		content := "2026-01-15 (aaaaaaaa) FIRST\n" +
			"    expenses:food  $10.00\n" +
			"    assets:checking\n" +
			"2026-01-16 (bbbbbbbb) SECOND\n" +
			"    expenses:rent  $20.00\n" +
			"    assets:checking\n"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("batch_remove", func(t *testing.T) {
		path := mkFile(t)
		if err := batchRemoveFromFile(path, []DeleteSpec{{HeaderLine: 1, FID: "aaaaaaaa"}}); err != nil {
			t.Fatalf("batchRemoveFromFile: %v", err)
		}
		data, _ := os.ReadFile(path)
		if !strings.Contains(string(data), "SECOND") {
			t.Errorf("adjacent transaction was destroyed by batch removal:\n%s", data)
		}
	})

	t.Run("batch_replace", func(t *testing.T) {
		path := mkFile(t)
		newText := "2026-01-15 (aaaaaaaa) FIRST EDITED\n" +
			"    expenses:food  $15.00\n" +
			"    assets:checking\n\n"
		if err := BatchReplaceTransactions(path, []BatchReplacement{{HeaderLine: 1, FID: "aaaaaaaa", NewText: newText}}); err != nil {
			t.Fatalf("BatchReplaceTransactions: %v", err)
		}
		data, _ := os.ReadFile(path)
		got := string(data)
		if !strings.Contains(got, "FIRST EDITED") {
			t.Errorf("replacement text missing:\n%s", got)
		}
		if !strings.Contains(got, "SECOND") {
			t.Errorf("adjacent transaction was destroyed by batch replacement:\n%s", got)
		}
	})
}
