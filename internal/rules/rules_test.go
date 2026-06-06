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
			got := Match(rules, tc.description, "")
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

func TestCompilePattern(t *testing.T) {
	// A valid pattern compiles, is case-insensitive, and returns the cached
	// *regexp.Regexp instance on repeated calls.
	re1, err := CompilePattern("starbucks")
	if err != nil {
		t.Fatalf("CompilePattern(valid) error: %v", err)
	}
	if !re1.MatchString("STARBUCKS #42") {
		t.Error("compiled pattern is not case-insensitive")
	}
	re2, err := CompilePattern("starbucks")
	if err != nil {
		t.Fatalf("CompilePattern(valid) second call error: %v", err)
	}
	if re1 != re2 {
		t.Error("CompilePattern did not return the cached instance on repeated calls")
	}

	// An invalid pattern returns an error, and the error is cached (same error
	// value) rather than recompiled each time.
	_, err1 := CompilePattern("[invalid")
	if err1 == nil {
		t.Fatal("CompilePattern(invalid) = nil error, want compile error")
	}
	_, err2 := CompilePattern("[invalid")
	if err2 != err1 {
		t.Errorf("CompilePattern(invalid) errors differ across calls: %v vs %v", err1, err2)
	}
}

func TestMatchPriority(t *testing.T) {
	// Lower priority number = higher priority (matched first).
	// Caller must pass rules already sorted by priority (as Load() does).
	rules := []Rule{
		{ID: "high", Pattern: "starbucks", Payee: "Starbucks", Priority: 1},
		{ID: "low", Pattern: "coffee", Payee: "Generic Coffee", Priority: 100},
	}

	got := Match(rules, "STARBUCKS COFFEE", "")
	if got == nil || got.ID != "high" {
		t.Errorf("Match = %v, want 'high'", got)
	}
}

func TestMatchAccount(t *testing.T) {
	rules := []Rule{
		{ID: "scoped", Pattern: "payment", Payee: "Rent", Priority: 1, MatchAccount: "assets:checking"},
		{ID: "unscoped", Pattern: "payment", Payee: "Generic", Priority: 2},
	}

	tests := []struct {
		description string
		account     string
		wantID      string
	}{
		// Scoped rule wins for the matching account.
		{"PAYMENT", "assets:checking", "scoped"},
		// Prefix match: child account also qualifies.
		{"PAYMENT", "assets:checking:bank1", "scoped"},
		// Different account falls through to the unscoped rule.
		{"PAYMENT", "assets:savings", "unscoped"},
		// No account provided: only unscoped rule matches.
		{"PAYMENT", "", "unscoped"},
		// No match at all.
		{"OTHER", "assets:checking", ""},
	}

	for _, tc := range tests {
		t.Run(tc.description+"/"+tc.account, func(t *testing.T) {
			got := Match(rules, tc.description, tc.account)
			if tc.wantID == "" {
				if got != nil {
					t.Errorf("Match(%q, %q) = %v, want nil", tc.description, tc.account, got.ID)
				}
			} else {
				if got == nil {
					t.Errorf("Match(%q, %q) = nil, want %q", tc.description, tc.account, tc.wantID)
				} else if got.ID != tc.wantID {
					t.Errorf("Match(%q, %q) = %q, want %q", tc.description, tc.account, got.ID, tc.wantID)
				}
			}
		})
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
	sp := func(s string) *string { return &s }

	amazon := Rule{ID: "amazon", Pattern: "amazon", Payee: "Amazon", Account: "expenses:shopping", Priority: 1}
	netflix := Rule{ID: "netflix", Pattern: "netflix", Account: "expenses:entertainment", AutoReviewed: true, Priority: 2}
	tagger := Rule{ID: "tagger", Pattern: "whole foods", Tags: map[string]string{"store": "wf"}, Priority: 3}

	tests := []struct {
		name     string
		rules    []Rule
		txns     []hledger.Transaction
		wantFIDs []string // FIDs of expected matches, in order
		// per-match assertions keyed by FID
		wantPayee        map[string]*string
		wantAccount      map[string]*string
		wantTags         map[string]map[string]string
		wantMarkReviewed map[string]bool
	}{
		{
			name:     "match_payee_and_account",
			rules:    []Rule{amazon},
			txns:     []hledger.Transaction{{FID: "aabb1122", Description: "AMAZON.COM", Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:unknown"}}}},
			wantFIDs: []string{"aabb1122"},
			wantPayee:   map[string]*string{"aabb1122": sp("Amazon")},
			wantAccount: map[string]*string{"aabb1122": sp("expenses:shopping")},
		},
		{
			name:     "no_match_unrecognized_description",
			rules:    []Rule{amazon},
			txns:     []hledger.Transaction{{FID: "aabb1122", Description: "UNKNOWN MERCHANT", Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:unknown"}}}},
			wantFIDs: []string{},
		},
		{
			name:     "skip_transaction_without_fid",
			rules:    []Rule{amazon},
			txns:     []hledger.Transaction{{Description: "AMAZON.COM", Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:unknown"}}}},
			wantFIDs: []string{},
		},
		{
			name: "skip_when_no_changes_needed",
			rules: []Rule{amazon},
			txns: []hledger.Transaction{{
				FID: "aabb1122", Description: "AMAZON.COM",
				Payee: sp("Amazon"),
				Postings: []hledger.Posting{
					{Account: "assets:checking"},
					{Account: "expenses:shopping"}, // already correct
				},
			}},
			wantFIDs: []string{},
		},
		{
			name:             "auto_reviewed_on_uncleared",
			rules:            []Rule{netflix},
			txns:             []hledger.Transaction{{FID: "cc112233", Description: "NETFLIX", Status: "", Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:unknown"}}}},
			wantFIDs:         []string{"cc112233"},
			wantAccount:      map[string]*string{"cc112233": sp("expenses:entertainment")},
			wantMarkReviewed: map[string]bool{"cc112233": true},
		},
		{
			name:  "auto_reviewed_skipped_when_already_cleared_but_other_changes_remain",
			rules: []Rule{netflix},
			txns: []hledger.Transaction{{
				FID: "cc112233", Description: "NETFLIX", Status: "Cleared",
				Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:unknown"}},
			}},
			wantFIDs:         []string{"cc112233"},
			wantAccount:      map[string]*string{"cc112233": sp("expenses:entertainment")},
			wantMarkReviewed: map[string]bool{"cc112233": false}, // already cleared
		},
		{
			name:     "tags_only_match",
			rules:    []Rule{tagger},
			txns:     []hledger.Transaction{{FID: "dd445566", Description: "WHOLE FOODS MARKET", Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:food"}}}},
			wantFIDs: []string{"dd445566"},
			wantTags: map[string]map[string]string{"dd445566": {"store": "wf"}},
		},
		{
			name:     "multiple_transactions_only_matching_ones_returned",
			rules:    []Rule{amazon},
			txns: []hledger.Transaction{
				{FID: "aabb1122", Description: "AMAZON.COM", Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:unknown"}}},
				{FID: "ccdd3344", Description: "STARBUCKS", Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:unknown"}}},
				{FID: "eeff5566", Description: "AMAZON PRIME", Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:unknown"}}},
			},
			wantFIDs:    []string{"aabb1122", "eeff5566"},
			wantPayee:   map[string]*string{"aabb1122": sp("Amazon"), "eeff5566": sp("Amazon")},
			wantAccount: map[string]*string{"aabb1122": sp("expenses:shopping"), "eeff5566": sp("expenses:shopping")},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			matches := Preview(tc.rules, tc.txns)

			gotFIDs := make([]string, len(matches))
			for i, m := range matches {
				gotFIDs[i] = m.Transaction.FID
			}

			if len(gotFIDs) != len(tc.wantFIDs) {
				t.Fatalf("Preview returned FIDs %v, want %v", gotFIDs, tc.wantFIDs)
			}
			for i, fid := range tc.wantFIDs {
				if gotFIDs[i] != fid {
					t.Errorf("match[%d].FID = %q, want %q", i, gotFIDs[i], fid)
				}
			}

			matchByFID := make(map[string]RuleMatch, len(matches))
			for _, m := range matches {
				matchByFID[m.Transaction.FID] = m
			}

			for fid, wantP := range tc.wantPayee {
				m := matchByFID[fid]
				if wantP == nil {
					if m.Changes.NewPayee != nil {
						t.Errorf("[%s] NewPayee = %q, want nil", fid, *m.Changes.NewPayee)
					}
				} else if m.Changes.NewPayee == nil || *m.Changes.NewPayee != *wantP {
					t.Errorf("[%s] NewPayee = %v, want %q", fid, m.Changes.NewPayee, *wantP)
				}
			}

			for fid, wantA := range tc.wantAccount {
				m := matchByFID[fid]
				if wantA == nil {
					if m.Changes.NewAccount != nil {
						t.Errorf("[%s] NewAccount = %q, want nil", fid, *m.Changes.NewAccount)
					}
				} else if m.Changes.NewAccount == nil || *m.Changes.NewAccount != *wantA {
					t.Errorf("[%s] NewAccount = %v, want %q", fid, m.Changes.NewAccount, *wantA)
				}
			}

			for fid, wantT := range tc.wantTags {
				m := matchByFID[fid]
				for k, v := range wantT {
					if m.Changes.AddTags[k] != v {
						t.Errorf("[%s] AddTags[%q] = %q, want %q", fid, k, m.Changes.AddTags[k], v)
					}
				}
			}

			for fid, wantR := range tc.wantMarkReviewed {
				m := matchByFID[fid]
				if wantR {
					if m.Changes.MarkReviewed == nil || !*m.Changes.MarkReviewed {
						t.Errorf("[%s] MarkReviewed = %v, want &true", fid, m.Changes.MarkReviewed)
					}
				} else {
					if m.Changes.MarkReviewed != nil {
						t.Errorf("[%s] MarkReviewed = &%v, want nil", fid, *m.Changes.MarkReviewed)
					}
				}
			}
		})
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
