package ledger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"
	"unicode"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/ai"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/slogctx"
	"github.com/brendanv/float/internal/suggestions"
)

const (
	// maxSuggestAccountsTxns bounds one AI prompt; larger backlogs are sent
	// in several batches.
	maxSuggestAccountsTxns = 50
	// maxSuggestAccountsExamples bounds precedents given per transaction.
	maxSuggestAccountsExamples = 5
)

// isUnknownAccount reports whether account is a placeholder that
// BulkEditTransactions' update_unknown_account would replace.
func isUnknownAccount(account string) bool {
	return strings.Contains(strings.ToLower(account), "unknown")
}

// suggestDebounce delays a suggestion pass after a write so a burst of
// writes (an import followed by rule application, say) costs one pass.
const suggestDebounce = 3 * time.Second

// loadSuggestions reads account-suggestions.json into memory. A missing or
// unreadable store starts empty; the file is rewritten on the next pass.
func (h *Handler) loadSuggestions() {
	store := suggestions.Store{}
	if h.dataDir != "" {
		loaded, err := suggestions.Load(h.dataDir)
		if err != nil {
			slog.Warn("load account suggestions failed; starting empty", "error", err)
		} else {
			store = loaded
		}
	}
	h.suggestions.Store(&store)
}

// accountSuggestion returns t's stored suggestion while t still has an
// unknown posting, else nil.
func (h *Handler) accountSuggestion(t hledger.Transaction) *floatv1.AccountSuggestion {
	store := h.suggestions.Load()
	if store == nil || t.FID == "" {
		return nil
	}
	s, ok := (*store)[t.FID]
	if !ok || s.Account == "" || !hasUnknownPosting(t) {
		return nil
	}
	return &floatv1.AccountSuggestion{Account: s.Account, Confidence: s.Confidence, Reasoning: s.Reasoning}
}

func hasUnknownPosting(t hledger.Transaction) bool {
	return slices.ContainsFunc(t.Postings, func(p hledger.Posting) bool { return isUnknownAccount(p.Account) })
}

// WakeAccountSuggester is a txlock.OnCommit hook that schedules a
// StartAccountSuggester pass. It never blocks.
func (h *Handler) WakeAccountSuggester(uint64) {
	select {
	case h.suggestWake <- struct{}{}:
	default:
	}
}

// StartAccountSuggester runs until ctx is done. After each journal write
// (signalled by WakeAccountSuggester, debounced) it asks the AI decision model for accounts for newly written
// transactions that landed in an "unknown" account, and stores the results
// for ListTransactions/GetAccountRegister to show. It never edits postings.
func (h *Handler) StartAccountSuggester(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.suggestWake:
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(suggestDebounce):
		}
		// Writes during the debounce are covered by this pass.
		select {
		case <-h.suggestWake:
		default:
		}
		if err := h.suggestPendingAccounts(ctx); err != nil {
			slogctx.FromContext(ctx).WarnContext(ctx, "account suggestions failed", "error", err)
		}
	}
}

// suggestPendingAccounts runs one pass: it finds unreviewed transactions with
// an unknown posting that have no stored result yet, asks the AI in batches,
// and saves the results (an empty account marks "no suggestion", so nothing
// is sent twice). Entries for transactions that no longer have an unknown
// posting are pruned. Without OPENROUTER_API_KEY it does nothing.
func (h *Handler) suggestPendingAccounts(ctx context.Context) error {
	if os.Getenv("OPENROUTER_API_KEY") == "" || h.dataDir == "" {
		return nil
	}
	store := *h.suggestions.Load()

	all, err := cachedTransactions(ctx, h.cache, h.hl, nil)
	if err != nil {
		return fmt.Errorf("fetch transactions: %w", err)
	}
	unknown := make(map[string]bool)
	var pending []hledger.Transaction
	for _, t := range all {
		if t.FID == "" || !hasUnknownPosting(t) {
			continue
		}
		unknown[t.FID] = true
		if _, done := store[t.FID]; !done && t.Status != "Cleared" {
			pending = append(pending, t)
		}
	}
	var stale []string
	for fid := range store {
		if !unknown[fid] {
			stale = append(stale, fid)
		}
	}
	if len(pending) == 0 && len(stale) == 0 {
		return nil
	}

	var results suggestions.Store
	var aiErr error
	if len(pending) > 0 {
		results, aiErr = h.computeSuggestions(ctx, all, pending)
	}
	if len(results) == 0 && len(stale) == 0 {
		return aiErr
	}

	next := make(suggestions.Store, len(store)+len(results))
	for fid, s := range store {
		next[fid] = s
	}
	for _, fid := range stale {
		delete(next, fid)
	}
	for fid, s := range results {
		next[fid] = s
	}
	err = h.lock.DoConfigPublish(ctx, fmt.Sprintf("store %d account suggestions", len(results)), []string{suggestions.FilePath(h.dataDir)}, func() (func(), error) {
		if err := suggestions.Save(h.dataDir, next); err != nil {
			return nil, err
		}
		return func() { h.suggestions.Store(&next) }, nil
	})
	if err != nil {
		return fmt.Errorf("save suggestions: %w", err)
	}
	slogctx.FromContext(ctx).InfoContext(ctx, "stored account suggestions", "processed", len(results), "pruned", len(stale))
	return aiErr
}

// computeSuggestions asks the AI about pending (oldest first) in batches of
// maxSuggestAccountsTxns, using all for accounts history. It returns the
// results of the batches that succeeded and the first error, so a failed
// batch is retried on the next pass.
func (h *Handler) computeSuggestions(ctx context.Context, all, pending []hledger.Transaction) (suggestions.Store, error) {
	model := h.effectiveDecisionModel()
	aiCl, err := h.aiClientFor(model)
	if err != nil {
		return nil, err
	}
	accounts, err := cachedAccounts(ctx, h.cache, h.hl)
	if err != nil {
		return nil, fmt.Errorf("fetch accounts: %w", err)
	}
	closed, err := cachedClosedAccounts(ctx, h.cache, h.hl)
	if err != nil {
		return nil, fmt.Errorf("fetch closed accounts: %w", err)
	}
	accountNames := make([]string, 0, len(accounts))
	for _, a := range accounts {
		if !closed[a.FullName] && !isUnknownAccount(a.FullName) {
			accountNames = append(accountNames, a.FullName)
		}
	}
	userGuidelines := ""
	if cfg := h.loadCfg(); cfg != nil {
		userGuidelines = cfg.AI.Prompt
	}
	history := newCategorizationHistory(all)
	now := time.Now().UTC().Format(time.RFC3339)

	out := make(suggestions.Store, len(pending))
	for start := 0; start < len(pending); start += maxSuggestAccountsTxns {
		batch := pending[start:min(start+maxSuggestAccountsTxns, len(pending))]
		inputs := make([]ai.UnknownTxn, len(batch))
		for i, t := range batch {
			inputs[i] = unknownTxnInput(t, history.examples(t))
		}
		var got []ai.AccountSuggestion
		if len(accountNames) > 0 {
			got, err = aiCl.SuggestAccounts(ctx, inputs, accountNames, userGuidelines)
			if err != nil {
				return out, fmt.Errorf("AI suggest accounts: %w", err)
			}
		}
		for _, t := range batch {
			out[t.FID] = suggestions.Suggestion{Model: model, CreatedAt: now}
		}
		for _, s := range got {
			out[s.FID] = suggestions.Suggestion{
				Account:    s.Account,
				Confidence: s.Confidence,
				Reasoning:  s.Reasoning,
				Model:      model,
				CreatedAt:  now,
			}
		}
	}
	return out, nil
}

// unknownTxnInput condenses t for the AI prompt: the amount of its first
// unknown posting and the accounts on its other postings.
func unknownTxnInput(t hledger.Transaction, examples []ai.CategorizedExample) ai.UnknownTxn {
	in := ai.UnknownTxn{
		FID:         t.FID,
		Description: t.Description,
		Date:        t.Date,
		Examples:    examples,
	}
	if t.Payee != nil {
		in.Payee = *t.Payee
	}
	var others []string
	for _, p := range t.Postings {
		if isUnknownAccount(p.Account) {
			if in.Amount == "" {
				in.Amount = postingAmount(p)
			}
			continue
		}
		if !slices.Contains(others, p.Account) {
			others = append(others, p.Account)
		}
	}
	in.Counterparty = strings.Join(others, ", ")
	return in
}

// categorizationHistory indexes already-categorized transactions so each
// unknown transaction can be shown similar precedents. It only builds prompt
// context; matching is by payee or the description's first distinctive word.
type categorizationHistory struct {
	byKey map[string][]ai.CategorizedExample // newest first
}

func newCategorizationHistory(txns []hledger.Transaction) *categorizationHistory {
	h := &categorizationHistory{byKey: make(map[string][]ai.CategorizedExample)}
	for i := len(txns) - 1; i >= 0; i-- {
		t := txns[i]
		account := categoryAccount(t)
		if account == "" || isUnknownAccount(account) {
			continue
		}
		ex := ai.CategorizedExample{Description: t.Description, Account: account}
		for _, k := range historyKeys(t) {
			h.byKey[k] = append(h.byKey[k], ex)
		}
	}
	return h
}

// examples returns up to maxSuggestAccountsExamples distinct precedents for
// t, payee matches first.
func (h *categorizationHistory) examples(t hledger.Transaction) []ai.CategorizedExample {
	var out []ai.CategorizedExample
	seen := make(map[ai.CategorizedExample]bool)
	for _, k := range historyKeys(t) {
		for _, ex := range h.byKey[k] {
			if len(out) == maxSuggestAccountsExamples {
				return out
			}
			if !seen[ex] {
				seen[ex] = true
				out = append(out, ex)
			}
		}
	}
	return out
}

// historyKeys returns t's lookup keys: its payee (if any), then the first
// distinctive word of its description.
func historyKeys(t hledger.Transaction) []string {
	var keys []string
	if t.Payee != nil && strings.TrimSpace(*t.Payee) != "" {
		keys = append(keys, "payee:"+strings.ToLower(strings.TrimSpace(*t.Payee)))
	}
	if w := firstDistinctiveWord(t.Description); w != "" {
		keys = append(keys, "word:"+w)
	}
	return keys
}

// descriptionNoiseWords are bank-statement prefixes that say nothing about
// the merchant.
var descriptionNoiseWords = map[string]bool{
	"ach": true, "card": true, "debit": true, "credit": true, "pos": true,
	"purchase": true, "payment": true, "online": true, "recurring": true,
	"the": true, "tst": true, "www": true, "com": true, "paypal": true,
	"checkcard": true, "visa": true, "withdrawal": true, "deposit": true,
}

// firstDistinctiveWord returns the first lowercase letters-only word of at
// least three letters that is not bank-statement noise.
func firstDistinctiveWord(desc string) string {
	words := strings.FieldsFunc(strings.ToLower(desc), func(r rune) bool { return !unicode.IsLetter(r) })
	for _, w := range words {
		if len(w) >= 3 && !descriptionNoiseWords[w] {
			return w
		}
	}
	return ""
}
