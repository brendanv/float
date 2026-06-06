package ledger_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/rules"
	serverledger "github.com/brendanv/float/internal/server/ledger"
	"github.com/brendanv/float/internal/testgen"

	"connectrpc.com/connect"
)

func TestSetStripeDailyImportEnabled(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 800, NumTxns: 1})
	h := mustHandlerWithConfig(t, dir, &config.Config{})

	if _, err := h.SetStripeDailyImportEnabled(t.Context(), connect.NewRequest(&floatv1.SetStripeDailyImportEnabledRequest{Enabled: true})); err != nil {
		t.Fatalf("SetStripeDailyImportEnabled(true): %v", err)
	}

	resp, err := h.GetStripeConfig(t.Context(), connect.NewRequest(&floatv1.GetStripeConfigRequest{}))
	if err != nil {
		t.Fatalf("GetStripeConfig: %v", err)
	}
	if !resp.Msg.DailyImportEnabled {
		t.Error("DailyImportEnabled = false after enabling, want true")
	}

	if _, err := h.SetStripeDailyImportEnabled(t.Context(), connect.NewRequest(&floatv1.SetStripeDailyImportEnabledRequest{Enabled: false})); err != nil {
		t.Fatalf("SetStripeDailyImportEnabled(false): %v", err)
	}
	resp, err = h.GetStripeConfig(t.Context(), connect.NewRequest(&floatv1.GetStripeConfigRequest{}))
	if err != nil {
		t.Fatalf("GetStripeConfig: %v", err)
	}
	if resp.Msg.DailyImportEnabled {
		t.Error("DailyImportEnabled = true after disabling, want false")
	}
}

func TestGetStripeConfigDailyImportFields(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 801, NumTxns: 1})
	h := mustHandlerWithConfig(t, dir, &config.Config{
		Stripe: config.StripeConfig{
			DailyImportEnabled: true,
			LastDailyImportAt:  "2026-05-15T12:00:00Z",
		},
	})

	resp, err := h.GetStripeConfig(t.Context(), connect.NewRequest(&floatv1.GetStripeConfigRequest{}))
	if err != nil {
		t.Fatalf("GetStripeConfig: %v", err)
	}
	if !resp.Msg.DailyImportEnabled {
		t.Error("DailyImportEnabled = false, want true (config has it enabled)")
	}
	if resp.Msg.LastDailyImportAt != "2026-05-15T12:00:00Z" {
		t.Errorf("LastDailyImportAt = %q, want %q", resp.Msg.LastDailyImportAt, "2026-05-15T12:00:00Z")
	}
}

// stripeAutoImportMockAPI registers Stripe mock handlers for a throttled refresh (so no
// refresh polling is needed) and a single-page transaction list returning txns.
func stripeAutoImportMockAPI(t *testing.T, accountID string, txns []map[string]any) {
	t.Helper()
	mux := http.NewServeMux()
	// MaybeRefreshTransactions: return throttled so auto-import skips the refresh poll.
	mux.HandleFunc("/v1/financial_connections/accounts/"+accountID, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": accountID, "object": "financial_connections.account",
			"status": "active", "livemode": false,
			"transaction_refresh": map[string]any{
				"id":                        "txnr_throttled",
				"status":                    "succeeded",
				"next_refresh_available_at": time.Now().Add(time.Hour).Unix(),
			},
		})
	})
	mux.HandleFunc("/v1/financial_connections/transactions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list", "data": txns, "has_more": false,
			"url": "/v1/financial_connections/transactions",
		})
	})
	mockStripeAPI(t, mux)
}

// TestRunDailyStripeImport_NoLinkedAccounts verifies the auto-importer is a clean no-op
// when nothing is configured. The internal helper runs to completion without touching
// Stripe (no accounts to iterate) and reports zero imports / zero errors.
func TestRunDailyStripeImport_NoLinkedAccounts(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 802, NumTxns: 1})
	h := mustHandlerWithConfig(t, dir, &config.Config{
		Stripe: config.StripeConfig{DailyImportEnabled: true},
	})

	imported, errs := serverledger.ExportedRunDailyStripeImport(h, t.Context())
	if imported != 0 {
		t.Errorf("imported = %d, want 0", imported)
	}
	if len(errs) != 0 {
		t.Errorf("errors = %v, want none", errs)
	}
}

// TestRunDailyStripeImport_NoDuplicateWhenRuleApplied verifies that the auto-import does
// not re-import a transaction that was previously imported via the manual flow with a rule
// applied (which modifies the description and/or account). Dedup is keyed on the Stripe
// transaction id, so a rule-modified journal entry is still recognized as already imported.
func TestRunDailyStripeImport_NoDuplicateWhenRuleApplied(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")

	stripeAutoImportMockAPI(t, "fca_abc", []map[string]any{
		{
			"id": "fca_txn_rule1", "object": "financial_connections.transaction",
			"account": "fca_abc", "amount": int64(-9999), "currency": "usd",
			"description": "AMAZON MARKETPLACE", "transacted_at": int64(1746835200),
			"status": "posted", "livemode": false,
		},
	})

	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 803, NumTxns: 1})

	// Write a rule that matches "AMAZON" and sets payee + account.
	ruleJSON, _ := json.Marshal([]rules.Rule{{
		ID:      "testrule1",
		Pattern: "AMAZON",
		Payee:   "Amazon",
		Account: "expenses:shopping",
	}})
	if err := os.WriteFile(filepath.Join(dir, "rules.json"), ruleJSON, 0644); err != nil {
		t.Fatalf("write rules.json: %v", err)
	}

	hl, err := hledger.New("hledger", dir+"/main.journal")
	if err != nil {
		t.Skipf("hledger unavailable: %v", err)
	}

	// Simulate a prior manual import: the transaction was written with the rule applied
	// (description modified to "Amazon | AMAZON MARKETPLACE", account to expenses:shopping).
	if _, err := journal.AppendTransaction(t.Context(), hl, dir, journal.TransactionInput{
		Date:        time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
		Description: "Amazon | AMAZON MARKETPLACE",
		FloatMeta:   map[string]string{"float-stripe-txn": "fca_txn_rule1"},
		Postings: []journal.PostingInput{
			{Account: "assets:checking", Commodity: "USD", Quantity: "-99.99"},
			{Account: "expenses:shopping", Commodity: "USD", Quantity: "99.99"},
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

	imported, errs := serverledger.ExportedRunDailyStripeImport(h, t.Context())
	if len(errs) != 0 {
		t.Fatalf("auto-import errors: %v", errs)
	}
	if imported != 0 {
		t.Errorf("imported = %d, want 0: auto-import must not duplicate a transaction already in the journal (even when a rule changed its fingerprint)", imported)
	}

	// Double-check: the journal should still have exactly one Stripe transaction.
	txns, err := hl.Transactions(t.Context(), "tag:float-stripe-txn=fca_txn_rule1")
	if err != nil {
		t.Fatalf("query transactions: %v", err)
	}
	if len(txns) != 1 {
		t.Errorf("journal has %d copies of fca_txn_rule1, want 1", len(txns))
	}
}

// TestRunDailyStripeImport_NoDuplicateOnConcurrentImport verifies that the auto-import
// re-checks already-imported Stripe IDs inside the txlock. It simulates a concurrent
// manual import landing in the race window between the pre-lock dedup and lock
// acquisition (via the afterDailyImportPreFetch hook). Without the in-lock re-check the
// auto-import would write a second copy of the same Stripe transaction.
func TestRunDailyStripeImport_NoDuplicateOnConcurrentImport(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")

	stripeAutoImportMockAPI(t, "fca_race", []map[string]any{
		{
			"id": "fca_txn_race", "object": "financial_connections.transaction",
			"account": "fca_race", "amount": int64(-1234), "currency": "usd",
			"description": "GROCERY STORE", "transacted_at": int64(1746835200),
			"status": "posted", "livemode": false,
		},
	})

	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 806, NumTxns: 1})
	hl, err := hledger.New("hledger", dir+"/main.journal")
	if err != nil {
		t.Skipf("hledger unavailable: %v", err)
	}

	cfg := &config.Config{
		Stripe: config.StripeConfig{
			LinkedAccounts: []config.StripeLinkedAccount{
				{StripeAccountID: "fca_race", HledgerAccount: "assets:checking"},
			},
		},
	}
	h := mustHandlerWithConfig(t, dir, cfg)

	// Simulate a concurrent manual import writing the same Stripe transaction after the
	// auto-import built its pre-lock dedup set but before it acquired the lock.
	serverledger.ExportedSetAfterDailyImportPreFetch(h, func() {
		if _, err := journal.AppendTransaction(t.Context(), hl, dir, journal.TransactionInput{
			Date:        time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
			Description: "GROCERY STORE",
			FloatMeta:   map[string]string{"float-stripe-txn": "fca_txn_race"},
			Postings: []journal.PostingInput{
				{Account: "assets:checking", Commodity: "USD", Quantity: "-12.34"},
				{Account: "expenses:groceries", Commodity: "USD", Quantity: "12.34"},
			},
		}); err != nil {
			t.Errorf("concurrent import write: %v", err)
		}
	})

	imported, errs := serverledger.ExportedRunDailyStripeImport(h, t.Context())
	if len(errs) != 0 {
		t.Fatalf("auto-import errors: %v", errs)
	}
	if imported != 0 {
		t.Errorf("imported = %d, want 0: auto-import must skip a transaction written by a concurrent import inside the lock", imported)
	}

	txns, err := hl.Transactions(t.Context(), "tag:float-stripe-txn=fca_txn_race")
	if err != nil {
		t.Fatalf("query transactions: %v", err)
	}
	if len(txns) != 1 {
		t.Errorf("journal has %d copies of fca_txn_race, want 1 (no duplicate from the race)", len(txns))
	}
}

// TestRunDailyStripeImport_SkipsPendingTransactions verifies that pending Stripe transactions
// are never written to the journal — only posted transactions are imported.
func TestRunDailyStripeImport_SkipsPendingTransactions(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")

	stripeAutoImportMockAPI(t, "fca_pend", []map[string]any{
		{
			"id": "fca_txn_settled", "object": "financial_connections.transaction",
			"account": "fca_pend", "amount": int64(-4500), "currency": "usd",
			"description": "COFFEE SHOP", "transacted_at": int64(1746835200),
			"status": "posted", "livemode": false,
		},
		{
			"id": "fca_txn_pending", "object": "financial_connections.transaction",
			"account": "fca_pend", "amount": int64(-1000), "currency": "usd",
			"description": "GAS STATION", "transacted_at": int64(1746921600),
			"status": "pending", "livemode": false,
		},
	})

	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 804, NumTxns: 1})
	hl, err := hledger.New("hledger", dir+"/main.journal")
	if err != nil {
		t.Skipf("hledger unavailable: %v", err)
	}

	cfg := &config.Config{
		Stripe: config.StripeConfig{
			LinkedAccounts: []config.StripeLinkedAccount{
				{StripeAccountID: "fca_pend", HledgerAccount: "assets:checking"},
			},
		},
	}
	h := mustHandlerWithConfig(t, dir, cfg)

	imported, errs := serverledger.ExportedRunDailyStripeImport(h, t.Context())
	if len(errs) != 0 {
		t.Fatalf("auto-import errors: %v", errs)
	}
	if imported != 1 {
		t.Errorf("imported = %d, want 1 (only the posted transaction)", imported)
	}

	settled, err := hl.Transactions(t.Context(), "tag:float-stripe-txn=fca_txn_settled")
	if err != nil {
		t.Fatalf("query settled transaction: %v", err)
	}
	if len(settled) != 1 {
		t.Errorf("journal has %d copies of fca_txn_settled, want 1", len(settled))
	}

	pending, err := hl.Transactions(t.Context(), "tag:float-stripe-txn=fca_txn_pending")
	if err != nil {
		t.Fatalf("query pending transaction: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("journal has %d copies of fca_txn_pending, want 0 (pending must not be imported)", len(pending))
	}
}

// TestRunDailyStripeImport_UsesTransactedAtNotPostedAt verifies that the auto-import records
// the transacted_at date on the hledger transaction, not the status_transitions.posted_at date.
// These can differ when a transaction is authorized on one day but settles on a later day.
func TestRunDailyStripeImport_UsesTransactedAtNotPostedAt(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")

	// transacted_at = 2025-05-10; posted_at = 2025-05-12 (2 days later).
	stripeAutoImportMockAPI(t, "fca_dates", []map[string]any{
		{
			"id": "fca_txn_datecheck", "object": "financial_connections.transaction",
			"account": "fca_dates", "amount": int64(-2000), "currency": "usd",
			"description": "HARDWARE STORE",
			"transacted_at": int64(1746835200), // 2025-05-10 00:00:00 UTC
			"status":        "posted", "livemode": false,
			"status_transitions": map[string]any{
				"posted_at": int64(1747008000), // 2025-05-12 00:00:00 UTC
			},
		},
	})

	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 805, NumTxns: 1})
	hl, err := hledger.New("hledger", dir+"/main.journal")
	if err != nil {
		t.Skipf("hledger unavailable: %v", err)
	}

	cfg := &config.Config{
		Stripe: config.StripeConfig{
			LinkedAccounts: []config.StripeLinkedAccount{
				{StripeAccountID: "fca_dates", HledgerAccount: "assets:checking"},
			},
		},
	}
	h := mustHandlerWithConfig(t, dir, cfg)

	imported, errs := serverledger.ExportedRunDailyStripeImport(h, t.Context())
	if len(errs) != 0 {
		t.Fatalf("auto-import errors: %v", errs)
	}
	if imported != 1 {
		t.Errorf("imported = %d, want 1", imported)
	}

	txns, err := hl.Transactions(t.Context(), "tag:float-stripe-txn=fca_txn_datecheck")
	if err != nil {
		t.Fatalf("query transaction: %v", err)
	}
	if len(txns) != 1 {
		t.Fatalf("got %d transactions for fca_txn_datecheck, want 1", len(txns))
	}
	if txns[0].Date != "2025-05-10" {
		t.Errorf("imported date = %q, want %q (transacted_at date, not posted_at date %q)",
			txns[0].Date, "2025-05-10", "2025-05-12")
	}
}
