package ledger_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/brendanv/float/internal/config"
	serverledger "github.com/brendanv/float/internal/server/ledger"
	"github.com/brendanv/float/internal/testgen"
)

const testStripeWebhookSecret = "whsec_test_secret"

// signStripeWebhook constructs a Stripe-Signature header value for body using
// secret. Mirrors Stripe's signing scheme: t=<unix>,v1=<HMAC-SHA256(t.body)>.
func signStripeWebhook(t *testing.T, body []byte, secret string, when time.Time) string {
	t.Helper()
	ts := fmt.Sprintf("%d", when.Unix())
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	return "t=" + ts + ",v1=" + sig
}

func newStripeWebhookRequest(t *testing.T, body []byte, sig string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, serverledger.StripeWebhookPath, bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if sig != "" {
		r.Header.Set("Stripe-Signature", sig)
	}
	return r
}

func TestStripeWebhook_NoSecret(t *testing.T) {
	// Explicitly unset (t.Setenv with empty restores the prior value at test end).
	t.Setenv("STRIPE_WEBHOOK_SECRET", "")

	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 900, NumTxns: 1})
	h := mustHandlerWithConfig(t, dir, &config.Config{})

	w := httptest.NewRecorder()
	h.StripeWebhookHandler().ServeHTTP(w, newStripeWebhookRequest(t, []byte("{}"), "t=1,v1=deadbeef"))

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestStripeWebhook_BadSignature(t *testing.T) {
	t.Setenv("STRIPE_WEBHOOK_SECRET", testStripeWebhookSecret)

	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 901, NumTxns: 1})
	h := mustHandlerWithConfig(t, dir, &config.Config{})

	body := []byte(`{"id":"evt_1","type":"financial_connections.account.refreshed_transactions"}`)
	// Sign with a different secret so verification fails.
	sig := signStripeWebhook(t, body, "whsec_wrong", time.Now())

	w := httptest.NewRecorder()
	h.StripeWebhookHandler().ServeHTTP(w, newStripeWebhookRequest(t, body, sig))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestStripeWebhook_MissingSignature(t *testing.T) {
	t.Setenv("STRIPE_WEBHOOK_SECRET", testStripeWebhookSecret)

	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 902, NumTxns: 1})
	h := mustHandlerWithConfig(t, dir, &config.Config{})

	w := httptest.NewRecorder()
	h.StripeWebhookHandler().ServeHTTP(w, newStripeWebhookRequest(t, []byte("{}"), ""))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestStripeWebhook_NotPOST(t *testing.T) {
	t.Setenv("STRIPE_WEBHOOK_SECRET", testStripeWebhookSecret)

	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 903, NumTxns: 1})
	h := mustHandlerWithConfig(t, dir, &config.Config{})

	r := httptest.NewRequest(http.MethodGet, serverledger.StripeWebhookPath, nil)
	w := httptest.NewRecorder()
	h.StripeWebhookHandler().ServeHTTP(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestStripeWebhook_BodyTooLarge(t *testing.T) {
	t.Setenv("STRIPE_WEBHOOK_SECRET", testStripeWebhookSecret)

	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 904, NumTxns: 1})
	h := mustHandlerWithConfig(t, dir, &config.Config{})

	// 2 MiB body — exceeds the 1 MiB cap.
	body := bytes.Repeat([]byte("a"), 2*(1<<20))
	sig := signStripeWebhook(t, body, testStripeWebhookSecret, time.Now())

	w := httptest.NewRecorder()
	h.StripeWebhookHandler().ServeHTTP(w, newStripeWebhookRequest(t, body, sig))

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}
}

// TestStripeWebhook_UnknownEventType verifies that valid signatures for event
// types we don't handle are acknowledged with 200 and do not blow up.
func TestStripeWebhook_UnknownEventType(t *testing.T) {
	t.Setenv("STRIPE_WEBHOOK_SECRET", testStripeWebhookSecret)

	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 905, NumTxns: 1})
	h := mustHandlerWithConfig(t, dir, &config.Config{})

	body := []byte(`{"id":"evt_unknown_1","object":"event","type":"customer.created","data":{"object":{"id":"cus_1"}}}`)
	sig := signStripeWebhook(t, body, testStripeWebhookSecret, time.Now())

	w := httptest.NewRecorder()
	h.StripeWebhookHandler().ServeHTTP(w, newStripeWebhookRequest(t, body, sig))

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
}

// TestStripeWebhook_RefreshedTransactions_Dispatches verifies that a valid
// refreshed_transactions webhook routes into the per-account import path,
// which hits the mocked Stripe API.
func TestStripeWebhook_RefreshedTransactions_Dispatches(t *testing.T) {
	t.Setenv("STRIPE_WEBHOOK_SECRET", testStripeWebhookSecret)
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")

	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 906, NumTxns: 1})
	cfg := &config.Config{Stripe: config.StripeConfig{
		LinkedAccounts: []config.StripeLinkedAccount{{
			StripeAccountID: "fca_test_1",
			HledgerAccount:  "Assets:Checking",
		}},
	}}
	h := mustHandlerWithConfig(t, dir, cfg)

	mux := http.NewServeMux()
	var listCalls atomic.Int32
	done := make(chan struct{})

	mux.HandleFunc("/v1/financial_connections/accounts/fca_test_1/refresh", func(w http.ResponseWriter, r *http.Request) {
		// Mimic a kickoff: account with no in-progress refresh (so MaybeRefresh proceeds).
		writeStripeJSON(w, map[string]any{
			"id":                  "fca_test_1",
			"object":              "financial_connections.account",
			"transaction_refresh": map[string]any{"id": "fcsrtxrefresh_1", "status": "pending"},
		})
	})
	mux.HandleFunc("/v1/financial_connections/accounts/fca_test_1", func(w http.ResponseWriter, r *http.Request) {
		// Polling endpoint — return succeeded so WaitForRefresh exits fast.
		writeStripeJSON(w, map[string]any{
			"id":                  "fca_test_1",
			"object":              "financial_connections.account",
			"transaction_refresh": map[string]any{"id": "fcsrtxrefresh_1", "status": "succeeded"},
			"next_refresh_available_at": time.Now().Add(time.Hour).Unix(),
		})
	})
	mux.HandleFunc("/v1/financial_connections/transactions", func(w http.ResponseWriter, r *http.Request) {
		// Return an empty page; the import path doesn't need real txns to
		// confirm dispatch reached this point.
		listCalls.Add(1)
		writeStripeJSON(w, map[string]any{
			"object":   "list",
			"data":     []any{},
			"has_more": false,
		})
		select {
		case <-done:
		default:
			close(done)
		}
	})
	mockStripeAPI(t, mux)

	body := []byte(`{"id":"evt_refresh_1","object":"event","type":"financial_connections.account.refreshed_transactions","data":{"object":{"id":"fca_test_1","object":"financial_connections.account"}}}`)
	sig := signStripeWebhook(t, body, testStripeWebhookSecret, time.Now())

	w := httptest.NewRecorder()
	h.StripeWebhookHandler().ServeHTTP(w, newStripeWebhookRequest(t, body, sig))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("import goroutine did not reach ListTransactions within timeout (listCalls=%d)", listCalls.Load())
	}
}

// TestStripeWebhook_DuplicateEvent verifies the in-memory dedup blocks a
// second dispatch when Stripe retries with the same event ID. Observed via
// the mocked Stripe API: the import path's ListTransactions endpoint should
// only be called once even though the webhook is fired twice.
func TestStripeWebhook_DuplicateEvent(t *testing.T) {
	t.Setenv("STRIPE_WEBHOOK_SECRET", testStripeWebhookSecret)
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")

	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 907, NumTxns: 1})
	cfg := &config.Config{Stripe: config.StripeConfig{
		LinkedAccounts: []config.StripeLinkedAccount{{
			StripeAccountID: "fca_test_dup",
			HledgerAccount:  "Assets:Checking",
		}},
	}}
	h := mustHandlerWithConfig(t, dir, cfg)

	mux := http.NewServeMux()
	var listCalls atomic.Int32
	firstList := make(chan struct{})

	mux.HandleFunc("/v1/financial_connections/accounts/fca_test_dup/refresh", func(w http.ResponseWriter, r *http.Request) {
		writeStripeJSON(w, map[string]any{
			"id":                  "fca_test_dup",
			"object":              "financial_connections.account",
			"transaction_refresh": map[string]any{"id": "fcsrtxrefresh_dup", "status": "pending"},
		})
	})
	mux.HandleFunc("/v1/financial_connections/accounts/fca_test_dup", func(w http.ResponseWriter, r *http.Request) {
		writeStripeJSON(w, map[string]any{
			"id":                  "fca_test_dup",
			"object":              "financial_connections.account",
			"transaction_refresh": map[string]any{"id": "fcsrtxrefresh_dup", "status": "succeeded"},
		})
	})
	mux.HandleFunc("/v1/financial_connections/transactions", func(w http.ResponseWriter, r *http.Request) {
		n := listCalls.Add(1)
		writeStripeJSON(w, map[string]any{"object": "list", "data": []any{}, "has_more": false})
		if n == 1 {
			close(firstList)
		}
	})
	mockStripeAPI(t, mux)

	body := []byte(`{"id":"evt_dup_1","object":"event","type":"financial_connections.account.refreshed_transactions","data":{"object":{"id":"fca_test_dup","object":"financial_connections.account"}}}`)
	sig := signStripeWebhook(t, body, testStripeWebhookSecret, time.Now())
	handler := h.StripeWebhookHandler()

	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, newStripeWebhookRequest(t, body, sig))
	if w1.Code != http.StatusOK {
		t.Fatalf("first call: status = %d, want 200", w1.Code)
	}

	select {
	case <-firstList:
	case <-time.After(5 * time.Second):
		t.Fatal("first dispatch did not reach ListTransactions")
	}

	// Replay with the same event ID; dedup should short-circuit before dispatch.
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, newStripeWebhookRequest(t, body, sig))
	if w2.Code != http.StatusOK {
		t.Fatalf("second call: status = %d, want 200", w2.Code)
	}

	// Give any erroneous second dispatch time to complete before checking.
	time.Sleep(200 * time.Millisecond)
	if got := listCalls.Load(); got != 1 {
		t.Errorf("ListTransactions calls = %d, want 1 (duplicate event triggered redundant import)", got)
	}
}

// TestStripeEventDedup_MarkIfNew unit-tests the in-memory dedup helper.
func TestStripeEventDedup_MarkIfNew(t *testing.T) {
	d := serverledger.NewStripeEventDedupForTest(time.Hour)
	if !d.MarkIfNew("evt_a") {
		t.Error("first MarkIfNew(evt_a) returned false, want true")
	}
	if d.MarkIfNew("evt_a") {
		t.Error("second MarkIfNew(evt_a) returned true, want false")
	}
	if !d.MarkIfNew("evt_b") {
		t.Error("MarkIfNew(evt_b) returned false, want true")
	}
}

// TestStripeEventDedup_TTL verifies entries expire after the TTL.
func TestStripeEventDedup_TTL(t *testing.T) {
	d := serverledger.NewStripeEventDedupForTest(50 * time.Millisecond)
	if !d.MarkIfNew("evt_ttl") {
		t.Fatal("first MarkIfNew returned false")
	}
	time.Sleep(80 * time.Millisecond)
	if !d.MarkIfNew("evt_ttl") {
		t.Error("MarkIfNew after TTL returned false, want true (entry should have expired)")
	}
}

