package ledger_test

import (
	"context"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/hledger"
	serverledger "github.com/brendanv/float/internal/server/ledger"
)

func TestListAccounts(t *testing.T) {
	h := mustHandler(t, map[string][]byte{
		"accounts": []byte(accountsText),
	})

	resp, err := h.ListAccounts(t.Context(), connect.NewRequest(&floatv1.ListAccountsRequest{}))
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}

	accounts := resp.Msg.Accounts
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(accounts))
	}

	tests := []struct {
		idx      int
		fullName string
		typ      string
	}{
		{0, "assets:checking", "A"},
		{1, "income:salary", "R"},
	}
	for _, tt := range tests {
		a := accounts[tt.idx]
		if a.FullName != tt.fullName {
			t.Errorf("accounts[%d].FullName = %q, want %q", tt.idx, a.FullName, tt.fullName)
		}
		if a.Type != tt.typ {
			t.Errorf("accounts[%d].Type = %q, want %q", tt.idx, a.Type, tt.typ)
		}
	}
}

func TestListAccounts_HledgerError(t *testing.T) {
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", errorRunner(t))
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil, nil)
	_, err = h.ListAccounts(t.Context(), connect.NewRequest(&floatv1.ListAccountsRequest{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("code = %v, want Internal", connect.CodeOf(err))
	}
}

func TestListAccounts_CacheHit(t *testing.T) {
	var calls atomic.Int64
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.99.6, linux-x86_64\n"), nil, nil
		}
		calls.Add(1)
		return []byte(accountsText), nil, nil
	}
	h, _ := mustHandlerWithCache(t, runner)

	req := connect.NewRequest(&floatv1.ListAccountsRequest{})
	if _, err := h.ListAccounts(t.Context(), req); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := h.ListAccounts(t.Context(), req); err != nil {
		t.Fatalf("second call: %v", err)
	}

	// One `accounts --types` call plus one `accounts tag:float-closed` call.
	if calls.Load() != 2 {
		t.Errorf("hledger called %d times, want 2", calls.Load())
	}
}

func TestListAccounts_CacheError(t *testing.T) {
	h, _ := mustHandlerWithCache(t, errorRunner(t))
	_, err := h.ListAccounts(t.Context(), connect.NewRequest(&floatv1.ListAccountsRequest{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("code = %v, want Internal", connect.CodeOf(err))
	}
}
