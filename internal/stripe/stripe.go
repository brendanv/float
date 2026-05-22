package stripe

import (
	"context"
	"fmt"
	"time"

	stripe "github.com/stripe/stripe-go/v82"
)

const (
	waitForRefreshPollInterval = 2 * time.Second
	waitForRefreshMaxInterval  = 30 * time.Second
	waitForRefreshTimeout      = 5 * time.Minute
)

// WaitForRefresh polls the Financial Connections account until the transaction
// refresh status changes from "pending" to "succeeded" or "failed". It returns
// the refresh ID on success, or an error if the refresh fails or times out.
// If account.TransactionRefresh is nil, it returns ("", nil) immediately.
func WaitForRefresh(ctx context.Context, secretKey, accountID string) (string, error) {
	c := newClient(secretKey)
	deadline := time.Now().Add(waitForRefreshTimeout)
	interval := waitForRefreshPollInterval

	for {
		acct, err := c.V1FinancialConnectionsAccounts.GetByID(ctx, accountID, &stripe.FinancialConnectionsAccountRetrieveParams{})
		if err != nil {
			return "", fmt.Errorf("stripe: wait for refresh %s: get account: %w", accountID, err)
		}

		if acct.TransactionRefresh == nil {
			// No refresh in progress; return empty ID.
			return "", nil
		}

		switch acct.TransactionRefresh.Status {
		case stripe.FinancialConnectionsAccountTransactionRefreshStatusSucceeded:
			return acct.TransactionRefresh.ID, nil
		case stripe.FinancialConnectionsAccountTransactionRefreshStatusFailed:
			return "", fmt.Errorf("stripe: transaction refresh %s failed for account %s", acct.TransactionRefresh.ID, accountID)
		}
		// Status is "pending" — continue polling.

		if time.Now().After(deadline) {
			return "", fmt.Errorf("stripe: timed out waiting for transaction refresh on account %s", accountID)
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}

		// Exponential backoff up to max interval.
		interval *= 2
		if interval > waitForRefreshMaxInterval {
			interval = waitForRefreshMaxInterval
		}
	}
}

// GetTransactionRefreshID retrieves the current transaction refresh ID for the
// given account. Returns "" if the account has no transaction refresh recorded.
func GetTransactionRefreshID(ctx context.Context, secretKey, accountID string) (string, error) {
	c := newClient(secretKey)
	acct, err := c.V1FinancialConnectionsAccounts.GetByID(ctx, accountID, &stripe.FinancialConnectionsAccountRetrieveParams{})
	if err != nil {
		return "", fmt.Errorf("stripe: get transaction refresh id for %s: %w", accountID, err)
	}
	if acct.TransactionRefresh == nil {
		return "", nil
	}
	return acct.TransactionRefresh.ID, nil
}

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

func CreateCustomer(ctx context.Context, secretKey string) (string, error) {
	c := newClient(secretKey)
	customer, err := c.V1Customers.Create(ctx, &stripe.CustomerCreateParams{})
	if err != nil {
		return "", fmt.Errorf("stripe: create customer: %w", err)
	}
	return customer.ID, nil
}

func CreateFCSession(ctx context.Context, secretKey, customerID string) (string, error) {
	c := newClient(secretKey)
	sess, err := c.V1FinancialConnectionsSessions.Create(ctx, &stripe.FinancialConnectionsSessionCreateParams{
		AccountHolder: &stripe.FinancialConnectionsSessionCreateAccountHolderParams{
			Type:     stripe.String("customer"),
			Customer: stripe.String(customerID),
		},
		Permissions: []*string{
			stripe.String("transactions"),
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

func ListAccounts(ctx context.Context, secretKey, customerID string) ([]Account, error) {
	c := newClient(secretKey)
	params := &stripe.FinancialConnectionsAccountListParams{}
	if customerID != "" {
		params.AccountHolder = &stripe.FinancialConnectionsAccountListAccountHolderParams{
			Customer: stripe.String(customerID),
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

// ListTransactions returns transactions for an account. When afterRefreshID is
// non-empty, only transactions captured by that refresh (or later) are returned.
// Pass "" to fetch all transactions.
func ListTransactions(ctx context.Context, secretKey, accountID string, afterRefreshID string) ([]Transaction, error) {
	c := newClient(secretKey)
	params := &stripe.FinancialConnectionsTransactionListParams{
		Account: stripe.String(accountID),
	}
	if afterRefreshID != "" {
		params.TransactionRefresh = &stripe.FinancialConnectionsTransactionListTransactionRefreshParams{
			After: stripe.String(afterRefreshID),
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
