package ledger

import (
	"context"
	"time"
)

// ExportedRunDailyStripeImport exposes runDailyStripeImport to tests in the _test package.
func ExportedRunDailyStripeImport(h *Handler, ctx context.Context) (int, map[string]error) {
	return h.runDailyStripeImport(ctx)
}

// StripeEventDedupForTest is the test-facing view of the webhook dedup helper.
type StripeEventDedupForTest struct{ inner *stripeEventDedup }

// NewStripeEventDedupForTest constructs a dedup with the given TTL for use in tests.
func NewStripeEventDedupForTest(ttl time.Duration) *StripeEventDedupForTest {
	return &StripeEventDedupForTest{inner: newStripeEventDedup(ttl)}
}

// MarkIfNew records id and returns true if it was not already present within the TTL.
func (d *StripeEventDedupForTest) MarkIfNew(id string) bool { return d.inner.markIfNew(id) }
