package ledger

import "context"

// ExportedRunDailyStripeImport exposes runDailyStripeImport to tests in the _test package.
func ExportedRunDailyStripeImport(h *Handler, ctx context.Context) (int, map[string]error) {
	return h.runDailyStripeImport(ctx)
}
