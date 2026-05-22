package ledger

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/rules"
	"github.com/brendanv/float/internal/slogctx"
	stripeClient "github.com/brendanv/float/internal/stripe"
)

const (
	dailyStripeImportInterval = 24 * time.Hour
	dailyStripeImportTick     = 1 * time.Hour
	dailyStripeImportStartup  = 30 * time.Second
)

// StartDailyStripeImport runs a background loop that, while the Stripe daily auto-import
// setting is enabled and at least 24h has elapsed since the last successful run, fetches
// new transactions for every linked account and imports all non-duplicate transactions.
// It returns when ctx is canceled.
func (h *Handler) StartDailyStripeImport(ctx context.Context) {
	logger := slog.Default().With("component", "stripe_auto_import")
	ctx = slogctx.WithLogger(ctx, logger)

	select {
	case <-ctx.Done():
		return
	case <-time.After(dailyStripeImportStartup):
	}

	h.maybeRunDailyStripeImport(ctx)

	ticker := time.NewTicker(dailyStripeImportTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.maybeRunDailyStripeImport(ctx)
		}
	}
}

func (h *Handler) maybeRunDailyStripeImport(ctx context.Context) {
	logger := slogctx.FromContext(ctx)
	if h.cfg == nil || !h.cfg.Stripe.DailyImportEnabled {
		return
	}
	if stripeSecretKey() == "" {
		logger.WarnContext(ctx, "daily stripe import skipped: STRIPE_SECRET_KEY not set")
		return
	}
	if last, ok := parseDailyImportTimestamp(h.cfg.Stripe.LastDailyImportAt); ok {
		if time.Since(last) < dailyStripeImportInterval {
			return
		}
	}

	imported, perAccountErrs := h.runDailyStripeImport(ctx)
	logger.InfoContext(ctx, "daily stripe import finished",
		"imported", imported,
		"account_errors", len(perAccountErrs),
	)

	now := time.Now().UTC().Format(time.RFC3339)
	if err := h.lock.Do(ctx, "stripe daily auto-import: update last run timestamp", func() error {
		h.cfg.Stripe.LastDailyImportAt = now
		return config.Save(h.configPath, h.cfg)
	}); err != nil {
		logger.ErrorContext(ctx, "daily stripe import: failed to persist last run timestamp", "error", err)
	}
}

// runDailyStripeImport fetches and imports for every linked account, tolerating per-account
// errors. Returns the total number of transactions imported and a map of accountID -> error
// for accounts that failed.
//
// Stripe API calls (refresh + list) are fanned out in parallel; dedup and journal writes
// are performed sequentially so the shared fpSet stays consistent across accounts.
func (h *Handler) runDailyStripeImport(ctx context.Context) (int, map[string]error) {
	logger := slogctx.FromContext(ctx)
	secretKey := stripeSecretKey()

	perAccountErrs := make(map[string]error)

	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		logger.ErrorContext(ctx, "daily stripe import: load rules failed", "error", err)
		return 0, perAccountErrs
	}

	existing, err := h.hl.Transactions(ctx)
	if err != nil {
		logger.ErrorContext(ctx, "daily stripe import: fetch existing transactions failed", "error", err)
		return 0, perAccountErrs
	}
	fpSet := make(map[string]bool, len(existing))
	for _, t := range existing {
		fpSet[journal.TxnFingerprint(t)] = true
	}

	var eligible []config.StripeLinkedAccount
	for _, linked := range h.cfg.Stripe.LinkedAccounts {
		if linked.HledgerAccount != "" {
			eligible = append(eligible, linked)
		}
	}

	// Phase 1: fetch from Stripe in parallel — each account needs a refresh + list call.
	type fetchResult struct {
		txns []stripeClient.Transaction
		err  error
	}
	fetched := make([]fetchResult, len(eligible))
	var wg sync.WaitGroup
	for i, linked := range eligible {
		i, linked := i, linked
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := stripeClient.RefreshTransactions(ctx, secretKey, linked.StripeAccountID); err != nil {
				fetched[i].err = fmt.Errorf("refresh: %w", err)
				return
			}
			var since time.Time
			if linked.LastFetchedAt != "" {
				since, _ = time.Parse(time.RFC3339, linked.LastFetchedAt)
			}
			txns, err := stripeClient.ListTransactions(ctx, secretKey, linked.StripeAccountID, since)
			if err != nil {
				fetched[i].err = fmt.Errorf("list: %w", err)
				return
			}
			fetched[i].txns = txns
		}()
	}
	wg.Wait()

	// Phase 2: dedup and import sequentially so fpSet stays consistent across accounts.
	totalImported := 0
	for i, linked := range eligible {
		if fetched[i].err != nil {
			logger.WarnContext(ctx, "daily stripe import: account fetch failed",
				"account", linked.StripeAccountID,
				"error", fetched[i].err,
			)
			perAccountErrs[linked.StripeAccountID] = fetched[i].err
			continue
		}

		imported, err := h.importFetchedStripeTransactions(ctx, linked, fetched[i].txns, rulesList, fpSet)
		if err != nil {
			logger.WarnContext(ctx, "daily stripe import: account import failed",
				"account", linked.StripeAccountID,
				"error", err,
			)
			perAccountErrs[linked.StripeAccountID] = err
			continue
		}
		totalImported += imported
		logger.InfoContext(ctx, "daily stripe import: account imported",
			"account", linked.StripeAccountID,
			"imported", imported,
		)
	}

	return totalImported, perAccountErrs
}

// importFetchedStripeTransactions deduplicates pre-fetched Stripe transactions against fpSet,
// writes new ones to the journal, and updates LastFetchedAt in config. fpSet is updated in
// place so that subsequent accounts in the same batch don't re-import the same transactions.
func (h *Handler) importFetchedStripeTransactions(
	ctx context.Context,
	linked config.StripeLinkedAccount,
	stripeTxns []stripeClient.Transaction,
	rulesList []rules.Rule,
	fpSet map[string]bool,
) (int, error) {
	var newTxns []stripeClient.Transaction
	for _, st := range stripeTxns {
		ht := stripeTransactionToHledger(st, linked.HledgerAccount)
		if fpSet[journal.TxnFingerprint(ht)] {
			continue
		}
		newTxns = append(newTxns, st)
	}

	fetchedAt := time.Now().UTC().Format(time.RFC3339)
	if len(newTxns) == 0 {
		err := h.lock.Do(ctx, fmt.Sprintf("stripe daily auto-import: update last_fetched_at for %s", linked.StripeAccountID), func() error {
			for i, la := range h.cfg.Stripe.LinkedAccounts {
				if la.StripeAccountID == linked.StripeAccountID {
					h.cfg.Stripe.LinkedAccounts[i].LastFetchedAt = fetchedAt
					break
				}
			}
			return config.Save(h.configPath, h.cfg)
		})
		if err != nil {
			return 0, fmt.Errorf("update last_fetched_at: %w", err)
		}
		return 0, nil
	}

	importBatchID := "stripe-" + stripeAccountSlug(linked.StripeAccountID) + "/" + time.Now().Format("2006-01-02") + "-" + journal.MintFID()
	imported := 0
	err := h.lock.Do(ctx, fmt.Sprintf("stripe daily auto-import: import %d txns for %s", len(newTxns), linked.StripeAccountID), func() error {
		for _, st := range newTxns {
			txInput := stripeTransactionToInput(st, linked.HledgerAccount, importBatchID)
			applyRuleToInput(&txInput, rules.Match(rulesList, st.Description, linked.HledgerAccount))
			if _, writeErr := journal.AppendTransaction(ctx, h.hl, h.dataDir, txInput); writeErr != nil {
				return fmt.Errorf("write %s: %w", st.ID, writeErr)
			}
			imported++
		}
		for i, la := range h.cfg.Stripe.LinkedAccounts {
			if la.StripeAccountID == linked.StripeAccountID {
				h.cfg.Stripe.LinkedAccounts[i].LastFetchedAt = fetchedAt
				break
			}
		}
		return config.Save(h.configPath, h.cfg)
	})
	if err != nil {
		return 0, err
	}

	for _, st := range newTxns {
		fpSet[journal.TxnFingerprint(stripeTransactionToHledger(st, linked.HledgerAccount))] = true
	}

	return imported, nil
}

func parseDailyImportTimestamp(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
