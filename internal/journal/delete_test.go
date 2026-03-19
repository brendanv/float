package journal

import (
	"strings"
	"testing"
	"time"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/testgen"
)

func TestDeleteTransaction(t *testing.T) {
	t.Run("not_found", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 1, NumTxns: 3, WithFIDs: true})
		err := DeleteTransaction(dir, "00000000")
		if err == nil {
			t.Fatal("expected error for non-existent fid, got nil")
		}
		if !strings.Contains(err.Error(), "no transaction found") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("removes_transaction", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 2, NumTxns: 3, WithFIDs: true})

		client, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Fatalf("hledger.New: %v", err)
		}

		// Add a transaction so we have a known fid.
		tx := TransactionInput{
			Date:        time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			Description: "TEST DELETE",
			Postings: []PostingInput{
				{Account: "expenses:food", Amount: "$10.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := AppendTransaction(t.Context(), client, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		// Verify it's present.
		rows, err := client.Register(t.Context(), "tag:fid="+fid)
		if err != nil {
			t.Fatalf("Register after add: %v", err)
		}
		if len(rows) == 0 {
			t.Fatal("transaction not found after add")
		}

		// Delete it.
		if err := DeleteTransaction(dir, fid); err != nil {
			t.Fatalf("DeleteTransaction: %v", err)
		}

		// Verify journal still passes hledger check.
		if err := client.Check(t.Context()); err != nil {
			t.Fatalf("hledger check after delete: %v", err)
		}

		// Verify it's gone.
		rows, err = client.Register(t.Context(), "tag:fid="+fid)
		if err != nil {
			t.Fatalf("Register after delete: %v", err)
		}
		if len(rows) != 0 {
			t.Errorf("transaction still present after delete, got %d rows", len(rows))
		}
	})

	t.Run("idempotent_after_delete", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 3, NumTxns: 2, WithFIDs: true})

		client, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Fatalf("hledger.New: %v", err)
		}

		tx := TransactionInput{
			Date:        time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
			Description: "IDEMPOTENT TEST",
			Postings: []PostingInput{
				{Account: "expenses:shopping", Amount: "$5.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := AppendTransaction(t.Context(), client, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		if err := DeleteTransaction(dir, fid); err != nil {
			t.Fatalf("first DeleteTransaction: %v", err)
		}

		// Second delete should return "not found".
		err = DeleteTransaction(dir, fid)
		if err == nil {
			t.Fatal("expected error on second delete, got nil")
		}
		if !strings.Contains(err.Error(), "no transaction found") {
			t.Errorf("unexpected error on second delete: %v", err)
		}
	})
}
