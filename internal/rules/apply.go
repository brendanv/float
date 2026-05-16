package rules

import (
	"context"
	"fmt"
	"strings"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
)

// ChangeSet describes the changes a rule would apply to a transaction.
type ChangeSet struct {
	NewPayee     *string           // nil = no change
	NewAccount   *string           // nil = no change (the category posting account)
	AddTags      map[string]string // tags to add (nil or empty = no change)
	MarkReviewed *bool             // nil = no change; true = mark Cleared
}

// RuleMatch pairs a transaction with the rule that matched it and the proposed changes.
type RuleMatch struct {
	Rule        Rule
	Transaction hledger.Transaction
	Changes     ChangeSet
}

// Preview checks all transactions against rules and returns proposed changes.
// Does NOT modify anything. Only transactions that match a rule are included.
func Preview(rules []Rule, transactions []hledger.Transaction) []RuleMatch {
	var matches []RuleMatch
	for _, txn := range transactions {
		if txn.FID == "" {
			continue // skip untagged transactions — can't update them
		}
		rule := Match(rules, txn.Description)
		if rule == nil {
			continue
		}
		changes := buildChangeSet(*rule, txn)
		if !hasChanges(changes) {
			continue
		}
		matches = append(matches, RuleMatch{
			Rule:        *rule,
			Transaction: txn,
			Changes:     changes,
		})
	}
	return matches
}

// Apply executes the changes from a preview. Must be called within txlock.Do().
// Returns the number of transactions successfully modified.
func Apply(ctx context.Context, client *hledger.Client, dataDir string, matches []RuleMatch) (int, error) {
	applied := 0
	for _, m := range matches {
		if err := applyMatch(ctx, client, dataDir, m); err != nil {
			return applied, fmt.Errorf("apply rule %s to txn %s: %w", m.Rule.ID, m.Transaction.FID, err)
		}
		applied++
	}
	return applied, nil
}

// applyMatch applies the changes from a single RuleMatch to the journal in a single write.
func applyMatch(ctx context.Context, client *hledger.Client, dataDir string, m RuleMatch) error {
	txn := m.Transaction
	changes := m.Changes

	// Re-fetch to get the current source location; prior calls in the same batch
	// may have shifted line numbers by removing and re-appending other transactions.
	txns, err := client.Transactions(ctx, "code:"+txn.FID)
	if err != nil {
		return fmt.Errorf("lookup fid %q: %w", txn.FID, err)
	}
	switch len(txns) {
	case 0:
		return fmt.Errorf("no transaction found with fid %q", txn.FID)
	case 1:
		// expected
	default:
		return fmt.Errorf("fid %q matched %d transactions (corrupt journal — run audit)", txn.FID, len(txns))
	}
	t := txns[0]
	src := &journal.SourceLocation{File: t.SourcePos[0].File, Line: t.SourcePos[0].Line}

	input, err := journal.InputFromTransaction(t)
	if err != nil {
		return err
	}

	// Apply payee and/or account changes.
	if changes.NewPayee != nil || changes.NewAccount != nil {
		desc := t.Description
		if changes.NewPayee != nil {
			newPayee := *changes.NewPayee
			if t.Note != nil {
				if newPayee != "" {
					desc = newPayee + " | " + *t.Note
				} else {
					desc = *t.Note
				}
			} else {
				if newPayee != "" {
					if idx := strings.Index(desc, "|"); idx >= 0 {
						desc = newPayee + " |" + desc[idx+1:]
					} else {
						desc = newPayee + " | " + desc
					}
				}
			}
		}
		input.Description = desc

		if changes.NewAccount != nil {
			for i := range input.Postings {
				if isCategoryPosting(t, i) {
					input.Postings[i].Account = *changes.NewAccount
				}
			}
		}
	}

	// Merge new tags into the existing set.
	if len(changes.AddTags) > 0 {
		merged := make(map[string]string)
		for k, v := range input.Tags {
			merged[k] = v
		}
		for k, v := range changes.AddTags {
			merged[k] = v
		}
		input.Tags = merged
	}

	// Apply reviewed status.
	if changes.MarkReviewed != nil && *changes.MarkReviewed {
		input.Status = "Cleared"
	}

	_, err = journal.WriteTransaction(ctx, client, dataDir, input, src)
	return err
}

// buildChangeSet constructs the ChangeSet for applying rule to txn.
// Only includes a change if it differs from the current value.
func buildChangeSet(rule Rule, txn hledger.Transaction) ChangeSet {
	var cs ChangeSet

	if rule.Payee != "" {
		currentPayee := ""
		if txn.Payee != nil {
			currentPayee = *txn.Payee
		}
		if rule.Payee != currentPayee {
			payee := rule.Payee
			cs.NewPayee = &payee
		}
	}

	if rule.Account != "" {
		// Only applicable to 2-posting transactions with a clear category posting.
		if idx := CategoryPostingIndex(txn); idx >= 0 {
			currentAccount := txn.Postings[idx].Account
			if rule.Account != currentAccount {
				acc := rule.Account
				cs.NewAccount = &acc
			}
		}
	}

	if len(rule.Tags) > 0 {
		// Only include tags that are new or have a different value.
		existingTags := make(map[string]string)
		for _, kv := range txn.Tags {
			existingTags[kv[0]] = kv[1]
		}
		newTags := make(map[string]string)
		for k, v := range rule.Tags {
			if existing, ok := existingTags[k]; !ok || existing != v {
				newTags[k] = v
			}
		}
		if len(newTags) > 0 {
			cs.AddTags = newTags
		}
	}

	if rule.AutoReviewed && txn.Status != "Cleared" {
		t := true
		cs.MarkReviewed = &t
	}

	return cs
}

// hasChanges returns true if cs would change anything.
func hasChanges(cs ChangeSet) bool {
	return cs.NewPayee != nil || cs.NewAccount != nil || len(cs.AddTags) > 0 || cs.MarkReviewed != nil
}

// CategoryPostingIndex returns the index of the "category" posting (the
// non-asset/liability posting in a 2-posting transaction), or -1 if the
// transaction is ambiguous (3+ postings, or both postings are same type).
func CategoryPostingIndex(txn hledger.Transaction) int {
	if len(txn.Postings) != 2 {
		return -1
	}
	for i, p := range txn.Postings {
		if !IsAssetOrLiabilityAccount(p.Account) {
			return i
		}
	}
	return -1 // both look like assets/liabilities
}

// isCategoryPosting returns true if posting i is the category (non-asset/liability) posting.
func isCategoryPosting(txn hledger.Transaction, idx int) bool {
	return CategoryPostingIndex(txn) == idx
}

// IsAssetOrLiabilityAccount returns true if the account name looks like an
// asset or liability account based on common prefixes.
func IsAssetOrLiabilityAccount(account string) bool {
	lower := strings.ToLower(account)
	return strings.HasPrefix(lower, "assets") ||
		strings.HasPrefix(lower, "liabilities") ||
		strings.HasPrefix(lower, "asset:") ||
		strings.HasPrefix(lower, "liability:")
}

// ApplyToInput applies a matched rule to a TransactionInput being prepared for
// import. Modifies txInput in place. No-op if r is nil.
func ApplyToInput(txInput *journal.TransactionInput, r *Rule) {
	if r == nil {
		return
	}
	if r.Payee != "" {
		txInput.Description = r.Payee + " | " + txInput.Description
	}
	if r.Account != "" && len(txInput.Postings) == 2 {
		for j, p := range txInput.Postings {
			if !IsAssetOrLiabilityAccount(p.Account) {
				txInput.Postings[j].Account = r.Account
			}
		}
	}
	if len(r.Tags) > 0 {
		if txInput.Tags == nil {
			txInput.Tags = make(map[string]string)
		}
		for k, v := range r.Tags {
			txInput.Tags[k] = v
		}
	}
	if r.AutoReviewed {
		txInput.Status = "Cleared"
	}
}

// ApplyPreviewToTransaction mutates a candidate hledger.Transaction to reflect
// the payee and account changes a rule would produce on import. Used so that
// preview responses show the transformed description and postings. Status and
// tag changes are intentionally omitted — callers track those separately (e.g.
// via MatchedRuleId).
func ApplyPreviewToTransaction(txn *hledger.Transaction, r *Rule) {
	if r == nil {
		return
	}
	if r.Payee != "" {
		txn.Description = r.Payee + " | " + txn.Description
	}
	if r.Account != "" && len(txn.Postings) == 2 {
		for j, p := range txn.Postings {
			if !IsAssetOrLiabilityAccount(p.Account) {
				txn.Postings[j].Account = r.Account
			}
		}
	}
}
