package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stripe/stripe-go/v82/webhook"
)

type fakeImporter struct {
	mu               sync.Mutex
	refreshedCalls   []string
	disconnectCalls  []string
	refreshErr       error
	disconnectErr    error
	importedReturn   int
	signalRefreshed  chan struct{}
	signalDisconnect chan struct{}
}

func newFakeImporter() *fakeImporter {
	return &fakeImporter{
		signalRefreshed:  make(chan struct{}, 1),
		signalDisconnect: make(chan struct{}, 1),
	}
}

func (f *fakeImporter) ImportRefreshedAccount(_ context.Context, id string) (int, error) {
	f.mu.Lock()
	f.refreshedCalls = append(f.refreshedCalls, id)
	err := f.refreshErr
	n := f.importedReturn
	f.mu.Unlock()
	select {
	case f.signalRefreshed <- struct{}{}:
	default:
	}
	return n, err
}

func (f *fakeImporter) AccountDisconnected(_ context.Context, id string) error {
	f.mu.Lock()
	f.disconnectCalls = append(f.disconnectCalls, id)
	err := f.disconnectErr
	f.mu.Unlock()
	select {
	case f.signalDisconnect <- struct{}{}:
	default:
	}
	return err
}

func signedRequest(t *testing.T, body []byte, secret string, ts time.Time) *http.Request {
	t.Helper()
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload:   body,
		Secret:    secret,
		Timestamp: ts,
	})
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(string(body)))
	req.Header.Set("Stripe-Signature", signed.Header)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func buildEvent(t *testing.T, eventType, accountID string) []byte {
	t.Helper()
	payload := map[string]any{
		"id":          "evt_test_123",
		"object":      "event",
		"api_version": "2024-04-10",
		"type":        eventType,
		"data": map[string]any{
			"object": map[string]any{
				"id":     accountID,
				"object": "financial_connections.account",
			},
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestStripeHandler_RefreshedTransactions_DispatchesImport(t *testing.T) {
	secret := "whsec_test_secret"
	imp := newFakeImporter()
	imp.importedReturn = 7

	handler := NewStripeHandler(secret, imp, discardLogger())
	body := buildEvent(t, "financial_connections.account.refreshed_transactions", "fca_abc")
	req := signedRequest(t, body, secret, time.Now())

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	select {
	case <-imp.signalRefreshed:
	case <-time.After(2 * time.Second):
		t.Fatal("ImportRefreshedAccount was not called within 2s")
	}
	imp.mu.Lock()
	defer imp.mu.Unlock()
	if got := imp.refreshedCalls; len(got) != 1 || got[0] != "fca_abc" {
		t.Fatalf("refreshedCalls = %v, want [fca_abc]", got)
	}
}

func TestStripeHandler_Disconnected_DispatchesHandler(t *testing.T) {
	secret := "whsec_test_secret"
	imp := newFakeImporter()
	handler := NewStripeHandler(secret, imp, discardLogger())

	body := buildEvent(t, "financial_connections.account.disconnected", "fca_gone")
	req := signedRequest(t, body, secret, time.Now())

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	select {
	case <-imp.signalDisconnect:
	case <-time.After(2 * time.Second):
		t.Fatal("AccountDisconnected was not called within 2s")
	}
	imp.mu.Lock()
	defer imp.mu.Unlock()
	if got := imp.disconnectCalls; len(got) != 1 || got[0] != "fca_gone" {
		t.Fatalf("disconnectCalls = %v, want [fca_gone]", got)
	}
}

func TestStripeHandler_BadSignature_Rejected(t *testing.T) {
	imp := newFakeImporter()
	handler := NewStripeHandler("whsec_test_secret", imp, discardLogger())

	body := buildEvent(t, "financial_connections.account.refreshed_transactions", "fca_abc")
	req := signedRequest(t, body, "wrong_secret", time.Now())

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	imp.mu.Lock()
	defer imp.mu.Unlock()
	if len(imp.refreshedCalls) != 0 {
		t.Fatalf("importer should not have been called on bad sig; got %v", imp.refreshedCalls)
	}
}

func TestStripeHandler_OldTimestamp_Rejected(t *testing.T) {
	secret := "whsec_test_secret"
	imp := newFakeImporter()
	handler := NewStripeHandler(secret, imp, discardLogger())

	body := buildEvent(t, "financial_connections.account.refreshed_transactions", "fca_abc")
	req := signedRequest(t, body, secret, time.Now().Add(-10*time.Minute))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (rejected for stale timestamp)", rr.Code)
	}
	imp.mu.Lock()
	defer imp.mu.Unlock()
	if len(imp.refreshedCalls) != 0 {
		t.Fatalf("importer should not have been called on old ts; got %v", imp.refreshedCalls)
	}
}

func TestStripeHandler_UnknownEventType_AckButNoCall(t *testing.T) {
	secret := "whsec_test_secret"
	imp := newFakeImporter()
	handler := NewStripeHandler(secret, imp, discardLogger())

	body := buildEvent(t, "financial_connections.account.refreshed_balance", "fca_abc")
	req := signedRequest(t, body, secret, time.Now())

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	// give any (incorrect) goroutine a moment to fire
	time.Sleep(50 * time.Millisecond)
	imp.mu.Lock()
	defer imp.mu.Unlock()
	if len(imp.refreshedCalls) != 0 || len(imp.disconnectCalls) != 0 {
		t.Fatalf("no handlers should fire for unknown type; refreshed=%v disconnect=%v",
			imp.refreshedCalls, imp.disconnectCalls)
	}
}

func TestStripeHandler_MethodNotAllowed(t *testing.T) {
	handler := NewStripeHandler("whsec_test_secret", newFakeImporter(), discardLogger())
	req := httptest.NewRequest(http.MethodGet, "/webhooks/stripe", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rr.Code)
	}
}

func TestStripeHandler_ImportError_StillAcks(t *testing.T) {
	secret := "whsec_test_secret"
	imp := newFakeImporter()
	imp.refreshErr = errors.New("downstream boom")
	handler := NewStripeHandler(secret, imp, discardLogger())

	body := buildEvent(t, "financial_connections.account.refreshed_transactions", "fca_abc")
	req := signedRequest(t, body, secret, time.Now())

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (import errors are async; client still acked)", rr.Code)
	}
	select {
	case <-imp.signalRefreshed:
	case <-time.After(2 * time.Second):
		t.Fatal("importer not invoked")
	}
}
