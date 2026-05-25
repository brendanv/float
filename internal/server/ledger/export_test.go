package ledger

import "context"

// ExportedRunDailyStripeImport exposes runDailyStripeImport to tests in the _test package.
func ExportedRunDailyStripeImport(h *Handler, ctx context.Context) (int, map[string]error) {
	return h.runDailyStripeImport(ctx)
}

// ExportedSetAfterImportAllPreFetch installs a hook that is called between the pre-fetch
// phase and the lock acquisition in ImportAllStripeTransactions. Use this in tests to
// simulate a concurrent config update (e.g., daily auto-import advancing
// LastTransactionRefreshID) in the race window between pre-fetch and lock.
func ExportedSetAfterImportAllPreFetch(h *Handler, fn func()) {
	h.afterImportAllPreFetch = fn
}

// ExportedSetAfterImportPreFetch installs a hook that is called between the pre-fetch
// phase and the lock acquisition in ImportStripeTransactions. Use this in tests to
// simulate a concurrent config update in the race window between pre-fetch and lock.
func ExportedSetAfterImportPreFetch(h *Handler, fn func()) {
	h.afterImportPreFetch = fn
}

// ExportedSetStripeRefreshID directly sets LastTransactionRefreshID for a linked account
// in the handler's in-memory config. Used with ExportedSetAfterImportAllPreFetch to
// simulate a concurrent write that races with ImportAllStripeTransactions.
func ExportedSetStripeRefreshID(h *Handler, accountID, refreshID string) {
	for i, la := range h.cfg.Stripe.LinkedAccounts {
		if la.StripeAccountID == accountID {
			h.cfg.Stripe.LinkedAccounts[i].LastTransactionRefreshID = refreshID
			return
		}
	}
}
