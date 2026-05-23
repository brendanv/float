package ledger

import (
	"context"
	"errors"
	"fmt"

	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/rules"
	"github.com/brendanv/float/internal/slogctx"
	stripeClient "github.com/brendanv/float/internal/stripe"
)

// WebhookImportStripeAccount fetches new transactions for stripeAccountID and
// imports any that aren't already in the journal. It's the webhook-driven
// counterpart to runDailyStripeImport: Stripe has already completed a refresh
// (that's the signal that triggered the webhook), so we skip the refresh
// kickoff and go straight to ListTransactions + import.
//
// Returns the number of transactions imported.
func (h *Handler) WebhookImportStripeAccount(ctx context.Context, stripeAccountID string) (int, error) {
	logger := slogctx.FromContext(ctx)
	if h.cfg == nil {
		return 0, errors.New("server has no config loaded")
	}
	secretKey := stripeSecretKey()
	if secretKey == "" {
		return 0, errors.New("STRIPE_SECRET_KEY not set")
	}

	linked, err := h.findLinkedAccount(stripeAccountID)
	if err != nil {
		return 0, err
	}
	if linked.HledgerAccount == "" {
		return 0, fmt.Errorf("stripe account %q has no hledger_account configured", stripeAccountID)
	}

	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		return 0, fmt.Errorf("load rules: %w", err)
	}

	stripeTxns, err := stripeClient.ListTransactions(ctx, secretKey, linked.StripeAccountID, linked.LastTransactionRefreshID)
	if err != nil {
		return 0, fmt.Errorf("list transactions: %w", err)
	}

	newRefreshID, err := stripeClient.GetTransactionRefreshID(ctx, secretKey, linked.StripeAccountID)
	if err != nil {
		// Not fatal — we can still import, just won't advance the refresh ID.
		logger.WarnContext(ctx, "webhook stripe import: get refresh id failed", "account", linked.StripeAccountID, "error", err)
		newRefreshID = ""
	}

	existing, err := h.hl.Transactions(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetch existing transactions: %w", err)
	}
	fpSet := make(map[string]bool, len(existing))
	for _, t := range existing {
		fpSet[journal.TxnFingerprint(t)] = true
	}

	return h.importFetchedStripeTransactions(ctx, linked, stripeTxns, newRefreshID, rulesList, fpSet)
}

// WebhookMarkStripeAccountDisconnected removes a linked Stripe account from
// config when Stripe notifies us that it has been disconnected upstream.
// Idempotent — returns nil if the account is already absent.
func (h *Handler) WebhookMarkStripeAccountDisconnected(ctx context.Context, stripeAccountID string) error {
	if h.cfg == nil {
		return errors.New("server has no config loaded")
	}
	return h.lock.Do(ctx, fmt.Sprintf("webhook: mark stripe account %s disconnected", stripeAccountID), func() error {
		updated := make([]config.StripeLinkedAccount, 0, len(h.cfg.Stripe.LinkedAccounts))
		removed := false
		for _, a := range h.cfg.Stripe.LinkedAccounts {
			if a.StripeAccountID == stripeAccountID {
				removed = true
				continue
			}
			updated = append(updated, a)
		}
		if !removed {
			return nil
		}
		h.cfg.Stripe.LinkedAccounts = updated
		return config.Save(h.configPath, h.cfg)
	})
}
