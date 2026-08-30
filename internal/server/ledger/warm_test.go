package ledger_test

import (
	"context"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	serverledger "github.com/brendanv/float/internal/server/ledger"

	"github.com/brendanv/float/internal/cache"
	"github.com/brendanv/float/internal/hledger"
)

// minimal `hledger is --monthly --tree -O json` output, enough for
// IncomeStatementTimeseries to parse successfully.
const warmISJSON = `{
"cbrDates":[[{"contents":"2026-01-01","tag":"Exact"},{"contents":"2026-02-01","tag":"Exact"}]],
"cbrSubreports":[],
"cbrTotals":{"prrAmounts":[[]],"prrAverage":[],"prrName":[],"prrTotal":[]}
}`

// warmDispatchRunner returns fixture JSON keyed by hledger subcommand
// (args[0]), covering every command WarmEntries' entries invoke.
func warmDispatchRunner(t *testing.T, calls *atomic.Int64) hledger.CommandRunner {
	t.Helper()
	return func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) > 0 && args[0] == "--version" {
			return []byte("hledger 1.52, linux-x86_64\n"), nil, nil
		}
		if calls != nil {
			calls.Add(1)
		}
		switch args[0] {
		case "print":
			return []byte(printJSON), nil, nil
		case "bal":
			return []byte(balJSON), nil, nil
		case "bs":
			return []byte(bsTimeseriesJSON), nil, nil
		case "is":
			return []byte(warmISJSON), nil, nil
		case "accounts":
			return []byte(accountsText), nil, nil
		case "tags":
			return []byte("shopping\n"), nil, nil
		case "payees":
			return []byte(payeesText), nil, nil
		case "areg":
			return []byte(aregJSON), nil, nil
		}
		t.Fatalf("warmDispatchRunner: unexpected command %v", args)
		return nil, nil, nil
	}
}

func mustWarmHandler(t *testing.T, calls *atomic.Int64) (*serverledger.Handler, *cache.Cache[any]) {
	t.Helper()
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", warmDispatchRunner(t, calls))
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	ch := cache.New[any](func() uint64 { return 0 })
	return serverledger.NewHandler(c, nil, "", "", ch, nil, nil, nil), ch
}

func TestWarmEntries_NilCacheReturnsNil(t *testing.T) {
	h := mustHandler(t, nil) // mustHandler (handler_test.go) passes a nil cache
	if entries := h.WarmEntries(); entries != nil {
		t.Errorf("WarmEntries() with nil cache = %v entries, want nil", entries)
	}
}

func TestWarmEntries_FixedSetAndExecution(t *testing.T) {
	h, _ := mustWarmHandler(t, nil)

	entries := h.WarmEntries()
	wantNames := []string{
		"transactions", "accounts", "tags", "payees",
		"balances", "balancesvalued depth 0", "balancesvalued depth 1",
		"networth", "incomestatement",
	}
	if len(entries) != len(wantNames) {
		t.Fatalf("WarmEntries() returned %d entries, want %d: %v", len(entries), len(wantNames), entries)
	}
	for i, e := range entries {
		if e.Name != wantNames[i] {
			t.Errorf("entries[%d].Name = %q, want %q", i, e.Name, wantNames[i])
		}
		if err := e.Load(t.Context()); err != nil {
			t.Errorf("entries[%d] (%s) Load failed: %v", i, e.Name, err)
		}
	}
}

func TestWarmEntries_IncludesRecentlyTouchedAccounts(t *testing.T) {
	h, _ := mustWarmHandler(t, nil)

	if _, err := h.GetAccountRegister(t.Context(), connect.NewRequest(&floatv1.GetAccountRegisterRequest{
		Account: "income:salary",
	})); err != nil {
		t.Fatalf("GetAccountRegister: %v", err)
	}

	entries := h.WarmEntries()
	found := false
	for _, e := range entries {
		if e.Name == "aregister:income:salary" {
			found = true
			if err := e.Load(t.Context()); err != nil {
				t.Errorf("aregister warm entry Load failed: %v", err)
			}
		}
	}
	if !found {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name
		}
		t.Errorf("WarmEntries() = %v, want an aregister entry for income:salary", names)
	}
}

func TestWarmEntries_LoadPopulatesCacheForRPCs(t *testing.T) {
	var calls atomic.Int64
	h, _ := mustWarmHandler(t, &calls)

	entries := h.WarmEntries()
	for _, e := range entries {
		if e.Name == "transactions" {
			if err := e.Load(t.Context()); err != nil {
				t.Fatalf("warm transactions entry: %v", err)
			}
		}
	}
	before := calls.Load()

	// A subsequent ListTransactions call should hit the cache the warm entry
	// populated, not invoke hledger again.
	if _, err := h.ListTransactions(t.Context(), connect.NewRequest(&floatv1.ListTransactionsRequest{})); err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if got := calls.Load(); got != before {
		t.Errorf("hledger invoked %d more time(s) after warm entry populated the cache, want 0", got-before)
	}
}
