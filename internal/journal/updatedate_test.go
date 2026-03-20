package journal

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/brendanv/float/internal/testgen"
)

func TestUpdateTransactionDate(t *testing.T) {
	t.Run("not_found", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 100, NumTxns: 3, WithFIDs: true})
		client := mustHledgerClient(t, dir)
		_, err := UpdateTransactionDate(t.Context(), client, dir, "00000000", "2026-03-01")
		if err == nil {
			t.Fatal("expected error for non-existent fid, got nil")
		}
		if !strings.Contains(err.Error(), "no transaction found") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("invalid_date", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 101, NumTxns: 2, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		tx := TransactionInput{
			Date:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			Description: "INVALID DATE TEST",
			Postings: []PostingInput{
				{Account: "expenses:food", Amount: "$10.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := AppendTransaction(t.Context(), client, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		_, err = UpdateTransactionDate(t.Context(), client, dir, fid, "not-a-date")
		if err == nil {
			t.Fatal("expected error for invalid date, got nil")
		}
		if !strings.Contains(err.Error(), "invalid date") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("same_month", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 102, NumTxns: 2, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		tx := TransactionInput{
			Date:        time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC),
			Description: "SAME MONTH TEST",
			Postings: []PostingInput{
				{Account: "expenses:food", Amount: "$20.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := AppendTransaction(t.Context(), client, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		updated, err := UpdateTransactionDate(t.Context(), client, dir, fid, "2026-02-20")
		if err != nil {
			t.Fatalf("UpdateTransactionDate: %v", err)
		}

		if updated.Date != "2026-02-20" {
			t.Errorf("Date = %q, want %q", updated.Date, "2026-02-20")
		}
		if updated.FID != fid {
			t.Errorf("FID = %q, want %q", updated.FID, fid)
		}

		if err := client.Check(t.Context()); err != nil {
			t.Fatalf("hledger check after same-month update: %v", err)
		}
	})

	t.Run("cross_month", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 103, NumTxns: 2, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		tx := TransactionInput{
			Date:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			Description: "CROSS MONTH TEST",
			Postings: []PostingInput{
				{Account: "expenses:shopping", Amount: "$50.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := AppendTransaction(t.Context(), client, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		updated, err := UpdateTransactionDate(t.Context(), client, dir, fid, "2026-03-05")
		if err != nil {
			t.Fatalf("UpdateTransactionDate: %v", err)
		}

		if updated.Date != "2026-03-05" {
			t.Errorf("Date = %q, want %q", updated.Date, "2026-03-05")
		}
		if updated.FID != fid {
			t.Errorf("FID = %q, want %q", updated.FID, fid)
		}

		// Verify new month file was created and included.
		newFile := dir + "/2026/03.journal"
		if _, err := os.Stat(newFile); os.IsNotExist(err) {
			t.Error("new month file 2026/03.journal not created")
		}

		mainData, err := os.ReadFile(dir + "/main.journal")
		if err != nil {
			t.Fatalf("read main.journal: %v", err)
		}
		if !strings.Contains(string(mainData), "include 2026/03.journal") {
			t.Error("main.journal missing include for 2026/03.journal")
		}

		if err := client.Check(t.Context()); err != nil {
			t.Fatalf("hledger check after cross-month update: %v", err)
		}
	})

	t.Run("preserves_postings", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 104, NumTxns: 1, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		tx := TransactionInput{
			Date:        time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			Description: "PRESERVE POSTINGS TEST",
			Postings: []PostingInput{
				{Account: "expenses:transport", Amount: "$15.00", Comment: "bus pass"},
				{Account: "assets:checking"},
			},
		}
		fid, err := AppendTransaction(t.Context(), client, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		updated, err := UpdateTransactionDate(t.Context(), client, dir, fid, "2026-02-15")
		if err != nil {
			t.Fatalf("UpdateTransactionDate: %v", err)
		}

		if len(updated.Postings) != 2 {
			t.Fatalf("expected 2 postings, got %d", len(updated.Postings))
		}
		if updated.Postings[0].Account != "expenses:transport" {
			t.Errorf("Posting[0].Account = %q, want %q", updated.Postings[0].Account, "expenses:transport")
		}
		if len(updated.Postings[0].Amounts) != 1 {
			t.Fatalf("expected 1 amount on first posting, got %d", len(updated.Postings[0].Amounts))
		}
		if updated.Postings[0].Amounts[0].Quantity.FloatingPoint != 15.0 {
			t.Errorf("Amount = %v, want 15.0", updated.Postings[0].Amounts[0].Quantity.FloatingPoint)
		}
	})

	t.Run("preserves_comment", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 105, NumTxns: 1, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		tx := TransactionInput{
			Date:        time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			Description: "PRESERVE COMMENT TEST",
			Comment:     "my note",
			Postings: []PostingInput{
				{Account: "expenses:misc", Amount: "$5.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := AppendTransaction(t.Context(), client, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		updated, err := UpdateTransactionDate(t.Context(), client, dir, fid, "2026-02-28")
		if err != nil {
			t.Fatalf("UpdateTransactionDate: %v", err)
		}

		if !strings.Contains(updated.Comment, "my note") {
			t.Errorf("Comment %q does not contain %q", updated.Comment, "my note")
		}
		if updated.FID != fid {
			t.Errorf("FID = %q, want %q", updated.FID, fid)
		}
	})
}
