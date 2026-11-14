package journal

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/testgen"
)

func TestUpdateTransaction(t *testing.T) {
	t.Run("not_found", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 110, NumTxns: 3, WithFIDs: true})
		client := mustHledgerClient(t, dir)
		_, err := UpdateTransaction(t.Context(), client, dir, "00000000", "DESCRIPTION", "", nil, nil, nil, "")
		if err == nil {
			t.Fatal("expected error for non-existent fid, got nil")
		}
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got: %v", err)
		}
	})

	t.Run("invalid_date", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 111, NumTxns: 2, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		fid, err := AppendTransaction(t.Context(), client, dir, TransactionInput{
			Date:        time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
			Description: "ORIGINAL",
			Postings: []PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "10.00"},
				{Account: "assets:checking"},
			},
		})
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		_, err = UpdateTransaction(t.Context(), client, dir, fid, "UPDATED", "not-a-date", nil, nil, nil, "")
		if err == nil {
			t.Fatal("expected error for invalid date, got nil")
		}
		if !errors.Is(err, ErrInvalidDate) {
			t.Errorf("expected ErrInvalidDate, got: %v", err)
		}
	})

	t.Run("updates_description", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 112, NumTxns: 2, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		fid, err := AppendTransaction(t.Context(), client, dir, TransactionInput{
			Date:        time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
			Description: "ORIGINAL DESCRIPTION",
			Postings: []PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "25.00"},
				{Account: "assets:checking"},
			},
		})
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		updated, err := UpdateTransaction(t.Context(), client, dir, fid, "UPDATED DESCRIPTION", "", nil, nil, []PostingInput{
			{Account: "expenses:food", Commodity: "USD", Quantity: "25.00"},
			{Account: "assets:checking"},
		}, "")
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
				{Account: "expenses:shopping", Commodity: "USD", Quantity: "40.00"},
				{Account: "assets:checking"},
			},
		})
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		updated, err := UpdateTransaction(t.Context(), client, dir, fid, "DATE UPDATE TEST", "2026-03-20", nil, nil, []PostingInput{
			{Account: "expenses:shopping", Commodity: "USD", Quantity: "40.00"},
			{Account: "assets:checking"},
		}, "")
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
				{Account: "expenses:food", Commodity: "USD", Quantity: "15.00"},
				{Account: "assets:checking"},
			},
		})
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		updated, err := UpdateTransaction(t.Context(), client, dir, fid, "CROSS MONTH TEST", "2026-04-01", nil, nil, []PostingInput{
			{Account: "expenses:food", Commodity: "USD", Quantity: "15.00"},
			{Account: "assets:checking"},
		}, "")
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
				{Account: "expenses:food", Commodity: "USD", Quantity: "30.00"},
				{Account: "assets:checking"},
			},
		})
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		updated, err := UpdateTransaction(t.Context(), client, dir, fid, "REPLACE POSTINGS TEST", "", nil, nil, []PostingInput{
			{Account: "expenses:shopping", Commodity: "USD", Quantity: "99.00"},
			{Account: "liabilities:credit-card"},
		}, "")
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
				{Account: "expenses:misc", Commodity: "USD", Quantity: "5.00"},
				{Account: "assets:checking"},
			},
		})
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		updated, err := UpdateTransaction(t.Context(), client, dir, fid, "COMMENT UPDATE TEST", "", strPtr("new note"), nil, []PostingInput{
			{Account: "expenses:misc", Commodity: "USD", Quantity: "5.00"},
			{Account: "assets:checking"},
		}, "")
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

	t.Run("nil_comment_and_tags_are_kept_non_nil_replace", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 118, NumTxns: 2, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		fid, err := AppendTransaction(t.Context(), client, dir, TransactionInput{
			Date:        time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC),
			Description: "KEEP COMMENT TEST",
			Comment:     "receipt in drawer",
			Tags:        map[string]string{"trip": "paris"},
			Postings: []PostingInput{
				{Account: "expenses:misc", Commodity: "USD", Quantity: "5.00"},
				{Account: "assets:checking"},
			},
		})
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}
		postings := []PostingInput{
			{Account: "expenses:misc", Commodity: "USD", Quantity: "6.00"},
			{Account: "assets:checking"},
		}

		kept, err := UpdateTransaction(t.Context(), client, dir, fid, "KEEP COMMENT TEST 2", "", nil, nil, postings, "")
		if err != nil {
			t.Fatalf("UpdateTransaction (keep): %v", err)
		}
		if !strings.Contains(kept.Comment, "receipt in drawer") {
			t.Errorf("nil comment: Comment %q lost %q", kept.Comment, "receipt in drawer")
		}
		if !hasTag(kept, "trip", "paris") {
			t.Errorf("nil tags: tags %v lost trip:paris", kept.Tags)
		}

		cleared, err := UpdateTransaction(t.Context(), client, dir, fid, "KEEP COMMENT TEST 3", "", strPtr(""), map[string]string{}, postings, "")
		if err != nil {
			t.Fatalf("UpdateTransaction (clear): %v", err)
		}
		if strings.Contains(cleared.Comment, "receipt in drawer") {
			t.Errorf("empty comment: Comment %q still contains old comment", cleared.Comment)
		}
		if hasTag(cleared, "trip", "") {
			t.Errorf("empty tags: tags %v still contain trip", cleared.Tags)
		}
		if err := client.Check(t.Context()); err != nil {
			t.Fatalf("hledger check: %v", err)
		}
	})

	t.Run("round_trips_multiline_posting_comment", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 119, NumTxns: 2, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		fid, err := AppendTransaction(t.Context(), client, dir, TransactionInput{
			Date:        time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC),
			Description: "POSTING COMMENT TEST",
			Postings: []PostingInput{
				{Account: "expenses:misc", Commodity: "USD", Quantity: "5.00", Comment: "date:2026-02-19, split\nsecond line"},
				{Account: "assets:checking"},
			},
		})
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}
		txns, err := client.Transactions(t.Context(), "code:"+fid)
		if err != nil || len(txns) != 1 {
			t.Fatalf("Transactions: %v (%d)", err, len(txns))
		}
		// Send hledger's reported posting comments straight back, as the web UI does.
		var postings []PostingInput
		for _, p := range txns[0].Postings {
			in := PostingInput{Account: p.Account, Comment: p.Comment}
			if p.Account == "expenses:misc" {
				in.Commodity, in.Quantity = "USD", "5.00"
			}
			postings = append(postings, in)
		}
		updated, err := UpdateTransaction(t.Context(), client, dir, fid, "POSTING COMMENT TEST 2", "", nil, nil, postings, "")
		if err != nil {
			t.Fatalf("UpdateTransaction: %v", err)
		}
		var misc string
		var miscDate *string
		for _, p := range updated.Postings {
			if p.Account == "expenses:misc" {
				misc = p.Comment
				miscDate = p.Date
			}
		}
		if !strings.Contains(misc, "split") || !strings.Contains(misc, "second line") {
			t.Errorf("posting comment = %q, want both lines kept", misc)
		}
		if miscDate == nil || *miscDate != "2026-02-19" {
			t.Errorf("posting date = %v, want 2026-02-19", miscDate)
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
				{Account: "expenses:food", Commodity: "USD", Quantity: "8.00"},
				{Account: "assets:checking"},
			},
		})
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		updated, err := UpdateTransaction(t.Context(), client, dir, fid, "FID PRESERVATION TEST UPDATED", "", nil, nil, []PostingInput{
			{Account: "expenses:food", Commodity: "USD", Quantity: "8.00"},
			{Account: "assets:checking"},
		}, "")
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
				{Account: "expenses:food", Commodity: "USD", Quantity: "12.00"},
				{Account: "assets:checking"},
			},
		})
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		updated, err := UpdateTransaction(t.Context(), client, dir, fid, "KEEP DATE TEST UPDATED", "", nil, nil, []PostingInput{
			{Account: "expenses:food", Commodity: "USD", Quantity: "12.00"},
			{Account: "assets:checking"},
		}, "")
		if err != nil {
			t.Fatalf("UpdateTransaction: %v", err)
		}

		if updated.Date != "2026-02-20" {
			t.Errorf("Date = %q, want %q (original date should be preserved)", updated.Date, "2026-02-20")
		}
	})
}

func strPtr(s string) *string { return &s }

func hasTag(t hledger.Transaction, key, value string) bool {
	for _, kv := range t.Tags {
		if kv[0] == key && (value == "" || kv[1] == value) {
			return true
		}
	}
	return false
}

// TestFreeTextCommentMatchesHledgerTags checks freeTextComment against the tags
// real hledger parses from the same comment: the free text must keep every
// non-tag word and drop every tag, so a rewrite neither loses the note nor
// writes a tag twice.
func TestFreeTextCommentMatchesHledgerTags(t *testing.T) {
	tests := []struct {
		comment  string
		wantText string
		wantTags map[string]string
	}{
		{"receipt in drawer", "receipt in drawer", map[string]string{}},
		{"receipt in drawer, trip:paris", "receipt in drawer", map[string]string{"trip": "paris"}},
		{"trip:paris, receipt in drawer", "receipt in drawer", map[string]string{"trip": "paris"}},
		{"see note: call bank, then file", "see, then file", map[string]string{"note": "call bank"}},
		{"apples, oranges", "apples, oranges", map[string]string{}},
		{"a:1, b:two words", "", map[string]string{"a": "1", "b": "two words"}},
	}
	for _, tt := range tests {
		t.Run(tt.comment, func(t *testing.T) {
			dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 120, NumTxns: 1, WithFIDs: true})
			client := mustHledgerClient(t, dir)
			fid, err := AppendTransaction(t.Context(), client, dir, TransactionInput{
				Date:        time.Date(2026, 2, 18, 0, 0, 0, 0, time.UTC),
				Description: "FREE TEXT",
				Comment:     tt.comment,
				Postings: []PostingInput{
					{Account: "expenses:misc", Commodity: "USD", Quantity: "5.00"},
					{Account: "assets:checking"},
				},
			})
			if err != nil {
				t.Fatalf("AppendTransaction: %v", err)
			}
			txns, err := client.Transactions(t.Context(), "code:"+fid)
			if err != nil || len(txns) != 1 {
				t.Fatalf("Transactions: %v (%d)", err, len(txns))
			}
			gotTags := userTagsFromTransaction(txns[0])
			if len(gotTags) != len(tt.wantTags) {
				t.Fatalf("hledger tags = %v, want %v (test case disagrees with hledger)", gotTags, tt.wantTags)
			}
			for k, v := range tt.wantTags {
				if gotTags[k] != v {
					t.Fatalf("hledger tags = %v, want %v (test case disagrees with hledger)", gotTags, tt.wantTags)
				}
			}
			if got := freeTextComment(txns[0].Comment); got != tt.wantText {
				t.Errorf("freeTextComment(%q) = %q, want %q", txns[0].Comment, got, tt.wantText)
			}

			// A rewrite that keeps the comment must not duplicate tags.
			updated, err := UpdateTransaction(t.Context(), client, dir, fid, "FREE TEXT 2", "", nil, nil, []PostingInput{
				{Account: "expenses:misc", Commodity: "USD", Quantity: "5.00"},
				{Account: "assets:checking"},
			}, "")
			if err != nil {
				t.Fatalf("UpdateTransaction: %v", err)
			}
			if len(updated.Tags) != len(txns[0].Tags) {
				t.Errorf("tags after rewrite = %v, want %v", updated.Tags, txns[0].Tags)
			}
			if tt.wantText != "" && !strings.Contains(updated.Comment, tt.wantText) {
				t.Errorf("comment after rewrite = %q, want it to keep %q", updated.Comment, tt.wantText)
			}
		})
	}
}
