package rules

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/brendanv/float/internal/hledger"
)

func TestMatch(t *testing.T) {
	rules := []Rule{
		{ID: "rule1", Pattern: "amazon", Payee: "Amazon", Priority: 10},
		{ID: "rule2", Pattern: "STARBUCKS", Payee: "Starbucks", Account: "expenses:coffee", Priority: 5},
		{ID: "rule3", Pattern: "^Whole Foods", Payee: "Whole Foods", Account: "expenses:groceries", Priority: 1},
		{ID: "rule4", Pattern: "[invalid", Payee: "Bad Rule", Priority: 20}, // invalid regex — should be skipped
	}

	tests := []struct {
		description string
		wantID      string
	}{
		{"AMAZON.COM purchase", "rule1"},
		{"Amazon Prime renewal", "rule1"},
		{"STARBUCKS #1234", "rule2"},
		{"Starbucks Coffee", "rule2"},
		{"Whole Foods Market", "rule3"},
		{"not whole foods", ""},       // doesn't match ^Whole Foods
		{"Unknown merchant", ""},
		{"starbucks daily", "rule2"},  // case-insensitive
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			got := Match(rules, tc.description)
			if tc.wantID == "" {
				if got != nil {
					t.Errorf("Match(%q) = %v, want nil", tc.description, got.ID)
				}
			} else {
				if got == nil {
					t.Errorf("Match(%q) = nil, want %q", tc.description, tc.wantID)
				} else if got.ID != tc.wantID {
					t.Errorf("Match(%q) = %q, want %q", tc.description, got.ID, tc.wantID)
				}
			}
		})
	}
}

func TestMatchPriority(t *testing.T) {
	// Lower priority number = higher priority (matched first).
	// Caller must pass rules already sorted by priority (as Load() does).
	rules := []Rule{
		{ID: "high", Pattern: "starbucks", Payee: "Starbucks", Priority: 1},
		{ID: "low", Pattern: "coffee", Payee: "Generic Coffee", Priority: 100},
	}

	got := Match(rules, "STARBUCKS COFFEE")
	if got == nil || got.ID != "high" {
		t.Errorf("Match = %v, want 'high'", got)
	}
}

func TestLoadSave(t *testing.T) {
	dir := t.TempDir()

	// Load from missing file returns empty slice (not error).
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load empty dir: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Load empty dir = %v, want []", got)
	}

	// Save and reload.
	input := []Rule{
		{ID: "aabbccdd", Pattern: "amazon", Payee: "Amazon", Account: "expenses:shopping", Tags: map[string]string{"source": "import"}, Priority: 5},
		{ID: "11223344", Pattern: "starbucks", Payee: "Starbucks", Priority: 10},
	}
	if err := Save(dir, input); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// File must exist.
	if _, err := os.Stat(filepath.Join(dir, "rules.json")); err != nil {
		t.Fatalf("rules.json not created: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("Load returned %d rules, want 2", len(loaded))
	}
	if loaded[0].ID != "aabbccdd" || loaded[0].Payee != "Amazon" {
		t.Errorf("loaded[0] = %+v", loaded[0])
	}
}

func TestLoadSortsByPriority(t *testing.T) {
	dir := t.TempDir()
	input := []Rule{
		{ID: "b", Pattern: "b", Priority: 10},
		{ID: "a", Pattern: "a", Priority: 1},
		{ID: "c", Pattern: "c", Priority: 5},
	}
	if err := Save(dir, input); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "c", "b"}
	for i, r := range loaded {
		if r.ID != want[i] {
			t.Errorf("loaded[%d].ID = %q, want %q", i, r.ID, want[i])
		}
	}
}

func TestPreview(t *testing.T) {
	rules := []Rule{
		{ID: "r1", Pattern: "amazon", Payee: "Amazon", Account: "expenses:shopping", Priority: 1},
	}

	txns := []hledger.Transaction{
		{
			FID:         "aabb1122",
			Description: "AMAZON.COM purchase",
			Postings: []hledger.Posting{
				{Account: "assets:checking"},
				{Account: "expenses:unknown"},
			},
		},
		{
			FID:         "ccdd3344",
			Description: "STARBUCKS",
			Postings: []hledger.Posting{
				{Account: "assets:checking"},
				{Account: "expenses:unknown"},
			},
		},
		{
			// No FID — should be skipped.
			Description: "AMAZON no fid",
			Postings: []hledger.Posting{
				{Account: "assets:checking"},
				{Account: "expenses:unknown"},
			},
		},
	}

	matches := Preview(rules, txns)
	if len(matches) != 1 {
		t.Fatalf("Preview returned %d matches, want 1", len(matches))
	}
	if matches[0].Transaction.FID != "aabb1122" {
		t.Errorf("match FID = %q, want aabb1122", matches[0].Transaction.FID)
	}
	if matches[0].Changes.NewPayee == nil || *matches[0].Changes.NewPayee != "Amazon" {
		t.Errorf("NewPayee = %v, want 'Amazon'", matches[0].Changes.NewPayee)
	}
	if matches[0].Changes.NewAccount == nil || *matches[0].Changes.NewAccount != "expenses:shopping" {
		t.Errorf("NewAccount = %v, want 'expenses:shopping'", matches[0].Changes.NewAccount)
	}
}

func TestCategoryPostingIndex(t *testing.T) {
	tests := []struct {
		name     string
		txn      hledger.Transaction
		wantIdx  int
	}{
		{
			name: "standard 2-posting (asset + expense)",
			txn: hledger.Transaction{Postings: []hledger.Posting{
				{Account: "assets:checking"},
				{Account: "expenses:food"},
			}},
			wantIdx: 1,
		},
		{
			name: "reversed (expense first)",
			txn: hledger.Transaction{Postings: []hledger.Posting{
				{Account: "expenses:food"},
				{Account: "assets:checking"},
			}},
			wantIdx: 0,
		},
		{
			name: "3-posting — ambiguous",
			txn: hledger.Transaction{Postings: []hledger.Posting{
				{Account: "assets:checking"},
				{Account: "expenses:food"},
				{Account: "expenses:tax"},
			}},
			wantIdx: -1,
		},
		{
			name: "both assets — ambiguous",
			txn: hledger.Transaction{Postings: []hledger.Posting{
				{Account: "assets:checking"},
				{Account: "assets:savings"},
			}},
			wantIdx: -1,
		},
		{
			name: "liabilities + expense",
			txn: hledger.Transaction{Postings: []hledger.Posting{
				{Account: "liabilities:creditcard"},
				{Account: "expenses:shopping"},
			}},
			wantIdx: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := categoryPostingIndex(tc.txn)
			if got != tc.wantIdx {
				t.Errorf("categoryPostingIndex = %d, want %d", got, tc.wantIdx)
			}
		})
	}
}
