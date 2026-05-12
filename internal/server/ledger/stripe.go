package ledger

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/rules"
	"github.com/brendanv/float/internal/slogctx"
	"github.com/brendanv/float/internal/stripeconn"
	"github.com/stripe/stripe-go/v82"
)

// stripeClient builds a Stripe client using the configured API key, allowing
// tests to override via h.StripeFactory. Returns CodeFailedPrecondition if
// no key is configured.
func (h *Handler) stripeClient() (stripeconn.Stripe, error) {
	if h.cfg == nil || h.cfg.Stripe.APIKey == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("stripe.api_key is not configured"))
	}
	factory := h.StripeFactory
	if factory == nil {
		factory = func(key string) (stripeconn.Stripe, error) {
			return stripeconn.NewLiveStripe(key)
		}
	}
	return factory(h.cfg.Stripe.APIKey)
}

func (h *Handler) GetStripeStatus(ctx context.Context, _ *connect.Request[floatv1.GetStripeStatusRequest]) (*connect.Response[floatv1.GetStripeStatusResponse], error) {
	if h.cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	resp := &floatv1.GetStripeStatusResponse{}
	if key := h.cfg.Stripe.APIKey; key != "" {
		resp.ApiKeyConfigured = true
		if len(key) > 7 {
			resp.ApiKeyPreview = key[:7] + "..."
		} else {
			resp.ApiKeyPreview = "..."
		}
	}
	store, err := stripeconn.Load(h.dataDir)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	resp.ConnectionCount = int32(len(store.Connections))
	return connect.NewResponse(resp), nil
}

func (h *Handler) SetStripeApiKey(ctx context.Context, req *connect.Request[floatv1.SetStripeApiKeyRequest]) (*connect.Response[floatv1.SetStripeApiKeyResponse], error) {
	if h.cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	if h.configPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server config path not set"))
	}

	oldKey := h.cfg.Stripe.APIKey
	err := h.lock.Do(ctx, "set stripe api key", func() error {
		h.cfg.Stripe.APIKey = req.Msg.ApiKey
		if err := config.Save(h.configPath, h.cfg); err != nil {
			h.cfg.Stripe.APIKey = oldKey
			return fmt.Errorf("save config: %w", err)
		}
		return nil
	})
	if err != nil {
		slogctx.FromContext(ctx).ErrorContext(ctx, "set stripe api key failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	slogctx.FromContext(ctx).InfoContext(ctx, "updated stripe api key")
	return connect.NewResponse(&floatv1.SetStripeApiKeyResponse{}), nil
}

func (h *Handler) CreateStripeSession(ctx context.Context, req *connect.Request[floatv1.CreateStripeSessionRequest]) (*connect.Response[floatv1.CreateStripeSessionResponse], error) {
	api, err := h.stripeClient()
	if err != nil {
		return nil, err
	}
	secret, err := api.CreateSession(ctx, stripeconn.SessionParams{
		ReturnURL: req.Msg.ReturnUrl,
	})
	if err != nil {
		slogctx.FromContext(ctx).ErrorContext(ctx, "create stripe session failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&floatv1.CreateStripeSessionResponse{
		ClientSecret: secret,
	}), nil
}

func (h *Handler) LinkStripeAccounts(ctx context.Context, req *connect.Request[floatv1.LinkStripeAccountsRequest]) (*connect.Response[floatv1.LinkStripeAccountsResponse], error) {
	if len(req.Msg.StripeAccountIds) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("stripe_account_ids is required"))
	}
	api, err := h.stripeClient()
	if err != nil {
		return nil, err
	}

	store, err := stripeconn.Load(h.dataDir)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	out := make([]*floatv1.StripeConnection, 0, len(req.Msg.StripeAccountIds))
	mutated := false
	for _, stripeAcctID := range req.Msg.StripeAccountIds {
		if existing := store.FindByStripeID(stripeAcctID); existing != nil {
			out = append(out, toProtoStripeConnection(*existing))
			continue
		}
		account, fetchErr := api.GetAccount(ctx, stripeAcctID)
		if fetchErr != nil {
			slogctx.FromContext(ctx).ErrorContext(ctx, "fetch stripe account failed", "stripe_account_id", stripeAcctID, "error", fetchErr)
			return nil, connect.NewError(connect.CodeInternal, fetchErr)
		}
		conn := newConnectionFromAccount(account)
		store.Upsert(conn)
		mutated = true
		out = append(out, toProtoStripeConnection(conn))
	}

	if mutated {
		err = h.lock.Do(ctx, "link stripe accounts", func() error {
			return stripeconn.Save(h.dataDir, store)
		})
		if err != nil {
			slogctx.FromContext(ctx).ErrorContext(ctx, "save stripe connections failed", "error", err)
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}

	return connect.NewResponse(&floatv1.LinkStripeAccountsResponse{Connections: out}), nil
}

func (h *Handler) ListStripeConnections(_ context.Context, _ *connect.Request[floatv1.ListStripeConnectionsRequest]) (*connect.Response[floatv1.ListStripeConnectionsResponse], error) {
	store, err := stripeconn.Load(h.dataDir)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	out := make([]*floatv1.StripeConnection, 0, len(store.Connections))
	for _, c := range store.Connections {
		out = append(out, toProtoStripeConnection(c))
	}
	return connect.NewResponse(&floatv1.ListStripeConnectionsResponse{Connections: out}), nil
}

func (h *Handler) UpdateStripeConnection(ctx context.Context, req *connect.Request[floatv1.UpdateStripeConnectionRequest]) (*connect.Response[floatv1.UpdateStripeConnectionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	store, err := stripeconn.Load(h.dataDir)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	conn := store.Find(req.Msg.Id)
	if conn == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("connection %s not found", req.Msg.Id))
	}
	conn.DisplayName = req.Msg.DisplayName
	conn.HledgerAccount = req.Msg.HledgerAccount
	conn.DefaultInflowAccount = req.Msg.DefaultInflowAccount
	conn.DefaultOutflowAccount = req.Msg.DefaultOutflowAccount

	updated := *conn
	err = h.lock.Do(ctx, "update stripe connection", func() error {
		return stripeconn.Save(h.dataDir, store)
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&floatv1.UpdateStripeConnectionResponse{
		Connection: toProtoStripeConnection(updated),
	}), nil
}

func (h *Handler) DeleteStripeConnection(ctx context.Context, req *connect.Request[floatv1.DeleteStripeConnectionRequest]) (*connect.Response[floatv1.DeleteStripeConnectionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	store, err := stripeconn.Load(h.dataDir)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if !store.Delete(req.Msg.Id) {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("connection %s not found", req.Msg.Id))
	}
	err = h.lock.Do(ctx, "delete stripe connection", func() error {
		return stripeconn.Save(h.dataDir, store)
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&floatv1.DeleteStripeConnectionResponse{}), nil
}

func (h *Handler) SyncStripeConnection(ctx context.Context, req *connect.Request[floatv1.SyncStripeConnectionRequest]) (*connect.Response[floatv1.SyncStripeConnectionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	api, err := h.stripeClient()
	if err != nil {
		return nil, err
	}
	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	result, err := stripeconn.Sync(ctx, h.hl, h.lock, h.dataDir, req.Msg.Id, rulesList, api)
	if err != nil {
		slogctx.FromContext(ctx).ErrorContext(ctx, "stripe sync failed", "id", req.Msg.Id, "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	store, err := stripeconn.Load(h.dataDir)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	conn := store.Find(req.Msg.Id)
	if conn == nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("connection %s vanished after sync", req.Msg.Id))
	}
	return connect.NewResponse(&floatv1.SyncStripeConnectionResponse{
		Imported:   int32(result.Imported),
		Skipped:    int32(result.Skipped),
		Connection: toProtoStripeConnection(*conn),
	}), nil
}

// newConnectionFromAccount initialises a Connection from a freshly-fetched
// Stripe FC account. hledger_account and default in/outflow accounts are
// left empty; the user fills them in via UpdateStripeConnection.
func newConnectionFromAccount(a *stripe.FinancialConnectionsAccount) stripeconn.Connection {
	c := stripeconn.Connection{
		ID:                 journal.MintFID(),
		StripeAccountID:    a.ID,
		InstitutionName:    a.InstitutionName,
		Last4:              a.Last4,
		AccountCategory:    string(a.Category),
		AccountSubcategory: string(a.Subcategory),
	}
	if a.Balance != nil {
		// Balance.Current is a map[currency]amount. We use the first
		// currency key we see — Financial Connections accounts are
		// single-currency in practice.
		for code := range a.Balance.Current {
			c.Currency = upper(code)
			break
		}
	}
	c.DisplayName = a.DisplayName
	if c.DisplayName == "" {
		c.DisplayName = fmt.Sprintf("%s · %s", a.InstitutionName, a.Last4)
	}
	return c
}

// toProtoStripeConnection converts a stripeconn.Connection into its proto
// form for transport.
func toProtoStripeConnection(c stripeconn.Connection) *floatv1.StripeConnection {
	out := &floatv1.StripeConnection{
		Id:                    c.ID,
		StripeAccountId:       c.StripeAccountID,
		DisplayName:           c.DisplayName,
		InstitutionName:       c.InstitutionName,
		Last4:                 c.Last4,
		AccountCategory:       c.AccountCategory,
		AccountSubcategory:    c.AccountSubcategory,
		Currency:              c.Currency,
		HledgerAccount:        c.HledgerAccount,
		DefaultInflowAccount:  c.DefaultInflowAccount,
		DefaultOutflowAccount: c.DefaultOutflowAccount,
		ImportedCount:         int32(len(c.ImportedIDs)),
	}
	if !c.LastSyncedAt.IsZero() {
		out.LastSyncedAt = c.LastSyncedAt.Format(rfc3339UTC)
	}
	if !c.CreatedAt.IsZero() {
		out.CreatedAt = c.CreatedAt.Format(rfc3339UTC)
	}
	return out
}

// upper is a small helper to uppercase a possibly-empty string without
// allocating when it's already uppercase.
func upper(s string) string {
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			b := []byte(s)
			for i := range b {
				if b[i] >= 'a' && b[i] <= 'z' {
					b[i] -= 'a' - 'A'
				}
			}
			return string(b)
		}
	}
	return s
}

const rfc3339UTC = "2006-01-02T15:04:05Z"
