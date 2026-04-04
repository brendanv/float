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
				{Account: "expenses:food", Amount: "$10.00"},
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
				{Account: "expenses:food", Amount: "$10.00"},
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
				{Account: "expenses:food", Amount: "$20.00"},
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
				{Account: "expenses:misc", Amount: "$1.00"},
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
				{Account: "expenses:food", Amount: "$5.00"},
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
				{Account: "expenses:food", Amount: "$15.00"},
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
				{Account: "expenses:food", Amount: "$10.00"},
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
				{Account: "expenses:food", Amount: "$99.00"},
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
				{Account: "expenses:food", Amount: "$10.00"},
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
				{Account: "expenses:food", Amount: "$10.00"},
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
				{Account: "expenses:food", Amount: "$25.00"},
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
				{Account: "expenses:food", Amount: "$25.00"},
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
			{Account: "expenses:misc", Amount: "$7.50"},
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
