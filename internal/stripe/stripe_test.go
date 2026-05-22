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

func TestCreateFCSession(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/financial_connections/sessions", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"id":            "fcsess_test123",
			"object":        "financial_connections.session",
			"client_secret": "fcsess_test123_secret_xyz",
			"livemode":      false,
			"account_holder": map[string]any{
				"type": "account",
			},
			"accounts": map[string]any{
				"object":   "list",
				"data":     []any{},
				"has_more": false,
				"url":      "/v1/financial_connections/accounts",
			},
			"permissions": []string{"payment_method", "transactions", "balances"},
		})
	})
	mockStripeBackend(t, mux)

	secret, err := CreateFCSession(context.Background(), "sk_test_xxx", "acct_test123")
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
						"type": "account",
					},
					"accounts": map[string]any{
						"object":   "list",
						"data":     tc.accounts,
						"has_more": false,
						"url":      "/v1/financial_connections/accounts",
					},
					"permissions": []string{"payment_method", "transactions", "balances"},
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

		txns, err := ListTransactions(context.Background(), "sk_test_xxx", "fca_test_abc", "")
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

	t.Run("with_refresh_id_sends_transaction_refresh_param", func(t *testing.T) {
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

		_, err := ListTransactions(context.Background(), "sk_test_xxx", "fca_test_abc", "txnr_abc123")
		if err != nil {
			t.Fatalf("ListTransactions: %v", err)
		}
		if !strings.Contains(gotQuery, "transaction_refresh") {
			t.Errorf("expected transaction_refresh in query params, got: %q", gotQuery)
		}
		if !strings.Contains(gotQuery, "txnr_abc123") {
			t.Errorf("expected refresh ID txnr_abc123 in query params, got: %q", gotQuery)
		}
	})

	t.Run("no_refresh_id_no_filter_param", func(t *testing.T) {
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

		_, err := ListTransactions(context.Background(), "sk_test_xxx", "fca_test_abc", "")
		if err != nil {
			t.Fatalf("ListTransactions: %v", err)
		}
		if strings.Contains(gotQuery, "transaction_refresh") {
			t.Errorf("expected no transaction_refresh in query, got: %q", gotQuery)
		}
	})
}

func TestWaitForRefresh(t *testing.T) {
	t.Run("returns refresh ID immediately when status is succeeded", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/accounts/fca_test_abc", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]any{
				"id":       "fca_test_abc",
				"object":   "financial_connections.account",
				"status":   "active",
				"livemode": false,
				"transaction_refresh": map[string]any{
					"id":          "txnr_success",
					"status":      "succeeded",
					"last_attempted_at": int64(1746835200),
				},
			})
		})
		mockStripeBackend(t, mux)

		refreshID, err := WaitForRefresh(context.Background(), nil, "sk_test_xxx", "fca_test_abc")
		if err != nil {
			t.Fatalf("WaitForRefresh: %v", err)
		}
		if refreshID != "txnr_success" {
			t.Errorf("refreshID = %q, want %q", refreshID, "txnr_success")
		}
	})

	t.Run("returns error when status is failed", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/accounts/fca_test_abc", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]any{
				"id":       "fca_test_abc",
				"object":   "financial_connections.account",
				"status":   "active",
				"livemode": false,
				"transaction_refresh": map[string]any{
					"id":          "txnr_failed",
					"status":      "failed",
					"last_attempted_at": int64(1746835200),
				},
			})
		})
		mockStripeBackend(t, mux)

		_, err := WaitForRefresh(context.Background(), nil, "sk_test_xxx", "fca_test_abc")
		if err == nil {
			t.Fatal("expected error for failed refresh, got nil")
		}
		if !strings.Contains(err.Error(), "failed") {
			t.Errorf("error %q does not mention 'failed'", err.Error())
		}
	})

	t.Run("returns empty string when no transaction_refresh", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/accounts/fca_test_abc", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]any{
				"id":       "fca_test_abc",
				"object":   "financial_connections.account",
				"status":   "active",
				"livemode": false,
			})
		})
		mockStripeBackend(t, mux)

		refreshID, err := WaitForRefresh(context.Background(), nil, "sk_test_xxx", "fca_test_abc")
		if err != nil {
			t.Fatalf("WaitForRefresh with no refresh: %v", err)
		}
		if refreshID != "" {
			t.Errorf("refreshID = %q, want empty string", refreshID)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/accounts/fca_test_abc", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]any{
				"id":       "fca_test_abc",
				"object":   "financial_connections.account",
				"status":   "active",
				"livemode": false,
				"transaction_refresh": map[string]any{
					"id":          "txnr_pending",
					"status":      "pending",
					"last_attempted_at": int64(1746835200),
				},
			})
		})
		mockStripeBackend(t, mux)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		_, err := WaitForRefresh(ctx, nil, "sk_test_xxx", "fca_test_abc")
		if err == nil {
			t.Fatal("expected error from cancelled context, got nil")
		}
	})
}

func TestGetTransactionRefreshID(t *testing.T) {
	t.Run("returns refresh ID from account", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/accounts/fca_test_abc", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]any{
				"id":       "fca_test_abc",
				"object":   "financial_connections.account",
				"status":   "active",
				"livemode": false,
				"transaction_refresh": map[string]any{
					"id":          "txnr_abc123",
					"status":      "succeeded",
					"last_attempted_at": int64(1746835200),
				},
			})
		})
		mockStripeBackend(t, mux)

		id, err := GetTransactionRefreshID(context.Background(), "sk_test_xxx", "fca_test_abc")
		if err != nil {
			t.Fatalf("GetTransactionRefreshID: %v", err)
		}
		if id != "txnr_abc123" {
			t.Errorf("id = %q, want %q", id, "txnr_abc123")
		}
	})

	t.Run("returns empty string when TransactionRefresh is nil", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/accounts/fca_test_abc", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]any{
				"id":       "fca_test_abc",
				"object":   "financial_connections.account",
				"status":   "active",
				"livemode": false,
			})
		})
		mockStripeBackend(t, mux)

		id, err := GetTransactionRefreshID(context.Background(), "sk_test_xxx", "fca_test_abc")
		if err != nil {
			t.Fatalf("GetTransactionRefreshID: %v", err)
		}
		if id != "" {
			t.Errorf("id = %q, want empty string", id)
		}
	})
}

func TestWaitForRefreshWithProgress(t *testing.T) {
	t.Run("emits starting then succeeded when status is already succeeded", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/accounts/fca_test_abc", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]any{
				"id":       "fca_test_abc",
				"object":   "financial_connections.account",
				"status":   "active",
				"livemode": false,
				"transaction_refresh": map[string]any{
					"id":                "txnr_success",
					"status":            "succeeded",
					"last_attempted_at": int64(1746835200),
				},
			})
		})
		mockStripeBackend(t, mux)

		var statuses []string
		var lastRefreshID string
		refreshID, err := WaitForRefreshWithProgress(context.Background(), nil, "sk_test_xxx", "fca_test_abc", func(p RefreshProgress) {
			statuses = append(statuses, p.Status)
			if p.RefreshID != "" {
				lastRefreshID = p.RefreshID
			}
		})
		if err != nil {
			t.Fatalf("WaitForRefreshWithProgress: %v", err)
		}
		if refreshID != "txnr_success" {
			t.Errorf("refreshID = %q, want %q", refreshID, "txnr_success")
		}
		if len(statuses) < 2 || statuses[0] != "starting" || statuses[len(statuses)-1] != "succeeded" {
			t.Errorf("statuses = %v, want starting...succeeded", statuses)
		}
		if lastRefreshID != "txnr_success" {
			t.Errorf("lastRefreshID = %q, want %q", lastRefreshID, "txnr_success")
		}
	})

	t.Run("emits polling progress then succeeded across multiple attempts", func(t *testing.T) {
		mux := http.NewServeMux()
		var calls int
		mux.HandleFunc("/v1/financial_connections/accounts/fca_test_abc", func(w http.ResponseWriter, _ *http.Request) {
			calls++
			status := "pending"
			if calls >= 2 {
				status = "succeeded"
			}
			writeJSON(w, map[string]any{
				"id":       "fca_test_abc",
				"object":   "financial_connections.account",
				"status":   "active",
				"livemode": false,
				"transaction_refresh": map[string]any{
					"id":                "txnr_eventual",
					"status":            status,
					"last_attempted_at": int64(1746835200),
				},
			})
		})
		mockStripeBackend(t, mux)

		// Use a context with a short deadline so the test fails fast if polling
		// gets stuck; the mock should transition on the second call.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var sawPolling, sawSucceeded bool
		var maxAttempt int
		refreshID, err := WaitForRefreshWithProgress(ctx, nil, "sk_test_xxx", "fca_test_abc", func(p RefreshProgress) {
			if p.Status == "polling" {
				sawPolling = true
			}
			if p.Status == "succeeded" {
				sawSucceeded = true
			}
			if p.Attempt > maxAttempt {
				maxAttempt = p.Attempt
			}
		})
		if err != nil {
			t.Fatalf("WaitForRefreshWithProgress: %v", err)
		}
		if refreshID != "txnr_eventual" {
			t.Errorf("refreshID = %q, want %q", refreshID, "txnr_eventual")
		}
		if !sawPolling {
			t.Error("expected at least one polling progress event")
		}
		if !sawSucceeded {
			t.Error("expected a succeeded progress event")
		}
		if maxAttempt < 2 {
			t.Errorf("maxAttempt = %d, want >= 2", maxAttempt)
		}
	})
}
