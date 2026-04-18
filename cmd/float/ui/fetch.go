package ui

import (
	"context"
	"fmt"

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

// RulesMsg carries all rules for the Rules tab.
type RulesMsg struct {
	Rules []*floatv1.TransactionRule
	Err   error
}

// AddRuleMsg carries the result of an AddRule RPC.
type AddRuleMsg struct {
	Rule *floatv1.TransactionRule
	Err  error
}

// UpdateRuleMsg carries the result of an UpdateRule RPC.
type UpdateRuleMsg struct {
	Rule *floatv1.TransactionRule
	Err  error
}

// DeleteRuleMsg carries the result of a DeleteRule RPC.
type DeleteRuleMsg struct {
	Err error
}

// PreviewApplyRulesMsg carries the preview results for apply rules.
type PreviewApplyRulesMsg struct {
	Previews []*floatv1.RuleApplicationPreview
	Err      error
}

// ApplyRulesMsg carries the result of applying rules.
type ApplyRulesMsg struct {
	AppliedCount int32
	Err          error
}

func FetchRules(client floatv1connect.LedgerServiceClient) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.ListRules(context.Background(), connect.NewRequest(&floatv1.ListRulesRequest{}))
		if err != nil {
			return RulesMsg{Err: err}
		}
		return RulesMsg{Rules: resp.Msg.Rules}
	}
}

func AddRuleCmd(client floatv1connect.LedgerServiceClient, req *floatv1.AddRuleRequest) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.AddRule(context.Background(), connect.NewRequest(req))
		if err != nil {
			return AddRuleMsg{Err: err}
		}
		return AddRuleMsg{Rule: resp.Msg.Rule}
	}
}

func UpdateRuleCmd(client floatv1connect.LedgerServiceClient, req *floatv1.UpdateRuleRequest) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.UpdateRule(context.Background(), connect.NewRequest(req))
		if err != nil {
			return UpdateRuleMsg{Err: err}
		}
		return UpdateRuleMsg{Rule: resp.Msg.Rule}
	}
}

func DeleteRuleCmd(client floatv1connect.LedgerServiceClient, id string) tea.Cmd {
	return func() tea.Msg {
		_, err := client.DeleteRule(context.Background(), connect.NewRequest(&floatv1.DeleteRuleRequest{Id: id}))
		return DeleteRuleMsg{Err: err}
	}
}

func PreviewApplyRulesCmd(client floatv1connect.LedgerServiceClient) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.PreviewApplyRules(context.Background(), connect.NewRequest(&floatv1.PreviewApplyRulesRequest{}))
		if err != nil {
			return PreviewApplyRulesMsg{Err: err}
		}
		return PreviewApplyRulesMsg{Previews: resp.Msg.Previews}
	}
}

// AccountRegisterMsg carries the result of a GetAccountRegister RPC for the Manager tab.
type AccountRegisterMsg struct {
	Account string
	Rows    []*floatv1.AccountRegisterRow
	Total   int32
	Err     error
}

// FetchAccountRegister fetches the account register for the given account name.
func FetchAccountRegister(client floatv1connect.LedgerServiceClient, account string) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.GetAccountRegister(context.Background(), connect.NewRequest(&floatv1.GetAccountRegisterRequest{
			Account: account,
		}))
		if err != nil {
			return AccountRegisterMsg{Account: account, Err: err}
		}
		return AccountRegisterMsg{
			Account: account,
			Rows:    resp.Msg.Rows,
			Total:   resp.Msg.Total,
		}
	}
}

// ManagerTxFetchedMsg carries a single transaction fetched for editing in the Manager register.
type ManagerTxFetchedMsg struct {
	Transaction *floatv1.Transaction
	Err         error
}

// FetchManagerTransaction fetches a single transaction by fid for the Manager tab edit flow.
func FetchManagerTransaction(client floatv1connect.LedgerServiceClient, fid string) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.ListTransactions(context.Background(), connect.NewRequest(&floatv1.ListTransactionsRequest{
			Query: []string{"code:" + fid},
		}))
		if err != nil {
			return ManagerTxFetchedMsg{Err: err}
		}
		if len(resp.Msg.Transactions) == 0 {
			return ManagerTxFetchedMsg{Err: fmt.Errorf("transaction not found")}
		}
		return ManagerTxFetchedMsg{Transaction: resp.Msg.Transactions[0]}
	}
}

func ApplyRulesCmd(client floatv1connect.LedgerServiceClient, fids []string) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.ApplyRules(context.Background(), connect.NewRequest(&floatv1.ApplyRulesRequest{
			Fids: fids,
		}))
		if err != nil {
			return ApplyRulesMsg{Err: err}
		}
		return ApplyRulesMsg{AppliedCount: resp.Msg.AppliedCount}
	}
}

// ImportsMsg carries the list of import summaries.
type ImportsMsg struct {
	Imports []*floatv1.ImportSummary
	Err     error
}

// ImportedTransactionsMsg carries transactions for a specific import batch.
type ImportedTransactionsMsg struct {
	BatchId      string
	Transactions []*floatv1.Transaction
	Err          error
}

func FetchImports(client floatv1connect.LedgerServiceClient) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.ListImports(context.Background(), connect.NewRequest(&floatv1.ListImportsRequest{}))
		if err != nil {
			return ImportsMsg{Err: err}
		}
		return ImportsMsg{Imports: resp.Msg.Imports}
	}
}

func FetchImportedTransactions(client floatv1connect.LedgerServiceClient, batchId string) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.GetImportedTransactions(context.Background(), connect.NewRequest(&floatv1.GetImportedTransactionsRequest{
			ImportBatchId: batchId,
		}))
		if err != nil {
			return ImportedTransactionsMsg{BatchId: batchId, Err: err}
		}
		return ImportedTransactionsMsg{BatchId: batchId, Transactions: resp.Msg.Transactions}
	}
}
