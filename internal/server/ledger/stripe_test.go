package ledger_test

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/hledger"
	serverledger "github.com/brendanv/float/internal/server/ledger"
	"github.com/brendanv/float/internal/stripeconn"
	"github.com/brendanv/float/internal/testgen"
	"github.com/brendanv/float/internal/txlock"
	"github.com/stripe/stripe-go/v82"
)

// fakeStripe is a stripeconn.Stripe stub for handler tests.
type fakeStripe struct {
	sessionSecret string
	account       *stripe.FinancialConnectionsAccount
	txns          []*stripe.FinancialConnectionsTransaction

	createSessionCalls int
	getAccountCalls    int
	listCalls          int
}

func (f *fakeStripe) CreateSession(ctx context.Context, params stripeconn.SessionParams) (string, error) {
	f.createSessionCalls++
	return f.sessionSecret, nil
}

func (f *fakeStripe) GetAccount(ctx context.Context, id string) (*stripe.FinancialConnectionsAccount, error) {
	f.getAccountCalls++
	if f.account == nil {
		return nil, nil
	}
	cpy := *f.account
	cpy.ID = id
	return &cpy, nil
}

func (f *fakeStripe) ListPostedTransactions(ctx context.Context, id string) ([]*stripe.FinancialConnectionsTransaction, error) {
	f.listCalls++
	return f.txns, nil
}

func newStripeHandler(t *testing.T, fake *fakeStripe) (*serverledger.Handler, string) {
	t.Helper()
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 42, NumTxns: 5, WithFIDs: true})

	cfgPath := dir + "/config.toml"
	cfg := &config.Config{Stripe: config.StripeConfig{APIKey: "sk_test_initial"}}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	c, err := hledger.New("hledger", dir+"/main.journal")
	if err != nil {
		t.Skipf("hledger unavailable: %v", err)
	}
	lock := txlock.New(dir, c)
	h := serverledger.NewHandler(c, lock, dir, cfgPath, nil, nil, cfg)
	h.StripeFactory = func(apiKey string) (stripeconn.Stripe, error) {
		return fake, nil
	}
	return h, dir
}

func TestGetStripeStatusUnconfigured(t *testing.T) {
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", versionRunner(t, nil))
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	cfg := &config.Config{}
	dir := t.TempDir()
	h := serverledger.NewHandler(c, nil, dir, "", nil, nil, cfg)
	resp, err := h.GetStripeStatus(t.Context(), connect.NewRequest(&floatv1.GetStripeStatusRequest{}))
	if err != nil {
		t.Fatalf("GetStripeStatus: %v", err)
	}
	if resp.Msg.ApiKeyConfigured {
		t.Error("ApiKeyConfigured should be false")
	}
	if resp.Msg.ConnectionCount != 0 {
		t.Errorf("ConnectionCount = %d, want 0", resp.Msg.ConnectionCount)
	}
}

func TestStripeLifecycle(t *testing.T) {
	fake := &fakeStripe{
		sessionSecret: "fcsess_secret_abc",
		account: &stripe.FinancialConnectionsAccount{
			InstitutionName: "Test Bank",
			Last4:           "9999",
			Category:        stripe.FinancialConnectionsAccountCategoryCash,
			Subcategory:     stripe.FinancialConnectionsAccountSubcategoryChecking,
			Balance: &stripe.FinancialConnectionsAccountBalance{
				Current: map[string]int64{"usd": 100000},
			},
		},
	}
	h, dir := newStripeHandler(t, fake)
	ctx := t.Context()

	// Status: api key configured, zero connections.
	st, err := h.GetStripeStatus(ctx, connect.NewRequest(&floatv1.GetStripeStatusRequest{}))
	if err != nil {
		t.Fatalf("GetStripeStatus: %v", err)
	}
	if !st.Msg.ApiKeyConfigured {
		t.Fatal("expected ApiKeyConfigured=true")
	}

	// CreateSession returns the fake's client_secret.
	sess, err := h.CreateStripeSession(ctx, connect.NewRequest(&floatv1.CreateStripeSessionRequest{}))
	if err != nil {
		t.Fatalf("CreateStripeSession: %v", err)
	}
	if sess.Msg.ClientSecret != "fcsess_secret_abc" {
		t.Errorf("ClientSecret = %q", sess.Msg.ClientSecret)
	}
	if fake.createSessionCalls != 1 {
		t.Errorf("createSessionCalls = %d, want 1", fake.createSessionCalls)
	}

	// Link a new account.
	link, err := h.LinkStripeAccounts(ctx, connect.NewRequest(&floatv1.LinkStripeAccountsRequest{
		StripeAccountIds: []string{"fca_test1"},
	}))
	if err != nil {
		t.Fatalf("LinkStripeAccounts: %v", err)
	}
	if len(link.Msg.Connections) != 1 {
		t.Fatalf("got %d connections", len(link.Msg.Connections))
	}
	conn := link.Msg.Connections[0]
	if conn.InstitutionName != "Test Bank" || conn.Currency != "USD" {
		t.Errorf("unexpected new connection: %+v", conn)
	}
	if conn.HledgerAccount != "" {
		t.Errorf("HledgerAccount should be empty on link: %q", conn.HledgerAccount)
	}

	// Calling Link again with the same id should be idempotent (no Stripe call).
	fake.getAccountCalls = 0
	link2, err := h.LinkStripeAccounts(ctx, connect.NewRequest(&floatv1.LinkStripeAccountsRequest{
		StripeAccountIds: []string{"fca_test1"},
	}))
	if err != nil {
		t.Fatalf("LinkStripeAccounts (idempotent): %v", err)
	}
	if fake.getAccountCalls != 0 {
		t.Errorf("getAccountCalls = %d, want 0 (should reuse existing)", fake.getAccountCalls)
	}
	if link2.Msg.Connections[0].Id != conn.Id {
		t.Error("expected same connection id on re-link")
	}

	// Update with the hledger mapping.
	upd, err := h.UpdateStripeConnection(ctx, connect.NewRequest(&floatv1.UpdateStripeConnectionRequest{
		Id:                    conn.Id,
		DisplayName:           "Test Checking",
		HledgerAccount:        "assets:test:checking",
		DefaultInflowAccount:  "income:unknown",
		DefaultOutflowAccount: "expenses:unknown",
	}))
	if err != nil {
		t.Fatalf("UpdateStripeConnection: %v", err)
	}
	if upd.Msg.Connection.HledgerAccount != "assets:test:checking" {
		t.Errorf("HledgerAccount = %q", upd.Msg.Connection.HledgerAccount)
	}

	// Sync: fake returns two posted txns.
	fake.txns = []*stripe.FinancialConnectionsTransaction{
		{
			ID:           "tx_a",
			Amount:       -2500,
			Currency:     "usd",
			Description:  "COFFEE",
			Status:       stripe.FinancialConnectionsTransactionStatusPosted,
			TransactedAt: time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC).Unix(),
		},
		{
			ID:           "tx_b",
			Amount:       50000,
			Currency:     "usd",
			Description:  "PAYCHECK",
			Status:       stripe.FinancialConnectionsTransactionStatusPosted,
			TransactedAt: time.Date(2026, 3, 15, 9, 0, 0, 0, time.UTC).Unix(),
		},
	}
	sync, err := h.SyncStripeConnection(ctx, connect.NewRequest(&floatv1.SyncStripeConnectionRequest{
		Id: conn.Id,
	}))
	if err != nil {
		t.Fatalf("SyncStripeConnection: %v", err)
	}
	if sync.Msg.Imported != 2 || sync.Msg.Skipped != 0 {
		t.Errorf("sync result: imported=%d skipped=%d, want 2/0", sync.Msg.Imported, sync.Msg.Skipped)
	}

	// Verify imported_count surfaces through the proto.
	if sync.Msg.Connection.ImportedCount != 2 {
		t.Errorf("ImportedCount = %d, want 2", sync.Msg.Connection.ImportedCount)
	}

	// Re-sync immediately should be blocked by MinSyncInterval.
	if _, err := h.SyncStripeConnection(ctx, connect.NewRequest(&floatv1.SyncStripeConnectionRequest{Id: conn.Id})); err == nil {
		t.Error("expected MinSyncInterval error on immediate re-sync")
	}

	// Delete the connection.
	if _, err := h.DeleteStripeConnection(ctx, connect.NewRequest(&floatv1.DeleteStripeConnectionRequest{Id: conn.Id})); err != nil {
		t.Fatalf("DeleteStripeConnection: %v", err)
	}
	list, err := h.ListStripeConnections(ctx, connect.NewRequest(&floatv1.ListStripeConnectionsRequest{}))
	if err != nil {
		t.Fatalf("ListStripeConnections: %v", err)
	}
	if len(list.Msg.Connections) != 0 {
		t.Errorf("after delete: %d connections", len(list.Msg.Connections))
	}
	_ = dir
}
