package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	connect "connectrpc.com/connect"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

type AccountsMsg struct {
	Accounts []*floatv1.Account
	Err      error
}

type BalancesMsg struct {
	Report *floatv1.BalanceReport
	Err    error
}

type TransactionsMsg struct {
	Transactions []*floatv1.Transaction
	Err          error
}

type RetryFetchMsg struct{}

func FetchAccounts(client floatv1connect.LedgerServiceClient) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.ListAccounts(context.Background(), connect.NewRequest(&floatv1.ListAccountsRequest{}))
		if err != nil {
			return AccountsMsg{Err: err}
		}
		return AccountsMsg{Accounts: resp.Msg.Accounts}
	}
}

func FetchBalances(client floatv1connect.LedgerServiceClient, depth int32, query []string) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.GetBalances(context.Background(), connect.NewRequest(&floatv1.GetBalancesRequest{
			Depth: depth,
			Query: query,
		}))
		if err != nil {
			return BalancesMsg{Err: err}
		}
		return BalancesMsg{Report: resp.Msg.Report}
	}
}

func FetchTransactions(client floatv1connect.LedgerServiceClient, query []string) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.ListTransactions(context.Background(), connect.NewRequest(&floatv1.ListTransactionsRequest{
			Query: query,
		}))
		if err != nil {
			return TransactionsMsg{Err: err}
		}
		return TransactionsMsg{Transactions: resp.Msg.Transactions}
	}
}
