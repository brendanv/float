package rules

import (
	"strings"
	"testing"
	"time"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/testgen"
)

func mustClient(t *testing.T, dataDir string) *hledger.Client {
	t.Helper()
	c, err := hledger.New("hledger", dataDir+"/main.journal")
	if err != nil {
		t.Skipf("hledger unavailable: %v", err)
	}
	return c
}

// userTagMap extracts non-float- tags from a transaction into a plain map.
func userTagMap(txn hledger.Transaction) map[string]string {
	m := make(map[string]string)
	for _, kv := range txn.Tags {
		if !strings.HasPrefix(kv[0], hledger.HiddenMetaPrefix) {
			m[kv[0]] = kv[1]
		}
	}
	return m
}

func TestBuildChangeSet(t *testing.T) {
	sp := func(s string) *string { return &s }

	tests := []struct {
		name             string
		rule             Rule
		txn              hledger.Transaction
		wantPayee        *string
		wantAccount      *string
		wantTags         map[string]string // nil or empty = no tag changes expected
		wantMarkReviewed bool              // true = MarkReviewed should be &true
	}{
		// ── Payee ────────────────────────────────────────────────────────────────
		{
			name: "payee_changed",
			rule: Rule{Payee: "Amazon"},
			txn: hledger.Transaction{
				Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:shopping"}},
			},
			wantPayee: sp("Amazon"),
		},
		{
			name: "payee_already_matches",
			rule: Rule{Payee: "Amazon"},
			txn: hledger.Transaction{
				Payee:    sp("Amazon"),
				Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:shopping"}},
			},
			// no change expected
		},
		{
			name: "payee_changed_from_existing",
			rule: Rule{Payee: "New Payee"},
			txn: hledger.Transaction{
				Payee:    sp("Old Payee"),
				Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:shopping"}},
			},
			wantPayee: sp("New Payee"),
		},

		// ── Account ──────────────────────────────────────────────────────────────
		{
			name: "account_changed",
			rule: Rule{Account: "expenses:groceries"},
			txn: hledger.Transaction{
				Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:unknown"}},
			},
			wantAccount: sp("expenses:groceries"),
		},
		{
			name: "account_already_matches",
			rule: Rule{Account: "expenses:groceries"},
			txn: hledger.Transaction{
				Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:groceries"}},
			},
			// no change expected
		},
		{
			name: "account_skipped_for_3_posting_transaction",
			rule: Rule{Account: "expenses:food"},
			txn: hledger.Transaction{
				Postings: []hledger.Posting{
					{Account: "assets:checking"},
					{Account: "expenses:food:tax"},
					{Account: "expenses:food:tip"},
				},
			},
			// 3 postings → categoryPostingIndex == -1 → no account change
		},
		{
			name: "account_skipped_for_ambiguous_2_posting",
			rule: Rule{Account: "expenses:food"},
			txn: hledger.Transaction{
				Postings: []hledger.Posting{
					{Account: "assets:checking"},
					{Account: "assets:savings"},
				},
			},
			// both look like assets → no category posting → no account change
		},

		// ── Payee + Account ───────────────────────────────────────────────────────
		{
			name: "payee_and_account_changed",
			rule: Rule{Payee: "Whole Foods", Account: "expenses:groceries"},
			txn: hledger.Transaction{
				Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:unknown"}},
			},
			wantPayee:   sp("Whole Foods"),
			wantAccount: sp("expenses:groceries"),
		},

		// ── Tags ──────────────────────────────────────────────────────────────────
		{
			name: "tags_all_new",
			rule: Rule{Tags: map[string]string{"category": "food", "source": "auto"}},
			txn: hledger.Transaction{
				Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:food"}},
			},
			wantTags: map[string]string{"category": "food", "source": "auto"},
		},
		{
			name: "tags_partially_overlap",
			rule: Rule{Tags: map[string]string{"category": "food", "new": "value"}},
			txn: hledger.Transaction{
				Tags:     [][2]string{{"category", "food"}},
				Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:food"}},
			},
			wantTags: map[string]string{"new": "value"}, // "category" already matches
		},
		{
			name: "tags_all_already_match",
			rule: Rule{Tags: map[string]string{"category": "food"}},
			txn: hledger.Transaction{
				Tags:     [][2]string{{"category", "food"}},
				Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:food"}},
			},
			// no tag changes expected
		},
		{
			name: "tags_with_different_value_for_existing_key",
			rule: Rule{Tags: map[string]string{"category": "groceries"}},
			txn: hledger.Transaction{
				Tags:     [][2]string{{"category", "food"}},
				Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:food"}},
			},
			wantTags: map[string]string{"category": "groceries"},
		},

		// ── AutoReviewed / MarkReviewed ──────────────────────────────────────────
		{
			name: "auto_reviewed_when_unmarked",
			rule: Rule{AutoReviewed: true},
			txn: hledger.Transaction{
				Status:   "",
				Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:food"}},
			},
			wantMarkReviewed: true,
		},
		{
			name: "auto_reviewed_when_pending",
			rule: Rule{AutoReviewed: true},
			txn: hledger.Transaction{
				Status:   "Pending",
				Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:food"}},
			},
			wantMarkReviewed: true,
		},
		{
			name: "auto_reviewed_skipped_when_already_cleared",
			rule: Rule{AutoReviewed: true},
			txn: hledger.Transaction{
				Status:   "Cleared",
				Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:food"}},
			},
			// already Cleared → no MarkReviewed change
		},
		{
			name: "auto_reviewed_false_has_no_effect",
			rule: Rule{AutoReviewed: false},
			txn: hledger.Transaction{
				Status:   "",
				Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:food"}},
			},
		},

		// ── Combined ──────────────────────────────────────────────────────────────
		{
			name: "all_changes",
			rule: Rule{
				Payee:        "Amazon",
				Account:      "expenses:shopping",
				Tags:         map[string]string{"store": "amazon"},
				AutoReviewed: true,
			},
			txn: hledger.Transaction{
				Status:   "",
				Postings: []hledger.Posting{{Account: "assets:checking"}, {Account: "expenses:unknown"}},
			},
			wantPayee:        sp("Amazon"),
			wantAccount:      sp("expenses:shopping"),
			wantTags:         map[string]string{"store": "amazon"},
			wantMarkReviewed: true,
		},
		{
			name: "no_changes_when_everything_already_matches",
			rule: Rule{
				Payee:        "Amazon",
				Account:      "expenses:shopping",
				Tags:         map[string]string{"store": "amazon"},
				AutoReviewed: true,
			},
			txn: hledger.Transaction{
				Payee:  sp("Amazon"),
				Status: "Cleared",
				Tags:   [][2]string{{"store", "amazon"}},
				Postings: []hledger.Posting{
					{Account: "assets:checking"},
					{Account: "expenses:shopping"},
				},
			},
			// all already match → empty ChangeSet, hasChanges == false
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cs := buildChangeSet(tc.rule, tc.txn)

			if tc.wantPayee == nil {
				if cs.NewPayee != nil {
					t.Errorf("NewPayee = %q, want nil", *cs.NewPayee)
				}
			} else {
				if cs.NewPayee == nil {
					t.Errorf("NewPayee = nil, want %q", *tc.wantPayee)
				} else if *cs.NewPayee != *tc.wantPayee {
					t.Errorf("NewPayee = %q, want %q", *cs.NewPayee, *tc.wantPayee)
				}
			}

			if tc.wantAccount == nil {
				if cs.NewAccount != nil {
					t.Errorf("NewAccount = %q, want nil", *cs.NewAccount)
				}
			} else {
				if cs.NewAccount == nil {
					t.Errorf("NewAccount = nil, want %q", *tc.wantAccount)
				} else if *cs.NewAccount != *tc.wantAccount {
					t.Errorf("NewAccount = %q, want %q", *cs.NewAccount, *tc.wantAccount)
				}
			}

			if len(tc.wantTags) == 0 {
				if len(cs.AddTags) != 0 {
					t.Errorf("AddTags = %v, want empty", cs.AddTags)
				}
			} else {
				for k, v := range tc.wantTags {
					if cs.AddTags[k] != v {
						t.Errorf("AddTags[%q] = %q, want %q (full map: %v)", k, cs.AddTags[k], v, cs.AddTags)
					}
				}
				if len(cs.AddTags) != len(tc.wantTags) {
					t.Errorf("AddTags has %d entries, want %d: %v", len(cs.AddTags), len(tc.wantTags), cs.AddTags)
				}
			}

			if tc.wantMarkReviewed {
				if cs.MarkReviewed == nil || !*cs.MarkReviewed {
					t.Errorf("MarkReviewed = %v, want &true", cs.MarkReviewed)
				}
			} else {
				if cs.MarkReviewed != nil {
					t.Errorf("MarkReviewed = &%v, want nil", *cs.MarkReviewed)
				}
			}
		})
	}
}

func TestApplyMatch(t *testing.T) {
	sp := func(s string) *string { return &s }

	testAccounts := testgen.Options{
		Seed:    99,
		NumTxns: 1,
		Accounts: []string{
			"assets:checking",
			"expenses:food",
			"expenses:shopping",
			"expenses:entertainment",
			"income:salary",
		},
		WithFIDs: true,
	}

	baseInput := func(desc, acct string) journal.TransactionInput {
		return journal.TransactionInput{
			Date:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			Description: desc,
			Postings: []journal.PostingInput{
				{Account: "assets:checking", Commodity: "USD", Quantity: "50.00"},
				{Account: acct},
			},
		}
	}

	tests := []struct {
		name        string
		input       journal.TransactionInput
		rule        Rule
		wantPayee   *string           // nil = no Payee (no pipe in description)
		wantNote    *string           // nil = no Note
		wantAccount string            // expected category account after apply
		wantTags    map[string]string // tags that must all be present in result
		wantStatus  string
	}{
		{
			name:        "payee_only",
			input:       baseInput("AMAZON.COM", "expenses:food"),
			rule:        Rule{ID: "r1", Payee: "Amazon"},
			wantPayee:   sp("Amazon"),
			wantNote:    sp("AMAZON.COM"),
			wantAccount: "expenses:food",
		},
		{
			name:        "account_only",
			input:       baseInput("WHOLE FOODS", "expenses:food"),
			rule:        Rule{ID: "r1", Account: "expenses:shopping"},
			wantAccount: "expenses:shopping",
		},
		{
			name:        "payee_and_account",
			input:       baseInput("WHOLE FOODS", "expenses:food"),
			rule:        Rule{ID: "r1", Payee: "Whole Foods", Account: "expenses:shopping"},
			wantPayee:   sp("Whole Foods"),
			wantNote:    sp("WHOLE FOODS"),
			wantAccount: "expenses:shopping",
		},
		{
			name:        "tags_only",
			input:       baseInput("NETFLIX", "expenses:entertainment"),
			rule:        Rule{ID: "r1", Tags: map[string]string{"subscription": "streaming"}},
			wantAccount: "expenses:entertainment",
			wantTags:    map[string]string{"subscription": "streaming"},
		},
		{
			name:        "payee_and_tags",
			input:       baseInput("NETFLIX", "expenses:entertainment"),
			rule:        Rule{ID: "r1", Payee: "Netflix", Tags: map[string]string{"subscription": "yes"}},
			wantPayee:   sp("Netflix"),
			wantNote:    sp("NETFLIX"),
			wantAccount: "expenses:entertainment",
			wantTags:    map[string]string{"subscription": "yes"},
		},
		{
			name:        "mark_reviewed_only",
			input:       baseInput("AMAZON.COM", "expenses:food"),
			rule:        Rule{ID: "r1", AutoReviewed: true},
			wantAccount: "expenses:food",
			wantStatus:  "Cleared",
		},
		{
			name: "all_changes",
			input: baseInput("AMAZON.COM", "expenses:food"),
			rule: Rule{
				ID:           "r1",
				Payee:        "Amazon",
				Account:      "expenses:shopping",
				Tags:         map[string]string{"store": "amazon"},
				AutoReviewed: true,
			},
			wantPayee:   sp("Amazon"),
			wantNote:    sp("AMAZON.COM"),
			wantAccount: "expenses:shopping",
			wantTags:    map[string]string{"store": "amazon"},
			wantStatus:  "Cleared",
		},
		{
			name: "tags_merged_with_existing",
			input: func() journal.TransactionInput {
				i := baseInput("AMAZON.COM", "expenses:food")
				i.Tags = map[string]string{"existing": "yes"}
				return i
			}(),
			rule:        Rule{ID: "r1", Tags: map[string]string{"new": "tag"}},
			wantAccount: "expenses:food",
			wantTags:    map[string]string{"existing": "yes", "new": "tag"},
		},
		{
			name:        "reviewed_and_tags_but_not_payee_or_account",
			input:       baseInput("AMAZON.COM", "expenses:food"),
			rule:        Rule{ID: "r1", Tags: map[string]string{"auto": "yes"}, AutoReviewed: true},
			wantAccount: "expenses:food",
			wantTags:    map[string]string{"auto": "yes"},
			wantStatus:  "Cleared",
		},
		{
			name:        "payee_update_preserves_note",
			input:       baseInput("Old Payee | original note", "expenses:food"),
			rule:        Rule{ID: "r1", Payee: "New Payee"},
			wantPayee:   sp("New Payee"),
			wantNote:    sp("original note"),
			wantAccount: "expenses:food",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := testgen.GenerateDataDir(t, testAccounts)
			client := mustClient(t, dir)

			fid, err := journal.AppendTransaction(t.Context(), client, dir, tc.input)
			if err != nil {
				t.Fatalf("AppendTransaction: %v", err)
			}

			initial, err := client.Transactions(t.Context(), "code:"+fid)
			if err != nil || len(initial) != 1 {
				t.Fatalf("initial fetch: err=%v len=%d", err, len(initial))
			}

			cs := buildChangeSet(tc.rule, initial[0])
			m := RuleMatch{Rule: tc.rule, Transaction: initial[0], Changes: cs}
			if err := applyMatch(t.Context(), client, dir, m); err != nil {
				t.Fatalf("applyMatch: %v", err)
			}

			results, err := client.Transactions(t.Context(), "code:"+fid)
			if err != nil || len(results) != 1 {
				t.Fatalf("result fetch: err=%v len=%d", err, len(results))
			}
			result := results[0]

			if tc.wantPayee == nil {
				if result.Payee != nil {
					t.Errorf("Payee = %q, want nil", *result.Payee)
				}
			} else {
				if result.Payee == nil {
					t.Errorf("Payee = nil, want %q", *tc.wantPayee)
				} else if *result.Payee != *tc.wantPayee {
					t.Errorf("Payee = %q, want %q", *result.Payee, *tc.wantPayee)
				}
			}

			if tc.wantNote != nil {
				if result.Note == nil {
					t.Errorf("Note = nil, want %q", *tc.wantNote)
				} else if *result.Note != *tc.wantNote {
					t.Errorf("Note = %q, want %q", *result.Note, *tc.wantNote)
				}
			}

			if tc.wantAccount != "" {
				idx := categoryPostingIndex(result)
				if idx < 0 {
					t.Errorf("no category posting in result")
				} else if result.Postings[idx].Account != tc.wantAccount {
					t.Errorf("account = %q, want %q", result.Postings[idx].Account, tc.wantAccount)
				}
			}

			if len(tc.wantTags) > 0 {
				got := userTagMap(result)
				for k, v := range tc.wantTags {
					if got[k] != v {
						t.Errorf("tag[%q] = %q, want %q (all tags: %v)", k, got[k], v, got)
					}
				}
			}

			// hledger returns "Unmarked" for transactions without a status marker;
			// the rest of the codebase uses "" for unmarked.
			gotStatus := result.Status
			if gotStatus == "Unmarked" {
				gotStatus = ""
			}
			if gotStatus != tc.wantStatus {
				t.Errorf("status = %q, want %q", result.Status, tc.wantStatus)
			}
		})
	}
}
