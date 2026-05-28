package ledger_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	stripelib "github.com/stripe/stripe-go/v82"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
	serverledger "github.com/brendanv/float/internal/server/ledger"
	"github.com/brendanv/float/internal/testgen"
	"github.com/brendanv/float/internal/txlock"
)

func mustHandlerWithConfig(t *testing.T, dir string, cfg *config.Config) *serverledger.Handler {
	t.Helper()
	c, err := hledger.New("hledger", dir+"/main.journal")
	if err != nil {
		t.Skipf("hledger unavailable: %v", err)
	}
	configPath := filepath.Join(dir, "config.toml")
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	lock := txlock.New(dir, c)
	return serverledger.NewHandler(c, lock, dir, configPath, nil, nil, cfg, nil)
}

func mockStripeAPI(t *testing.T, mux *http.ServeMux) {
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

func writeStripeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func importStripeTransactions(t *testing.T, h *serverledger.Handler, req *floatv1.ImportStripeTransactionsRequest) (*floatv1.ImportTransactionsResult, error) {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := floatv1connect.NewLedgerServiceHandler(h)
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := floatv1connect.NewLedgerServiceClient(srv.Client(), srv.URL)
	stream, err := client.ImportStripeTransactions(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { _ = stream.Close() })

	var result *floatv1.ImportTransactionsResult
	for stream.Receive() {
		if p, ok := stream.Msg().Payload.(*floatv1.ImportTransactionsResponse_Result); ok {
			result = p.Result
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func TestGetStripeConfig(t *testing.T) {
	t.Run("disabled when env var not set", func(t *testing.T) {
		t.Setenv("STRIPE_SECRET_KEY", "")
		t.Setenv("STRIPE_PUBLISHABLE_KEY", "")
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 700, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{
			Stripe: config.StripeConfig{
				LinkedAccounts: []config.StripeLinkedAccount{
					{StripeAccountID: "fca_abc", HledgerAccount: "assets:checking"},
				},
			},
		})
		resp, err := h.GetStripeConfig(t.Context(), connect.NewRequest(&floatv1.GetStripeConfigRequest{}))
		if err != nil {
			t.Fatalf("GetStripeConfig: %v", err)
		}
		if resp.Msg.Enabled {
			t.Error("Enabled = true, want false when STRIPE_SECRET_KEY is not set")
		}
		if resp.Msg.PublishableKey != "" {
			t.Errorf("PublishableKey = %q, want empty", resp.Msg.PublishableKey)
		}
		if resp.Msg.LinkedAccountCount != 1 {
			t.Errorf("LinkedAccountCount = %d, want 1", resp.Msg.LinkedAccountCount)
		}
	})

	t.Run("enabled when env var is set", func(t *testing.T) {
		t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")
		t.Setenv("STRIPE_PUBLISHABLE_KEY", "pk_test_xxx")
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 701, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{})
		resp, err := h.GetStripeConfig(t.Context(), connect.NewRequest(&floatv1.GetStripeConfigRequest{}))
		if err != nil {
			t.Fatalf("GetStripeConfig: %v", err)
		}
		if !resp.Msg.Enabled {
			t.Error("Enabled = false, want true when STRIPE_SECRET_KEY is set")
		}
		if resp.Msg.PublishableKey != "pk_test_xxx" {
			t.Errorf("PublishableKey = %q, want %q", resp.Msg.PublishableKey, "pk_test_xxx")
		}
		if resp.Msg.LinkedAccountCount != 0 {
			t.Errorf("LinkedAccountCount = %d, want 0", resp.Msg.LinkedAccountCount)
		}
	})

	t.Run("no config returns failed precondition", func(t *testing.T) {
		c, err := hledger.New("hledger", "testdata/simple.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}
		h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil, nil)
		_, err = h.GetStripeConfig(t.Context(), connect.NewRequest(&floatv1.GetStripeConfigRequest{}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Errorf("code = %v, want FailedPrecondition", connect.CodeOf(err))
		}
	})
}

func stripeAccountListResponse(accounts []map[string]any) map[string]any {
	return map[string]any{
		"object":   "list",
		"data":     accounts,
		"has_more": false,
		"url":      "/v1/financial_connections/accounts",
	}
}

func TestListStripeLinkedAccounts(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")

	t.Run("merges stripe accounts with config overlay", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/accounts", func(w http.ResponseWriter, _ *http.Request) {
			writeStripeJSON(w, stripeAccountListResponse([]map[string]any{
				{"id": "fca_abc", "object": "financial_connections.account", "display_name": "Stripe Name", "institution_name": "Chase", "last4": "1234", "livemode": false, "status": "active"},
				{"id": "fca_xyz", "object": "financial_connections.account", "display_name": "Savings Account", "institution_name": "Ally", "last4": "5678", "livemode": false, "status": "active"},
			}))
		})
		mockStripeAPI(t, mux)

		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 710, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{
			Stripe: config.StripeConfig{
				LinkedAccounts: []config.StripeLinkedAccount{
					{StripeAccountID: "fca_abc", HledgerAccount: "assets:checking", DisplayName: "Chase Checking", LastFetchedAt: "2026-05-01T00:00:00Z"},
					{StripeAccountID: "fca_xyz", HledgerAccount: "assets:savings", DisplayName: ""},
				},
			},
		})
		resp, err := h.ListStripeLinkedAccounts(t.Context(), connect.NewRequest(&floatv1.ListStripeLinkedAccountsRequest{}))
		if err != nil {
			t.Fatalf("ListStripeLinkedAccounts: %v", err)
		}
		if len(resp.Msg.Accounts) != 2 {
			t.Fatalf("got %d accounts, want 2", len(resp.Msg.Accounts))
		}
		got := resp.Msg.Accounts[0]
		if got.StripeAccountId != "fca_abc" {
			t.Errorf("StripeAccountId = %q, want %q", got.StripeAccountId, "fca_abc")
		}
		if got.HledgerAccount != "assets:checking" {
			t.Errorf("HledgerAccount = %q, want %q", got.HledgerAccount, "assets:checking")
		}
		if got.DisplayName != "Chase Checking" {
			t.Errorf("DisplayName = %q, want %q (config name should win)", got.DisplayName, "Chase Checking")
		}
		if got.LastFetchedAt != "2026-05-01T00:00:00Z" {
			t.Errorf("LastFetchedAt = %q, want %q", got.LastFetchedAt, "2026-05-01T00:00:00Z")
		}
		if got.InstitutionName != "Chase" {
			t.Errorf("InstitutionName = %q, want %q", got.InstitutionName, "Chase")
		}
		got2 := resp.Msg.Accounts[1]
		if got2.DisplayName != "Savings Account" {
			t.Errorf("account[1].DisplayName = %q, want %q (stripe name when config display_name is empty)", got2.DisplayName, "Savings Account")
		}
	})

	t.Run("unconfigured stripe accounts appear without hledger mapping", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/accounts", func(w http.ResponseWriter, _ *http.Request) {
			writeStripeJSON(w, stripeAccountListResponse([]map[string]any{
				{"id": "fca_new", "object": "financial_connections.account", "display_name": "New Bank", "institution_name": "WF", "last4": "9999", "livemode": false, "status": "active"},
			}))
		})
		mockStripeAPI(t, mux)

		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 711, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{})
		resp, err := h.ListStripeLinkedAccounts(t.Context(), connect.NewRequest(&floatv1.ListStripeLinkedAccountsRequest{}))
		if err != nil {
			t.Fatalf("ListStripeLinkedAccounts: %v", err)
		}
		if len(resp.Msg.Accounts) != 1 {
			t.Fatalf("got %d accounts, want 1", len(resp.Msg.Accounts))
		}
		got := resp.Msg.Accounts[0]
		if got.HledgerAccount != "" {
			t.Errorf("HledgerAccount = %q, want empty for unconfigured account", got.HledgerAccount)
		}
		if got.InstitutionName != "WF" {
			t.Errorf("InstitutionName = %q, want %q", got.InstitutionName, "WF")
		}
	})

	t.Run("disconnected accounts are excluded", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/accounts", func(w http.ResponseWriter, _ *http.Request) {
			writeStripeJSON(w, stripeAccountListResponse([]map[string]any{
				{"id": "fca_active", "object": "financial_connections.account", "display_name": "Active Bank", "institution_name": "Chase", "last4": "1111", "livemode": false, "status": "active"},
				{"id": "fca_disconnected", "object": "financial_connections.account", "display_name": "Old Bank", "institution_name": "BoA", "last4": "2222", "livemode": false, "status": "disconnected"},
			}))
		})
		mockStripeAPI(t, mux)

		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 713, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{})
		resp, err := h.ListStripeLinkedAccounts(t.Context(), connect.NewRequest(&floatv1.ListStripeLinkedAccountsRequest{}))
		if err != nil {
			t.Fatalf("ListStripeLinkedAccounts: %v", err)
		}
		if len(resp.Msg.Accounts) != 1 {
			t.Fatalf("got %d accounts, want 1 (disconnected should be excluded)", len(resp.Msg.Accounts))
		}
		if resp.Msg.Accounts[0].StripeAccountId != "fca_active" {
			t.Errorf("StripeAccountId = %q, want %q", resp.Msg.Accounts[0].StripeAccountId, "fca_active")
		}
	})

	t.Run("empty list when stripe returns no accounts", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/accounts", func(w http.ResponseWriter, _ *http.Request) {
			writeStripeJSON(w, stripeAccountListResponse([]map[string]any{}))
		})
		mockStripeAPI(t, mux)

		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 712, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{})
		resp, err := h.ListStripeLinkedAccounts(t.Context(), connect.NewRequest(&floatv1.ListStripeLinkedAccountsRequest{}))
		if err != nil {
			t.Fatalf("ListStripeLinkedAccounts: %v", err)
		}
		if len(resp.Msg.Accounts) != 0 {
			t.Errorf("got %d accounts, want 0", len(resp.Msg.Accounts))
		}
	})
}

func TestUnlinkStripeAccount(t *testing.T) {
	t.Run("missing stripe_account_id returns invalid argument", func(t *testing.T) {
		t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 720, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{
			Stripe: config.StripeConfig{
				LinkedAccounts: []config.StripeLinkedAccount{
					{StripeAccountID: "fca_abc", HledgerAccount: "assets:checking"},
				},
			},
		})
		_, err := h.UnlinkStripeAccount(t.Context(), connect.NewRequest(&floatv1.UnlinkStripeAccountRequest{StripeAccountId: ""}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("missing STRIPE_SECRET_KEY returns failed precondition", func(t *testing.T) {
		t.Setenv("STRIPE_SECRET_KEY", "")
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 723, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{
			Stripe: config.StripeConfig{
				LinkedAccounts: []config.StripeLinkedAccount{
					{StripeAccountID: "fca_abc", HledgerAccount: "assets:checking"},
				},
			},
		})
		_, err := h.UnlinkStripeAccount(t.Context(), connect.NewRequest(&floatv1.UnlinkStripeAccountRequest{StripeAccountId: "fca_abc"}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Errorf("code = %v, want FailedPrecondition", connect.CodeOf(err))
		}
	})

	t.Run("removes matching account after stripe disconnect", func(t *testing.T) {
		t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/accounts/fca_remove/disconnect", func(w http.ResponseWriter, _ *http.Request) {
			writeStripeJSON(w, map[string]any{
				"id":     "fca_remove",
				"object": "financial_connections.account",
				"status": "disconnected",
			})
		})
		mockStripeAPI(t, mux)

		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 721, NumTxns: 1})
		cfg := &config.Config{
			Stripe: config.StripeConfig{
				LinkedAccounts: []config.StripeLinkedAccount{
					{StripeAccountID: "fca_keep", HledgerAccount: "assets:savings"},
					{StripeAccountID: "fca_remove", HledgerAccount: "assets:checking"},
				},
			},
		}
		h := mustHandlerWithConfig(t, dir, cfg)

		_, err := h.UnlinkStripeAccount(t.Context(), connect.NewRequest(&floatv1.UnlinkStripeAccountRequest{
			StripeAccountId: "fca_remove",
		}))
		if err != nil {
			t.Fatalf("UnlinkStripeAccount: %v", err)
		}

		configPath := filepath.Join(dir, "config.toml")
		savedCfg, err := config.Load(configPath)
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		if len(savedCfg.Stripe.LinkedAccounts) != 1 {
			t.Fatalf("got %d linked accounts, want 1", len(savedCfg.Stripe.LinkedAccounts))
		}
		if savedCfg.Stripe.LinkedAccounts[0].StripeAccountID != "fca_keep" {
			t.Errorf("remaining account = %q, want %q", savedCfg.Stripe.LinkedAccounts[0].StripeAccountID, "fca_keep")
		}
	})

	t.Run("stripe disconnect error leaves config unchanged", func(t *testing.T) {
		t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/accounts/fca_does_not_exist/disconnect", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"type":    "invalid_request_error",
					"message": "No such financial connections account: fca_does_not_exist",
					"code":    "resource_missing",
				},
			})
		})
		mockStripeAPI(t, mux)

		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 722, NumTxns: 1})
		cfg := &config.Config{
			Stripe: config.StripeConfig{
				LinkedAccounts: []config.StripeLinkedAccount{
					{StripeAccountID: "fca_keep", HledgerAccount: "assets:checking"},
				},
			},
		}
		h := mustHandlerWithConfig(t, dir, cfg)

		_, err := h.UnlinkStripeAccount(t.Context(), connect.NewRequest(&floatv1.UnlinkStripeAccountRequest{
			StripeAccountId: "fca_does_not_exist",
		}))
		if err == nil {
			t.Fatal("expected error from stripe disconnect, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInternal {
			t.Errorf("code = %v, want Internal", connect.CodeOf(err))
		}

		configPath := filepath.Join(dir, "config.toml")
		savedCfg, err := config.Load(configPath)
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		if len(savedCfg.Stripe.LinkedAccounts) != 1 {
			t.Errorf("got %d linked accounts, want 1 (config should be unchanged)", len(savedCfg.Stripe.LinkedAccounts))
		}
	})
}

func TestCreateStripeLinkSession(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")

	t.Run("creates customer and session when no customer id in config", func(t *testing.T) {
		customerCreated := false
		sessionCreated := false
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/customers", func(w http.ResponseWriter, _ *http.Request) {
			customerCreated = true
			writeStripeJSON(w, map[string]any{
				"id":     "cus_new123",
				"object": "customer",
			})
		})
		mux.HandleFunc("/v1/financial_connections/sessions", func(w http.ResponseWriter, _ *http.Request) {
			sessionCreated = true
			writeStripeJSON(w, map[string]any{
				"id":            "fcsess_new123",
				"object":        "financial_connections.session",
				"client_secret": "fcsess_new123_secret",
				"livemode":      false,
				"account_holder": map[string]any{"type": "customer"},
				"accounts": map[string]any{
					"object": "list", "data": []any{}, "has_more": false,
					"url": "/v1/financial_connections/accounts",
				},
				"permissions": []string{"transactions"},
			})
		})
		mockStripeAPI(t, mux)

		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 730, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{})
		resp, err := h.CreateStripeLinkSession(t.Context(), connect.NewRequest(&floatv1.CreateStripeLinkSessionRequest{}))
		if err != nil {
			t.Fatalf("CreateStripeLinkSession: %v", err)
		}
		if !customerCreated {
			t.Error("customer was not created")
		}
		if !sessionCreated {
			t.Error("session was not created")
		}
		if resp.Msg.ClientSecret != "fcsess_new123_secret" {
			t.Errorf("ClientSecret = %q, want %q", resp.Msg.ClientSecret, "fcsess_new123_secret")
		}
		configPath := filepath.Join(dir, "config.toml")
		savedCfg, err := config.Load(configPath)
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		if savedCfg.Stripe.CustomerID != "cus_new123" {
			t.Errorf("CustomerID = %q, want %q", savedCfg.Stripe.CustomerID, "cus_new123")
		}
	})

	t.Run("reuses existing customer id from config", func(t *testing.T) {
		customerCreated := false
		sessionCreated := false
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/customers", func(w http.ResponseWriter, _ *http.Request) {
			customerCreated = true
			writeStripeJSON(w, map[string]any{"id": "cus_should_not_be_created", "object": "customer"})
		})
		mux.HandleFunc("/v1/financial_connections/sessions", func(w http.ResponseWriter, _ *http.Request) {
			sessionCreated = true
			writeStripeJSON(w, map[string]any{
				"id":            "fcsess_reuse123",
				"object":        "financial_connections.session",
				"client_secret": "fcsess_reuse123_secret",
				"livemode":      false,
				"account_holder": map[string]any{"type": "customer"},
				"accounts": map[string]any{
					"object": "list", "data": []any{}, "has_more": false,
					"url": "/v1/financial_connections/accounts",
				},
				"permissions": []string{"transactions"},
			})
		})
		mockStripeAPI(t, mux)

		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 731, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{Stripe: config.StripeConfig{CustomerID: "cus_existing456"}})
		resp, err := h.CreateStripeLinkSession(t.Context(), connect.NewRequest(&floatv1.CreateStripeLinkSessionRequest{}))
		if err != nil {
			t.Fatalf("CreateStripeLinkSession: %v", err)
		}
		if customerCreated {
			t.Error("customer should not have been created when one already exists")
		}
		if !sessionCreated {
			t.Error("session was not created")
		}
		if resp.Msg.ClientSecret != "fcsess_reuse123_secret" {
			t.Errorf("ClientSecret = %q, want %q", resp.Msg.ClientSecret, "fcsess_reuse123_secret")
		}
	})

	t.Run("missing secret key returns failed precondition", func(t *testing.T) {
		t.Setenv("STRIPE_SECRET_KEY", "")
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 732, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{})
		_, err := h.CreateStripeLinkSession(t.Context(), connect.NewRequest(&floatv1.CreateStripeLinkSessionRequest{}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Errorf("code = %v, want FailedPrecondition", connect.CodeOf(err))
		}
	})
}

func TestCompleteStripeLinking(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")

	t.Run("empty accounts returns invalid argument", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 740, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{})
		_, err := h.CompleteStripeLinking(t.Context(), connect.NewRequest(&floatv1.CompleteStripeLinkingRequest{
			Accounts: []*floatv1.LinkedAccountInput{},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("missing hledger_account returns invalid argument", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 741, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{})
		_, err := h.CompleteStripeLinking(t.Context(), connect.NewRequest(&floatv1.CompleteStripeLinkingRequest{
			Accounts: []*floatv1.LinkedAccountInput{
				{StripeAccountId: "fca_abc", HledgerAccount: ""},
			},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("saves linked accounts and subscribes", func(t *testing.T) {
		subscribed := []string{}
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/accounts/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/subscribe") {
				parts := strings.Split(r.URL.Path, "/")
				acctID := parts[len(parts)-2]
				subscribed = append(subscribed, acctID)
				writeStripeJSON(w, map[string]any{
					"id": acctID, "object": "financial_connections.account",
					"status": "active", "subscriptions": []string{"transactions"}, "livemode": false,
				})
				return
			}
			http.NotFound(w, r)
		})
		mockStripeAPI(t, mux)

		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 742, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{})

		resp, err := h.CompleteStripeLinking(t.Context(), connect.NewRequest(&floatv1.CompleteStripeLinkingRequest{
			Accounts: []*floatv1.LinkedAccountInput{
				{StripeAccountId: "fca_new1", HledgerAccount: "assets:checking", DisplayName: "Chase Checking"},
				{StripeAccountId: "fca_new2", HledgerAccount: "assets:savings", DisplayName: "Savings"},
			},
		}))
		if err != nil {
			t.Fatalf("CompleteStripeLinking: %v", err)
		}
		if len(resp.Msg.LinkedAccounts) != 2 {
			t.Fatalf("got %d linked accounts, want 2", len(resp.Msg.LinkedAccounts))
		}
		if len(subscribed) != 2 {
			t.Errorf("subscribed %d accounts, want 2: %v", len(subscribed), subscribed)
		}

		configPath := filepath.Join(dir, "config.toml")
		savedCfg, err := config.Load(configPath)
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		if len(savedCfg.Stripe.LinkedAccounts) != 2 {
			t.Fatalf("saved %d linked accounts, want 2", len(savedCfg.Stripe.LinkedAccounts))
		}
		if savedCfg.Stripe.LinkedAccounts[0].HledgerAccount != "assets:checking" {
			t.Errorf("account[0].HledgerAccount = %q, want %q", savedCfg.Stripe.LinkedAccounts[0].HledgerAccount, "assets:checking")
		}
	})

	t.Run("updates existing linked account", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/accounts/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/subscribe") {
				parts := strings.Split(r.URL.Path, "/")
				acctID := parts[len(parts)-2]
				writeStripeJSON(w, map[string]any{
					"id": acctID, "object": "financial_connections.account",
					"status": "active", "subscriptions": []string{"transactions"}, "livemode": false,
				})
				return
			}
			http.NotFound(w, r)
		})
		mockStripeAPI(t, mux)

		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 743, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{
			Stripe: config.StripeConfig{
				LinkedAccounts: []config.StripeLinkedAccount{
					{StripeAccountID: "fca_existing", HledgerAccount: "assets:old", DisplayName: "Old Name"},
				},
			},
		})

		resp, err := h.CompleteStripeLinking(t.Context(), connect.NewRequest(&floatv1.CompleteStripeLinkingRequest{
			Accounts: []*floatv1.LinkedAccountInput{
				{StripeAccountId: "fca_existing", HledgerAccount: "assets:checking", DisplayName: "New Name"},
			},
		}))
		if err != nil {
			t.Fatalf("CompleteStripeLinking: %v", err)
		}
		if len(resp.Msg.LinkedAccounts) != 1 {
			t.Fatalf("got %d linked accounts, want 1", len(resp.Msg.LinkedAccounts))
		}
		got := resp.Msg.LinkedAccounts[0]
		if got.HledgerAccount != "assets:checking" {
			t.Errorf("HledgerAccount = %q, want %q", got.HledgerAccount, "assets:checking")
		}
		if got.DisplayName != "New Name" {
			t.Errorf("DisplayName = %q, want %q", got.DisplayName, "New Name")
		}
	})
}

func TestFetchStripeTransactions(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")

	t.Run("missing stripe_account_id returns invalid argument", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 750, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{
			Stripe: config.StripeConfig{
				LinkedAccounts: []config.StripeLinkedAccount{
					{StripeAccountID: "fca_abc", HledgerAccount: "assets:checking"},
				},
			},
		})
		_, err := h.FetchStripeTransactions(t.Context(), connect.NewRequest(&floatv1.FetchStripeTransactionsRequest{
			StripeAccountId: "",
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("unknown account returns not found", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/accounts/", func(w http.ResponseWriter, _ *http.Request) {
			writeStripeJSON(w, map[string]any{"id": "fca_abc", "object": "financial_connections.account", "status": "active", "livemode": false})
		})
		mockStripeAPI(t, mux)

		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 751, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{})
		_, err := h.FetchStripeTransactions(t.Context(), connect.NewRequest(&floatv1.FetchStripeTransactionsRequest{
			StripeAccountId: "fca_not_linked",
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("code = %v, want NotFound", connect.CodeOf(err))
		}
	})

	t.Run("returns candidates from stripe transactions", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/accounts/", func(w http.ResponseWriter, _ *http.Request) {
			writeStripeJSON(w, map[string]any{"id": "fca_abc", "object": "financial_connections.account", "status": "active", "livemode": false})
		})
		mux.HandleFunc("/v1/financial_connections/transactions", func(w http.ResponseWriter, _ *http.Request) {
			writeStripeJSON(w, map[string]any{
				"object": "list",
				"data": []any{
					map[string]any{
						"id": "fca_txn_new1", "object": "financial_connections.transaction",
						"account": "fca_abc", "amount": int64(9999), "currency": "usd",
						"description": "AMAZON MARKETPLACE", "transacted_at": int64(1746835200),
						"status": "posted", "livemode": false,
					},
					map[string]any{
						"id": "fca_txn_new2", "object": "financial_connections.transaction",
						"account": "fca_abc", "amount": int64(1500), "currency": "usd",
						"description": "COFFEE SHOP", "transacted_at": int64(1746748800),
						"status": "pending", "livemode": false,
					},
				},
				"has_more": false, "url": "/v1/financial_connections/transactions",
			})
		})
		mockStripeAPI(t, mux)

		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 752, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{
			Stripe: config.StripeConfig{
				LinkedAccounts: []config.StripeLinkedAccount{
					{StripeAccountID: "fca_abc", HledgerAccount: "assets:checking"},
				},
			},
		})

		resp, err := h.FetchStripeTransactions(t.Context(), connect.NewRequest(&floatv1.FetchStripeTransactionsRequest{
			StripeAccountId: "fca_abc",
		}))
		if err != nil {
			t.Fatalf("FetchStripeTransactions: %v", err)
		}
		if len(resp.Msg.Candidates) != 1 {
			t.Fatalf("got %d candidates, want 1 (pending excluded)", len(resp.Msg.Candidates))
		}
		for _, c := range resp.Msg.Candidates {
			if c.Transaction == nil {
				t.Error("candidate has nil transaction")
			}
		}
		if resp.Msg.Candidates[0].IsDuplicate {
			t.Errorf("candidate[0].IsDuplicate = true, want false")
		}
	})

	t.Run("omits transaction whose stripe id is already imported", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/accounts/", func(w http.ResponseWriter, _ *http.Request) {
			writeStripeJSON(w, map[string]any{"id": "fca_abc", "object": "financial_connections.account", "status": "active", "livemode": false})
		})
		mux.HandleFunc("/v1/financial_connections/transactions", func(w http.ResponseWriter, _ *http.Request) {
			writeStripeJSON(w, map[string]any{
				"object": "list",
				"data": []any{
					map[string]any{
						"id": "fca_txn_dup", "object": "financial_connections.transaction",
						"account": "fca_abc", "amount": int64(5000), "currency": "usd",
						"description": "GROCERY STORE", "transacted_at": int64(1746835200),
						"status": "posted", "livemode": false,
					},
					map[string]any{
						"id": "fca_txn_fresh", "object": "financial_connections.transaction",
						"account": "fca_abc", "amount": int64(2500), "currency": "usd",
						"description": "BOOK STORE", "transacted_at": int64(1746835200),
						"status": "posted", "livemode": false,
					},
				},
				"has_more": false, "url": "/v1/financial_connections/transactions",
			})
		})
		mockStripeAPI(t, mux)

		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 753, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{Stripe: config.StripeConfig{LinkedAccounts: []config.StripeLinkedAccount{{StripeAccountID: "fca_abc", HledgerAccount: "assets:checking"}}}})

		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Fatalf("hledger client: %v", err)
		}
		if _, err := journal.AppendTransaction(t.Context(), c, dir, journal.TransactionInput{
			Date:        time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
			Description: "already imported",
			FloatMeta:   map[string]string{"float-stripe-txn": "fca_txn_dup"},
			Postings: []journal.PostingInput{{Account: "assets:checking", Commodity: "USD", Quantity: "50.00"}, {Account: "expenses:unknown", Commodity: "USD", Quantity: "-50.00"}},
		}); err != nil {
			t.Fatalf("append transaction: %v", err)
		}

		resp, err := h.FetchStripeTransactions(t.Context(), connect.NewRequest(&floatv1.FetchStripeTransactionsRequest{StripeAccountId: "fca_abc"}))
		if err != nil {
			t.Fatalf("FetchStripeTransactions: %v", err)
		}
		// The already-imported transaction is hidden by the backend; only the fresh one remains.
		if len(resp.Msg.Candidates) != 1 {
			t.Fatalf("got %d candidates, want 1 (duplicate hidden)", len(resp.Msg.Candidates))
		}
		if resp.Msg.Candidates[0].SourceId != "fca_txn_fresh" {
			t.Errorf("candidate SourceId = %q, want %q", resp.Msg.Candidates[0].SourceId, "fca_txn_fresh")
		}
	})
}

func TestFetchStripeTransactions_OnlySettled(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/financial_connections/accounts/", func(w http.ResponseWriter, _ *http.Request) {
		writeStripeJSON(w, map[string]any{"id": "fca_abc", "object": "financial_connections.account", "status": "active", "livemode": false})
	})
	mux.HandleFunc("/v1/financial_connections/transactions", func(w http.ResponseWriter, _ *http.Request) {
		writeStripeJSON(w, map[string]any{
			"object": "list",
			"data": []any{
				map[string]any{
					"id": "fca_txn_excl_posted", "object": "financial_connections.transaction",
					"account": "fca_abc", "amount": int64(5000), "currency": "usd",
					"description": "GROCERY STORE", "transacted_at": int64(1746835200),
					"status": "posted", "livemode": false,
				},
				map[string]any{
					"id": "fca_txn_excl_pending", "object": "financial_connections.transaction",
					"account": "fca_abc", "amount": int64(1200), "currency": "usd",
					"description": "GAS STATION", "transacted_at": int64(1746921600),
					"status": "pending", "livemode": false,
				},
				map[string]any{
					"id": "fca_txn_excl_void", "object": "financial_connections.transaction",
					"account": "fca_abc", "amount": int64(800), "currency": "usd",
					"description": "REFUNDED CHARGE", "transacted_at": int64(1746921600),
					"status": "void", "livemode": false,
				},
			},
			"has_more": false, "url": "/v1/financial_connections/transactions",
		})
	})
	mockStripeAPI(t, mux)

	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 754, NumTxns: 1})
	h := mustHandlerWithConfig(t, dir, &config.Config{
		Stripe: config.StripeConfig{
			LinkedAccounts: []config.StripeLinkedAccount{
				{StripeAccountID: "fca_abc", HledgerAccount: "assets:checking"},
			},
		},
	})

	resp, err := h.FetchStripeTransactions(t.Context(), connect.NewRequest(&floatv1.FetchStripeTransactionsRequest{
		StripeAccountId: "fca_abc",
	}))
	if err != nil {
		t.Fatalf("FetchStripeTransactions: %v", err)
	}
	if len(resp.Msg.Candidates) != 1 {
		t.Fatalf("got %d candidates, want 1 (only posted/settled transactions)", len(resp.Msg.Candidates))
	}
	if resp.Msg.Candidates[0].SourceId != "fca_txn_excl_posted" {
		t.Errorf("candidate SourceId = %q, want %q", resp.Msg.Candidates[0].SourceId, "fca_txn_excl_posted")
	}
}

func TestImportStripeTransactions(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")

	t.Run("missing stripe_account_id returns invalid argument", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 760, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{
			Stripe: config.StripeConfig{
				LinkedAccounts: []config.StripeLinkedAccount{
					{StripeAccountID: "fca_abc", HledgerAccount: "assets:checking"},
				},
			},
		})
		_, err := importStripeTransactions(t, h, &floatv1.ImportStripeTransactionsRequest{
			StripeAccountId:      "",
			StripeTransactionIds: []string{"fca_txn_001"},
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("empty stripe_transaction_ids returns invalid argument", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 761, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{
			Stripe: config.StripeConfig{
				LinkedAccounts: []config.StripeLinkedAccount{
					{StripeAccountID: "fca_abc", HledgerAccount: "assets:checking"},
				},
			},
		})
		_, err := importStripeTransactions(t, h, &floatv1.ImportStripeTransactionsRequest{
			StripeAccountId:      "fca_abc",
			StripeTransactionIds: []string{},
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("imports selected transactions", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/transactions", func(w http.ResponseWriter, _ *http.Request) {
			writeStripeJSON(w, map[string]any{
				"object": "list",
				"data": []any{
					map[string]any{
						"id": "fca_txn_imp1", "object": "financial_connections.transaction",
						"account": "fca_abc", "amount": int64(5000), "currency": "usd",
						"description": "GROCERY STORE", "transacted_at": int64(1746835200),
						"status": "posted", "livemode": false,
					},
					map[string]any{
						"id": "fca_txn_imp2", "object": "financial_connections.transaction",
						"account": "fca_abc", "amount": int64(1200), "currency": "usd",
						"description": "COFFEE SHOP", "transacted_at": int64(1746748800),
						"status": "posted", "livemode": false,
					},
				},
				"has_more": false, "url": "/v1/financial_connections/transactions",
			})
		})
		mockStripeAPI(t, mux)

		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 762, NumTxns: 1})
		if err := os.WriteFile(filepath.Join(dir, "rules.json"), []byte(`[]`), 0644); err != nil {
			t.Fatalf("write rules.json: %v", err)
		}
		cfg := &config.Config{
			Stripe: config.StripeConfig{
				LinkedAccounts: []config.StripeLinkedAccount{
					{StripeAccountID: "fca_abc", HledgerAccount: "assets:checking"},
				},
			},
		}
		h := mustHandlerWithConfig(t, dir, cfg)

		result, err := importStripeTransactions(t, h, &floatv1.ImportStripeTransactionsRequest{
			StripeAccountId:      "fca_abc",
			StripeTransactionIds: []string{"fca_txn_imp1"},
		})
		if err != nil {
			t.Fatalf("ImportStripeTransactions: %v", err)
		}
		if result.ImportedCount != 1 {
			t.Errorf("ImportedCount = %d, want 1", result.ImportedCount)
		}
		if !strings.HasPrefix(result.ImportBatchId, "stripe-fca-abc/") {
			t.Errorf("ImportBatchId = %q, expected prefix %q", result.ImportBatchId, "stripe-fca-abc/")
		}

		configPath := filepath.Join(dir, "config.toml")
		savedCfg, err := config.Load(configPath)
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		if savedCfg.Stripe.LinkedAccounts[0].LastFetchedAt == "" {
			t.Error("LastFetchedAt was not updated after import")
		}
	})

	t.Run("skips already imported stripe transaction ids", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/accounts/", func(w http.ResponseWriter, _ *http.Request) {
			writeStripeJSON(w, map[string]any{"id": "fca_abc", "object": "financial_connections.account", "status": "active", "livemode": false})
		})
		mux.HandleFunc("/v1/financial_connections/transactions", func(w http.ResponseWriter, _ *http.Request) {
			writeStripeJSON(w, map[string]any{"object": "list", "data": []any{map[string]any{"id": "fca_txn_dup", "object": "financial_connections.transaction", "account": "fca_abc", "amount": int64(5000), "currency": "usd", "description": "GROCERY STORE", "transacted_at": int64(1746835200), "status": "posted", "livemode": false}}, "has_more": false, "url": "/v1/financial_connections/transactions"})
		})
		mockStripeAPI(t, mux)

		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 763, NumTxns: 1})
		if err := os.WriteFile(filepath.Join(dir, "rules.json"), []byte(`[]`), 0644); err != nil {
			t.Fatalf("write rules.json: %v", err)
		}
		cfg := &config.Config{Stripe: config.StripeConfig{LinkedAccounts: []config.StripeLinkedAccount{{StripeAccountID: "fca_abc", HledgerAccount: "assets:checking"}}}}
		h := mustHandlerWithConfig(t, dir, cfg)

		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Fatalf("hledger client: %v", err)
		}
		if _, err := journal.AppendTransaction(t.Context(), c, dir, journal.TransactionInput{Date: time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC), Description: "already imported", FloatMeta: map[string]string{"float-stripe-txn": "fca_txn_dup"}, Postings: []journal.PostingInput{{Account: "assets:checking", Commodity: "USD", Quantity: "50.00"}, {Account: "expenses:unknown", Commodity: "USD", Quantity: "-50.00"}}}); err != nil {
			t.Fatalf("append transaction: %v", err)
		}

		result, err := importStripeTransactions(t, h, &floatv1.ImportStripeTransactionsRequest{StripeAccountId: "fca_abc", StripeTransactionIds: []string{"fca_txn_dup"}})
		if err != nil {
			t.Fatalf("ImportStripeTransactions: %v", err)
		}
		if result.ImportedCount != 0 {
			t.Errorf("ImportedCount = %d, want 0", result.ImportedCount)
		}
	})

	t.Run("does not duplicate transaction when concurrent auto-import wrote it between pre-fetch and lock", func(t *testing.T) {
		// Simulates the race where daily auto-import writes a transaction between the time
		// ImportStripeTransactions builds its importedStripeTxnSet and when it acquires the
		// lock. Without the in-lock refresh the same transaction would be appended twice.
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/accounts/", func(w http.ResponseWriter, _ *http.Request) {
			writeStripeJSON(w, map[string]any{
				"id": "fca_abc", "object": "financial_connections.account",
				"status": "active", "livemode": false,
			})
		})
		mux.HandleFunc("/v1/financial_connections/transactions", func(w http.ResponseWriter, _ *http.Request) {
			writeStripeJSON(w, map[string]any{
				"object": "list",
				"data": []any{
					map[string]any{
						"id": "fca_txn_dedup1", "object": "financial_connections.transaction",
						"account": "fca_abc", "amount": int64(-3000), "currency": "usd",
						"description": "COFFEE SHOP", "transacted_at": int64(1746835200),
						"status": "posted", "livemode": false,
					},
				},
				"has_more": false, "url": "/v1/financial_connections/transactions",
			})
		})
		mockStripeAPI(t, mux)

		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 772, NumTxns: 1})
		if err := os.WriteFile(filepath.Join(dir, "rules.json"), []byte(`[]`), 0644); err != nil {
			t.Fatalf("write rules.json: %v", err)
		}
		cfg := &config.Config{
			Stripe: config.StripeConfig{
				LinkedAccounts: []config.StripeLinkedAccount{
					{StripeAccountID: "fca_abc", HledgerAccount: "assets:checking"},
				},
			},
		}
		h := mustHandlerWithConfig(t, dir, cfg)

		c, cErr := hledger.New("hledger", dir+"/main.journal")
		if cErr != nil {
			t.Fatalf("hledger client: %v", cErr)
		}

		// Hook writes the transaction to the journal between the pre-fetch set build and
		// the lock — simulating a concurrent daily auto-import write.
		serverledger.ExportedSetAfterImportPreFetch(h, func() {
			if _, writeErr := journal.AppendTransaction(t.Context(), c, dir, journal.TransactionInput{
				Date:        time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
				Description: "COFFEE SHOP",
				FloatMeta:   map[string]string{"float-stripe-txn": "fca_txn_dedup1"},
				Postings: []journal.PostingInput{
					{Account: "assets:checking", Commodity: "USD", Quantity: "-30.00"},
					{Account: "expenses:unknown", Commodity: "USD", Quantity: "30.00"},
				},
			}); writeErr != nil {
				t.Logf("hook write: %v", writeErr)
			}
		})
		t.Cleanup(func() { serverledger.ExportedSetAfterImportPreFetch(h, nil) })

		result, err := importStripeTransactions(t, h, &floatv1.ImportStripeTransactionsRequest{
			StripeAccountId:      "fca_abc",
			StripeTransactionIds: []string{"fca_txn_dedup1"},
		})
		if err != nil {
			t.Fatalf("ImportStripeTransactions: %v", err)
		}
		// The transaction was already written by the hook, so ImportStripeTransactions must
		// skip it — ImportedCount must be 0, not 1.
		if result.ImportedCount != 0 {
			t.Errorf("ImportedCount = %d, want 0 (transaction was already written by concurrent import)", result.ImportedCount)
		}

		// Verify there is exactly one copy in the journal by scanning all transactions.
		allTxns, fetchErr := c.Transactions(t.Context())
		if fetchErr != nil {
			t.Fatalf("fetch transactions: %v", fetchErr)
		}
		count := 0
		for _, txn := range allTxns {
			if id, ok := txn.FloatMeta["float-stripe-txn"]; ok && id == "fca_txn_dedup1" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("got %d journal entries for fca_txn_dedup1, want exactly 1 (no duplicates)", count)
		}
	})
}

func importAllStripeTransactions(t *testing.T, h *serverledger.Handler, req *floatv1.ImportAllStripeTransactionsRequest) (*floatv1.ImportTransactionsResult, error) {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := floatv1connect.NewLedgerServiceHandler(h)
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := floatv1connect.NewLedgerServiceClient(srv.Client(), srv.URL)
	stream, err := client.ImportAllStripeTransactions(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { _ = stream.Close() })

	var result *floatv1.ImportTransactionsResult
	for stream.Receive() {
		if p, ok := stream.Msg().Payload.(*floatv1.ImportTransactionsResponse_Result); ok {
			result = p.Result
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// stripeImportAllMockAPI registers Stripe mock handlers for ImportAllStripeTransactions.
// The account endpoint returns a single refresh ID; the transaction list returns txns.
func stripeImportAllMockAPI(t *testing.T, txns []map[string]any) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/financial_connections/transactions", func(w http.ResponseWriter, _ *http.Request) {
		writeStripeJSON(w, map[string]any{
			"object": "list", "data": txns, "has_more": false,
			"url": "/v1/financial_connections/transactions",
		})
	})
	mockStripeAPI(t, mux)
}

func TestImportAllStripeTransactions(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")

	t.Run("imports selected transactions and updates last_fetched_at", func(t *testing.T) {
		stripeImportAllMockAPI(t, []map[string]any{
			{
				"id": "fca_txn_a1", "object": "financial_connections.transaction",
				"account": "fca_abc", "amount": int64(-5000), "currency": "usd",
				"description": "GROCERY STORE", "transacted_at": int64(1746835200),
				"status": "posted", "livemode": false,
			},
		})
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 830, NumTxns: 1})
		if err := os.WriteFile(filepath.Join(dir, "rules.json"), []byte(`[]`), 0644); err != nil {
			t.Fatalf("write rules.json: %v", err)
		}
		cfg := &config.Config{
			Stripe: config.StripeConfig{
				LinkedAccounts: []config.StripeLinkedAccount{
					{StripeAccountID: "fca_abc", HledgerAccount: "assets:checking"},
				},
			},
		}
		h := mustHandlerWithConfig(t, dir, cfg)

		result, err := importAllStripeTransactions(t, h, &floatv1.ImportAllStripeTransactionsRequest{
			Selections: []*floatv1.AccountTransactionSelection{
				{StripeAccountId: "fca_abc", StripeTransactionIds: []string{"fca_txn_a1"}},
			},
		})
		if err != nil {
			t.Fatalf("ImportAllStripeTransactions: %v", err)
		}
		if result.ImportedCount != 1 {
			t.Errorf("ImportedCount = %d, want 1", result.ImportedCount)
		}

		savedCfg, err := config.Load(filepath.Join(dir, "config.toml"))
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		if savedCfg.Stripe.LinkedAccounts[0].LastFetchedAt == "" {
			t.Error("LastFetchedAt was not updated after import")
		}
	})

	t.Run("skips already imported stripe transaction ids", func(t *testing.T) {
		stripeImportAllMockAPI(t, []map[string]any{
			{
				"id": "fca_txn_c1", "object": "financial_connections.transaction",
				"account": "fca_abc", "amount": int64(-5000), "currency": "usd",
				"description": "GROCERY STORE", "transacted_at": int64(1746835200),
				"status": "posted", "livemode": false,
			},
		})
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 832, NumTxns: 1})
		if err := os.WriteFile(filepath.Join(dir, "rules.json"), []byte(`[]`), 0644); err != nil {
			t.Fatalf("write rules.json: %v", err)
		}

		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}
		if _, err := journal.AppendTransaction(t.Context(), c, dir, journal.TransactionInput{
			Date:        time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
			Description: "already imported",
			FloatMeta:   map[string]string{"float-stripe-txn": "fca_txn_c1"},
			Postings: []journal.PostingInput{
				{Account: "assets:checking", Commodity: "USD", Quantity: "-50.00"},
				{Account: "expenses:unknown", Commodity: "USD", Quantity: "50.00"},
			},
		}); err != nil {
			t.Fatalf("append pre-existing transaction: %v", err)
		}

		cfg := &config.Config{
			Stripe: config.StripeConfig{
				LinkedAccounts: []config.StripeLinkedAccount{
					{StripeAccountID: "fca_abc", HledgerAccount: "assets:checking"},
				},
			},
		}
		h := mustHandlerWithConfig(t, dir, cfg)

		result, err := importAllStripeTransactions(t, h, &floatv1.ImportAllStripeTransactionsRequest{
			Selections: []*floatv1.AccountTransactionSelection{
				{StripeAccountId: "fca_abc", StripeTransactionIds: []string{"fca_txn_c1"}},
			},
		})
		if err != nil {
			t.Fatalf("ImportAllStripeTransactions: %v", err)
		}
		if result.ImportedCount != 0 {
			t.Errorf("ImportedCount = %d, want 0 (already imported)", result.ImportedCount)
		}
	})
}

func refreshStripeAccount(t *testing.T, h *serverledger.Handler, req *floatv1.RefreshStripeAccountRequest) ([]*floatv1.RefreshStripeAccountResponse, error) {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := floatv1connect.NewLedgerServiceHandler(h)
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := floatv1connect.NewLedgerServiceClient(srv.Client(), srv.URL)
	stream, err := client.RefreshStripeAccount(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { _ = stream.Close() })

	var events []*floatv1.RefreshStripeAccountResponse
	for stream.Receive() {
		events = append(events, stream.Msg())
	}
	if err := stream.Err(); err != nil {
		return events, err
	}
	return events, nil
}

func TestRefreshStripeAccount(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")

	t.Run("streams progress and result for a succeeding refresh", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/accounts/fca_abc/refresh", func(w http.ResponseWriter, _ *http.Request) {
			writeStripeJSON(w, map[string]any{
				"id":     "fca_abc",
				"object": "financial_connections.account",
				"status": "active",
				"transaction_refresh": map[string]any{
					"id":                "txnr_pending",
					"status":            "pending",
					"last_attempted_at": int64(1746835200),
				},
			})
		})
		mux.HandleFunc("/v1/financial_connections/accounts/fca_abc", func(w http.ResponseWriter, _ *http.Request) {
			writeStripeJSON(w, map[string]any{
				"id":     "fca_abc",
				"object": "financial_connections.account",
				"status": "active",
				"transaction_refresh": map[string]any{
					"id":                "txnr_done",
					"status":            "succeeded",
					"last_attempted_at": int64(1746835200),
				},
			})
		})
		mockStripeAPI(t, mux)

		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 780, NumTxns: 1})
		cfg := &config.Config{
			Stripe: config.StripeConfig{
				LinkedAccounts: []config.StripeLinkedAccount{
					{
						StripeAccountID: "fca_abc",
						HledgerAccount:  "assets:checking",
						LastFetchedAt:   "2026-05-01T00:00:00Z",
					},
				},
			},
		}
		h := mustHandlerWithConfig(t, dir, cfg)

		events, err := refreshStripeAccount(t, h, &floatv1.RefreshStripeAccountRequest{StripeAccountId: "fca_abc"})
		if err != nil {
			t.Fatalf("RefreshStripeAccount: %v", err)
		}
		if len(events) == 0 {
			t.Fatal("expected at least one event, got 0")
		}
		var sawProgress, sawResult bool
		var resultRefreshID string
		var resultSucceeded bool
		for _, ev := range events {
			switch p := ev.Payload.(type) {
			case *floatv1.RefreshStripeAccountResponse_Progress:
				sawProgress = true
				if p.Progress.StripeAccountId != "fca_abc" {
					t.Errorf("progress.StripeAccountId = %q, want fca_abc", p.Progress.StripeAccountId)
				}
			case *floatv1.RefreshStripeAccountResponse_Result:
				sawResult = true
				resultRefreshID = p.Result.RefreshId
				resultSucceeded = p.Result.Succeeded
			}
		}
		if !sawProgress {
			t.Error("no progress events emitted")
		}
		if !sawResult {
			t.Fatal("no result event emitted")
		}
		if !resultSucceeded {
			t.Errorf("result.Succeeded = false, want true")
		}
		if resultRefreshID != "txnr_done" {
			t.Errorf("result.RefreshId = %q, want txnr_done", resultRefreshID)
		}

		// Invariant: config must be untouched by RefreshStripeAccount.
		configPath := filepath.Join(dir, "config.toml")
		savedCfg, err := config.Load(configPath)
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		la := savedCfg.Stripe.LinkedAccounts[0]
		if la.LastFetchedAt != "2026-05-01T00:00:00Z" {
			t.Errorf("LastFetchedAt = %q, want unchanged", la.LastFetchedAt)
		}
	})

	t.Run("missing stripe_account_id returns invalid argument", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 781, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{
			Stripe: config.StripeConfig{
				LinkedAccounts: []config.StripeLinkedAccount{
					{StripeAccountID: "fca_abc", HledgerAccount: "assets:checking"},
				},
			},
		})
		_, err := refreshStripeAccount(t, h, &floatv1.RefreshStripeAccountRequest{StripeAccountId: ""})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("kickoff error surfaces as result with succeeded=false", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/financial_connections/accounts/fca_abc/refresh", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"type":    "invalid_request_error",
					"message": "refresh not available",
					"code":    "refresh_not_available",
				},
			})
		})
		mockStripeAPI(t, mux)

		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 782, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{
			Stripe: config.StripeConfig{
				LinkedAccounts: []config.StripeLinkedAccount{
					{StripeAccountID: "fca_abc", HledgerAccount: "assets:checking"},
				},
			},
		})

		events, err := refreshStripeAccount(t, h, &floatv1.RefreshStripeAccountRequest{StripeAccountId: "fca_abc"})
		if err != nil {
			t.Fatalf("RefreshStripeAccount: %v", err)
		}
		var result *floatv1.RefreshStripeAccountResult
		for _, ev := range events {
			if p, ok := ev.Payload.(*floatv1.RefreshStripeAccountResponse_Result); ok {
				result = p.Result
			}
		}
		if result == nil {
			t.Fatal("no result event emitted")
		}
		if result.Succeeded {
			t.Error("result.Succeeded = true, want false on kickoff error")
		}
		if result.ErrorMessage == "" {
			t.Error("result.ErrorMessage is empty, want a kickoff failure message")
		}
	})

	t.Run("throttled refresh skips kickoff and reports next_refresh_available_at", func(t *testing.T) {
		mux := http.NewServeMux()
		futureUnix := time.Now().Add(3 * time.Hour).Unix()
		var refreshCalls int
		mux.HandleFunc("/v1/financial_connections/accounts/fca_abc/refresh", func(w http.ResponseWriter, _ *http.Request) {
			refreshCalls++
			writeStripeJSON(w, map[string]any{})
		})
		mux.HandleFunc("/v1/financial_connections/accounts/fca_abc", func(w http.ResponseWriter, _ *http.Request) {
			writeStripeJSON(w, map[string]any{
				"id":     "fca_abc",
				"object": "financial_connections.account",
				"status": "active",
				"transaction_refresh": map[string]any{
					"id":                        "txnr_existing",
					"status":                    "succeeded",
					"last_attempted_at":         int64(1746835200),
					"next_refresh_available_at": futureUnix,
				},
			})
		})
		mockStripeAPI(t, mux)

		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 783, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, &config.Config{
			Stripe: config.StripeConfig{
				LinkedAccounts: []config.StripeLinkedAccount{
					{StripeAccountID: "fca_abc", HledgerAccount: "assets:checking"},
				},
			},
		})

		events, err := refreshStripeAccount(t, h, &floatv1.RefreshStripeAccountRequest{StripeAccountId: "fca_abc"})
		if err != nil {
			t.Fatalf("RefreshStripeAccount: %v", err)
		}
		if refreshCalls != 0 {
			t.Errorf("stripe refresh endpoint hit %d times, want 0 (throttled)", refreshCalls)
		}
		var result *floatv1.RefreshStripeAccountResult
		var sawThrottledProgress bool
		for _, ev := range events {
			switch p := ev.Payload.(type) {
			case *floatv1.RefreshStripeAccountResponse_Progress:
				if p.Progress.Status == "throttled" {
					sawThrottledProgress = true
				}
			case *floatv1.RefreshStripeAccountResponse_Result:
				result = p.Result
			}
		}
		if !sawThrottledProgress {
			t.Error("no progress event with status=throttled emitted")
		}
		if result == nil {
			t.Fatal("no result event emitted")
		}
		if !result.Succeeded {
			t.Errorf("result.Succeeded = false, want true (no error, just throttled)")
		}
		if !result.Throttled {
			t.Error("result.Throttled = false, want true")
		}
		if result.NextRefreshAvailableAt != futureUnix {
			t.Errorf("result.NextRefreshAvailableAt = %d, want %d", result.NextRefreshAvailableAt, futureUnix)
		}
		if result.RefreshId != "txnr_existing" {
			t.Errorf("result.RefreshId = %q, want txnr_existing", result.RefreshId)
		}
	})
}
