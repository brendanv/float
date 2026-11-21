package ledger_test

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/hledger"
	serverledger "github.com/brendanv/float/internal/server/ledger"
	"github.com/brendanv/float/internal/testgen"
)

// importFromTxns is one transaction on each side of a 2025-05-10 cutoff
// (transacted_at 2025-05-09 and 2025-05-10, 00:00 UTC).
func importFromTxns(accountID string) []map[string]any {
	return []map[string]any{
		{
			"id": "fca_txn_before", "object": "financial_connections.transaction",
			"account": accountID, "amount": int64(-1200), "currency": "usd",
			"description": "BEFORE CUTOFF", "transacted_at": int64(1746748800),
			"status": "posted", "livemode": false,
		},
		{
			"id": "fca_txn_on", "object": "financial_connections.transaction",
			"account": accountID, "amount": int64(-3400), "currency": "usd",
			"description": "ON CUTOFF", "transacted_at": int64(1746835200),
			"status": "posted", "livemode": false,
		},
	}
}

func importFromConfig(accountID, importFrom string) *config.Config {
	return &config.Config{Stripe: config.StripeConfig{LinkedAccounts: []config.StripeLinkedAccount{
		{StripeAccountID: accountID, HledgerAccount: "assets:checking", ImportFrom: importFrom},
	}}}
}

func stripeTxnCount(t *testing.T, dir, id string) int {
	t.Helper()
	hl, err := hledger.New("hledger", dir+"/main.journal")
	if err != nil {
		t.Fatalf("hledger client: %v", err)
	}
	txns, err := hl.Transactions(t.Context(), "tag:float-stripe-txn="+id)
	if err != nil {
		t.Fatalf("query %s: %v", id, err)
	}
	return len(txns)
}

func TestSetStripeAccountImportFrom(t *testing.T) {
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 860, NumTxns: 1})
	h := mustHandlerWithConfig(t, dir, importFromConfig("fca_abc", ""))
	configPath := filepath.Join(dir, "config.toml")

	set := func(id, from string) (*floatv1.SetStripeAccountImportFromResponse, error) {
		resp, err := h.SetStripeAccountImportFrom(t.Context(), connect.NewRequest(&floatv1.SetStripeAccountImportFromRequest{
			StripeAccountId: id, ImportFrom: from,
		}))
		if err != nil {
			return nil, err
		}
		return resp.Msg, nil
	}
	savedImportFrom := func() string {
		t.Helper()
		cfg, err := config.Load(configPath)
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		return cfg.Stripe.LinkedAccounts[0].ImportFrom
	}

	resp, err := set("fca_abc", " 2026-03-01 ")
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if resp.Account.ImportFrom != "2026-03-01" || resp.Account.HledgerAccount != "assets:checking" {
		t.Errorf("response account = %+v, want import_from 2026-03-01 on assets:checking", resp.Account)
	}
	if got := savedImportFrom(); got != "2026-03-01" {
		t.Errorf("saved import_from = %q, want 2026-03-01", got)
	}

	if _, err := set("fca_abc", ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got := savedImportFrom(); got != "" {
		t.Errorf("saved import_from after clear = %q, want empty", got)
	}

	for _, tc := range []struct {
		name, id, from string
		want           connect.Code
	}{
		{"invalid date", "fca_abc", "03/01/2026", connect.CodeInvalidArgument},
		{"missing account id", "", "2026-03-01", connect.CodeInvalidArgument},
		{"unlinked account", "fca_other", "2026-03-01", connect.CodeNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := set(tc.id, tc.from)
			if connect.CodeOf(err) != tc.want {
				t.Errorf("code = %v (err %v), want %v", connect.CodeOf(err), err, tc.want)
			}
		})
	}
}

func TestCompleteStripeLinking_ImportFrom(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/financial_connections/accounts/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/subscribe") {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(r.URL.Path, "/")
		writeStripeJSON(w, map[string]any{"id": parts[len(parts)-2], "object": "financial_connections.account", "status": "active", "livemode": false})
	})
	mockStripeAPI(t, mux)

	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 861, NumTxns: 1})
	h := mustHandlerWithConfig(t, dir, importFromConfig("fca_old", "2026-01-15"))

	link := func(in ...*floatv1.LinkedAccountInput) (map[string]string, error) {
		resp, err := h.CompleteStripeLinking(t.Context(), connect.NewRequest(&floatv1.CompleteStripeLinkingRequest{Accounts: in}))
		if err != nil {
			return nil, err
		}
		got := map[string]string{}
		for _, a := range resp.Msg.LinkedAccounts {
			got[a.StripeAccountId] = a.ImportFrom
		}
		return got, nil
	}

	got, err := link(
		&floatv1.LinkedAccountInput{StripeAccountId: "fca_new", HledgerAccount: "liabilities:amex", ImportFrom: "2026-11-01"},
		// Re-mapping without a cutoff keeps the existing one.
		&floatv1.LinkedAccountInput{StripeAccountId: "fca_old", HledgerAccount: "assets:checking"},
	)
	if err != nil {
		t.Fatalf("CompleteStripeLinking: %v", err)
	}
	if got["fca_new"] != "2026-11-01" || got["fca_old"] != "2026-01-15" {
		t.Errorf("import_from = %v, want fca_new=2026-11-01 fca_old=2026-01-15", got)
	}

	_, err = link(&floatv1.LinkedAccountInput{StripeAccountId: "fca_bad", HledgerAccount: "assets:checking", ImportFrom: "soon"})
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("invalid import_from: code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestFetchStripeTransactions_ImportFrom(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")
	stripeAutoImportMockAPI(t, "fca_cut", importFromTxns("fca_cut"))

	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 862, NumTxns: 1})
	h := mustHandlerWithConfig(t, dir, importFromConfig("fca_cut", "2025-05-10"))

	resp, err := h.FetchStripeTransactions(t.Context(), connect.NewRequest(&floatv1.FetchStripeTransactionsRequest{StripeAccountId: "fca_cut"}))
	if err != nil {
		t.Fatalf("FetchStripeTransactions: %v", err)
	}
	if len(resp.Msg.Candidates) != 1 || resp.Msg.Candidates[0].SourceId != "fca_txn_on" {
		t.Errorf("candidates = %v, want only fca_txn_on", resp.Msg.Candidates)
	}

	all, err := h.FetchAllStripeTransactions(t.Context(), connect.NewRequest(&floatv1.FetchAllStripeTransactionsRequest{}))
	if err != nil {
		t.Fatalf("FetchAllStripeTransactions: %v", err)
	}
	if len(all.Msg.AccountCandidates) != 1 {
		t.Fatalf("got %d account groups, want 1", len(all.Msg.AccountCandidates))
	}
	ac := all.Msg.AccountCandidates[0]
	if len(ac.Candidates) != 1 || ac.Candidates[0].SourceId != "fca_txn_on" {
		t.Errorf("fetch-all candidates = %v, want only fca_txn_on", ac.Candidates)
	}
	if ac.Account.ImportFrom != "2025-05-10" {
		t.Errorf("fetch-all account import_from = %q, want 2025-05-10", ac.Account.ImportFrom)
	}
}

func TestImportStripeTransactions_ImportFrom(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")
	stripeAutoImportMockAPI(t, "fca_cut", importFromTxns("fca_cut"))

	t.Run("single account skips selected transactions before the cutoff", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 863, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, importFromConfig("fca_cut", "2025-05-10"))
		res, err := importStripeTransactions(t, h, &floatv1.ImportStripeTransactionsRequest{
			StripeAccountId:      "fca_cut",
			StripeTransactionIds: []string{"fca_txn_before", "fca_txn_on"},
		})
		if err != nil {
			t.Fatalf("ImportStripeTransactions: %v", err)
		}
		if res.ImportedCount != 1 {
			t.Errorf("imported = %d, want 1", res.ImportedCount)
		}
		if n := stripeTxnCount(t, dir, "fca_txn_before"); n != 0 {
			t.Errorf("fca_txn_before written %d times, want 0", n)
		}
	})

	t.Run("all accounts skips selected transactions before the cutoff", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 864, NumTxns: 1})
		h := mustHandlerWithConfig(t, dir, importFromConfig("fca_cut", "2025-05-10"))
		res, err := importAllStripeTransactions(t, h, &floatv1.ImportAllStripeTransactionsRequest{
			Selections: []*floatv1.AccountTransactionSelection{{
				StripeAccountId:      "fca_cut",
				StripeTransactionIds: []string{"fca_txn_before", "fca_txn_on"},
			}},
		})
		if err != nil {
			t.Fatalf("ImportAllStripeTransactions: %v", err)
		}
		if res.ImportedCount != 1 {
			t.Errorf("imported = %d, want 1", res.ImportedCount)
		}
		if n := stripeTxnCount(t, dir, "fca_txn_before"); n != 0 {
			t.Errorf("fca_txn_before written %d times, want 0", n)
		}
	})
}

func TestRunDailyStripeImport_ImportFrom(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")
	stripeAutoImportMockAPI(t, "fca_cut", importFromTxns("fca_cut"))

	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 865, NumTxns: 1})
	h := mustHandlerWithConfig(t, dir, importFromConfig("fca_cut", "2025-05-10"))

	imported, errs := serverledger.ExportedRunDailyStripeImport(h, t.Context())
	if len(errs) != 0 {
		t.Fatalf("auto-import errors: %v", errs)
	}
	if imported != 1 {
		t.Errorf("imported = %d, want 1 (only the transaction on the cutoff date)", imported)
	}
	if n := stripeTxnCount(t, dir, "fca_txn_before"); n != 0 {
		t.Errorf("fca_txn_before written %d times, want 0", n)
	}
	if n := stripeTxnCount(t, dir, "fca_txn_on"); n != 1 {
		t.Errorf("fca_txn_on written %d times, want 1", n)
	}
}
