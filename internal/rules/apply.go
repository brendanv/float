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
	NewPayee   *string           // nil = no change
	NewAccount *string           // nil = no change (the category posting account)
	AddTags    map[string]string // tags to add (nil or empty = no change)
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

// applyMatch applies the changes from a single RuleMatch to the journal.
func applyMatch(ctx context.Context, client *hledger.Client, dataDir string, m RuleMatch) error {
	txn := m.Transaction
	changes := m.Changes

	// Apply payee and/or account changes together via UpdateTransaction.
	if changes.NewPayee != nil || changes.NewAccount != nil {
		// Build updated description.
		desc := txn.Description
		if changes.NewPayee != nil {
			newPayee := *changes.NewPayee
			// Reconstruct description: "payee | note" or just "note" if payee is cleared.
			if txn.Note != nil {
				if newPayee != "" {
					desc = newPayee + " | " + *txn.Note
				} else {
					desc = *txn.Note
				}
			} else {
				// No note — payee becomes the whole description.
				if newPayee != "" {
					// Preserve any existing note part after "|" from raw description.
					if idx := strings.Index(desc, "|"); idx >= 0 {
						desc = newPayee + " |" + desc[idx+1:]
					} else {
						desc = newPayee + " | " + desc
					}
				}
			}
		}

		// Build updated postings.
		postings := make([]journal.PostingInput, len(txn.Postings))
		for i, p := range txn.Postings {
			acc := p.Account
			if changes.NewAccount != nil && isCategoryPosting(txn, i) {
				acc = *changes.NewAccount
			}
			var amtStr string
			if len(p.Amounts) > 0 {
				a := p.Amounts[0]
				val := float64(a.Quantity.DecimalMantissa)
				for k := 0; k < a.Quantity.DecimalPlaces; k++ {
					val /= 10
				}
				amtStr = fmt.Sprintf("%s%.2f", a.Commodity, val)
			}
			postings[i] = journal.PostingInput{
				Account: acc,
				Amount:  amtStr,
				Comment: strings.TrimSpace(p.Comment),
			}
		}

		_, err := journal.UpdateTransaction(ctx, client, dataDir, txn.FID, desc, "", txn.Comment, postings)
		if err != nil {
			return fmt.Errorf("update transaction: %w", err)
		}
	}

	// Apply tag changes.
	if len(changes.AddTags) > 0 {
		// Merge with existing tags.
		merged := make(map[string]string)
		for _, kv := range txn.Tags {
			if !strings.HasPrefix(kv[0], hledger.HiddenMetaPrefix) {
				merged[kv[0]] = kv[1]
			}
		}
		for k, v := range changes.AddTags {
			merged[k] = v
		}
		if err := journal.ModifyTags(ctx, client, dataDir, txn.FID, merged); err != nil {
			return fmt.Errorf("modify tags: %w", err)
		}
	}

	return nil
}

// buildChangeSet constructs the ChangeSet for applying rule to txn.
func buildChangeSet(rule Rule, txn hledger.Transaction) ChangeSet {
	var cs ChangeSet

	if rule.Payee != "" {
		payee := rule.Payee
		cs.NewPayee = &payee
	}

	if rule.Account != "" {
		// Only applicable to 2-posting transactions with a clear category posting.
		if categoryPostingIndex(txn) >= 0 {
			acc := rule.Account
			cs.NewAccount = &acc
		}
	}

	if len(rule.Tags) > 0 {
		cs.AddTags = rule.Tags
	}

	return cs
}

// hasChanges returns true if cs would change anything.
func hasChanges(cs ChangeSet) bool {
	return cs.NewPayee != nil || cs.NewAccount != nil || len(cs.AddTags) > 0
}

// categoryPostingIndex returns the index of the "category" posting (the
// non-asset/liability posting in a 2-posting transaction), or -1 if the
// transaction is ambiguous (3+ postings, or both postings are same type).
func categoryPostingIndex(txn hledger.Transaction) int {
	if len(txn.Postings) != 2 {
		return -1
	}
	for i, p := range txn.Postings {
		if !isAssetOrLiabilityAccount(p.Account) {
			return i
		}
	}
	return -1 // both look like assets/liabilities
}

// isCategoryPosting returns true if posting i is the category (non-asset/liability) posting.
func isCategoryPosting(txn hledger.Transaction, idx int) bool {
	return categoryPostingIndex(txn) == idx
}

// isAssetOrLiabilityAccount returns true if the account name looks like an
// asset or liability account based on its prefix.
func isAssetOrLiabilityAccount(account string) bool {
	lower := strings.ToLower(account)
	return strings.HasPrefix(lower, "assets") ||
		strings.HasPrefix(lower, "liabilities") ||
		strings.HasPrefix(lower, "asset:") ||
		strings.HasPrefix(lower, "liability:")
}
