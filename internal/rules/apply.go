package rules

import (
	"context"
	"fmt"
	"strings"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/slogctx"
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

// sourceAccount returns the source (asset/liability) account of a transaction,
// i.e. the first posting whose account name looks like an asset or liability.
// Returns "" if no such posting is found.
func sourceAccount(txn hledger.Transaction) string {
	for _, p := range txn.Postings {
		if isAssetOrLiabilityAccount(p.Account) {
			return p.Account
		}
	}
	return ""
}

// Preview checks all transactions against rules and returns proposed changes.
// Does NOT modify anything. Only transactions that match a rule are included.
func Preview(rules []Rule, transactions []hledger.Transaction) []RuleMatch {
	var matches []RuleMatch
	for _, txn := range transactions {
		if txn.FID == "" {
			continue // skip untagged transactions — can't update them
		}
		rule := Match(rules, txn.Description, sourceAccount(txn))
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
	var applied int32
	err := ApplyBatch(ctx, client, dataDir, matches, func(a, _ int32) { applied = a })
	return int(applied), err
}

// ApplyBatch applies a set of RuleMatches efficiently by batching hledger format
// calls and grouping file writes. Must be called within txlock.Do().
// onProgress is called after each file group is written with cumulative counts.
func ApplyBatch(ctx context.Context, client *hledger.Client, dataDir string, matches []RuleMatch, onProgress func(applied, total int32)) error {
	if len(matches) == 0 {
		return nil
	}
	total := int32(len(matches))

	// Build all inputs from existing transaction data — no per-transaction re-fetch.
	type pending struct {
		input journal.TransactionInput
		src   journal.SourceLocation
		fid   string
	}
	items := make([]pending, len(matches))
	for i, m := range matches {
		if len(m.Transaction.SourcePos) == 0 || m.Transaction.SourcePos[0].File == "" {
			// Fall back to the slow path for transactions without source info.
			if err := applyMatch(ctx, client, dataDir, m); err != nil {
				return fmt.Errorf("apply rule %s to txn %s: %w", m.Rule.ID, m.Transaction.FID, err)
			}
			if onProgress != nil {
				onProgress(int32(i+1), total)
			}
			continue
		}
		inp, src, err := buildApplyInput(m)
		if err != nil {
			return fmt.Errorf("build input for txn %s: %w", m.Transaction.FID, err)
		}
		items[i] = pending{input: inp, src: src, fid: m.Transaction.FID}
	}

	// Collect the items that need batch formatting (those with a source file).
	var batchInputs []journal.TransactionInput
	var batchFIDs []string
	var batchIdx []int // index into items
	for i, it := range items {
		if it.fid != "" {
			batchInputs = append(batchInputs, it.input)
			batchFIDs = append(batchFIDs, it.fid)
			batchIdx = append(batchIdx, i)
		}
	}

	if len(batchInputs) == 0 {
		return nil
	}

	// One hledger subprocess for all formatting.
	texts, err := journal.BatchFormatViaHledger(ctx, client, batchInputs, batchFIDs)
	if err != nil {
		return fmt.Errorf("batch format: %w", err)
	}

	// Group replacements by source file.
	type replacement struct {
		src  journal.SourceLocation
		fid  string
		text string
	}
	byFile := make(map[string][]replacement)
	for i, bi := range batchIdx {
		it := items[bi]
		byFile[it.src.File] = append(byFile[it.src.File], replacement{
			src:  it.src,
			fid:  it.fid,
			text: texts[i],
		})
	}

	// Apply all replacements per file in one read+write cycle.
	var applied int32
	for file, reps := range byFile {
		brs := make([]journal.BatchReplacement, len(reps))
		for i, r := range reps {
			brs[i] = journal.BatchReplacement{
				HeaderLine: r.src.Line,
				FID:        r.fid,
				NewText:    r.text,
			}
		}
		if err := journal.BatchReplaceTransactions(file, brs); err != nil {
			return fmt.Errorf("batch replace in %s: %w", file, err)
		}
		applied += int32(len(reps))
		slogctx.FromContext(ctx).Info("rules: batch replaced transactions", "file", file, "count", len(reps))
		if onProgress != nil {
			onProgress(applied, total)
		}
	}
	return nil
}

// buildApplyInput constructs a TransactionInput and SourceLocation from a
// RuleMatch using the data already in m.Transaction. No hledger call is made.
// The caller is responsible for processing matches in descending source-line
// order within each file so that prior writes do not shift later positions.
func buildApplyInput(m RuleMatch) (journal.TransactionInput, journal.SourceLocation, error) {
	t := m.Transaction
	changes := m.Changes

	src := journal.SourceLocation{File: t.SourcePos[0].File, Line: t.SourcePos[0].Line}

	input, err := journal.InputFromTransaction(t)
	if err != nil {
		return journal.TransactionInput{}, journal.SourceLocation{}, err
	}

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

	if changes.MarkReviewed != nil && *changes.MarkReviewed {
		input.Status = "Cleared"
	}

	return input, src, nil
}

// applyMatch applies the changes from a single RuleMatch to the journal.
// Re-fetches the transaction to get a current source location, then delegates
// to buildApplyInput for the change logic. Used as a fallback for transactions
// that lack source position metadata.
func applyMatch(ctx context.Context, client *hledger.Client, dataDir string, m RuleMatch) error {
	txns, err := client.Transactions(ctx, "code:"+m.Transaction.FID)
	if err != nil {
		return fmt.Errorf("lookup fid %q: %w", m.Transaction.FID, err)
	}
	switch len(txns) {
	case 0:
		return fmt.Errorf("no transaction found with fid %q", m.Transaction.FID)
	case 1:
		// expected
	default:
		return fmt.Errorf("fid %q matched %d transactions (corrupt journal — run audit)", m.Transaction.FID, len(txns))
	}
	fresh := RuleMatch{Rule: m.Rule, Transaction: txns[0], Changes: m.Changes}
	input, src, err := buildApplyInput(fresh)
	if err != nil {
		return err
	}
	_, err = journal.WriteTransaction(ctx, client, dataDir, input, &src)
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
		if idx := categoryPostingIndex(txn); idx >= 0 {
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
