package journal

import (
	"strings"
	"testing"
	"time"

	"github.com/brendanv/float/internal/testgen"
)

func TestUpdateTransactionStatus(t *testing.T) {
	t.Run("invalid_status", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 10, NumTxns: 2, WithFIDs: true})
		client := mustHledgerClient(t, dir)
		err := UpdateTransactionStatus(t.Context(), client, dir, "00000000", "Invalid")
		if err == nil {
			t.Fatal("expected error for invalid status, got nil")
		}
		if !strings.Contains(err.Error(), "invalid status") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 11, NumTxns: 2, WithFIDs: true})
		client := mustHledgerClient(t, dir)
		err := UpdateTransactionStatus(t.Context(), client, dir, "00000000", "Cleared")
		if err == nil {
			t.Fatal("expected error for non-existent fid, got nil")
		}
		if !strings.Contains(err.Error(), "no transaction found") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	tests := []struct {
		name       string
		initial    string // initial status for the new transaction ("", "Pending", "Cleared")
		newStatus  string
		wantStatus string
	}{
		{"unmarked_to_pending", "", "Pending", "Pending"},
		{"unmarked_to_cleared", "", "Cleared", "Cleared"},
		{"pending_to_cleared", "Pending", "Cleared", "Cleared"},
		{"cleared_to_pending", "Cleared", "Pending", "Pending"},
		{"pending_to_unmarked", "Pending", "", "Unmarked"},
		{"cleared_to_unmarked", "Cleared", "", "Unmarked"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 20, NumTxns: 1, WithFIDs: true})
			client := mustHledgerClient(t, dir)

			tx := TransactionInput{
				Date:        time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
				Description: "STATUS TEST",
				Status:      tc.initial,
				Postings: []PostingInput{
					{Account: "expenses:food", Amount: "$10.00"},
					{Account: "assets:checking"},
				},
			}
			fid, err := AppendTransaction(t.Context(), client, dir, tx)
			if err != nil {
				t.Fatalf("AppendTransaction: %v", err)
			}

			if err := UpdateTransactionStatus(t.Context(), client, dir, fid, tc.newStatus); err != nil {
				t.Fatalf("UpdateTransactionStatus: %v", err)
			}

			// Verify journal still passes hledger check.
			if err := client.Check(t.Context()); err != nil {
				t.Fatalf("hledger check after status update: %v", err)
			}

			// Re-fetch and verify status.
			txns, err := client.Transactions(t.Context(), "tag:fid="+fid)
			if err != nil {
				t.Fatalf("Transactions after update: %v", err)
			}
			if len(txns) != 1 {
				t.Fatalf("expected 1 transaction, got %d", len(txns))
			}
			// hledger uses "Unmarked" for no marker; our UpdateTransactionStatus
			// writes "" which hledger reads back as "Unmarked".
			if txns[0].Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", txns[0].Status, tc.wantStatus)
			}
		})
	}

	t.Run("description_preserved", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 30, NumTxns: 1, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		tx := TransactionInput{
			Date:        time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC),
			Description: "PRESERVE ME",
			Comment:     "important note",
			Status:      "Pending",
			Postings: []PostingInput{
				{Account: "expenses:food", Amount: "$5.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := AppendTransaction(t.Context(), client, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		if err := UpdateTransactionStatus(t.Context(), client, dir, fid, "Cleared"); err != nil {
			t.Fatalf("UpdateTransactionStatus: %v", err)
		}

		txns, err := client.Transactions(t.Context(), "tag:fid="+fid)
		if err != nil {
			t.Fatalf("Transactions after update: %v", err)
		}
		if len(txns) != 1 {
			t.Fatalf("expected 1 transaction, got %d", len(txns))
		}
		got := txns[0]
		if got.Description != "PRESERVE ME" {
			t.Errorf("description = %q, want %q", got.Description, "PRESERVE ME")
		}
		if got.Status != "Cleared" {
			t.Errorf("status = %q, want Cleared", got.Status)
		}
		if got.FID != fid {
			t.Errorf("fid = %q, want %q", got.FID, fid)
		}
	})
}
