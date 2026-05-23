// Package webhooks provides HTTP handlers for external webhook receivers.
//
// Handlers in this package verify webhook signatures, parse the event, and
// dispatch to a domain-specific importer interface. They are designed to be
// mounted on a separate listener (e.g. the Tailscale Funnel listener) so the
// rest of the floatd API stays on its private listener.
package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	stripe "github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"
)

const maxWebhookBody = 1 << 20 // 1 MiB

// StripeImporter is implemented by callers who want to react to verified
// Stripe Financial Connections webhook events. Implementations should be
// safe to call from a background goroutine and must not depend on the
// caller's request lifecycle.
type StripeImporter interface {
	ImportRefreshedAccount(ctx context.Context, stripeAccountID string) (int, error)
	AccountDisconnected(ctx context.Context, stripeAccountID string) error
}

// StripeHandler is an http.Handler that verifies Stripe webhook signatures
// and dispatches recognized event types to the configured StripeImporter.
type StripeHandler struct {
	secret   string
	importer StripeImporter
	logger   *slog.Logger
}

func NewStripeHandler(secret string, importer StripeImporter, logger *slog.Logger) *StripeHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &StripeHandler{secret: secret, importer: importer, logger: logger}
}

func (h *StripeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBody+1))
	if err != nil {
		h.logger.WarnContext(r.Context(), "stripe webhook: read body failed", "error", err)
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	if len(body) > maxWebhookBody {
		h.logger.WarnContext(r.Context(), "stripe webhook: body too large", "size", len(body))
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}

	event, err := webhook.ConstructEventWithOptions(body, r.Header.Get("Stripe-Signature"), h.secret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		// Truncate signature in logs so we don't leak it.
		sig := r.Header.Get("Stripe-Signature")
		if len(sig) > 16 {
			sig = sig[:16] + "…"
		}
		h.logger.WarnContext(r.Context(), "stripe webhook: signature verification failed",
			"error", err, "signature_prefix", sig, "remote", r.RemoteAddr)
		http.Error(w, "bad signature", http.StatusBadRequest)
		return
	}

	accountID, err := extractFinancialConnectionsAccountID(event.Data.Raw)
	if err != nil {
		h.logger.WarnContext(r.Context(), "stripe webhook: could not extract account id",
			"type", event.Type, "error", err)
		// Ack 200 anyway — Stripe shouldn't retry malformed payloads we can't act on.
		writeAck(w)
		return
	}

	logger := h.logger.With("event_id", event.ID, "type", event.Type, "account", accountID)

	switch event.Type {
	case stripe.EventTypeFinancialConnectionsAccountRefreshedTransactions:
		logger.InfoContext(r.Context(), "stripe webhook: refreshed_transactions received")
		go func() {
			ctx := context.WithoutCancel(r.Context())
			imported, err := h.importer.ImportRefreshedAccount(ctx, accountID)
			if err != nil {
				logger.ErrorContext(ctx, "stripe webhook: import failed", "error", err)
				return
			}
			logger.InfoContext(ctx, "stripe webhook: import succeeded", "imported", imported)
		}()
	case stripe.EventTypeFinancialConnectionsAccountDisconnected:
		logger.InfoContext(r.Context(), "stripe webhook: account disconnected received")
		go func() {
			ctx := context.WithoutCancel(r.Context())
			if err := h.importer.AccountDisconnected(ctx, accountID); err != nil {
				logger.ErrorContext(ctx, "stripe webhook: disconnect handling failed", "error", err)
			}
		}()
	default:
		logger.DebugContext(r.Context(), "stripe webhook: unhandled event type")
	}

	writeAck(w)
}

func writeAck(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "{\"ok\":true}\n")
}

// extractFinancialConnectionsAccountID pulls the FC account ID from the
// event's data.object payload. All FC account-scoped events use the same
// shape: data.object.id is the fca_... identifier.
func extractFinancialConnectionsAccountID(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", errors.New("empty event data")
	}
	var obj struct {
		ID     string `json:"id"`
		Object string `json:"object"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", fmt.Errorf("decode event object: %w", err)
	}
	if obj.ID == "" {
		return "", errors.New("event object has no id")
	}
	return obj.ID, nil
}
