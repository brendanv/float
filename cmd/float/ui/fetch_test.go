package ui

import (
	"context"
	"errors"
	"testing"

	connect "connectrpc.com/connect"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

type mockLedgerClient struct {
	floatv1connect.LedgerServiceClient
	listAccountsFn     func(context.Context, *connect.Request[floatv1.ListAccountsRequest]) (*connect.Response[floatv1.ListAccountsResponse], error)
	getBalancesFn      func(context.Context, *connect.Request[floatv1.GetBalancesRequest]) (*connect.Response[floatv1.GetBalancesResponse], error)
	listTransactionsFn func(context.Context, *connect.Request[floatv1.ListTransactionsRequest]) (*connect.Response[floatv1.ListTransactionsResponse], error)
}

func (m *mockLedgerClient) ListAccounts(ctx context.Context, req *connect.Request[floatv1.ListAccountsRequest]) (*connect.Response[floatv1.ListAccountsResponse], error) {
	return m.listAccountsFn(ctx, req)
}

func (m *mockLedgerClient) GetBalances(ctx context.Context, req *connect.Request[floatv1.GetBalancesRequest]) (*connect.Response[floatv1.GetBalancesResponse], error) {
	return m.getBalancesFn(ctx, req)
}

func (m *mockLedgerClient) ListTransactions(ctx context.Context, req *connect.Request[floatv1.ListTransactionsRequest]) (*connect.Response[floatv1.ListTransactionsResponse], error) {
	return m.listTransactionsFn(ctx, req)
}

func TestFetchAccounts_HappyPath(t *testing.T) {
	want := []*floatv1.Account{{Name: "checking", FullName: "assets:checking", Type: "A"}}
	client := &mockLedgerClient{
		listAccountsFn: func(_ context.Context, _ *connect.Request[floatv1.ListAccountsRequest]) (*connect.Response[floatv1.ListAccountsResponse], error) {
			return connect.NewResponse(&floatv1.ListAccountsResponse{Accounts: want}), nil
		},
	}
	cmd := FetchAccounts(client)
	msg := cmd().(AccountsMsg)
	if msg.Err != nil {
		t.Fatalf("unexpected error: %v", msg.Err)
	}
	if len(msg.Accounts) != 1 || msg.Accounts[0].Name != "checking" {
		t.Fatalf("unexpected accounts: %v", msg.Accounts)
	}
}

func TestFetchAccounts_Error(t *testing.T) {
	client := &mockLedgerClient{
		listAccountsFn: func(_ context.Context, _ *connect.Request[floatv1.ListAccountsRequest]) (*connect.Response[floatv1.ListAccountsResponse], error) {
			return nil, errors.New("connection refused")
		},
	}
	cmd := FetchAccounts(client)
	msg := cmd().(AccountsMsg)
	if msg.Err == nil {
		t.Fatal("expected error, got nil")
	}
	if msg.Accounts != nil {
		t.Fatalf("expected nil accounts, got %v", msg.Accounts)
	}
}

func TestFetchBalances_PropagatesDepthAndQuery(t *testing.T) {
	var gotDepth int32
	var gotQuery []string
	client := &mockLedgerClient{
		getBalancesFn: func(_ context.Context, req *connect.Request[floatv1.GetBalancesRequest]) (*connect.Response[floatv1.GetBalancesResponse], error) {
			gotDepth = req.Msg.Depth
			gotQuery = req.Msg.Query
			return connect.NewResponse(&floatv1.GetBalancesResponse{
				Report: &floatv1.BalanceReport{},
			}), nil
		},
	}
	cmd := FetchBalances(client, 2, []string{"expenses"})
	msg := cmd().(BalancesMsg)
	if msg.Err != nil {
		t.Fatalf("unexpected error: %v", msg.Err)
	}
	if gotDepth != 2 {
		t.Errorf("expected depth 2, got %d", gotDepth)
	}
	if len(gotQuery) != 1 || gotQuery[0] != "expenses" {
		t.Errorf("expected query [expenses], got %v", gotQuery)
	}
}

func TestFetchTransactions_PropagatesQuery(t *testing.T) {
	var gotQuery []string
	client := &mockLedgerClient{
		listTransactionsFn: func(_ context.Context, req *connect.Request[floatv1.ListTransactionsRequest]) (*connect.Response[floatv1.ListTransactionsResponse], error) {
			gotQuery = req.Msg.Query
			return connect.NewResponse(&floatv1.ListTransactionsResponse{}), nil
		},
	}
	cmd := FetchTransactions(client, []string{"date:2026-01"})
	msg := cmd().(TransactionsMsg)
	if msg.Err != nil {
		t.Fatalf("unexpected error: %v", msg.Err)
	}
	if len(gotQuery) != 1 || gotQuery[0] != "date:2026-01" {
		t.Errorf("expected query [date:2026-01], got %v", gotQuery)
	}
}
