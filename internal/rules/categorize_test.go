package rules

import (
	"maps"
	"testing"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
)

func TestIsAssetOrLiabilityAccount(t *testing.T) {
	tests := []struct {
		account string
		want    bool
	}{
		{"assets", true},
		{"assets:checking", true},
		{"Assets:Checking", true},
		{"asset:checking", true},
		{"asset", true},
		{"liabilities:credit card", true},
		{"LIABILITIES", true},
		{"liability:card", true},
		{"expenses:food", false},
		{"income:salary", false},
		{"equity:transfers", false},
		{"assetsfoo", false},
		{"assetsfoo:bar", false},
		{"liabilitiesx:card", false},
		{"expenses:assets", false},
		{"", false},
	}
	for _, tc := range tests {
		t.Run(tc.account, func(t *testing.T) {
			if got := IsAssetOrLiabilityAccount(tc.account); got != tc.want {
				t.Errorf("IsAssetOrLiabilityAccount(%q) = %v, want %v", tc.account, got, tc.want)
			}
		})
	}
}

func TestSourceAccount(t *testing.T) {
	tests := []struct {
		name     string
		accounts []string
		want     string
	}{
		{"asset first", []string{"assets:checking", "expenses:food"}, "assets:checking"},
		{"asset second", []string{"expenses:food", "liabilities:card"}, "liabilities:card"},
		{"first of two", []string{"assets:a", "assets:b"}, "assets:a"},
		{"none", []string{"expenses:food", "income:salary"}, ""},
		{"empty", nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var ps []hledger.Posting
			var ins []journal.PostingInput
			for _, a := range tc.accounts {
				ps = append(ps, hledger.Posting{Account: a})
				ins = append(ins, journal.PostingInput{Account: a})
			}
			if got := SourceAccount(ps); got != tc.want {
				t.Errorf("SourceAccount = %q, want %q", got, tc.want)
			}
			if got := InputSourceAccount(ins); got != tc.want {
				t.Errorf("InputSourceAccount = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestApplyToCandidate(t *testing.T) {
	base := func(accounts ...string) hledger.Transaction {
		txn := hledger.Transaction{Description: "AMAZON 123"}
		for _, a := range accounts {
			txn.Postings = append(txn.Postings, hledger.Posting{Account: a})
		}
		return txn
	}
	tests := []struct {
		name         string
		txn          hledger.Transaction
		rule         *Rule
		wantDesc     string
		wantAccounts []string
	}{
		{
			name:         "nil rule",
			txn:          base("assets:checking", "expenses:unknown"),
			wantDesc:     "AMAZON 123",
			wantAccounts: []string{"assets:checking", "expenses:unknown"},
		},
		{
			name:         "payee and account",
			txn:          base("assets:checking", "expenses:unknown"),
			rule:         &Rule{Payee: "Amazon", Account: "expenses:shopping"},
			wantDesc:     "Amazon | AMAZON 123",
			wantAccounts: []string{"assets:checking", "expenses:shopping"},
		},
		{
			name:         "account only",
			txn:          base("expenses:unknown", "liabilities:card"),
			rule:         &Rule{Account: "expenses:shopping"},
			wantDesc:     "AMAZON 123",
			wantAccounts: []string{"expenses:shopping", "liabilities:card"},
		},
		{
			name:         "three postings keep accounts",
			txn:          base("assets:checking", "expenses:a", "expenses:b"),
			rule:         &Rule{Payee: "Amazon", Account: "expenses:shopping"},
			wantDesc:     "Amazon | AMAZON 123",
			wantAccounts: []string{"assets:checking", "expenses:a", "expenses:b"},
		},
		{
			name:         "tags and status are not candidate effects",
			txn:          base("assets:checking", "expenses:unknown"),
			rule:         &Rule{Tags: map[string]string{"k": "v"}, AutoReviewed: true},
			wantDesc:     "AMAZON 123",
			wantAccounts: []string{"assets:checking", "expenses:unknown"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orig := make([]hledger.Posting, len(tc.txn.Postings))
			copy(orig, tc.txn.Postings)

			got := ApplyToCandidate(tc.txn, tc.rule)
			if got.Description != tc.wantDesc {
				t.Errorf("Description = %q, want %q", got.Description, tc.wantDesc)
			}
			if len(got.Postings) != len(tc.wantAccounts) {
				t.Fatalf("got %d postings, want %d", len(got.Postings), len(tc.wantAccounts))
			}
			for i, want := range tc.wantAccounts {
				if got.Postings[i].Account != want {
					t.Errorf("posting %d account = %q, want %q", i, got.Postings[i].Account, want)
				}
			}
			for i := range orig {
				if tc.txn.Postings[i].Account != orig[i].Account {
					t.Errorf("input posting %d mutated: %q -> %q", i, orig[i].Account, tc.txn.Postings[i].Account)
				}
			}
		})
	}
}

func TestApplyToInput(t *testing.T) {
	newInput := func() journal.TransactionInput {
		return journal.TransactionInput{
			Description: "COFFEE SHOP",
			Status:      "",
			Tags:        map[string]string{"keep": "1", "over": "old"},
			Postings: []journal.PostingInput{
				{Account: "liabilities:card"},
				{Account: "expenses:unknown"},
			},
		}
	}
	tests := []struct {
		name        string
		rule        *Rule
		wantDesc    string
		wantAccount string
		wantStatus  string
		wantTags    map[string]string
	}{
		{
			name:        "nil rule",
			wantDesc:    "COFFEE SHOP",
			wantAccount: "expenses:unknown",
			wantTags:    map[string]string{"keep": "1", "over": "old"},
		},
		{
			name: "all effects",
			rule: &Rule{
				Payee:        "Cafe",
				Account:      "expenses:coffee",
				Tags:         map[string]string{"over": "new", "add": "x"},
				AutoReviewed: true,
			},
			wantDesc:    "Cafe | COFFEE SHOP",
			wantAccount: "expenses:coffee",
			wantStatus:  "Cleared",
			wantTags:    map[string]string{"keep": "1", "over": "new", "add": "x"},
		},
		{
			name:        "tags only",
			rule:        &Rule{Tags: map[string]string{"add": "x"}},
			wantDesc:    "COFFEE SHOP",
			wantAccount: "expenses:unknown",
			wantTags:    map[string]string{"keep": "1", "over": "old", "add": "x"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := newInput()
			ApplyToInput(&in, tc.rule)
			if in.Description != tc.wantDesc {
				t.Errorf("Description = %q, want %q", in.Description, tc.wantDesc)
			}
			if in.Postings[0].Account != "liabilities:card" {
				t.Errorf("source posting changed to %q", in.Postings[0].Account)
			}
			if in.Postings[1].Account != tc.wantAccount {
				t.Errorf("category account = %q, want %q", in.Postings[1].Account, tc.wantAccount)
			}
			if in.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", in.Status, tc.wantStatus)
			}
			if !maps.Equal(in.Tags, tc.wantTags) {
				t.Errorf("Tags = %v, want %v", in.Tags, tc.wantTags)
			}
		})
	}
}

func TestApplyTagsAndStatusNilTags(t *testing.T) {
	var in journal.TransactionInput
	ApplyTagsAndStatus(&in, &Rule{Tags: map[string]string{"a": "b"}})
	if in.Tags["a"] != "b" {
		t.Errorf("Tags = %v, want a=b", in.Tags)
	}
	if in.Status != "" {
		t.Errorf("Status = %q, want unchanged", in.Status)
	}
}

func TestSetCategoryAccount(t *testing.T) {
	ps := []journal.PostingInput{{Account: "assets:checking"}, {Account: "expenses:unknown"}}
	SetCategoryAccount(ps, "")
	if ps[1].Account != "expenses:unknown" {
		t.Errorf("empty account changed posting to %q", ps[1].Account)
	}
	SetCategoryAccount(ps, "expenses:food")
	if ps[0].Account != "assets:checking" || ps[1].Account != "expenses:food" {
		t.Errorf("postings = %v", ps)
	}
	three := []journal.PostingInput{{Account: "assets:a"}, {Account: "expenses:a"}, {Account: "expenses:b"}}
	SetCategoryAccount(three, "expenses:food")
	if three[1].Account != "expenses:a" || three[2].Account != "expenses:b" {
		t.Errorf("3-posting input changed: %v", three)
	}
}
