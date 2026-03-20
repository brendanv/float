package journal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/brendanv/float/internal/testgen"
)

func TestModifyTags(t *testing.T) {
	t.Run("not_found", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 10, NumTxns: 3, WithFIDs: true})
		client := mustHledgerClient(t, dir)
		err := ModifyTags(t.Context(), client, dir, "00000000", map[string]string{"category": "food"})
		if err == nil {
			t.Fatal("expected error for non-existent fid, got nil")
		}
		if !strings.Contains(err.Error(), "no transaction found") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("adds_tags", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 11, NumTxns: 2, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		tx := TransactionInput{
			Date:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			Description: "TEST MODIFY TAGS",
			Postings: []PostingInput{
				{Account: "expenses:food", Amount: "$20.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := AppendTransaction(t.Context(), client, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		if err := ModifyTags(t.Context(), client, dir, fid, map[string]string{
			"category": "food",
			"source":   "manual",
		}); err != nil {
			t.Fatalf("ModifyTags: %v", err)
		}

		// Verify journal still passes hledger check.
		if err := client.Check(t.Context()); err != nil {
			t.Fatalf("hledger check after modify-tags: %v", err)
		}

		// Verify new tags are present via hledger query.
		txns, err := client.Transactions(t.Context(), "tag:fid="+fid)
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
		if tagMap["category"] != "food" {
			t.Errorf("category tag = %q, want %q", tagMap["category"], "food")
		}
		if tagMap["source"] != "manual" {
			t.Errorf("source tag = %q, want %q", tagMap["source"], "manual")
		}
		if tagMap["fid"] != fid {
			t.Errorf("fid tag = %q, want %q", tagMap["fid"], fid)
		}
	})

	t.Run("replaces_existing_tags", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 12, NumTxns: 2, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		tx := TransactionInput{
			Date:        time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			Description: "REPLACE TAGS TEST",
			Postings: []PostingInput{
				{Account: "expenses:shopping", Amount: "$50.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := AppendTransaction(t.Context(), client, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		// First ModifyTags: add category and source.
		if err := ModifyTags(t.Context(), client, dir, fid, map[string]string{
			"category": "shopping",
			"source":   "import",
		}); err != nil {
			t.Fatalf("first ModifyTags: %v", err)
		}

		// Second ModifyTags: replace with only category (different value), no source.
		if err := ModifyTags(t.Context(), client, dir, fid, map[string]string{
			"category": "household",
		}); err != nil {
			t.Fatalf("second ModifyTags: %v", err)
		}

		if err := client.Check(t.Context()); err != nil {
			t.Fatalf("hledger check after second modify-tags: %v", err)
		}

		txns, err := client.Transactions(t.Context(), "tag:fid="+fid)
		if err != nil {
			t.Fatalf("Transactions after second modify-tags: %v", err)
		}
		if len(txns) != 1 {
			t.Fatalf("expected 1 transaction, got %d", len(txns))
		}
		tagMap := make(map[string]string)
		for _, tag := range txns[0].Tags {
			tagMap[tag[0]] = tag[1]
		}
		if tagMap["category"] != "household" {
			t.Errorf("category tag = %q, want %q", tagMap["category"], "household")
		}
		if _, ok := tagMap["source"]; ok {
			t.Errorf("source tag should have been removed, got %q", tagMap["source"])
		}
		if tagMap["fid"] != fid {
			t.Errorf("fid tag = %q, want %q", tagMap["fid"], fid)
		}
	})

	t.Run("clears_all_tags", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 13, NumTxns: 2, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		tx := TransactionInput{
			Date:        time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC),
			Description: "CLEAR TAGS TEST",
			Postings: []PostingInput{
				{Account: "expenses:misc", Amount: "$5.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := AppendTransaction(t.Context(), client, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		if err := ModifyTags(t.Context(), client, dir, fid, map[string]string{
			"category": "misc",
		}); err != nil {
			t.Fatalf("ModifyTags to add: %v", err)
		}

		// Clear all non-fid tags by passing empty map.
		if err := ModifyTags(t.Context(), client, dir, fid, map[string]string{}); err != nil {
			t.Fatalf("ModifyTags to clear: %v", err)
		}

		if err := client.Check(t.Context()); err != nil {
			t.Fatalf("hledger check after clear: %v", err)
		}

		txns, err := client.Transactions(t.Context(), "tag:fid="+fid)
		if err != nil {
			t.Fatalf("Transactions after clear: %v", err)
		}
		if len(txns) != 1 {
			t.Fatalf("expected 1 transaction, got %d", len(txns))
		}
		tagMap := make(map[string]string)
		for _, tag := range txns[0].Tags {
			tagMap[tag[0]] = tag[1]
		}
		if _, ok := tagMap["category"]; ok {
			t.Errorf("category tag should have been removed, got %q", tagMap["category"])
		}
		if tagMap["fid"] != fid {
			t.Errorf("fid tag = %q, want %q", tagMap["fid"], fid)
		}
	})

	t.Run("preserves_fid_in_file", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 14, NumTxns: 1, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		tx := TransactionInput{
			Date:        time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC),
			Description: "FID PRESERVED TEST",
			Postings: []PostingInput{
				{Account: "expenses:food", Amount: "$15.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := AppendTransaction(t.Context(), client, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		if err := ModifyTags(t.Context(), client, dir, fid, map[string]string{
			"category": "groceries",
		}); err != nil {
			t.Fatalf("ModifyTags: %v", err)
		}

		// Read the raw file and verify fid is still on the header line.
		journalPath := filepath.Join(dir, "2026/03.journal")
		data, err := os.ReadFile(journalPath)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "fid:"+fid) {
			t.Errorf("fid tag not found in file after ModifyTags:\n%s", content)
		}
		// The new tag should be on a separate comment line.
		if !strings.Contains(content, "category:groceries") {
			t.Errorf("category tag not found in file:\n%s", content)
		}
	})
}

func TestStripNonFidTagsFromHeaderLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		fid  string
		want string
	}{
		{
			name: "fid only",
			line: "2026-01-15 Test  ; fid:abc12345",
			fid:  "abc12345",
			want: "2026-01-15 Test  ; fid:abc12345",
		},
		{
			name: "fid with trailing tag",
			line: "2026-01-15 Test  ; fid:abc12345, category:food",
			fid:  "abc12345",
			want: "2026-01-15 Test  ; fid:abc12345",
		},
		{
			name: "fid with multiple trailing tags",
			line: "2026-01-15 Test  ; fid:abc12345, category:food, source:manual",
			fid:  "abc12345",
			want: "2026-01-15 Test  ; fid:abc12345",
		},
		{
			name: "fid with free text preserved",
			line: "2026-01-15 Test  ; fid:abc12345 imported",
			fid:  "abc12345",
			want: "2026-01-15 Test  ; fid:abc12345 imported",
		},
		{
			name: "no comment",
			line: "2026-01-15 Test",
			fid:  "abc12345",
			want: "2026-01-15 Test",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripNonFidTagsFromHeaderLine(tt.line, tt.fid)
			if got != tt.want {
				t.Errorf("stripNonFidTagsFromHeaderLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStripTagsFromCommentLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{
			name: "tag-only line",
			line: "    ; category:food",
			want: "",
		},
		{
			name: "multiple tags",
			line: "    ; category:food, source:manual",
			want: "",
		},
		{
			name: "mixed line preserves non-tag text",
			line: "    ; imported from bank, category:food",
			want: "    ; imported from bank",
		},
		{
			name: "no tags",
			line: "    ; just a comment",
			want: "    ; just a comment",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripTagsFromCommentLine(tt.line)
			if got != tt.want {
				t.Errorf("stripTagsFromCommentLine() = %q, want %q", got, tt.want)
			}
		})
	}
}
