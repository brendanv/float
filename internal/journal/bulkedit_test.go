package journal

import (
	"strings"
	"testing"
	"time"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/testgen"
)

func ptr[T any](v T) *T { return &v }

func TestBulkEditTransactions_NotFound(t *testing.T) {
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 300, NumTxns: 2, WithFIDs: true})
	client := mustHledgerClient(t, dir)
	err := BulkEditTransactions(t.Context(), client, dir, []string{"00000000"}, nil, "", "", "", nil)
	if err == nil {
		t.Fatal("expected error for non-existent fid, got nil")
	}
	if !strings.Contains(err.Error(), "no transaction found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBulkEditTransactions_Reviewed(t *testing.T) {
	tests := []struct {
		name       string
		initial    string // initial status
		reviewed   bool
		wantStatus string
	}{
		{"mark_reviewed", "", true, "Cleared"},
		{"unmark_reviewed_to_pending", "Cleared", false, "Pending"},
		{"mark_already_cleared", "Cleared", true, "Cleared"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 301, NumTxns: 1, WithFIDs: true})
			client := mustHledgerClient(t, dir)

			tx := TransactionInput{
				Date:        time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
				Description: "BULK REVIEW TEST",
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

			if err := BulkEditTransactions(t.Context(), client, dir, []string{fid}, ptr(tc.reviewed), "", "", "", nil); err != nil {
				t.Fatalf("BulkEditTransactions: %v", err)
			}

			if err := client.Check(t.Context()); err != nil {
				t.Fatalf("hledger check after bulk edit: %v", err)
			}

			txns, err := client.Transactions(t.Context(), "code:"+fid)
			if err != nil {
				t.Fatalf("Transactions after bulk edit: %v", err)
			}
			if txns[0].Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", txns[0].Status, tc.wantStatus)
			}
		})
	}
}

func TestBulkEditTransactions_AddTag(t *testing.T) {
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 310, NumTxns: 1, WithFIDs: true})
	client := mustHledgerClient(t, dir)

	tx := TransactionInput{
		Date:        time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
		Description: "BULK TAG TEST",
		Postings: []PostingInput{
			{Account: "expenses:food", Amount: "$20.00"},
			{Account: "assets:checking"},
		},
	}
	fid, err := AppendTransaction(t.Context(), client, dir, tx)
	if err != nil {
		t.Fatalf("AppendTransaction: %v", err)
	}

	if err := BulkEditTransactions(t.Context(), client, dir, []string{fid}, nil, "category", "groceries", "", nil); err != nil {
		t.Fatalf("BulkEditTransactions: %v", err)
	}

	if err := client.Check(t.Context()); err != nil {
		t.Fatalf("hledger check: %v", err)
	}

	txns, err := client.Transactions(t.Context(), "code:"+fid)
	if err != nil {
		t.Fatalf("Transactions: %v", err)
	}
	found := false
	for _, kv := range txns[0].Tags {
		if kv[0] == "category" && kv[1] == "groceries" {
			found = true
		}
	}
	if !found {
		t.Errorf("tag category:groceries not found in tags: %v", txns[0].Tags)
	}
}

func TestBulkEditTransactions_RemoveTag(t *testing.T) {
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 320, NumTxns: 1, WithFIDs: true})
	client := mustHledgerClient(t, dir)

	tx := TransactionInput{
		Date:        time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
		Description: "BULK REMOVE TAG TEST",
		Postings: []PostingInput{
			{Account: "expenses:food", Amount: "$15.00"},
			{Account: "assets:checking"},
		},
	}
	fid, err := AppendTransaction(t.Context(), client, dir, tx)
	if err != nil {
		t.Fatalf("AppendTransaction: %v", err)
	}

	// First add a tag.
	if err := ModifyTags(t.Context(), client, dir, fid, map[string]string{"category": "food", "source": "manual"}); err != nil {
		t.Fatalf("ModifyTags: %v", err)
	}

	// Now remove "source" via BulkEditTransactions.
	if err := BulkEditTransactions(t.Context(), client, dir, []string{fid}, nil, "", "", "source", nil); err != nil {
		t.Fatalf("BulkEditTransactions: %v", err)
	}

	if err := client.Check(t.Context()); err != nil {
		t.Fatalf("hledger check: %v", err)
	}

	txns, err := client.Transactions(t.Context(), "code:"+fid)
	if err != nil {
		t.Fatalf("Transactions: %v", err)
	}
	for _, kv := range txns[0].Tags {
		if kv[0] == "source" {
			t.Errorf("tag source should have been removed, but found: %v", kv)
		}
	}
	// category should still be present.
	found := false
	for _, kv := range txns[0].Tags {
		if kv[0] == "category" && kv[1] == "food" {
			found = true
		}
	}
	if !found {
		t.Errorf("tag category:food should still be present after removing source")
	}
}

func TestBulkEditTransactions_SetPayee(t *testing.T) {
	tests := []struct {
		name        string
		initialDesc string  // description passed to TransactionInput
		newPayee    string  // value for set_payee
		wantPayee   *string // nil if no payee expected
		wantNote    *string // nil if no note expected (no "|" in description)
	}{
		{
			name:        "set_payee_on_plain_description",
			initialDesc: "Some Transaction",
			newPayee:    "Acme Corp",
			wantPayee:   ptr("Acme Corp"),
			wantNote:    ptr("Some Transaction"),
		},
		{
			// After clearing payee, description becomes "The Note" (no "|"),
			// so hledger does not split into payee/note — both nil.
			name:        "clear_payee",
			initialDesc: "Old Payee | The Note",
			newPayee:    "",
			wantPayee:   nil,
			wantNote:    nil,
		},
		{
			name:        "replace_payee",
			initialDesc: "Old Payee | The Note",
			newPayee:    "New Payee",
			wantPayee:   ptr("New Payee"),
			wantNote:    ptr("The Note"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 330, NumTxns: 1, WithFIDs: true})
			client := mustHledgerClient(t, dir)

			tx := TransactionInput{
				Date:        time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
				Description: tc.initialDesc,
				Postings: []PostingInput{
					{Account: "expenses:food", Amount: "$25.00"},
					{Account: "assets:checking"},
				},
			}
			fid, err := AppendTransaction(t.Context(), client, dir, tx)
			if err != nil {
				t.Fatalf("AppendTransaction: %v", err)
			}

			if err := BulkEditTransactions(t.Context(), client, dir, []string{fid}, nil, "", "", "", ptr(tc.newPayee)); err != nil {
				t.Fatalf("BulkEditTransactions: %v", err)
			}

			if err := client.Check(t.Context()); err != nil {
				t.Fatalf("hledger check: %v", err)
			}

			txns, err := client.Transactions(t.Context(), "code:"+fid)
			if err != nil {
				t.Fatalf("Transactions: %v", err)
			}
			got := txns[0]

			if tc.wantPayee == nil {
				if got.Payee != nil {
					t.Errorf("payee = %q, want nil", *got.Payee)
				}
			} else {
				if got.Payee == nil {
					t.Errorf("payee = nil, want %q", *tc.wantPayee)
				} else if *got.Payee != *tc.wantPayee {
					t.Errorf("payee = %q, want %q", *got.Payee, *tc.wantPayee)
				}
			}

			if tc.wantNote == nil {
				if got.Note != nil {
					t.Errorf("note = %q, want nil", *got.Note)
				}
			} else {
				if got.Note == nil {
					t.Errorf("note = nil, want %q", *tc.wantNote)
				} else if *got.Note != *tc.wantNote {
					t.Errorf("note = %q, want %q", *got.Note, *tc.wantNote)
				}
			}
		})
	}
}

func TestBulkEditTransactions_MultipleFIDs(t *testing.T) {
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 340, NumTxns: 1, WithFIDs: true})
	client := mustHledgerClient(t, dir)

	var fids []string
	for i := range 3 {
		tx := TransactionInput{
			Date:        time.Date(2026, 2, 15+i, 0, 0, 0, 0, time.UTC),
			Description: "MULTI TX",
			Postings: []PostingInput{
				{Account: "expenses:food", Amount: "$10.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := AppendTransaction(t.Context(), client, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction %d: %v", i, err)
		}
		fids = append(fids, fid)
	}

	// Mark all reviewed and add a tag in one call.
	if err := BulkEditTransactions(t.Context(), client, dir, fids, ptr(true), "batch", "2026q1", "", nil); err != nil {
		t.Fatalf("BulkEditTransactions: %v", err)
	}

	if err := client.Check(t.Context()); err != nil {
		t.Fatalf("hledger check: %v", err)
	}

	for _, fid := range fids {
		txns, err := client.Transactions(t.Context(), "code:"+fid)
		if err != nil {
			t.Fatalf("Transactions %s: %v", fid, err)
		}
		if txns[0].Status != "Cleared" {
			t.Errorf("fid %s: status = %q, want Cleared", fid, txns[0].Status)
		}
		found := false
		for _, kv := range txns[0].Tags {
			if kv[0] == "batch" && kv[1] == "2026q1" {
				found = true
			}
		}
		if !found {
			t.Errorf("fid %s: tag batch:2026q1 not found", fid)
		}
	}
}

func TestBulkEditTransactions_CombinedOperations(t *testing.T) {
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 350, NumTxns: 1, WithFIDs: true})
	client := mustHledgerClient(t, dir)

	tx := TransactionInput{
		Date:        time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
		Description: "Old Payee | Purchase",
		Postings: []PostingInput{
			{Account: "expenses:food", Amount: "$30.00"},
			{Account: "assets:checking"},
		},
	}
	fid, err := AppendTransaction(t.Context(), client, dir, tx)
	if err != nil {
		t.Fatalf("AppendTransaction: %v", err)
	}

	// Reviewed + add tag + set payee in one call.
	if err := BulkEditTransactions(t.Context(), client, dir, []string{fid},
		ptr(true), "category", "food", "", ptr("New Payee")); err != nil {
		t.Fatalf("BulkEditTransactions: %v", err)
	}

	if err := client.Check(t.Context()); err != nil {
		t.Fatalf("hledger check: %v", err)
	}

	txns, err := client.Transactions(t.Context(), "code:"+fid)
	if err != nil {
		t.Fatalf("Transactions: %v", err)
	}
	got := txns[0]

	if got.Status != "Cleared" {
		t.Errorf("status = %q, want Cleared", got.Status)
	}
	foundTag := false
	for _, kv := range got.Tags {
		if kv[0] == "category" && kv[1] == "food" {
			foundTag = true
		}
	}
	if !foundTag {
		t.Errorf("tag category:food not found after bulk edit")
	}
	if got.Payee == nil || *got.Payee != "New Payee" {
		t.Errorf("payee = %v, want %q", got.Payee, "New Payee")
	}
	if got.Note == nil || *got.Note != "Purchase" {
		t.Errorf("note = %v, want %q", got.Note, "Purchase")
	}
}

func TestBulkEditTransactions_StampsUpdatedAt(t *testing.T) {
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 360, NumTxns: 1, WithFIDs: true})
	client := mustHledgerClient(t, dir)

	tx := TransactionInput{
		Date:        time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
		Description: "STAMP TEST",
		Postings: []PostingInput{
			{Account: "expenses:food", Amount: "$10.00"},
			{Account: "assets:checking"},
		},
	}
	fid, err := AppendTransaction(t.Context(), client, dir, tx)
	if err != nil {
		t.Fatalf("AppendTransaction: %v", err)
	}

	if err := BulkEditTransactions(t.Context(), client, dir, []string{fid}, nil, "category", "food", "", nil); err != nil {
		t.Fatalf("BulkEditTransactions: %v", err)
	}

	txns, err := client.Transactions(t.Context(), "code:"+fid)
	if err != nil {
		t.Fatalf("Transactions: %v", err)
	}
	if _, ok := txns[0].FloatMeta[hledger.HiddenMetaPrefix+"updated-at"]; !ok {
		t.Error("float-updated-at not stamped after bulk edit")
	}
}

func TestUpdateTransactionPayee(t *testing.T) {
	tests := []struct {
		name        string
		initialDesc string
		newPayee    string
		wantPayee   *string
		wantNote    *string
	}{
		{
			name:        "set_payee_on_plain_description",
			initialDesc: "Some Transaction",
			newPayee:    "Acme Corp",
			wantPayee:   ptr("Acme Corp"),
			wantNote:    ptr("Some Transaction"),
		},
		{
			name:        "replace_payee",
			initialDesc: "Old Payee | The Note",
			newPayee:    "New Payee",
			wantPayee:   ptr("New Payee"),
			wantNote:    ptr("The Note"),
		},
		{
			// Clearing payee leaves just the note as the description (no "|"), so hledger
			// does not split payee/note — both are nil.
			name:        "clear_payee",
			initialDesc: "Old Payee | The Note",
			newPayee:    "",
			wantPayee:   nil,
			wantNote:    nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 370, NumTxns: 1, WithFIDs: true})
			client := mustHledgerClient(t, dir)

			tx := TransactionInput{
				Date:        time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
				Description: tc.initialDesc,
				Postings: []PostingInput{
					{Account: "expenses:food", Amount: "$25.00"},
					{Account: "assets:checking"},
				},
			}
			fid, err := AppendTransaction(t.Context(), client, dir, tx)
			if err != nil {
				t.Fatalf("AppendTransaction: %v", err)
			}

			got, err := UpdateTransactionPayee(t.Context(), client, dir, fid, tc.newPayee)
			if err != nil {
				t.Fatalf("UpdateTransactionPayee: %v", err)
			}

			if err := client.Check(t.Context()); err != nil {
				t.Fatalf("hledger check: %v", err)
			}

			if tc.wantPayee == nil {
				if got.Payee != nil {
					t.Errorf("payee = %q, want nil", *got.Payee)
				}
			} else {
				if got.Payee == nil {
					t.Errorf("payee = nil, want %q", *tc.wantPayee)
				} else if *got.Payee != *tc.wantPayee {
					t.Errorf("payee = %q, want %q", *got.Payee, *tc.wantPayee)
				}
			}

			if tc.wantNote == nil {
				if got.Note != nil {
					t.Errorf("note = %q, want nil", *got.Note)
				}
			} else {
				if got.Note == nil {
					t.Errorf("note = nil, want %q", *tc.wantNote)
				} else if *got.Note != *tc.wantNote {
					t.Errorf("note = %q, want %q", *got.Note, *tc.wantNote)
				}
			}

			// Verify float-updated-at is stamped.
			if _, ok := got.FloatMeta[hledger.HiddenMetaPrefix+"updated-at"]; !ok {
				t.Error("float-updated-at not stamped after UpdateTransactionPayee")
			}
		})
	}
}
