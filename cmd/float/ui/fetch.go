package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"
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

type AddTransactionMsg struct {
	Transaction *floatv1.Transaction
	Err         error
}

func AddTransactionCmd(client floatv1connect.LedgerServiceClient, req *floatv1.AddTransactionRequest) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.AddTransaction(context.Background(), connect.NewRequest(req))
		if err != nil {
			return AddTransactionMsg{Err: err}
		}
		return AddTransactionMsg{Transaction: resp.Msg.Transaction}
	}
}

type UpdateTransactionMsg struct {
	Transaction *floatv1.Transaction
	Err         error
}

func UpdateTransactionCmd(client floatv1connect.LedgerServiceClient, req *floatv1.UpdateTransactionRequest) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.UpdateTransaction(context.Background(), connect.NewRequest(req))
		if err != nil {
			return UpdateTransactionMsg{Err: err}
		}
		return UpdateTransactionMsg{Transaction: resp.Msg.Transaction}
	}
}

type DeleteTransactionMsg struct {
	Err error
}

func DeleteTransactionCmd(client floatv1connect.LedgerServiceClient, fid string) tea.Cmd {
	return func() tea.Msg {
		_, err := client.DeleteTransaction(context.Background(), connect.NewRequest(&floatv1.DeleteTransactionRequest{
			Fid: fid,
		}))
		return DeleteTransactionMsg{Err: err}
	}
}

type UpdateTransactionStatusMsg struct {
	Err error
}

func UpdateTransactionStatusCmd(client floatv1connect.LedgerServiceClient, fid, status string) tea.Cmd {
	return func() tea.Msg {
		_, err := client.UpdateTransactionStatus(context.Background(), connect.NewRequest(&floatv1.UpdateTransactionStatusRequest{
			Fid:    fid,
			Status: status,
		}))
		return UpdateTransactionStatusMsg{Err: err}
	}
}

type InsightsMsg struct {
	Report *floatv1.BalanceReport
	Err    error
}

func FetchInsights(client floatv1connect.LedgerServiceClient, periodQuery string) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.GetBalances(context.Background(), connect.NewRequest(&floatv1.GetBalancesRequest{
			Depth: 2,
			Query: []string{periodQuery},
		}))
		if err != nil {
			return InsightsMsg{Err: err}
		}
		return InsightsMsg{Report: resp.Msg.Report}
	}
}

type NetWorthMsg struct {
	Snapshots []*floatv1.NetWorthSnapshot
	Err       error
}

func FetchNetWorth(client floatv1connect.LedgerServiceClient) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.GetNetWorthTimeseries(context.Background(), connect.NewRequest(&floatv1.GetNetWorthTimeseriesRequest{}))
		if err != nil {
			return NetWorthMsg{Err: err}
		}
		return NetWorthMsg{Snapshots: resp.Msg.Snapshots}
	}
}

// HomeNetWorthMsg is a distinct type from NetWorthMsg to prevent TrendsTab
// from consuming messages intended for HomeTab's chart panel.
type HomeNetWorthMsg struct {
	Snapshots []*floatv1.NetWorthSnapshot
	Err       error
}

// FetchHomeNetWorth fetches the net worth timeseries for the home tab chart panel.
func FetchHomeNetWorth(client floatv1connect.LedgerServiceClient) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.GetNetWorthTimeseries(context.Background(), connect.NewRequest(&floatv1.GetNetWorthTimeseriesRequest{}))
		if err != nil {
			return HomeNetWorthMsg{Err: err}
		}
		return HomeNetWorthMsg{Snapshots: resp.Msg.Snapshots}
	}
}

// ManagerAccountsMsg carries accounts for the Manager tab.
// Using a distinct type prevents HomeTab from consuming this message.
type ManagerAccountsMsg struct {
	Accounts []*floatv1.Account
	Err      error
}

// ManagerBalancesMsg carries depth-0 balances for the account tree.
type ManagerBalancesMsg struct {
	Report *floatv1.BalanceReport
	Err    error
}

// ManagerSummaryMsg carries depth-1 balances for the summary panel.
type ManagerSummaryMsg struct {
	Report *floatv1.BalanceReport
	Err    error
}

// FetchManagerAccounts fetches all accounts and returns ManagerAccountsMsg.
func FetchManagerAccounts(client floatv1connect.LedgerServiceClient) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.ListAccounts(context.Background(), connect.NewRequest(&floatv1.ListAccountsRequest{}))
		if err != nil {
			return ManagerAccountsMsg{Err: err}
		}
		return ManagerAccountsMsg{Accounts: resp.Msg.Accounts}
	}
}

// FetchManagerBalances fetches depth-0 balances (all accounts) for tree display.
func FetchManagerBalances(client floatv1connect.LedgerServiceClient) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.GetBalances(context.Background(), connect.NewRequest(&floatv1.GetBalancesRequest{
			Depth: 0,
		}))
		if err != nil {
			return ManagerBalancesMsg{Err: err}
		}
		return ManagerBalancesMsg{Report: resp.Msg.Report}
	}
}

// FetchManagerSummary fetches depth-1 balances (top-level account type totals) for the summary panel.
func FetchManagerSummary(client floatv1connect.LedgerServiceClient) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.GetBalances(context.Background(), connect.NewRequest(&floatv1.GetBalancesRequest{
			Depth: 1,
		}))
		if err != nil {
			return ManagerSummaryMsg{Err: err}
		}
		return ManagerSummaryMsg{Report: resp.Msg.Report}
	}
}
