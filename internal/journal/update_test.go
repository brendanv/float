package journal

import (
	"strings"
	"testing"
	"time"

	"github.com/brendanv/float/internal/testgen"
)

func TestUpdateTransaction(t *testing.T) {
	t.Run("not_found", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 110, NumTxns: 3, WithFIDs: true})
		client := mustHledgerClient(t, dir)
		_, err := UpdateTransaction(t.Context(), client, dir, "00000000", "DESCRIPTION", "", "", nil)
		if err == nil {
			t.Fatal("expected error for non-existent fid, got nil")
		}
		if !strings.Contains(err.Error(), "no transaction found") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("invalid_date", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 111, NumTxns: 2, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		fid, err := AppendTransaction(t.Context(), client, dir, TransactionInput{
			Date:        time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
			Description: "ORIGINAL",
			Postings: []PostingInput{
				{Account: "expenses:food", Amount: "$10.00"},
				{Account: "assets:checking"},
			},
		})
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		_, err = UpdateTransaction(t.Context(), client, dir, fid, "UPDATED", "not-a-date", "", nil)
		if err == nil {
			t.Fatal("expected error for invalid date, got nil")
		}
		if !strings.Contains(err.Error(), "invalid date") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("updates_description", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 112, NumTxns: 2, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		fid, err := AppendTransaction(t.Context(), client, dir, TransactionInput{
			Date:        time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
			Description: "ORIGINAL DESCRIPTION",
			Postings: []PostingInput{
				{Account: "expenses:food", Amount: "$25.00"},
				{Account: "assets:checking"},
			},
		})
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		updated, err := UpdateTransaction(t.Context(), client, dir, fid, "UPDATED DESCRIPTION", "", "", []PostingInput{
			{Account: "expenses:food", Amount: "$25.00"},
			{Account: "assets:checking"},
		})
		if err != nil {
			t.Fatalf("UpdateTransaction: %v", err)
		}

		if updated.Description != "UPDATED DESCRIPTION" {
			t.Errorf("Description = %q, want %q", updated.Description, "UPDATED DESCRIPTION")
		}
		if updated.FID != fid {
			t.Errorf("FID = %q, want %q", updated.FID, fid)
		}
		if err := client.Check(t.Context()); err != nil {
			t.Fatalf("hledger check: %v", err)
		}
	})

	t.Run("updates_date_same_month", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 113, NumTxns: 2, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		fid, err := AppendTransaction(t.Context(), client, dir, TransactionInput{
			Date:        time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC),
			Description: "DATE UPDATE TEST",
			Postings: []PostingInput{
				{Account: "expenses:shopping", Amount: "$40.00"},
				{Account: "assets:checking"},
			},
		})
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		updated, err := UpdateTransaction(t.Context(), client, dir, fid, "DATE UPDATE TEST", "2026-03-20", "", []PostingInput{
			{Account: "expenses:shopping", Amount: "$40.00"},
			{Account: "assets:checking"},
		})
		if err != nil {
			t.Fatalf("UpdateTransaction: %v", err)
		}

		if updated.Date != "2026-03-20" {
			t.Errorf("Date = %q, want %q", updated.Date, "2026-03-20")
		}
		if updated.FID != fid {
			t.Errorf("FID = %q, want %q", updated.FID, fid)
		}
		if err := client.Check(t.Context()); err != nil {
			t.Fatalf("hledger check: %v", err)
		}
	})

	t.Run("updates_date_cross_month", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 114, NumTxns: 2, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		fid, err := AppendTransaction(t.Context(), client, dir, TransactionInput{
			Date:        time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
			Description: "CROSS MONTH TEST",
			Postings: []PostingInput{
				{Account: "expenses:food", Amount: "$15.00"},
				{Account: "assets:checking"},
			},
		})
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		updated, err := UpdateTransaction(t.Context(), client, dir, fid, "CROSS MONTH TEST", "2026-04-01", "", []PostingInput{
			{Account: "expenses:food", Amount: "$15.00"},
			{Account: "assets:checking"},
		})
		if err != nil {
			t.Fatalf("UpdateTransaction: %v", err)
		}

		if updated.Date != "2026-04-01" {
			t.Errorf("Date = %q, want %q", updated.Date, "2026-04-01")
		}
		if updated.FID != fid {
			t.Errorf("FID = %q, want %q", updated.FID, fid)
		}
		if err := client.Check(t.Context()); err != nil {
			t.Fatalf("hledger check: %v", err)
		}
	})

	t.Run("replaces_postings", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 115, NumTxns: 2, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		fid, err := AppendTransaction(t.Context(), client, dir, TransactionInput{
			Date:        time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC),
			Description: "REPLACE POSTINGS TEST",
			Postings: []PostingInput{
				{Account: "expenses:food", Amount: "$30.00"},
				{Account: "assets:checking"},
			},
		})
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		updated, err := UpdateTransaction(t.Context(), client, dir, fid, "REPLACE POSTINGS TEST", "", "", []PostingInput{
			{Account: "expenses:shopping", Amount: "$99.00"},
			{Account: "liabilities:credit-card"},
		})
		if err != nil {
			t.Fatalf("UpdateTransaction: %v", err)
		}

		if len(updated.Postings) != 2 {
			t.Fatalf("expected 2 postings, got %d", len(updated.Postings))
		}
		if updated.Postings[0].Account != "expenses:shopping" {
			t.Errorf("Posting[0].Account = %q, want %q", updated.Postings[0].Account, "expenses:shopping")
		}
		if len(updated.Postings[0].Amounts) != 1 || updated.Postings[0].Amounts[0].Quantity.FloatingPoint != 99.0 {
			t.Errorf("Posting[0].Amount = %v, want 99.0", updated.Postings[0].Amounts)
		}
		if updated.Postings[1].Account != "liabilities:credit-card" {
			t.Errorf("Posting[1].Account = %q, want %q", updated.Postings[1].Account, "liabilities:credit-card")
		}
		if err := client.Check(t.Context()); err != nil {
			t.Fatalf("hledger check: %v", err)
		}
	})

	t.Run("updates_comment", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 116, NumTxns: 2, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		fid, err := AppendTransaction(t.Context(), client, dir, TransactionInput{
			Date:        time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
			Description: "COMMENT UPDATE TEST",
			Comment:     "old note",
			Postings: []PostingInput{
				{Account: "expenses:misc", Amount: "$5.00"},
				{Account: "assets:checking"},
			},
		})
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		updated, err := UpdateTransaction(t.Context(), client, dir, fid, "COMMENT UPDATE TEST", "", "new note", []PostingInput{
			{Account: "expenses:misc", Amount: "$5.00"},
			{Account: "assets:checking"},
		})
		if err != nil {
			t.Fatalf("UpdateTransaction: %v", err)
		}

		if !strings.Contains(updated.Comment, "new note") {
			t.Errorf("Comment %q does not contain %q", updated.Comment, "new note")
		}
		if strings.Contains(updated.Comment, "old note") {
			t.Errorf("Comment %q still contains old comment %q", updated.Comment, "old note")
		}
		if err := client.Check(t.Context()); err != nil {
			t.Fatalf("hledger check: %v", err)
		}
	})

	t.Run("preserves_fid", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 117, NumTxns: 2, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		fid, err := AppendTransaction(t.Context(), client, dir, TransactionInput{
			Date:        time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			Description: "FID PRESERVATION TEST",
			Postings: []PostingInput{
				{Account: "expenses:food", Amount: "$8.00"},
				{Account: "assets:checking"},
			},
		})
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		updated, err := UpdateTransaction(t.Context(), client, dir, fid, "FID PRESERVATION TEST UPDATED", "", "", []PostingInput{
			{Account: "expenses:food", Amount: "$8.00"},
			{Account: "assets:checking"},
		})
		if err != nil {
			t.Fatalf("UpdateTransaction: %v", err)
		}

		if updated.FID != fid {
			t.Errorf("FID changed: got %q, want %q", updated.FID, fid)
		}

		// Ensure only one transaction exists with this fid.
		txns, err := client.Transactions(t.Context(), "code:"+fid)
		if err != nil {
			t.Fatalf("Transactions: %v", err)
		}
		if len(txns) != 1 {
			t.Errorf("expected 1 transaction with fid %q, got %d", fid, len(txns))
		}
	})

	t.Run("empty_date_keeps_existing", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 118, NumTxns: 2, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		fid, err := AppendTransaction(t.Context(), client, dir, TransactionInput{
			Date:        time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC),
			Description: "KEEP DATE TEST",
			Postings: []PostingInput{
				{Account: "expenses:food", Amount: "$12.00"},
				{Account: "assets:checking"},
			},
		})
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		updated, err := UpdateTransaction(t.Context(), client, dir, fid, "KEEP DATE TEST UPDATED", "", "", []PostingInput{
			{Account: "expenses:food", Amount: "$12.00"},
			{Account: "assets:checking"},
		})
		if err != nil {
			t.Fatalf("UpdateTransaction: %v", err)
		}

		if updated.Date != "2026-02-20" {
			t.Errorf("Date = %q, want %q (original date should be preserved)", updated.Date, "2026-02-20")
		}
	})
}
