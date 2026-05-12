package stripeconn

import (
	"context"
	"errors"
	"fmt"

	"github.com/stripe/stripe-go/v82"
)

// Stripe is the subset of the Stripe Financial Connections API that float
// uses. Defined as an interface so that import.go and the handlers can be
// tested against a fake implementation without making real network calls.
type Stripe interface {
	// CreateSession bootstraps a Financial Connections session and returns its
	// client_secret, which the web UI passes to Stripe.js to run
	// collectFinancialConnectionsAccounts.
	CreateSession(ctx context.Context, params SessionParams) (clientSecret string, err error)

	// GetAccount fetches metadata for a single Financial Connections account.
	GetAccount(ctx context.Context, stripeAccountID string) (*stripe.FinancialConnectionsAccount, error)

	// ListPostedTransactions returns all transactions for the given Stripe
	// FC account that have status == "posted". The walk paginates internally
	// and may issue several API calls.
	ListPostedTransactions(ctx context.Context, stripeAccountID string) ([]*stripe.FinancialConnectionsTransaction, error)
}

// SessionParams collects the options float passes to CreateSession.
type SessionParams struct {
	// ReturnURL is used by Stripe for webview integrations. Optional.
	ReturnURL string
}

// LiveStripe is the production implementation of Stripe, backed by the
// stripe-go SDK. Each instance is bound to a single API key.
type LiveStripe struct {
	c *stripe.Client
}

// NewLiveStripe constructs a LiveStripe bound to apiKey. Returns an error if
// the key is empty.
func NewLiveStripe(apiKey string) (*LiveStripe, error) {
	if apiKey == "" {
		return nil, errors.New("stripeconn: api key is empty")
	}
	return &LiveStripe{c: stripe.NewClient(apiKey)}, nil
}

// CreateSession requests permissions to read accounts, balances and
// transactions. The returned client_secret is short-lived and is consumed by
// Stripe.js on the front-end.
func (s *LiveStripe) CreateSession(ctx context.Context, params SessionParams) (string, error) {
	p := &stripe.FinancialConnectionsSessionCreateParams{
		AccountHolder: &stripe.FinancialConnectionsSessionCreateAccountHolderParams{
			Type: stripe.String("customer"),
		},
		Permissions: []*string{
			stripe.String("balances"),
			stripe.String("transactions"),
		},
		Prefetch: []*string{
			stripe.String("balances"),
			stripe.String("transactions"),
		},
	}
	if params.ReturnURL != "" {
		p.ReturnURL = stripe.String(params.ReturnURL)
	}
	out, err := s.c.V1FinancialConnectionsSessions.Create(ctx, p)
	if err != nil {
		return "", fmt.Errorf("stripeconn: create session: %w", err)
	}
	return out.ClientSecret, nil
}

// GetAccount fetches a single account by id.
func (s *LiveStripe) GetAccount(ctx context.Context, stripeAccountID string) (*stripe.FinancialConnectionsAccount, error) {
	a, err := s.c.V1FinancialConnectionsAccounts.GetByID(ctx, stripeAccountID, nil)
	if err != nil {
		return nil, fmt.Errorf("stripeconn: get account %s: %w", stripeAccountID, err)
	}
	return a, nil
}

// ListPostedTransactions iterates every page of FC transactions for an
// account and returns the ones with status == "posted". Pending
// transactions are dropped because their Stripe ids change when they post.
func (s *LiveStripe) ListPostedTransactions(ctx context.Context, stripeAccountID string) ([]*stripe.FinancialConnectionsTransaction, error) {
	p := &stripe.FinancialConnectionsTransactionListParams{
		Account: stripe.String(stripeAccountID),
	}
	p.Limit = stripe.Int64(100)

	var out []*stripe.FinancialConnectionsTransaction
	for tx, err := range s.c.V1FinancialConnectionsTransactions.List(ctx, p) {
		if err != nil {
			return nil, fmt.Errorf("stripeconn: list transactions for %s: %w", stripeAccountID, err)
		}
		if tx == nil {
			continue
		}
		if tx.Status != stripe.FinancialConnectionsTransactionStatusPosted {
			continue
		}
		out = append(out, tx)
	}
	return out, nil
}
