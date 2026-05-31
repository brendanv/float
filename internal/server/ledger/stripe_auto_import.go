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
	dailyStripeImportInterval = 8 * time.Hour
	dailyStripeImportTick     = 1 * time.Hour
	dailyStripeImportStartup  = 30 * time.Second
)

// StartDailyStripeImport runs a background loop that, while the Stripe daily auto-import
// setting is enabled and at least 8h has elapsed since the last successful run, fetches
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
}

// runDailyStripeImport fetches and imports for every linked account, tolerating per-account
// fetch errors. Returns the total number of transactions imported and a map of accountID ->
// error for accounts that failed to fetch.
//
// Stripe API calls (refresh + list) are fanned out in parallel. Dedup runs sequentially
// across all accounts, then all journal writes and config updates are committed in a single
// snapshot.
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
	stripeTxnSet := importedStripeTxnIDs(existing)

	var eligible []config.StripeLinkedAccount
	for _, linked := range h.cfg.Stripe.LinkedAccounts {
		if linked.HledgerAccount != "" {
			eligible = append(eligible, linked)
		}
	}

	// Phase 1: fetch from Stripe in parallel. Each account is refreshed (so Stripe syncs
	// fresh data from the bank) and then fully listed.
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
			kickoff, err := stripeClient.MaybeRefreshTransactions(ctx, logger, secretKey, linked.StripeAccountID)
			if err != nil {
				fetched[i].err = fmt.Errorf("refresh: %w", err)
				return
			}
			if kickoff.Status == stripeClient.RefreshKickoffThrottled {
				logger.InfoContext(ctx, "daily stripe import: refresh throttled, listing current data",
					"account", linked.StripeAccountID,
					"next_refresh_available_at", kickoff.NextRefreshAvailableAt.Format(time.RFC3339),
				)
			} else if _, err := stripeClient.WaitForRefresh(ctx, logger, secretKey, linked.StripeAccountID); err != nil {
				fetched[i].err = fmt.Errorf("wait for refresh: %w", err)
				return
			}
			txns, err := stripeClient.ListTransactions(ctx, secretKey, linked.StripeAccountID)
			if err != nil {
				fetched[i].err = fmt.Errorf("list: %w", err)
				return
			}
			fetched[i].txns = txns
		}()
	}
	wg.Wait()

	// Phase 2: dedup all accounts, then write everything in a single snapshot.
	type accountBatch struct {
		linked  config.StripeLinkedAccount
		newTxns []stripeClient.Transaction
		batchID string
	}

	fetchedAt := time.Now().UTC().Format(time.RFC3339)
	var batches []accountBatch
	for i, linked := range eligible {
		if fetched[i].err != nil {
			logger.WarnContext(ctx, "daily stripe import: account fetch failed",
				"account", linked.StripeAccountID,
				"error", fetched[i].err,
			)
			perAccountErrs[linked.StripeAccountID] = fetched[i].err
			continue
		}
		var newTxns []stripeClient.Transaction
		for _, st := range fetched[i].txns {
			if !stripeTxnSettled(st) || stripeTxnSet[st.ID] {
				continue
			}
			newTxns = append(newTxns, st)
		}
		batches = append(batches, accountBatch{
			linked:  linked,
			newTxns: newTxns,
			batchID: "stripe-" + stripeAccountSlug(linked.StripeAccountID) + "/" + time.Now().Format("2006-01-02") + "-" + journal.MintFID(),
		})
	}

	now := time.Now().UTC().Format(time.RFC3339)
	totalImported := 0
	if err := h.lock.Do(ctx, "stripe daily auto-import", func() error {
		for _, batch := range batches {
			for _, st := range batch.newTxns {
				txInput := stripeTransactionToInput(st, batch.linked.HledgerAccount, batch.batchID, h.cfg.Location())
				applyRuleToInput(&txInput, rules.Match(rulesList, st.Description, batch.linked.HledgerAccount))
				if _, writeErr := journal.AppendTransaction(ctx, h.hl, h.dataDir, txInput); writeErr != nil {
					return fmt.Errorf("write %s: %w", st.ID, writeErr)
				}
				totalImported++
			}
			for j, la := range h.cfg.Stripe.LinkedAccounts {
				if la.StripeAccountID == batch.linked.StripeAccountID {
					h.cfg.Stripe.LinkedAccounts[j].LastFetchedAt = fetchedAt
					break
				}
			}
		}
		h.cfg.Stripe.LastDailyImportAt = now
		return config.Save(h.configPath, h.cfg)
	}); err != nil {
		logger.ErrorContext(ctx, "daily stripe import: write failed", "error", err)
		return 0, perAccountErrs
	}

	for _, batch := range batches {
		for _, st := range batch.newTxns {
			stripeTxnSet[st.ID] = true
		}
		logger.InfoContext(ctx, "daily stripe import: account imported",
			"account", batch.linked.StripeAccountID,
			"imported", len(batch.newTxns),
		)
	}

	return totalImported, perAccountErrs
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
