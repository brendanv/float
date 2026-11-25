package rules

import (
	"strings"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
)

// This file holds the rule effects applied to transactions as they are
// imported (CSV, Stripe, structured import), as opposed to Preview/Apply,
// which rewrite transactions already on disk.
//
// On import, a rule's account replaces every non-asset/liability posting of a
// 2-posting transaction (SetCategoryAccount). Preview/Apply instead change
// only the single posting CategoryPostingIndex picks.

// IsAssetOrLiabilityAccount reports whether account's top-level segment is
// assets, asset, liabilities or liability (case-insensitive). Such postings
// are the "source" (bank) side of a transaction; the rest are its category.
func IsAssetOrLiabilityAccount(account string) bool {
	root, _, _ := strings.Cut(account, ":")
	switch strings.ToLower(root) {
	case "assets", "asset", "liabilities", "liability":
		return true
	}
	return false
}

// SourceAccount returns the first asset/liability account among postings, or
// "" if there is none. It is the account argument to Match.
func SourceAccount(postings []hledger.Posting) string {
	for _, p := range postings {
		if IsAssetOrLiabilityAccount(p.Account) {
			return p.Account
		}
	}
	return ""
}

// InputSourceAccount is SourceAccount for journal posting inputs.
func InputSourceAccount(postings []journal.PostingInput) string {
	for _, p := range postings {
		if IsAssetOrLiabilityAccount(p.Account) {
			return p.Account
		}
	}
	return ""
}

// CategoryPostingIndex returns the index of the category posting (the first
// non-asset/liability posting) of a 2-posting transaction, or -1 if the
// transaction does not have exactly 2 postings or both are asset/liability.
func CategoryPostingIndex(txn hledger.Transaction) int {
	if len(txn.Postings) != 2 {
		return -1
	}
	for i, p := range txn.Postings {
		if !IsAssetOrLiabilityAccount(p.Account) {
			return i
		}
	}
	return -1
}

// SetCategoryAccount sets every non-asset/liability posting to account when
// there are exactly 2 postings; otherwise it does nothing. postings is
// modified in place.
func SetCategoryAccount(postings []journal.PostingInput, account string) {
	if account == "" || len(postings) != 2 {
		return
	}
	for i := range postings {
		if !IsAssetOrLiabilityAccount(postings[i].Account) {
			postings[i].Account = account
		}
	}
}

// ApplyToCandidate returns a copy of an import candidate with the rule effects
// that journal.TxnFingerprint sees applied: the payee prefix on the
// description and the category account. Duplicate detection must fingerprint
// this form, since it is what gets written. The postings slice is copied, so
// t is not mutated. A nil r returns t unchanged.
func ApplyToCandidate(t hledger.Transaction, r *Rule) hledger.Transaction {
	if r == nil {
		return t
	}
	if r.Payee != "" {
		t.Description = r.Payee + " | " + t.Description
	}
	if r.Account != "" && len(t.Postings) == 2 {
		postings := make([]hledger.Posting, len(t.Postings))
		copy(postings, t.Postings)
		for i, p := range postings {
			if !IsAssetOrLiabilityAccount(p.Account) {
				postings[i].Account = r.Account
			}
		}
		t.Postings = postings
	}
	return t
}

// ApplyTagsAndStatus applies the rule effects that journal.TxnFingerprint
// ignores: it adds r's tags (overwriting existing values) and marks the
// transaction Cleared when r.AutoReviewed. A nil r does nothing.
func ApplyTagsAndStatus(in *journal.TransactionInput, r *Rule) {
	if r == nil {
		return
	}
	if len(r.Tags) > 0 {
		if in.Tags == nil {
			in.Tags = make(map[string]string, len(r.Tags))
		}
		for k, v := range r.Tags {
			in.Tags[k] = v
		}
	}
	if r.AutoReviewed {
		in.Status = "Cleared"
	}
}

// ApplyToInput applies every effect of r to a transaction about to be
// imported: the payee prefix, category account, tags and auto-review status.
// in.Postings is modified in place. A nil r does nothing.
func ApplyToInput(in *journal.TransactionInput, r *Rule) {
	if r == nil {
		return
	}
	if r.Payee != "" {
		in.Description = r.Payee + " | " + in.Description
	}
	SetCategoryAccount(in.Postings, r.Account)
	ApplyTagsAndStatus(in, r)
}
