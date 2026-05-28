package stripe

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	stripe "github.com/stripe/stripe-go/v82"
)

const (
	waitForRefreshPollInterval = 2 * time.Second
	waitForRefreshMaxInterval  = 30 * time.Second
	waitForRefreshTimeout      = 5 * time.Minute
)

// RefreshProgress is reported by WaitForRefreshWithProgress on each poll tick
// and on terminal states.
type RefreshProgress struct {
	// Status is one of: "starting", "polling", "succeeded", "failed", "timeout", "skipped".
	// "skipped" means no transaction_refresh is currently associated with the account.
	Status       string
	Attempt      int
	Elapsed      time.Duration
	NextInterval time.Duration // 0 on terminal states
	RefreshID    string        // populated when known
	Err          error         // populated on "failed"/"timeout"
}

// WaitForRefresh polls the Financial Connections account until the transaction
// refresh status changes from "pending" to "succeeded" or "failed". It returns
// the refresh ID on success, or an error if the refresh fails or times out.
// If account.TransactionRefresh is nil, it returns ("", nil) immediately.
//
// Pass a logger to receive structured progress logs; pass nil to use slog.Default().
func WaitForRefresh(ctx context.Context, logger *slog.Logger, secretKey, accountID string) (string, error) {
	return WaitForRefreshWithProgress(ctx, logger, secretKey, accountID, nil)
}

// WaitForRefreshWithProgress is like WaitForRefresh but invokes onProgress
// at start, on each "pending" poll, and once on the terminal state.
// onProgress may be nil.
func WaitForRefreshWithProgress(
	ctx context.Context,
	logger *slog.Logger,
	secretKey, accountID string,
	onProgress func(RefreshProgress),
) (string, error) {
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("account", accountID)

	c := newClient(secretKey)
	start := time.Now()
	deadline := start.Add(waitForRefreshTimeout)
	interval := waitForRefreshPollInterval
	attempt := 0

	logger.InfoContext(ctx, "stripe: waiting for transaction refresh")
	if onProgress != nil {
		onProgress(RefreshProgress{Status: "starting"})
	}

	emit := func(p RefreshProgress) {
		if onProgress != nil {
			onProgress(p)
		}
	}

	for {
		attempt++
		acct, err := c.V1FinancialConnectionsAccounts.GetByID(ctx, accountID, &stripe.FinancialConnectionsAccountRetrieveParams{})
		if err != nil {
			elapsed := time.Since(start)
			logger.ErrorContext(ctx, "stripe: refresh poll failed", "attempt", attempt, "elapsed_seconds", int(elapsed.Seconds()), "error", err)
			wrapped := fmt.Errorf("stripe: wait for refresh %s: get account: %w", accountID, err)
			emit(RefreshProgress{Status: "failed", Attempt: attempt, Elapsed: elapsed, Err: wrapped})
			return "", wrapped
		}

		elapsed := time.Since(start)

		if acct.TransactionRefresh == nil {
			logger.InfoContext(ctx, "stripe: no refresh in progress", "attempts", attempt, "elapsed_seconds", int(elapsed.Seconds()))
			emit(RefreshProgress{Status: "skipped", Attempt: attempt, Elapsed: elapsed})
			return "", nil
		}

		switch acct.TransactionRefresh.Status {
		case stripe.FinancialConnectionsAccountTransactionRefreshStatusSucceeded:
			logger.InfoContext(ctx, "stripe: refresh succeeded",
				"refresh_id", acct.TransactionRefresh.ID,
				"attempts", attempt,
				"elapsed_seconds", int(elapsed.Seconds()))
			emit(RefreshProgress{Status: "succeeded", Attempt: attempt, Elapsed: elapsed, RefreshID: acct.TransactionRefresh.ID})
			return acct.TransactionRefresh.ID, nil
		case stripe.FinancialConnectionsAccountTransactionRefreshStatusFailed:
			logger.WarnContext(ctx, "stripe: refresh failed",
				"refresh_id", acct.TransactionRefresh.ID,
				"attempts", attempt,
				"elapsed_seconds", int(elapsed.Seconds()))
			wrapped := fmt.Errorf("stripe: transaction refresh %s failed for account %s", acct.TransactionRefresh.ID, accountID)
			emit(RefreshProgress{Status: "failed", Attempt: attempt, Elapsed: elapsed, RefreshID: acct.TransactionRefresh.ID, Err: wrapped})
			return "", wrapped
		}
		// Status is "pending" — continue polling.

		if time.Now().After(deadline) {
			logger.WarnContext(ctx, "stripe: refresh timed out",
				"refresh_id", acct.TransactionRefresh.ID,
				"attempts", attempt,
				"elapsed_seconds", int(elapsed.Seconds()))
			wrapped := fmt.Errorf("stripe: timed out waiting for transaction refresh on account %s", accountID)
			emit(RefreshProgress{Status: "timeout", Attempt: attempt, Elapsed: elapsed, RefreshID: acct.TransactionRefresh.ID, Err: wrapped})
			return "", wrapped
		}

		logger.DebugContext(ctx, "stripe: refresh still pending",
			"refresh_id", acct.TransactionRefresh.ID,
			"attempt", attempt,
			"elapsed_seconds", int(elapsed.Seconds()),
			"next_interval_seconds", int(interval.Seconds()))
		emit(RefreshProgress{
			Status:       "polling",
			Attempt:      attempt,
			Elapsed:      elapsed,
			NextInterval: interval,
			RefreshID:    acct.TransactionRefresh.ID,
		})

		select {
		case <-ctx.Done():
			logger.WarnContext(ctx, "stripe: refresh wait cancelled", "attempts", attempt, "elapsed_seconds", int(elapsed.Seconds()), "error", ctx.Err())
			emit(RefreshProgress{Status: "failed", Attempt: attempt, Elapsed: elapsed, Err: ctx.Err()})
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

// RefreshKickoffStatus describes the outcome of MaybeRefreshTransactions.
type RefreshKickoffStatus int

const (
	// RefreshKickoffStarted means a new transaction refresh was initiated.
	RefreshKickoffStarted RefreshKickoffStatus = iota
	// RefreshKickoffAlreadyPending means a refresh was already in progress;
	// the caller should poll for the existing one rather than start a new one.
	RefreshKickoffAlreadyPending
	// RefreshKickoffThrottled means the account's next_refresh_available_at
	// is in the future; no refresh was initiated.
	RefreshKickoffThrottled
)

// RefreshKickoff describes the outcome of MaybeRefreshTransactions.
type RefreshKickoff struct {
	Status RefreshKickoffStatus
	// CurrentRefreshID is the ID of the existing transaction_refresh on the
	// account at the time of the check, if any.
	CurrentRefreshID string
	// NextRefreshAvailableAt is the time the next refresh becomes available.
	// Zero when Status is not RefreshKickoffThrottled.
	NextRefreshAvailableAt time.Time
}

// MaybeRefreshTransactions inspects the account state and either initiates a
// new transaction refresh, joins a pending one, or reports that refreshes are
// currently throttled (per Stripe's next_refresh_available_at field).
//
// Use this rather than RefreshTransactions directly when honoring Stripe's
// per-account refresh throttle. Pass nil logger to use slog.Default().
func MaybeRefreshTransactions(ctx context.Context, logger *slog.Logger, secretKey, accountID string) (RefreshKickoff, error) {
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("account", accountID)

	c := newClient(secretKey)
	acct, err := c.V1FinancialConnectionsAccounts.GetByID(ctx, accountID, &stripe.FinancialConnectionsAccountRetrieveParams{})
	if err != nil {
		return RefreshKickoff{}, fmt.Errorf("stripe: get account %s for refresh check: %w", accountID, err)
	}

	if acct.TransactionRefresh != nil && acct.TransactionRefresh.Status == stripe.FinancialConnectionsAccountTransactionRefreshStatusPending {
		logger.InfoContext(ctx, "stripe: refresh already pending, joining existing", "refresh_id", acct.TransactionRefresh.ID)
		return RefreshKickoff{
			Status:           RefreshKickoffAlreadyPending,
			CurrentRefreshID: acct.TransactionRefresh.ID,
		}, nil
	}

	if acct.TransactionRefresh != nil && acct.TransactionRefresh.NextRefreshAvailableAt > 0 {
		nextAt := time.Unix(acct.TransactionRefresh.NextRefreshAvailableAt, 0).UTC()
		if time.Now().Before(nextAt) {
			logger.InfoContext(ctx, "stripe: refresh throttled",
				"refresh_id", acct.TransactionRefresh.ID,
				"next_refresh_available_at", nextAt.Format(time.RFC3339))
			return RefreshKickoff{
				Status:                 RefreshKickoffThrottled,
				CurrentRefreshID:       acct.TransactionRefresh.ID,
				NextRefreshAvailableAt: nextAt,
			}, nil
		}
	}

	if err := RefreshTransactions(ctx, secretKey, accountID); err != nil {
		return RefreshKickoff{}, err
	}
	currentID := ""
	if acct.TransactionRefresh != nil {
		currentID = acct.TransactionRefresh.ID
	}
	logger.InfoContext(ctx, "stripe: refresh kicked off", "previous_refresh_id", currentID)
	return RefreshKickoff{
		Status:           RefreshKickoffStarted,
		CurrentRefreshID: currentID,
	}, nil
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

// ListTransactions returns all available transactions for an account.
func ListTransactions(ctx context.Context, secretKey, accountID string) ([]Transaction, error) {
	c := newClient(secretKey)
	params := &stripe.FinancialConnectionsTransactionListParams{
		Account: stripe.String(accountID),
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
