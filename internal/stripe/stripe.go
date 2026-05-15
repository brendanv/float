package stripe

import (
	"context"
	"fmt"
	"time"

	stripe "github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/financialconnections/account"
	"github.com/stripe/stripe-go/v82/financialconnections/session"
	"github.com/stripe/stripe-go/v82/financialconnections/transaction"
)

type Account struct {
	ID          string
	DisplayName string
	Institution string
	Last4       string
	Status      string // "active", "inactive", or "disconnected"
}

type Transaction struct {
	ID          string
	AccountID   string
	AmountCents int64
	Currency    string
	Description string
	TransactedAt time.Time
	Status      string // "posted" or "pending"
}

func CreateFCSession(ctx context.Context, secretKey, accountID string) (string, error) {
	stripe.Key = secretKey
	params := &stripe.FinancialConnectionsSessionParams{
		AccountHolder: &stripe.FinancialConnectionsSessionAccountHolderParams{
			Type:    stripe.String("account"),
			Account: stripe.String(accountID),
		},
		Permissions: []*string{
			stripe.String("payment_method"),
			stripe.String("transactions"),
			stripe.String("balances"),
		},
		Filters: &stripe.FinancialConnectionsSessionFiltersParams{
			Countries: []*string{stripe.String("US")},
		},
	}
	params.Context = ctx
	sess, err := session.New(params)
	if err != nil {
		return "", fmt.Errorf("stripe: create fc session: %w", err)
	}
	return sess.ClientSecret, nil
}

func ListSessionAccounts(ctx context.Context, secretKey, sessionID string) ([]Account, error) {
	stripe.Key = secretKey
	params := &stripe.FinancialConnectionsSessionParams{}
	params.Context = ctx
	params.AddExpand("accounts")
	sess, err := session.Get(sessionID, params)
	if err != nil {
		return nil, fmt.Errorf("stripe: list session accounts: %w", err)
	}
	var accounts []Account
	if sess.Accounts != nil {
		for _, a := range sess.Accounts.Data {
			acc := Account{
				ID: a.ID,
			}
			if a.DisplayName != "" {
				acc.DisplayName = a.DisplayName
			}
			if a.InstitutionName != "" {
				acc.Institution = a.InstitutionName
			}
			if a.Last4 != "" {
				acc.Last4 = a.Last4
			}
			accounts = append(accounts, acc)
		}
	}
	return accounts, nil
}

func ListAccounts(ctx context.Context, secretKey, accountID string) ([]Account, error) {
	stripe.Key = secretKey
	params := &stripe.FinancialConnectionsAccountListParams{}
	if accountID != "" {
		params.AccountHolder = &stripe.FinancialConnectionsAccountListAccountHolderParams{
			Account: stripe.String(accountID),
		}
	}
	params.Context = ctx
	var accounts []Account
	iter := account.List(params)
	for iter.Next() {
		a := iter.FinancialConnectionsAccount()
		accounts = append(accounts, Account{
			ID:          a.ID,
			DisplayName: a.DisplayName,
			Institution: a.InstitutionName,
			Last4:       a.Last4,
			Status:      string(a.Status),
		})
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("stripe: list accounts: %w", err)
	}
	return accounts, nil
}

func SubscribeTransactions(ctx context.Context, secretKey, accountID string) error {
	stripe.Key = secretKey
	params := &stripe.FinancialConnectionsAccountSubscribeParams{
		Features: []*string{stripe.String("transactions")},
	}
	params.Context = ctx
	_, err := account.Subscribe(accountID, params)
	if err != nil {
		return fmt.Errorf("stripe: subscribe transactions for %s: %w", accountID, err)
	}
	return nil
}

func RefreshTransactions(ctx context.Context, secretKey, accountID string) error {
	stripe.Key = secretKey
	params := &stripe.FinancialConnectionsAccountRefreshParams{
		Features: []*string{stripe.String("transactions")},
	}
	params.Context = ctx
	_, err := account.Refresh(accountID, params)
	if err != nil {
		return fmt.Errorf("stripe: refresh transactions for %s: %w", accountID, err)
	}
	return nil
}

func DisconnectAccount(ctx context.Context, secretKey, accountID string) error {
	stripe.Key = secretKey
	params := &stripe.FinancialConnectionsAccountDisconnectParams{}
	params.Context = ctx
	_, err := account.Disconnect(accountID, params)
	if err != nil {
		return fmt.Errorf("stripe: disconnect account %s: %w", accountID, err)
	}
	return nil
}

func ListTransactions(ctx context.Context, secretKey, accountID string, since time.Time) ([]Transaction, error) {
	stripe.Key = secretKey
	params := &stripe.FinancialConnectionsTransactionListParams{
		Account: stripe.String(accountID),
	}
	if !since.IsZero() {
		params.TransactedAtRange = &stripe.RangeQueryParams{
			GreaterThan: since.Unix(),
		}
	}
	params.Context = ctx

	var txns []Transaction
	iter := transaction.List(params)
	for iter.Next() {
		t := iter.FinancialConnectionsTransaction()
		txns = append(txns, Transaction{
			ID:           t.ID,
			AccountID:    accountID,
			AmountCents:  t.Amount,
			Currency:     string(t.Currency),
			Description:  t.Description,
			TransactedAt: time.Unix(t.TransactedAt, 0).UTC(),
			Status:       string(t.Status),
		})
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("stripe: list transactions for %s: %w", accountID, err)
	}
	return txns, nil
}
