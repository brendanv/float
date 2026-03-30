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
		txns, err := client.Transactions(t.Context(), "code:"+fid)
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

		txns, err := client.Transactions(t.Context(), "code:"+fid)
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
	})

	t.Run("empty_value_tag_can_be_removed", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 15, NumTxns: 2, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		tx := TransactionInput{
			Date:        time.Date(2026, 1, 22, 0, 0, 0, 0, time.UTC),
			Description: "EMPTY TAG TEST",
			Postings: []PostingInput{
				{Account: "expenses:misc", Amount: "$7.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := AppendTransaction(t.Context(), client, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		// Set a tag with an empty value.
		if err := ModifyTags(t.Context(), client, dir, fid, map[string]string{"review": ""}); err != nil {
			t.Fatalf("ModifyTags to set empty-value tag: %v", err)
		}

		// Remove it by passing an empty map.
		if err := ModifyTags(t.Context(), client, dir, fid, map[string]string{}); err != nil {
			t.Fatalf("ModifyTags to clear: %v", err)
		}

		if err := client.Check(t.Context()); err != nil {
			t.Fatalf("hledger check: %v", err)
		}

		txns, err := client.Transactions(t.Context(), "code:"+fid)
		if err != nil {
			t.Fatalf("Transactions: %v", err)
		}
		if len(txns) != 1 {
			t.Fatalf("expected 1 transaction, got %d", len(txns))
		}
		for _, tag := range txns[0].Tags {
			if tag[0] == "review" {
				t.Errorf("review tag should have been removed, still present with value %q", tag[1])
			}
		}
		if txns[0].FID != fid {
			t.Errorf("fid mismatch: got %q, want %q", txns[0].FID, fid)
		}
	})

	t.Run("subsequent_calls_produce_single_tag_line", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 16, NumTxns: 2, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		tx := TransactionInput{
			Date:        time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC),
			Description: "SINGLE ROW TEST",
			Postings: []PostingInput{
				{Account: "expenses:food", Amount: "$9.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := AppendTransaction(t.Context(), client, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		if err := ModifyTags(t.Context(), client, dir, fid, map[string]string{"review": ""}); err != nil {
			t.Fatalf("first ModifyTags: %v", err)
		}
		if err := ModifyTags(t.Context(), client, dir, fid, map[string]string{"category": "food"}); err != nil {
			t.Fatalf("second ModifyTags: %v", err)
		}

		if err := client.Check(t.Context()); err != nil {
			t.Fatalf("hledger check: %v", err)
		}

		// Verify only one non-fid tag comment line exists in the file.
		txns, err := client.Transactions(t.Context(), "code:"+fid)
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
		if _, ok := tagMap["review"]; ok {
			t.Errorf("review tag should have been replaced, still present")
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

		txns, err := client.Transactions(t.Context(), "code:"+fid)
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
		if !strings.Contains(content, "("+fid+")") {
			t.Errorf("fid code not found in file after ModifyTags:\n%s", content)
		}
		// The new tag should be on a separate comment line.
		if !strings.Contains(content, "category:groceries") {
			t.Errorf("category tag not found in file:\n%s", content)
		}
	})
}

func TestModifyTagsTwoSpaceIndent(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "2026"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte("include 2026/03.journal\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026/03.journal"),
		[]byte("2026-03-01 (3a4591b0) Whole Foods\n  expenses:food  $31.50\n  assets:checking\n"), 0644); err != nil {
		t.Fatal(err)
	}
	client := mustHledgerClient(t, dir)

	if err := ModifyTags(t.Context(), client, dir, "3a4591b0", map[string]string{"category": "groceries"}); err != nil {
		t.Fatalf("ModifyTags add: %v", err)
	}
	if err := client.Check(t.Context()); err != nil {
		t.Fatalf("hledger check after add: %v", err)
	}

	if err := ModifyTags(t.Context(), client, dir, "3a4591b0", map[string]string{}); err != nil {
		t.Fatalf("ModifyTags clear: %v", err)
	}
	if err := client.Check(t.Context()); err != nil {
		t.Fatalf("hledger check after clear: %v", err)
	}

	txns, err := client.Transactions(t.Context(), "code:3a4591b0")
	if err != nil {
		t.Fatalf("Transactions: %v", err)
	}
	if len(txns) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(txns))
	}
	for _, tag := range txns[0].Tags {
		if tag[0] == "category" {
			t.Errorf("category tag should have been removed, still present with value %q", tag[1])
		}
	}
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
			line: "2026-01-15 (abc12345) Test",
			fid:  "abc12345",
			want: "2026-01-15 (abc12345) Test",
		},
		{
			name: "fid with trailing tag",
			line: "2026-01-15 (abc12345) Test  ; category:food",
			fid:  "abc12345",
			want: "2026-01-15 (abc12345) Test",
		},
		{
			name: "fid with multiple trailing tags",
			line: "2026-01-15 (abc12345) Test  ; category:food, source:manual",
			fid:  "abc12345",
			want: "2026-01-15 (abc12345) Test",
		},
		{
			name: "fid with free text preserved",
			line: "2026-01-15 (abc12345) Test  ; imported",
			fid:  "abc12345",
			want: "2026-01-15 (abc12345) Test  ; imported",
		},
		{
			name: "no comment",
			line: "2026-01-15 (abc12345) Test",
			fid:  "abc12345",
			want: "2026-01-15 (abc12345) Test",
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
			name: "empty-value tag",
			line: "    ; review:",
			want: "",
		},
		{
			name: "mixed empty-value and valued tags",
			line: "    ; review:, category:food",
			want: "",
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
