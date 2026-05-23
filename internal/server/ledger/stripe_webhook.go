package ledger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	stripelib "github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"

	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/rules"
	"github.com/brendanv/float/internal/slogctx"
	stripeClient "github.com/brendanv/float/internal/stripe"
)

const (
	// StripeWebhookPath is the HTTP path the receiver is mounted at.
	StripeWebhookPath = "/webhooks/stripe"

	// stripeWebhookMaxBodyBytes caps incoming payload size. Real Stripe events
	// are well under 64 KiB; the cap exists to keep a misbehaving sender from
	// allocating huge buffers.
	stripeWebhookMaxBodyBytes = 1 << 20

	// stripeWebhookEventTTL controls how long processed Stripe event IDs stay in
	// the in-memory dedup cache. The downstream import is idempotent already
	// (fingerprint dedup against the journal); this prevents redundant fetch
	// goroutines when Stripe retries within a typical window.
	stripeWebhookEventTTL = 1 * time.Hour

	// stripeWebhookImportTimeout bounds an async per-event import. The hard cap
	// covers refresh wait (5 min in stripeClient.WaitForRefresh) plus list/import
	// time with margin.
	stripeWebhookImportTimeout = 10 * time.Minute
)

// stripeWebhookSecret returns the configured Stripe webhook signing secret.
func stripeWebhookSecret() string {
	return os.Getenv("STRIPE_WEBHOOK_SECRET")
}

// StripeWebhookHandler returns an http.Handler that accepts Stripe webhooks.
// It verifies the Stripe-Signature header against STRIPE_WEBHOOK_SECRET and
// dispatches financial_connections events to per-account imports.
//
// If STRIPE_WEBHOOK_SECRET is unset the endpoint returns 503 so a misconfigured
// deployment cannot silently accept unauthenticated requests.
func (h *Handler) StripeWebhookHandler() http.Handler {
	dedup := newStripeEventDedup(stripeWebhookEventTTL)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.serveStripeWebhook(w, r, dedup)
	})
}

func (h *Handler) serveStripeWebhook(w http.ResponseWriter, r *http.Request, dedup *stripeEventDedup) {
	reqID := newWebhookRequestID()
	logger := slog.Default().With("component", "stripe_webhook", "request_id", reqID)
	ctx := slogctx.WithLogger(r.Context(), logger)

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	secret := stripeWebhookSecret()
	if secret == "" {
		logger.ErrorContext(ctx, "rejected: STRIPE_WEBHOOK_SECRET is not set")
		http.Error(w, "webhook receiver not configured", http.StatusServiceUnavailable)
		return
	}

	limited := io.LimitReader(r.Body, stripeWebhookMaxBodyBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		logger.WarnContext(ctx, "read body failed", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if int64(len(body)) > stripeWebhookMaxBodyBytes {
		logger.WarnContext(ctx, "body too large", "bytes", len(body))
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}

	sig := r.Header.Get("Stripe-Signature")
	// IgnoreAPIVersionMismatch lets us accept events even when the Stripe webhook
	// endpoint is pinned to an API version different from stripe-go's expected
	// version. We only read event.ID, event.Type, and the object id from
	// event.Data.Raw — none of which are sensitive to API version drift.
	event, err := webhook.ConstructEventWithOptions(body, sig, secret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		logger.WarnContext(ctx, "signature verification failed", "error", err)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	logger = logger.With("event_id", event.ID, "event_type", string(event.Type))

	if !dedup.markIfNew(event.ID) {
		logger.InfoContext(ctx, "duplicate event, skipping import")
		w.WriteHeader(http.StatusOK)
		return
	}

	// Ack first so Stripe doesn't time out and retry on a slow import.
	w.WriteHeader(http.StatusOK)

	// Run the import on a fresh background context — the request context is
	// canceled the moment the response completes.
	go h.dispatchStripeWebhookEvent(reqID, event)
}

func (h *Handler) dispatchStripeWebhookEvent(reqID string, event stripelib.Event) {
	ctx, cancel := context.WithTimeout(context.Background(), stripeWebhookImportTimeout)
	defer cancel()
	logger := slog.Default().With(
		"component", "stripe_webhook",
		"request_id", reqID,
		"event_id", event.ID,
		"event_type", string(event.Type),
	)
	ctx = slogctx.WithLogger(ctx, logger)

	switch event.Type {
	case "financial_connections.account.refreshed_transactions":
		accountID, err := stripeAccountIDFromEvent(event.Data.Raw)
		if err != nil {
			logger.ErrorContext(ctx, "extract account id failed", "error", err)
			return
		}
		logger.InfoContext(ctx, "import triggered by webhook", "account", accountID)
		imported, err := h.importStripeAccountByID(ctx, accountID)
		if err != nil {
			logger.ErrorContext(ctx, "import failed", "account", accountID, "error", err)
			return
		}
		logger.InfoContext(ctx, "import complete", "account", accountID, "imported", imported)

	default:
		logger.InfoContext(ctx, "ignoring unhandled event type")
	}
}

func stripeAccountIDFromEvent(raw json.RawMessage) (string, error) {
	var obj struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", fmt.Errorf("parse event object: %w", err)
	}
	if obj.ID == "" {
		return "", fmt.Errorf("event object missing id field")
	}
	return obj.ID, nil
}

// importStripeAccountByID fetches and imports new transactions for one linked
// Stripe account. It mirrors the per-account flow of runDailyStripeImport:
// kick the Stripe refresh (honoring the throttle), list transactions after the
// account's last imported refresh ID, dedupe against existing journal entries,
// apply categorization rules, and persist via txlock. Returns the number of
// transactions written.
func (h *Handler) importStripeAccountByID(ctx context.Context, accountID string) (int, error) {
	logger := slogctx.FromContext(ctx)
	if h.cfg == nil {
		return 0, fmt.Errorf("no config loaded")
	}
	secretKey := stripeSecretKey()
	if secretKey == "" {
		return 0, fmt.Errorf("STRIPE_SECRET_KEY is not set")
	}

	linked, err := h.findLinkedAccount(accountID)
	if err != nil {
		return 0, err
	}
	if linked.HledgerAccount == "" {
		return 0, fmt.Errorf("stripe account %q has no hledger account mapping", accountID)
	}

	kickoff, err := stripeClient.MaybeRefreshTransactions(ctx, logger, secretKey, accountID)
	if err != nil {
		return 0, fmt.Errorf("refresh kickoff: %w", err)
	}
	var newRefreshID string
	if kickoff.Status == stripeClient.RefreshKickoffThrottled {
		logger.InfoContext(ctx, "refresh throttled; listing against current refresh",
			"account", accountID,
			"next_refresh_available_at", kickoff.NextRefreshAvailableAt.Format(time.RFC3339),
		)
		newRefreshID = kickoff.CurrentRefreshID
	} else {
		newRefreshID, err = stripeClient.WaitForRefresh(ctx, logger, secretKey, accountID)
		if err != nil {
			return 0, fmt.Errorf("wait for refresh: %w", err)
		}
	}

	txns, err := stripeClient.ListTransactions(ctx, secretKey, accountID, linked.LastTransactionRefreshID)
	if err != nil {
		return 0, fmt.Errorf("list transactions: %w", err)
	}

	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		return 0, fmt.Errorf("load rules: %w", err)
	}

	existing, err := h.hl.Transactions(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetch existing transactions: %w", err)
	}
	fpSet := make(map[string]bool, len(existing))
	for _, t := range existing {
		fpSet[journal.TxnFingerprint(t)] = true
	}

	return h.importFetchedStripeTransactions(ctx, linked, txns, newRefreshID, rulesList, fpSet)
}

// stripeEventDedup remembers recently-seen Stripe event IDs to skip duplicate
// processing when Stripe retries a delivery.
type stripeEventDedup struct {
	mu  sync.Mutex
	ttl time.Duration
	at  map[string]time.Time
}

func newStripeEventDedup(ttl time.Duration) *stripeEventDedup {
	return &stripeEventDedup{ttl: ttl, at: make(map[string]time.Time)}
}

// markIfNew returns true if id has not been seen within the TTL window. It
// records id under the current time before returning. Expired entries are
// swept opportunistically on each call.
func (d *stripeEventDedup) markIfNew(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	for k, t := range d.at {
		if now.Sub(t) > d.ttl {
			delete(d.at, k)
		}
	}
	if _, ok := d.at[id]; ok {
		return false
	}
	d.at[id] = now
	return true
}

func newWebhookRequestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
