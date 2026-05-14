package stripe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	stripelib "github.com/stripe/stripe-go/v82"
)

func mockStripeBackend(t *testing.T, mux *http.ServeMux) {
	t.Helper()
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	zero := int64(0)
	backend := stripelib.GetBackendWithConfig(stripelib.APIBackend, &stripelib.BackendConfig{
		URL:               stripelib.String(ts.URL),
		HTTPClient:        ts.Client(),
		LeveledLogger:     &stripelib.LeveledLogger{Level: stripelib.LevelNull},
		MaxNetworkRetries: &zero,
	})
	old := stripelib.GetBackend(stripelib.APIBackend)
	stripelib.SetBackend(stripelib.APIBackend, backend)
	t.Cleanup(func() { stripelib.SetBackend(stripelib.APIBackend, old) })
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func TestCreateCustomer(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/customers", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"id": "cus_test123", "object": "customer"})
	})
	mockStripeBackend(t, mux)

	id, err := CreateCustomer(context.Background(), "sk_test_xxx")
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	if id != "cus_test123" {
		t.Errorf("got %q, want %q", id, "cus_test123")
	}
}

func TestCreateFCSession(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/financial_connections/sessions", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"id":            "fcsess_test123",
			"object":        "financial_connections.session",
			"client_secret": "fcsess_test123_secret_xyz",
			"livemode":      false,
			"account_holder": map[string]any{
				"customer": "cus_test123",
				"type":     "customer",
			},
			"accounts": map[string]any{
				"object":   "list",
				"data":     []any{},
				"has_more": false,
				"url":      "/v1/financial_connections/accounts",
			},
			"permissions": []string{"transactions", "balances"},
		})
	})
	mockStripeBackend(t, mux)

	secret, err := CreateFCSession(context.Background(), "sk_test_xxx", "cus_test123")
	if err != nil {
		t.Fatalf("CreateFCSession: %v", err)
	}
	if secret != "fcsess_test123_secret_xyz" {
		t.Errorf("got %q, want %q", secret, "fcsess_test123_secret_xyz")
	}
}

func TestListSessionAccounts(t *testing.T) {
	tests := []struct {
		name     string
		accounts []any
		wantLen  int
	}{
		{
			name: "single account",
			accounts: []any{
				map[string]any{
					"id":               "fca_test_abc",
					"object":           "financial_connections.account",
					"display_name":     "Chase Checking",
					"institution_name": "Chase",
					"last4":            "1234",
					"livemode":         false,
					"status":           "active",
				},
			},
			wantLen: 1,
		},
		{
			name:     "empty list",
			accounts: []any{},
			wantLen:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/v1/financial_connections/sessions/", func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, map[string]any{
					"id":            "fcsess_test123",
					"object":        "financial_connections.session",
					"client_secret": "fcsess_test123_secret_xyz",
					"livemode":      false,
					"account_holder": map[string]any{
						"customer": "cus_test123",
						"type":     "customer",
					},
					"accounts": map[string]any{
						"object":   "list",
						"data":     tc.accounts,
						"has_more": false,
						"url":      "/v1/financial_connections/accounts",
					},
					"permissions": []string{"transactions", "balances"},
				})
			})
			mockStripeBackend(t, mux)

			accounts, err := ListSessionAccounts(context.Background(), "sk_test_xxx", "fcsess_test123")
			if err != nil {
				t.Fatalf("ListSessionAccounts: %v", err)
			}
			if len(accounts) != tc.wantLen {
				t.Fatalf("got %d accounts, want %d", len(accounts), tc.wantLen)
			}
			if tc.wantLen > 0 {
				a := accounts[0]
				if a.ID != "fca_test_abc" {
					t.Errorf("ID = %q, want %q", a.ID, "fca_test_abc")
				}
				if a.DisplayName != "Chase Checking" {
					t.Errorf("DisplayName = %q, want %q", a.DisplayName, "Chase Checking")
				}
				if a.Institution != "Chase" {
					t.Errorf("Institution = %q, want %q", a.Institution, "Chase")
				}
				if a.Last4 != "1234" {
					t.Errorf("Last4 = %q, want %q", a.Last4, "1234")
				}
			}
		})
	}
}

func TestSubscribeTransactions(t *testing.T) {
	called := false
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/financial_connections/accounts/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/subscribe") {
			called = true
			writeJSON(w, map[string]any{
				"id":            "fca_test_abc",
				"object":        "financial_connections.account",
				"status":        "active",
				"subscriptions": []string{"transactions"},
				"livemode":      false,
			})
			return
		}
		http.NotFound(w, r)
	})
	mockStripeBackend(t, mux)

	err := SubscribeTransactions(context.Background(), "sk_test_xxx", "fca_test_abc")
	if err != nil {
		t.Fatalf("SubscribeTransactions: %v", err)
	}
	if !called {
		t.Error("subscribe endpoint was not called")
	}
}

func TestRefreshTransactions(t *testing.T) {
	called := false
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/financial_connections/accounts/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/refresh") {
			called = true
			writeJSON(w, map[string]any{
				"id":       "fca_test_abc",
				"object":   "financial_connections.account",
				"status":   "active",
				"livemode": false,
			})
			return
		}
		http.NotFound(w, r)
	})
	mockStripeBackend(t, mux)

	err := RefreshTransactions(context.Background(), "sk_test_xxx", "fca_test_abc")
	if err != nil {
		t.Fatalf("RefreshTransactions: %v", err)
	}
	if !called {
		t.Error("refresh endpoint was not called")
	}
}

func TestListTransactions(t *testing.T) {
	txnTime := time.Date(2026, 5, 10, 14, 30, 0, 0, time.UTC)

	t.Run("returns transactions with correct fields", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/transactions", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]any{
				"object": "list",
				"data": []any{
					map[string]any{
						"id":            "fca_txn_001",
						"object":        "financial_connections.transaction",
						"account":       "fca_test_abc",
						"amount":        int64(12345),
						"currency":      "usd",
						"description":   "Coffee Shop",
						"transacted_at": txnTime.Unix(),
						"status":        "posted",
						"livemode":      false,
					},
					map[string]any{
						"id":            "fca_txn_002",
						"object":        "financial_connections.transaction",
						"account":       "fca_test_abc",
						"amount":        int64(-500),
						"currency":      "usd",
						"description":   "Refund",
						"transacted_at": txnTime.Add(-24 * time.Hour).Unix(),
						"status":        "pending",
						"livemode":      false,
					},
				},
				"has_more": false,
				"url":      "/v1/financial_connections/transactions",
			})
		})
		mockStripeBackend(t, mux)

		txns, err := ListTransactions(context.Background(), "sk_test_xxx", "fca_test_abc", time.Time{})
		if err != nil {
			t.Fatalf("ListTransactions: %v", err)
		}
		if len(txns) != 2 {
			t.Fatalf("got %d transactions, want 2", len(txns))
		}

		first := txns[0]
		if first.ID != "fca_txn_001" {
			t.Errorf("ID = %q, want %q", first.ID, "fca_txn_001")
		}
		if first.AmountCents != 12345 {
			t.Errorf("AmountCents = %d, want 12345", first.AmountCents)
		}
		if first.Currency != "usd" {
			t.Errorf("Currency = %q, want %q", first.Currency, "usd")
		}
		if first.Description != "Coffee Shop" {
			t.Errorf("Description = %q, want %q", first.Description, "Coffee Shop")
		}
		if first.AccountID != "fca_test_abc" {
			t.Errorf("AccountID = %q, want %q", first.AccountID, "fca_test_abc")
		}
		if first.Status != "posted" {
			t.Errorf("Status = %q, want %q", first.Status, "posted")
		}
		wantTime := time.Unix(txnTime.Unix(), 0).UTC()
		if !first.TransactedAt.Equal(wantTime) {
			t.Errorf("TransactedAt = %v, want %v", first.TransactedAt, wantTime)
		}

		second := txns[1]
		if second.Status != "pending" {
			t.Errorf("second Status = %q, want %q", second.Status, "pending")
		}
		if second.AmountCents != -500 {
			t.Errorf("second AmountCents = %d, want -500", second.AmountCents)
		}
	})

	t.Run("with_since_sends_range_param", func(t *testing.T) {
		since := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
		var gotQuery string
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/transactions", func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.RawQuery
			writeJSON(w, map[string]any{
				"object":   "list",
				"data":     []any{},
				"has_more": false,
				"url":      "/v1/financial_connections/transactions",
			})
		})
		mockStripeBackend(t, mux)

		_, err := ListTransactions(context.Background(), "sk_test_xxx", "fca_test_abc", since)
		if err != nil {
			t.Fatalf("ListTransactions: %v", err)
		}
		if !strings.Contains(gotQuery, "transacted_at") {
			t.Errorf("expected transacted_at range in query params, got: %q", gotQuery)
		}
	})

	t.Run("no_since_no_range_param", func(t *testing.T) {
		var gotQuery string
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/transactions", func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.RawQuery
			writeJSON(w, map[string]any{
				"object":   "list",
				"data":     []any{},
				"has_more": false,
				"url":      "/v1/financial_connections/transactions",
			})
		})
		mockStripeBackend(t, mux)

		_, err := ListTransactions(context.Background(), "sk_test_xxx", "fca_test_abc", time.Time{})
		if err != nil {
			t.Fatalf("ListTransactions: %v", err)
		}
		if strings.Contains(gotQuery, "transacted_at") {
			t.Errorf("expected no transacted_at range in query, got: %q", gotQuery)
		}
	})
}
