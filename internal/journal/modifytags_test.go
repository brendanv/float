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

func TestModifyTagsPreservesFloatMeta(t *testing.T) {
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 20, NumTxns: 1, WithFIDs: true})
	client := mustHledgerClient(t, dir)

	tx := TransactionInput{
		Date:        time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
		Description: "HIDDEN META PRESERVE TEST",
		Postings: []PostingInput{
			{Account: "expenses:food", Amount: "$10.00"},
			{Account: "assets:checking"},
		},
		FloatMeta: map[string]string{
			"float-import-id": "batch999",
		},
	}
	fid, err := AppendTransaction(t.Context(), client, dir, tx)
	if err != nil {
		t.Fatalf("AppendTransaction: %v", err)
	}

	// Modify user tags — hidden meta must survive.
	if err := ModifyTags(t.Context(), client, dir, fid, map[string]string{"category": "food"}); err != nil {
		t.Fatalf("ModifyTags: %v", err)
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
	tagMap := make(map[string]string)
	for _, kv := range txns[0].Tags {
		tagMap[kv[0]] = kv[1]
	}
	if tagMap["category"] != "food" {
		t.Errorf("category = %q, want %q", tagMap["category"], "food")
	}
	if tagMap["float-import-id"] != "batch999" {
		t.Errorf("float-import-id = %q, want %q (hidden meta should survive ModifyTags)", tagMap["float-import-id"], "batch999")
	}
}

func TestModifyFloatMeta(t *testing.T) {
	t.Run("sets_hidden_meta", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 21, NumTxns: 1, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		tx := TransactionInput{
			Date:        time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
			Description: "SET HIDDEN META TEST",
			Postings: []PostingInput{
				{Account: "expenses:food", Amount: "$12.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := AppendTransaction(t.Context(), client, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		if err := ModifyFloatMeta(t.Context(), client, dir, fid, map[string]string{
			"float-import-id":  "batch42",
			"float-updated-at": "2026-02-05T00:00:00Z",
		}); err != nil {
			t.Fatalf("ModifyFloatMeta: %v", err)
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
		if txns[0].FloatMeta["float-import-id"] != "batch42" {
			t.Errorf("float-import-id = %q, want %q", txns[0].FloatMeta["float-import-id"], "batch42")
		}
		if txns[0].FloatMeta["float-updated-at"] != "2026-02-05T00:00:00Z" {
			t.Errorf("float-updated-at = %q, want %q", txns[0].FloatMeta["float-updated-at"], "2026-02-05T00:00:00Z")
		}
	})

	t.Run("preserves_user_tags", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 22, NumTxns: 1, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		tx := TransactionInput{
			Date:        time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			Description: "PRESERVE USER TAGS TEST",
			Postings: []PostingInput{
				{Account: "expenses:food", Amount: "$8.00"},
				{Account: "assets:checking"},
			},
		}
		fid, err := AppendTransaction(t.Context(), client, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		// Set user tags first.
		if err := ModifyTags(t.Context(), client, dir, fid, map[string]string{"category": "groceries"}); err != nil {
			t.Fatalf("ModifyTags: %v", err)
		}

		// Now set hidden meta — user tags must survive.
		if err := ModifyFloatMeta(t.Context(), client, dir, fid, map[string]string{"float-import-id": "batchABC"}); err != nil {
			t.Fatalf("ModifyFloatMeta: %v", err)
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
		tagMap := make(map[string]string)
		for _, kv := range txns[0].Tags {
			tagMap[kv[0]] = kv[1]
		}
		if tagMap["category"] != "groceries" {
			t.Errorf("category = %q, want %q (user tag should survive ModifyFloatMeta)", tagMap["category"], "groceries")
		}
		if tagMap["float-import-id"] != "batchABC" {
			t.Errorf("float-import-id = %q, want %q", tagMap["float-import-id"], "batchABC")
		}
	})

	t.Run("replaces_existing_hidden_meta", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 23, NumTxns: 1, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		tx := TransactionInput{
			Date:        time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
			Description: "REPLACE HIDDEN META TEST",
			Postings: []PostingInput{
				{Account: "expenses:food", Amount: "$6.00"},
				{Account: "assets:checking"},
			},
			FloatMeta: map[string]string{"float-import-id": "old-batch"},
		}
		fid, err := AppendTransaction(t.Context(), client, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		if err := ModifyFloatMeta(t.Context(), client, dir, fid, map[string]string{"float-import-id": "new-batch"}); err != nil {
			t.Fatalf("ModifyFloatMeta: %v", err)
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
		if txns[0].FloatMeta["float-import-id"] != "new-batch" {
			t.Errorf("float-import-id = %q, want %q", txns[0].FloatMeta["float-import-id"], "new-batch")
		}
	})

	t.Run("clears_hidden_meta", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 24, NumTxns: 1, WithFIDs: true})
		client := mustHledgerClient(t, dir)

		tx := TransactionInput{
			Date:        time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			Description: "CLEAR HIDDEN META TEST",
			Postings: []PostingInput{
				{Account: "expenses:food", Amount: "$5.00"},
				{Account: "assets:checking"},
			},
			FloatMeta: map[string]string{"float-import-id": "will-be-removed"},
		}
		fid, err := AppendTransaction(t.Context(), client, dir, tx)
		if err != nil {
			t.Fatalf("AppendTransaction: %v", err)
		}

		if err := ModifyFloatMeta(t.Context(), client, dir, fid, map[string]string{}); err != nil {
			t.Fatalf("ModifyFloatMeta clear: %v", err)
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
		if txns[0].FloatMeta != nil {
			t.Errorf("expected nil FloatMeta after clearing, got %v", txns[0].FloatMeta)
		}
		for _, kv := range txns[0].Tags {
			if strings.HasPrefix(kv[0], "float-") {
				t.Errorf("float- tag %q should have been removed", kv[0])
			}
		}
	})

	t.Run("not_found", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 25, NumTxns: 1, WithFIDs: true})
		client := mustHledgerClient(t, dir)
		err := ModifyFloatMeta(t.Context(), client, dir, "00000000", map[string]string{"float-x": "y"})
		if err == nil {
			t.Fatal("expected error for non-existent fid, got nil")
		}
		if !strings.Contains(err.Error(), "no transaction found") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestIsFloatMetaLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "single float- tag",
			line: "    ; float-import-id:batch123",
			want: true,
		},
		{
			name: "multiple float- tags comma separated",
			line: "    ; float-import-id:batch123, float-updated-at:2026-01-15",
			want: true,
		},
		{
			name: "user tag only",
			line: "    ; category:food",
			want: false,
		},
		{
			name: "mixed user and hidden meta tag",
			line: "    ; category:food, float-import-id:batch123",
			want: false,
		},
		{
			name: "free text comment",
			line: "    ; some notes here",
			want: false,
		},
		{
			name: "float- tag with text",
			line: "    ; imported batch float-import-id:batch123",
			want: false,
		},
		{
			name: "no semicolon",
			line: "    expenses:food  $10.00",
			want: false,
		},
		{
			name: "float- empty value",
			line: "    ; float-flag:",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isFloatMetaLine(tt.line)
			if got != tt.want {
				t.Errorf("isFloatMetaLine(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
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

func TestStripHeaderInlineComment(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		wantClean     string
		wantFreeText  string
	}{
		{
			name:         "no comment",
			line:         "2026-01-15 (abc12345) Test",
			wantClean:    "2026-01-15 (abc12345) Test",
			wantFreeText: "",
		},
		{
			name:         "tag-only inline comment",
			line:         "2026-01-15 (abc12345) Test  ; category:food",
			wantClean:    "2026-01-15 (abc12345) Test",
			wantFreeText: "",
		},
		{
			name:         "multiple tags inline comment",
			line:         "2026-01-15 (abc12345) Test  ; category:food, source:manual",
			wantClean:    "2026-01-15 (abc12345) Test",
			wantFreeText: "",
		},
		{
			name:         "free text inline comment moved out",
			line:         "2026-01-15 (abc12345) Test  ; imported",
			wantClean:    "2026-01-15 (abc12345) Test",
			wantFreeText: "imported",
		},
		{
			name:         "mixed free text and tag: free text returned, tag dropped",
			line:         "2026-01-15 (abc12345) Test  ; imported category:food",
			wantClean:    "2026-01-15 (abc12345) Test",
			wantFreeText: "imported",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotClean, gotFreeText := stripHeaderInlineComment(tt.line)
			if gotClean != tt.wantClean {
				t.Errorf("stripHeaderInlineComment() cleanLine = %q, want %q", gotClean, tt.wantClean)
			}
			if gotFreeText != tt.wantFreeText {
				t.Errorf("stripHeaderInlineComment() freeText = %q, want %q", gotFreeText, tt.wantFreeText)
			}
		})
	}
}

func TestModifyTagsOrdering(t *testing.T) {
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 30, NumTxns: 1, WithFIDs: true})
	client := mustHledgerClient(t, dir)

	tx := TransactionInput{
		Date:        time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
		Description: "ORDERING TEST",
		Comment:     "my note",
		Postings: []PostingInput{
			{Account: "expenses:food", Amount: "$10.00"},
			{Account: "assets:checking"},
		},
		FloatMeta: map[string]string{
			"float-import-id": "batch1",
		},
	}
	fid, err := AppendTransaction(t.Context(), client, dir, tx)
	if err != nil {
		t.Fatalf("AppendTransaction: %v", err)
	}

	if err := ModifyTags(t.Context(), client, dir, fid, map[string]string{"category": "food"}); err != nil {
		t.Fatalf("ModifyTags: %v", err)
	}

	if err := client.Check(t.Context()); err != nil {
		t.Fatalf("hledger check: %v", err)
	}

	// Read the raw file and verify line ordering: free-text → user-tag → float-meta → posting.
	txns, err := client.Transactions(t.Context(), "code:"+fid)
	if err != nil {
		t.Fatalf("Transactions: %v", err)
	}
	if len(txns) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(txns))
	}

	sourceFile := txns[0].SourcePos[0].File
	data, err := os.ReadFile(sourceFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	content := string(data)
	noteIdx := strings.Index(content, "my note")
	tagIdx := strings.Index(content, "category:food")
	metaIdx := strings.Index(content, "float-import-id:")
	postingIdx := strings.Index(content, "expenses:food")

	if noteIdx < 0 {
		t.Fatalf("free-text comment 'my note' not found in file:\n%s", content)
	}
	if tagIdx < 0 {
		t.Fatalf("user tag 'category:food' not found in file:\n%s", content)
	}
	if metaIdx < 0 {
		t.Fatalf("float meta 'float-import-id:' not found in file:\n%s", content)
	}
	if postingIdx < 0 {
		t.Fatalf("posting 'expenses:food' not found in file:\n%s", content)
	}

	// Verify canonical ordering: free-text < user-tag < float-meta < posting.
	if noteIdx > tagIdx {
		t.Errorf("free-text comment appears after user tag (want: free-text first):\n%s", content)
	}
	if tagIdx > metaIdx {
		t.Errorf("user tag appears after float-meta (want: user-tag before float-meta):\n%s", content)
	}
	if metaIdx > postingIdx {
		t.Errorf("float-meta appears after posting (want: float-meta before postings):\n%s", content)
	}
}

func TestModifyTagsMovesHeaderInlineComment(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "2026"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte("include 2026/01.journal\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Hand-crafted transaction with a free-text inline comment on the header line.
	if err := os.WriteFile(filepath.Join(dir, "2026/01.journal"),
		[]byte("2026-01-20 (aabbccdd) INLINE NOTE  ; a note here\n    expenses:food  $5.00\n    assets:checking\n"), 0644); err != nil {
		t.Fatal(err)
	}
	client := mustHledgerClient(t, dir)

	if err := ModifyTags(t.Context(), client, dir, "aabbccdd", map[string]string{"category": "food"}); err != nil {
		t.Fatalf("ModifyTags: %v", err)
	}
	if err := client.Check(t.Context()); err != nil {
		t.Fatalf("hledger check: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "2026/01.journal"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	lines := strings.Split(content, "\n")

	// Header line must not contain a semicolon.
	if strings.Contains(lines[0], ";") {
		t.Errorf("header line still has inline comment: %q", lines[0])
	}
	// The note should appear as a separate comment line.
	if !strings.Contains(content, "\n    ; a note here\n") {
		t.Errorf("inline note not moved to separate comment line:\n%s", content)
	}
	// The user tag must also be present.
	if !strings.Contains(content, "category:food") {
		t.Errorf("user tag missing:\n%s", content)
	}
}

func TestModifyFloatMetaMovesHeaderInlineComment(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "2026"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte("include 2026/02.journal\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Hand-crafted transaction with a free-text inline comment on the header line.
	if err := os.WriteFile(filepath.Join(dir, "2026/02.journal"),
		[]byte("2026-02-10 (11223344) INLINE META  ; a legacy note\n    expenses:misc  $3.00\n    assets:checking\n"), 0644); err != nil {
		t.Fatal(err)
	}
	client := mustHledgerClient(t, dir)

	if err := ModifyFloatMeta(t.Context(), client, dir, "11223344", map[string]string{"float-import-id": "batch99"}); err != nil {
		t.Fatalf("ModifyFloatMeta: %v", err)
	}
	if err := client.Check(t.Context()); err != nil {
		t.Fatalf("hledger check: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "2026/02.journal"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	lines := strings.Split(content, "\n")

	// Header line must not contain a semicolon.
	if strings.Contains(lines[0], ";") {
		t.Errorf("header line still has inline comment: %q", lines[0])
	}
	// The note should appear as a separate comment line.
	if !strings.Contains(content, "\n    ; a legacy note\n") {
		t.Errorf("inline note not moved to separate comment line:\n%s", content)
	}
	// The float meta must be present.
	if !strings.Contains(content, "float-import-id:batch99") {
		t.Errorf("float-meta missing:\n%s", content)
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
