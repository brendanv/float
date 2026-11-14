package ledger

import "context"

// ExportedRunDailyStripeImport exposes runDailyStripeImport to tests in the _test package.
func ExportedRunDailyStripeImport(h *Handler, ctx context.Context) (int, map[string]error) {
	return h.runDailyStripeImport(ctx)
}

// ExportedSetAfterImportAllPreFetch installs a hook that is called between the pre-fetch
// phase and the lock acquisition in ImportAllStripeTransactions. Use this in tests to
// simulate a concurrent import in the race window between pre-fetch and lock.
func ExportedSetAfterImportAllPreFetch(h *Handler, fn func()) {
	h.afterImportAllPreFetch = fn
}

// ExportedSetAfterImportPreFetch installs a hook that is called between the pre-fetch
// phase and the lock acquisition in ImportStripeTransactions. Use this in tests to
// simulate a concurrent import in the race window between pre-fetch and lock.
func ExportedSetAfterImportPreFetch(h *Handler, fn func()) {
	h.afterImportPreFetch = fn
}

// ExportedSetAfterDailyImportPreFetch installs a hook that is called between the
// pre-lock dedup phase and the lock acquisition in runDailyStripeImport. Use this in
// tests to simulate a concurrent import in the race window between dedup and lock.
func ExportedSetAfterDailyImportPreFetch(h *Handler, fn func()) {
	h.afterDailyImportPreFetch = fn
}

// ExportedSetBeforeApplyRulesLock installs a hook that is called in ApplyRules just
// before lock acquisition. Use this in tests to simulate a concurrent write landing
// before the rule matches are computed and applied.
func ExportedSetBeforeApplyRulesLock(h *Handler, fn func()) {
	h.beforeApplyRulesLock = fn
}
