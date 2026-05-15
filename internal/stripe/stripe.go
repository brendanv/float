package stripe

import (
	"context"
	"fmt"
	"time"

	stripe "github.com/stripe/stripe-go/v82"
)

type Account struct {
	ID          string
	DisplayName string
	Institution string
	Last4       string
	Status      string // "active", "inactive", or "disconnected"
}

type Transaction struct {
	ID           string
	AccountID    string
	AmountCents  int64
	Currency     string
	Description  string
	TransactedAt time.Time
	Status       string // "posted" or "pending"
}

func newClient(secretKey string) *stripe.Client {
	return stripe.NewClient(secretKey)
}

func CreateFCSession(ctx context.Context, secretKey, accountID string) (string, error) {
	c := newClient(secretKey)
	sess, err := c.V1FinancialConnectionsSessions.Create(ctx, &stripe.FinancialConnectionsSessionCreateParams{
		AccountHolder: &stripe.FinancialConnectionsSessionCreateAccountHolderParams{
			Type:    stripe.String("account"),
			Account: stripe.String(accountID),
		},
		Permissions: []*string{
			stripe.String("payment_method"),
			stripe.String("transactions"),
			stripe.String("balances"),
		},
		Filters: &stripe.FinancialConnectionsSessionCreateFiltersParams{
			Countries: []*string{stripe.String("US")},
		},
	})
	if err != nil {
		return "", fmt.Errorf("stripe: create fc session: %w", err)
	}
	return sess.ClientSecret, nil
}

func ListSessionAccounts(ctx context.Context, secretKey, sessionID string) ([]Account, error) {
	c := newClient(secretKey)
	params := &stripe.FinancialConnectionsSessionRetrieveParams{}
	params.AddExpand("accounts")
	sess, err := c.V1FinancialConnectionsSessions.Retrieve(ctx, sessionID, params)
	if err != nil {
		return nil, fmt.Errorf("stripe: list session accounts: %w", err)
	}
	var accounts []Account
	if sess.Accounts != nil {
		for _, a := range sess.Accounts.Data {
			acc := Account{
				ID:     a.ID,
				Status: string(a.Status),
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
	c := newClient(secretKey)
	params := &stripe.FinancialConnectionsAccountListParams{}
	if accountID != "" {
		params.AccountHolder = &stripe.FinancialConnectionsAccountListAccountHolderParams{
			Account: stripe.String(accountID),
		}
	}
	var accounts []Account
	for a, err := range c.V1FinancialConnectionsAccounts.List(ctx, params) {
		if err != nil {
			return nil, fmt.Errorf("stripe: list accounts: %w", err)
		}
		accounts = append(accounts, Account{
			ID:          a.ID,
			DisplayName: a.DisplayName,
			Institution: a.InstitutionName,
			Last4:       a.Last4,
			Status:      string(a.Status),
		})
	}
	return accounts, nil
}

func SubscribeTransactions(ctx context.Context, secretKey, accountID string) error {
	c := newClient(secretKey)
	_, err := c.V1FinancialConnectionsAccounts.Subscribe(ctx, accountID, &stripe.FinancialConnectionsAccountSubscribeParams{
		Features: []*string{stripe.String("transactions")},
	})
	if err != nil {
		return fmt.Errorf("stripe: subscribe transactions for %s: %w", accountID, err)
	}
	return nil
}

func RefreshTransactions(ctx context.Context, secretKey, accountID string) error {
	c := newClient(secretKey)
	_, err := c.V1FinancialConnectionsAccounts.Refresh(ctx, accountID, &stripe.FinancialConnectionsAccountRefreshParams{
		Features: []*string{stripe.String("transactions")},
	})
	if err != nil {
		return fmt.Errorf("stripe: refresh transactions for %s: %w", accountID, err)
	}
	return nil
}

func DisconnectAccount(ctx context.Context, secretKey, accountID string) error {
	c := newClient(secretKey)
	_, err := c.V1FinancialConnectionsAccounts.Disconnect(ctx, accountID, &stripe.FinancialConnectionsAccountDisconnectParams{})
	if err != nil {
		return fmt.Errorf("stripe: disconnect account %s: %w", accountID, err)
	}
	return nil
}

func ListTransactions(ctx context.Context, secretKey, accountID string, since time.Time) ([]Transaction, error) {
	c := newClient(secretKey)
	params := &stripe.FinancialConnectionsTransactionListParams{
		Account: stripe.String(accountID),
	}
	if !since.IsZero() {
		params.TransactedAtRange = &stripe.RangeQueryParams{
			GreaterThan: since.Unix(),
		}
	}

	var txns []Transaction
	for t, err := range c.V1FinancialConnectionsTransactions.List(ctx, params) {
		if err != nil {
			return nil, fmt.Errorf("stripe: list transactions for %s: %w", accountID, err)
		}
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
	return txns, nil
}
