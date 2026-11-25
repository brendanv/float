package ledger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/rules"
	"github.com/brendanv/float/internal/slogctx"
	stripeClient "github.com/brendanv/float/internal/stripe"
	"golang.org/x/sync/errgroup"
)

func stripeSecretKey() string {
	return os.Getenv("STRIPE_SECRET_KEY")
}

func (h *Handler) GetStripeConfig(ctx context.Context, _ *connect.Request[floatv1.GetStripeConfigRequest]) (*connect.Response[floatv1.GetStripeConfigResponse], error) {
	cfg := h.loadCfg()
	if cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	secretKey := stripeSecretKey()
	resp := &floatv1.GetStripeConfigResponse{
		Enabled:            secretKey != "",
		PublishableKey:     os.Getenv("STRIPE_PUBLISHABLE_KEY"),
		LinkedAccountCount: int32(len(cfg.Stripe.LinkedAccounts)),
		CustomerId:         cfg.Stripe.CustomerID,
		DailyImportEnabled: cfg.Stripe.DailyImportEnabled,
		LastDailyImportAt:  cfg.Stripe.LastDailyImportAt,
	}
	return connect.NewResponse(resp), nil
}

func (h *Handler) SetStripeDailyImportEnabled(ctx context.Context, req *connect.Request[floatv1.SetStripeDailyImportEnabledRequest]) (*connect.Response[floatv1.SetStripeDailyImportEnabledResponse], error) {
	if h.loadCfg() == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	if err := h.lock.DoConfig(ctx, "set stripe daily import enabled", []string{h.configPath}, func() error {
		cur := h.loadCfg()
		newCfg := *cur
		newCfg.Stripe.DailyImportEnabled = req.Msg.Enabled
		if err := config.Save(h.configPath, &newCfg); err != nil {
			return err
		}
		h.cfg.Store(&newCfg)
		return nil
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save stripe daily import setting: %w", err))
	}
	return connect.NewResponse(&floatv1.SetStripeDailyImportEnabledResponse{}), nil
}

func (h *Handler) SetStripeCustomerId(ctx context.Context, req *connect.Request[floatv1.SetStripeCustomerIdRequest]) (*connect.Response[floatv1.SetStripeCustomerIdResponse], error) {
	if h.loadCfg() == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	newID := strings.TrimSpace(req.Msg.CustomerId)
	if err := h.lock.DoConfig(ctx, "set stripe customer id", []string{h.configPath}, func() error {
		cur := h.loadCfg()
		newCfg := *cur
		newCfg.Stripe.CustomerID = newID
		if err := config.Save(h.configPath, &newCfg); err != nil {
			return err
		}
		h.cfg.Store(&newCfg)
		return nil
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save stripe customer id: %w", err))
	}
	return connect.NewResponse(&floatv1.SetStripeCustomerIdResponse{}), nil
}

func (h *Handler) CreateStripeLinkSession(ctx context.Context, _ *connect.Request[floatv1.CreateStripeLinkSessionRequest]) (*connect.Response[floatv1.CreateStripeLinkSessionResponse], error) {
	logger := slogctx.FromContext(ctx)
	cfg, secretKey, err := h.stripePrereqs()
	if err != nil {
		return nil, err
	}
	customerID := cfg.Stripe.CustomerID
	if customerID == "" {
		newCustomerID, err := stripeClient.CreateCustomer(ctx, secretKey)
		if err != nil {
			logger.ErrorContext(ctx, "create stripe customer failed", "error", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create Stripe customer"))
		}
		if err := h.lock.DoConfig(ctx, "save stripe customer id", []string{h.configPath}, func() error {
			cur := h.loadCfg()
			newCfg := *cur
			newCfg.Stripe.CustomerID = newCustomerID
			if err := config.Save(h.configPath, &newCfg); err != nil {
				return err
			}
			h.cfg.Store(&newCfg)
			return nil
		}); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save stripe customer id: %w", err))
		}
		customerID = newCustomerID
	}

	clientSecret, err := stripeClient.CreateFCSession(ctx, secretKey, customerID)
	if err != nil {
		logger.ErrorContext(ctx, "create stripe fc session failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create Stripe link session"))
	}
	return connect.NewResponse(&floatv1.CreateStripeLinkSessionResponse{ClientSecret: clientSecret}), nil
}

func (h *Handler) CompleteStripeLinking(ctx context.Context, req *connect.Request[floatv1.CompleteStripeLinkingRequest]) (*connect.Response[floatv1.CompleteStripeLinkingResponse], error) {
	logger := slogctx.FromContext(ctx)
	_, secretKey, err := h.stripePrereqs()
	if err != nil {
		return nil, err
	}
	if len(req.Msg.Accounts) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("accounts must not be empty"))
	}

	importFroms := make([]string, len(req.Msg.Accounts))
	for i, a := range req.Msg.Accounts {
		if a.StripeAccountId == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("stripe_account_id is required for each account"))
		}
		if a.HledgerAccount == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("hledger_account is required for each account"))
		}
		importFrom, err := normalizeImportFrom(a.ImportFrom)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		importFroms[i] = importFrom
		if err := stripeClient.SubscribeTransactions(ctx, secretKey, a.StripeAccountId); err != nil {
			logger.ErrorContext(ctx, "subscribe transactions failed", "account", a.StripeAccountId, "error", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to subscribe to transaction updates"))
		}
	}

	err = h.lock.DoConfig(ctx, "link stripe accounts", []string{h.configPath}, func() error {
		cur := h.loadCfg()
		accounts := make([]config.StripeLinkedAccount, len(cur.Stripe.LinkedAccounts))
		copy(accounts, cur.Stripe.LinkedAccounts)
		for ai, a := range req.Msg.Accounts {
			found := false
			for i, existing := range accounts {
				if existing.StripeAccountID == a.StripeAccountId {
					accounts[i].HledgerAccount = a.HledgerAccount
					accounts[i].DisplayName = a.DisplayName
					// An empty cutoff keeps the existing one, so re-mapping an
					// account never silently drops it.
					if importFroms[ai] != "" {
						accounts[i].ImportFrom = importFroms[ai]
					}
					found = true
					break
				}
			}
			if !found {
				accounts = append(accounts, config.StripeLinkedAccount{
					StripeAccountID: a.StripeAccountId,
					HledgerAccount:  a.HledgerAccount,
					DisplayName:     a.DisplayName,
					ImportFrom:      importFroms[ai],
				})
			}
		}
		newCfg := *cur
		newCfg.Stripe.LinkedAccounts = accounts
		if err := config.Save(h.configPath, &newCfg); err != nil {
			return err
		}
		h.cfg.Store(&newCfg)
		return nil
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "link stripe accounts failed")
	}

	cfg := h.loadCfg()
	out := make([]*floatv1.StripeLinkedAccount, len(cfg.Stripe.LinkedAccounts))
	for i, a := range cfg.Stripe.LinkedAccounts {
		out[i] = configToProtoLinkedAccount(a)
	}
	return connect.NewResponse(&floatv1.CompleteStripeLinkingResponse{LinkedAccounts: out}), nil
}

func (h *Handler) ListStripeLinkedAccounts(ctx context.Context, _ *connect.Request[floatv1.ListStripeLinkedAccountsRequest]) (*connect.Response[floatv1.ListStripeLinkedAccountsResponse], error) {
	logger := slogctx.FromContext(ctx)
	cfg, secretKey, err := h.stripePrereqs()
	if err != nil {
		return nil, err
	}
	stripeAccounts, err := stripeClient.ListAccounts(ctx, secretKey, cfg.Stripe.CustomerID)
	if err != nil {
		logger.ErrorContext(ctx, "list stripe accounts failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list Stripe accounts"))
	}
	cfgMap := make(map[string]config.StripeLinkedAccount, len(cfg.Stripe.LinkedAccounts))
	for _, a := range cfg.Stripe.LinkedAccounts {
		cfgMap[a.StripeAccountID] = a
	}
	out := make([]*floatv1.StripeLinkedAccount, 0, len(stripeAccounts))
	for _, sa := range stripeAccounts {
		if sa.Status == "disconnected" {
			continue
		}
		pa := &floatv1.StripeLinkedAccount{
			StripeAccountId: sa.ID,
			DisplayName:     sa.DisplayName,
			InstitutionName: sa.Institution,
		}
		if cfg, ok := cfgMap[sa.ID]; ok {
			pa.HledgerAccount = cfg.HledgerAccount
			pa.LastFetchedAt = cfg.LastFetchedAt
			pa.ImportFrom = cfg.ImportFrom
			if cfg.DisplayName != "" {
				pa.DisplayName = cfg.DisplayName
			}
		}
		out = append(out, pa)
	}
	return connect.NewResponse(&floatv1.ListStripeLinkedAccountsResponse{Accounts: out}), nil
}

func (h *Handler) UnlinkStripeAccount(ctx context.Context, req *connect.Request[floatv1.UnlinkStripeAccountRequest]) (*connect.Response[floatv1.UnlinkStripeAccountResponse], error) {
	logger := slogctx.FromContext(ctx)
	if h.loadCfg() == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	if req.Msg.StripeAccountId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("stripe_account_id is required"))
	}

	secretKey := stripeSecretKey()
	if secretKey == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("STRIPE_SECRET_KEY is not set"))
	}
	if err := stripeClient.DisconnectAccount(ctx, secretKey, req.Msg.StripeAccountId); err != nil {
		logger.ErrorContext(ctx, "stripe disconnect failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to disconnect Stripe account"))
	}

	err := h.lock.DoConfig(ctx, fmt.Sprintf("unlink stripe account %s", req.Msg.StripeAccountId), []string{h.configPath}, func() error {
		cur := h.loadCfg()
		updated := make([]config.StripeLinkedAccount, 0, len(cur.Stripe.LinkedAccounts))
		for _, a := range cur.Stripe.LinkedAccounts {
			if a.StripeAccountID != req.Msg.StripeAccountId {
				updated = append(updated, a)
			}
		}
		newCfg := *cur
		newCfg.Stripe.LinkedAccounts = updated
		if err := config.Save(h.configPath, &newCfg); err != nil {
			return err
		}
		h.cfg.Store(&newCfg)
		return nil
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "unlink stripe account failed")
	}
	return connect.NewResponse(&floatv1.UnlinkStripeAccountResponse{}), nil
}

// SetStripeAccountImportFrom sets or clears a linked account's import cutoff
// date. It only changes float's config; nothing is sent to Stripe.
func (h *Handler) SetStripeAccountImportFrom(ctx context.Context, req *connect.Request[floatv1.SetStripeAccountImportFromRequest]) (*connect.Response[floatv1.SetStripeAccountImportFromResponse], error) {
	if h.loadCfg() == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	if req.Msg.StripeAccountId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("stripe_account_id is required"))
	}
	importFrom, err := normalizeImportFrom(req.Msg.ImportFrom)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	var updated config.StripeLinkedAccount
	errNotLinked := fmt.Errorf("stripe account %q not linked", req.Msg.StripeAccountId)
	err = h.lock.DoConfigPublish(ctx, fmt.Sprintf("set stripe account %s import cutoff", req.Msg.StripeAccountId), []string{h.configPath}, func() (func(), error) {
		cur := h.loadCfg()
		accounts := make([]config.StripeLinkedAccount, len(cur.Stripe.LinkedAccounts))
		copy(accounts, cur.Stripe.LinkedAccounts)
		found := false
		for i := range accounts {
			if accounts[i].StripeAccountID == req.Msg.StripeAccountId {
				accounts[i].ImportFrom = importFrom
				updated = accounts[i]
				found = true
				break
			}
		}
		if !found {
			return nil, errNotLinked
		}
		newCfg := *cur
		newCfg.Stripe.LinkedAccounts = accounts
		return h.saveCfg(&newCfg)
	})
	if errors.Is(err, errNotLinked) {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if err != nil {
		return nil, rpcErr(ctx, err, "set stripe account import cutoff failed")
	}
	return connect.NewResponse(&floatv1.SetStripeAccountImportFromResponse{
		Account: configToProtoLinkedAccount(updated),
	}), nil
}

func (h *Handler) FetchStripeTransactions(ctx context.Context, req *connect.Request[floatv1.FetchStripeTransactionsRequest]) (*connect.Response[floatv1.FetchStripeTransactionsResponse], error) {
	logger := slogctx.FromContext(ctx)
	cfg, secretKey, err := h.stripePrereqs()
	if err != nil {
		return nil, err
	}
	if req.Msg.StripeAccountId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("stripe_account_id is required"))
	}

	linked, err := h.findLinkedAccount(req.Msg.StripeAccountId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	logger.InfoContext(ctx, "fetch stripe transactions", "account", linked.StripeAccountID)
	stripeTxns, err := stripeClient.ListTransactions(ctx, secretKey, linked.StripeAccountID)
	if err != nil {
		logger.ErrorContext(ctx, "list stripe transactions failed", "account", linked.StripeAccountID, "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list Stripe transactions"))
	}

	imported, rulesList, err := h.loadStripeImportState(ctx)
	if err != nil {
		return nil, err
	}
	candidates := stripeCandidates(stripeTxns, linked, imported, rulesList, cfg.Location())
	return connect.NewResponse(&floatv1.FetchStripeTransactionsResponse{Candidates: candidates}), nil
}

func (h *Handler) ImportStripeTransactions(ctx context.Context, req *connect.Request[floatv1.ImportStripeTransactionsRequest], stream *connect.ServerStream[floatv1.ImportTransactionsResponse]) error {
	_, secretKey, err := h.stripePrereqs()
	if err != nil {
		return err
	}
	if req.Msg.StripeAccountId == "" {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("stripe_account_id is required"))
	}
	if len(req.Msg.StripeTransactionIds) == 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("stripe_transaction_ids must not be empty"))
	}

	linked, err := h.findLinkedAccount(req.Msg.StripeAccountId)
	if err != nil {
		return connect.NewError(connect.CodeNotFound, err)
	}

	ids := req.Msg.StripeTransactionIds
	batchID := "stripe-" + stripeAccountSlug(linked.StripeAccountID) + "/" + time.Now().Format("2006-01-02") + "-" + journal.MintFID()
	return h.runStripeImport(ctx, stream, secretKey, stripeImport{
		selections:    []stripeImportSelection{{accountID: linked.StripeAccountID, txnIDs: ids}},
		batchID:       batchID,
		lockMsg:       fmt.Sprintf("import %d stripe transactions (batch %s)", len(ids), batchID),
		afterPreFetch: h.afterImportPreFetch,
	})
}

func (h *Handler) FetchAllStripeTransactions(ctx context.Context, _ *connect.Request[floatv1.FetchAllStripeTransactionsRequest]) (*connect.Response[floatv1.FetchAllStripeTransactionsResponse], error) {
	logger := slogctx.FromContext(ctx)
	cfg, secretKey, err := h.stripePrereqs()
	if err != nil {
		return nil, err
	}

	imported, rulesList, err := h.loadStripeImportState(ctx)
	if err != nil {
		return nil, err
	}

	var eligible []config.StripeLinkedAccount
	for _, linked := range cfg.Stripe.LinkedAccounts {
		if linked.HledgerAccount != "" {
			eligible = append(eligible, linked)
		}
	}

	results := make([][]*floatv1.ImportCandidate, len(eligible))
	g, gctx := errgroup.WithContext(ctx)
	for i, linked := range eligible {
		g.Go(func() error {
			logger.InfoContext(gctx, "fetch all: list stripe transactions", "account", linked.StripeAccountID)
			stripeTxns, err := stripeClient.ListTransactions(gctx, secretKey, linked.StripeAccountID)
			if err != nil {
				logger.ErrorContext(gctx, "list stripe transactions failed", "account", linked.StripeAccountID, "error", err)
				return fmt.Errorf("failed to list transactions for account %s", linked.StripeAccountID)
			}
			results[i] = stripeCandidates(stripeTxns, linked, imported, rulesList, cfg.Location())
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	accountCandidates := make([]*floatv1.AccountCandidates, 0, len(eligible))
	for i, linked := range eligible {
		accountCandidates = append(accountCandidates, &floatv1.AccountCandidates{
			Account:    configToProtoLinkedAccount(linked),
			Candidates: results[i],
		})
	}

	return connect.NewResponse(&floatv1.FetchAllStripeTransactionsResponse{
		AccountCandidates: accountCandidates,
	}), nil
}

func (h *Handler) ImportAllStripeTransactions(ctx context.Context, req *connect.Request[floatv1.ImportAllStripeTransactionsRequest], stream *connect.ServerStream[floatv1.ImportTransactionsResponse]) error {
	_, secretKey, err := h.stripePrereqs()
	if err != nil {
		return err
	}
	if len(req.Msg.Selections) == 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("selections must not be empty"))
	}

	var selections []stripeImportSelection
	for _, sel := range req.Msg.Selections {
		if len(sel.StripeTransactionIds) == 0 {
			continue
		}
		linked, err := h.findLinkedAccount(sel.StripeAccountId)
		if err != nil {
			return connect.NewError(connect.CodeNotFound, err)
		}
		selections = append(selections, stripeImportSelection{accountID: linked.StripeAccountID, txnIDs: sel.StripeTransactionIds})
	}
	if len(selections) == 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("no transactions selected"))
	}

	return h.runStripeImport(ctx, stream, secretKey, stripeImport{
		selections:    selections,
		batchID:       "stripe-all/" + time.Now().Format("2006-01-02") + "-" + journal.MintFID(),
		lockMsg:       fmt.Sprintf("import all stripe transactions (%d selections)", len(req.Msg.Selections)),
		afterPreFetch: h.afterImportAllPreFetch,
	})
}

// stripeImportSelection is the Stripe transactions to import from one linked
// account. accountID is the canonical linked.StripeAccountID.
type stripeImportSelection struct {
	accountID string
	txnIDs    []string
}

// stripeImport describes one ImportStripeTransactions or
// ImportAllStripeTransactions call for runStripeImport.
type stripeImport struct {
	selections []stripeImportSelection // non-empty txnIDs; an account may repeat
	batchID    string
	lockMsg    string
	// afterPreFetch is a test hook run between the pre-fetch and lock
	// acquisition. Nil in production.
	afterPreFetch func()
}

// runStripeImport imports the selected Stripe transactions as one import
// batch, streaming progress and then the result.
//
// Stripe is listed once per account before taking the lock, so the lock never
// waits on the network. Inside the lock the already-imported Stripe IDs are
// re-read, so a concurrent import (e.g. the daily auto-import) landing between
// the pre-fetch and lock acquisition is not written twice, and each account is
// re-resolved, so one unlinked meanwhile fails the import. Every selected
// account's LastFetchedAt is stamped in the same write.
func (h *Handler) runStripeImport(ctx context.Context, stream *connect.ServerStream[floatv1.ImportTransactionsResponse], secretKey string, imp stripeImport) error {
	logger := slogctx.FromContext(ctx)

	var total int32
	txnsByAccount := make(map[string]map[string]stripeClient.Transaction)
	for _, sel := range imp.selections {
		total += int32(len(sel.txnIDs))
		if _, seen := txnsByAccount[sel.accountID]; seen {
			continue
		}
		txns, err := stripeClient.ListTransactions(ctx, secretKey, sel.accountID)
		if err != nil {
			logger.ErrorContext(ctx, "list stripe transactions failed", "account", sel.accountID, "error", err)
			return connect.NewError(connect.CodeInternal, errors.New("failed to list Stripe transactions"))
		}
		byID := make(map[string]stripeClient.Transaction, len(txns))
		for _, st := range txns {
			byID[st.ID] = st
		}
		txnsByAccount[sel.accountID] = byID
	}

	imported, rulesList, err := h.loadStripeImportState(ctx)
	if err != nil {
		return err
	}

	if imp.afterPreFetch != nil {
		imp.afterPreFetch()
	}

	fetchedAt := time.Now().UTC().Format(time.RFC3339)
	var importedFIDs []string
	err = h.lock.DoWithPublish(ctx, imp.lockMsg, []string{h.configPath}, func() (func(), error) {
		if err := h.addImportedStripeTxnIDs(ctx, imported); err != nil {
			return nil, err
		}

		cur := h.loadCfg()
		loc := cur.Location()
		for _, sel := range imp.selections {
			linked, err := h.findLinkedAccount(sel.accountID)
			if err != nil {
				return nil, fmt.Errorf("account %s: %w", sel.accountID, err)
			}
			byID := txnsByAccount[sel.accountID]
			for _, txnID := range sel.txnIDs {
				if imported[txnID] {
					continue
				}
				st, ok := byID[txnID]
				if !ok {
					return nil, fmt.Errorf("stripe transaction %q not found in current fetch results", txnID)
				}
				if !stripeTxnImportable(st, linked, loc) {
					continue
				}
				fid, err := h.appendStripeTxn(ctx, st, linked.HledgerAccount, imp.batchID, loc, rulesList)
				if err != nil {
					return nil, fmt.Errorf("write transaction %s: %w", txnID, err)
				}
				importedFIDs = append(importedFIDs, fid)
				imported[txnID] = true

				if err := stream.Send(&floatv1.ImportTransactionsResponse{
					Payload: &floatv1.ImportTransactionsResponse_Progress{
						Progress: &floatv1.ImportProgress{
							Imported: int32(len(importedFIDs)),
							Total:    total,
						},
					},
				}); err != nil {
					return nil, err
				}
			}
		}

		newCfg := *cur
		newCfg.Stripe.LinkedAccounts = withLastFetchedAt(cur.Stripe.LinkedAccounts, txnsByAccount, fetchedAt)
		return h.saveCfg(&newCfg)
	})
	if err != nil {
		logger.ErrorContext(ctx, "import stripe transactions failed", "batch", imp.batchID, "error", err)
		return connect.NewError(connect.CodeInternal, errors.New("failed to import Stripe transactions"))
	}

	return stream.Send(&floatv1.ImportTransactionsResponse{
		Payload: &floatv1.ImportTransactionsResponse_Result{
			Result: &floatv1.ImportTransactionsResult{
				ImportedCount: int32(len(importedFIDs)),
				Transactions:  h.importedTransactionProtos(ctx, importedFIDs),
				ImportBatchId: imp.batchID,
			},
		},
	})
}

// stripePrereqs returns the loaded config and the Stripe secret key, or a
// FailedPrecondition error if either is missing.
func (h *Handler) stripePrereqs() (*config.Config, string, error) {
	cfg := h.loadCfg()
	if cfg == nil {
		return nil, "", connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	secretKey := stripeSecretKey()
	if secretKey == "" {
		return nil, "", connect.NewError(connect.CodeFailedPrecondition, errors.New("STRIPE_SECRET_KEY is not set"))
	}
	return cfg, secretKey, nil
}

// loadStripeImportState returns the Stripe transaction IDs already in the
// journal and the categorization rules, which every Stripe fetch and import
// needs. Errors are logged and returned as CodeInternal.
func (h *Handler) loadStripeImportState(ctx context.Context) (map[string]bool, []rules.Rule, error) {
	logger := slogctx.FromContext(ctx)
	existing, err := h.hl.Transactions(ctx)
	if err != nil {
		logger.ErrorContext(ctx, "fetch existing transactions failed", "error", err)
		return nil, nil, connect.NewError(connect.CodeInternal, errors.New("failed to fetch existing transactions"))
	}
	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		logger.ErrorContext(ctx, "load rules failed", "error", err)
		return nil, nil, connect.NewError(connect.CodeInternal, errors.New("failed to load rules"))
	}
	return importedStripeTxnIDs(existing), rulesList, nil
}

// addImportedStripeTxnIDs re-reads the journal and adds every imported Stripe
// transaction ID to set. Call it inside the lock before writing, so writes
// that landed after the pre-lock dedup are not imported twice.
func (h *Handler) addImportedStripeTxnIDs(ctx context.Context, set map[string]bool) error {
	existing, err := h.hl.Transactions(ctx)
	if err != nil {
		return fmt.Errorf("refresh existing transactions: %w", err)
	}
	for id := range importedStripeTxnIDs(existing) {
		set[id] = true
	}
	return nil
}

// appendStripeTxn categorizes a Stripe transaction with the first matching
// rule and appends it to the journal. Must be called inside the lock.
func (h *Handler) appendStripeTxn(ctx context.Context, st stripeClient.Transaction, hledgerAccount, batchID string, loc *time.Location, rulesList []rules.Rule) (string, error) {
	in := stripeTransactionToInput(st, hledgerAccount, batchID, loc)
	rules.ApplyToInput(&in, rules.Match(rulesList, st.Description, hledgerAccount))
	return journal.AppendTransaction(ctx, h.hl, h.dataDir, in)
}

// stripeCandidates converts listed Stripe transactions into import candidates
// with rules applied. Only settled transactions on or after the account's
// cutoff are importable; already-imported ones (by Stripe ID) are hidden so
// the UI never shows them.
func stripeCandidates(txns []stripeClient.Transaction, linked config.StripeLinkedAccount, imported map[string]bool, rulesList []rules.Rule, loc *time.Location) []*floatv1.ImportCandidate {
	candidates := make([]*floatv1.ImportCandidate, 0, len(txns))
	for _, st := range txns {
		if !stripeTxnImportable(st, linked, loc) || imported[st.ID] {
			continue
		}
		ht := stripeTransactionToHledger(st, linked.HledgerAccount, loc)
		candidate := &floatv1.ImportCandidate{SourceId: st.ID}
		if r := rules.Match(rulesList, ht.Description, linked.HledgerAccount); r != nil {
			candidate.MatchedRuleId = r.ID
			ht = rules.ApplyToCandidate(ht, r)
		}
		candidate.Transaction = toProtoTransaction(ht)
		candidates = append(candidates, candidate)
	}
	return candidates
}

// withLastFetchedAt returns a copy of accounts with LastFetchedAt set to ts on
// every account whose StripeAccountID is a key of fetched.
func withLastFetchedAt[V any](accounts []config.StripeLinkedAccount, fetched map[string]V, ts string) []config.StripeLinkedAccount {
	out := make([]config.StripeLinkedAccount, len(accounts))
	copy(out, accounts)
	for i, la := range out {
		if _, ok := fetched[la.StripeAccountID]; ok {
			out[i].LastFetchedAt = ts
		}
	}
	return out
}

// RefreshStripeAccount triggers a transaction refresh on Stripe and streams
// polling progress until the refresh terminates. A refresh asks Stripe to sync
// fresh data from the bank; it does not mutate config.
func (h *Handler) RefreshStripeAccount(
	ctx context.Context,
	req *connect.Request[floatv1.RefreshStripeAccountRequest],
	stream *connect.ServerStream[floatv1.RefreshStripeAccountResponse],
) error {
	logger := slogctx.FromContext(ctx)
	_, secretKey, err := h.stripePrereqs()
	if err != nil {
		return err
	}
	if req.Msg.StripeAccountId == "" {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("stripe_account_id is required"))
	}

	linked, err := h.findLinkedAccount(req.Msg.StripeAccountId)
	if err != nil {
		return connect.NewError(connect.CodeNotFound, err)
	}

	return streamRefreshOne(ctx, logger, stream, secretKey, linked.StripeAccountID)
}

// RefreshAllStripeAccounts triggers refresh for every eligible linked account
// (one with a configured HledgerAccount) in parallel, multiplexing progress
// from all accounts onto a single stream. Like RefreshStripeAccount, this does
// NOT mutate config.
func (h *Handler) RefreshAllStripeAccounts(
	ctx context.Context,
	_ *connect.Request[floatv1.RefreshAllStripeAccountsRequest],
	stream *connect.ServerStream[floatv1.RefreshStripeAccountResponse],
) error {
	logger := slogctx.FromContext(ctx)
	cfg, secretKey, err := h.stripePrereqs()
	if err != nil {
		return err
	}

	var eligible []config.StripeLinkedAccount
	for _, la := range cfg.Stripe.LinkedAccounts {
		if la.HledgerAccount != "" {
			eligible = append(eligible, la)
		}
	}
	if len(eligible) == 0 {
		return nil
	}

	logger.InfoContext(ctx, "refresh all stripe accounts: starting", "accounts", len(eligible))

	// Single writer goroutine drains the events channel so concurrent goroutines
	// don't race on stream.Send.
	events := make(chan *floatv1.RefreshStripeAccountResponse, len(eligible)*4)
	sendDone := make(chan error, 1)
	go func() {
		for ev := range events {
			if err := stream.Send(ev); err != nil {
				sendDone <- err
				// Drain remaining events so producers can exit cleanly.
				for range events {
				}
				return
			}
		}
		sendDone <- nil
	}()

	var wg sync.WaitGroup
	for _, la := range eligible {
		la := la
		wg.Add(1)
		go func() {
			defer wg.Done()
			runRefreshOne(ctx, logger, secretKey, la.StripeAccountID, func(ev *floatv1.RefreshStripeAccountResponse) {
				select {
				case events <- ev:
				case <-ctx.Done():
				}
			})
		}()
	}
	wg.Wait()
	close(events)
	return <-sendDone
}

// streamRefreshOne kicks off a refresh and streams events for a single account.
func streamRefreshOne(
	ctx context.Context,
	logger *slog.Logger,
	stream *connect.ServerStream[floatv1.RefreshStripeAccountResponse],
	secretKey, accountID string,
) error {
	var sendErr error
	runRefreshOne(ctx, logger, secretKey, accountID, func(ev *floatv1.RefreshStripeAccountResponse) {
		if sendErr != nil {
			return
		}
		if err := stream.Send(ev); err != nil {
			sendErr = err
		}
	})
	return sendErr
}

// runRefreshOne performs the refresh + poll loop for one account, delivering
// progress and a single terminal result event through emit. It never mutates
// config.
func runRefreshOne(
	ctx context.Context,
	logger *slog.Logger,
	secretKey, accountID string,
	emit func(*floatv1.RefreshStripeAccountResponse),
) {
	logger.InfoContext(ctx, "refresh stripe account: starting", "account", accountID)

	kickoff, err := stripeClient.MaybeRefreshTransactions(ctx, logger, secretKey, accountID)
	if err != nil {
		logger.ErrorContext(ctx, "refresh stripe account: kickoff failed", "account", accountID, "error", err)
		emit(&floatv1.RefreshStripeAccountResponse{
			Payload: &floatv1.RefreshStripeAccountResponse_Result{
				Result: &floatv1.RefreshStripeAccountResult{
					StripeAccountId: accountID,
					Succeeded:       false,
					ErrorMessage:    "failed to start refresh: " + err.Error(),
				},
			},
		})
		return
	}

	if kickoff.Status == stripeClient.RefreshKickoffThrottled {
		nextAt := kickoff.NextRefreshAvailableAt.Unix()
		emit(&floatv1.RefreshStripeAccountResponse{
			Payload: &floatv1.RefreshStripeAccountResponse_Progress{
				Progress: &floatv1.RefreshStripeAccountProgress{
					StripeAccountId: accountID,
					Status:          "throttled",
					RefreshId:       kickoff.CurrentRefreshID,
					Message:         "next refresh available at " + kickoff.NextRefreshAvailableAt.Format(time.RFC3339),
				},
			},
		})
		emit(&floatv1.RefreshStripeAccountResponse{
			Payload: &floatv1.RefreshStripeAccountResponse_Result{
				Result: &floatv1.RefreshStripeAccountResult{
					StripeAccountId:        accountID,
					RefreshId:              kickoff.CurrentRefreshID,
					Succeeded:              true,
					Throttled:              true,
					NextRefreshAvailableAt: nextAt,
				},
			},
		})
		return
	}

	refreshID, err := stripeClient.WaitForRefreshWithProgress(ctx, logger, secretKey, accountID, func(p stripeClient.RefreshProgress) {
		emit(&floatv1.RefreshStripeAccountResponse{
			Payload: &floatv1.RefreshStripeAccountResponse_Progress{
				Progress: &floatv1.RefreshStripeAccountProgress{
					StripeAccountId: accountID,
					Status:          p.Status,
					Attempt:         int32(p.Attempt),
					ElapsedSeconds:  int64(p.Elapsed.Seconds()),
					RefreshId:       p.RefreshID,
					Message:         refreshProgressMessage(p),
				},
			},
		})
	})

	result := &floatv1.RefreshStripeAccountResult{
		StripeAccountId: accountID,
		RefreshId:       refreshID,
		Succeeded:       err == nil,
	}
	if err != nil {
		result.ErrorMessage = err.Error()
	}
	emit(&floatv1.RefreshStripeAccountResponse{
		Payload: &floatv1.RefreshStripeAccountResponse_Result{Result: result},
	})
}

func refreshProgressMessage(p stripeClient.RefreshProgress) string {
	switch p.Status {
	case "starting":
		return "starting refresh"
	case "polling":
		if p.NextInterval > 0 {
			return fmt.Sprintf("polling (next in %ds)", int(p.NextInterval.Seconds()))
		}
		return "polling"
	case "succeeded":
		return "refresh succeeded"
	case "failed":
		if p.Err != nil {
			return "refresh failed: " + p.Err.Error()
		}
		return "refresh failed"
	case "timeout":
		return "refresh timed out"
	case "skipped":
		return "no refresh in progress"
	case "throttled":
		return "refresh throttled"
	}
	return p.Status
}

func (h *Handler) findLinkedAccount(stripeAccountID string) (config.StripeLinkedAccount, error) {
	cfg := h.loadCfg()
	if cfg != nil {
		for _, a := range cfg.Stripe.LinkedAccounts {
			if a.StripeAccountID == stripeAccountID {
				return a, nil
			}
		}
	}
	return config.StripeLinkedAccount{}, fmt.Errorf("stripe account %q not linked", stripeAccountID)
}

func configToProtoLinkedAccount(a config.StripeLinkedAccount) *floatv1.StripeLinkedAccount {
	return &floatv1.StripeLinkedAccount{
		StripeAccountId: a.StripeAccountID,
		HledgerAccount:  a.HledgerAccount,
		DisplayName:     a.DisplayName,
		LastFetchedAt:   a.LastFetchedAt,
		ImportFrom:      a.ImportFrom,
	}
}

// stripeTxnSettled reports whether a Stripe transaction is settled ("posted")
// and therefore eligible for import. Pending and void transactions are ignored.
func stripeTxnSettled(t stripeClient.Transaction) bool {
	return t.Status == "posted"
}

// stripeTxnImportable reports whether a Stripe transaction may be fetched or
// imported for linked: it is settled and not dated (transacted_at in loc)
// before the account's ImportFrom cutoff.
func stripeTxnImportable(t stripeClient.Transaction, linked config.StripeLinkedAccount, loc *time.Location) bool {
	if !stripeTxnSettled(t) {
		return false
	}
	if linked.ImportFrom == "" {
		return true
	}
	return stripeTransactedAtDate(t.TransactedAt, loc).Format(time.DateOnly) >= linked.ImportFrom
}

// normalizeImportFrom validates an import cutoff date and returns it in
// YYYY-MM-DD form; empty input returns "".
func normalizeImportFrom(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return "", fmt.Errorf("import_from %q must be a YYYY-MM-DD date", s)
	}
	return d.Format(time.DateOnly), nil
}

// importedStripeTxnIDs returns the set of Stripe transaction ids already present
// in the journal (recorded via the float-stripe-txn meta tag).
func importedStripeTxnIDs(txns []hledger.Transaction) map[string]bool {
	set := make(map[string]bool, len(txns))
	for _, t := range txns {
		if id, ok := t.FloatMeta["float-stripe-txn"]; ok && id != "" {
			set[id] = true
		}
	}
	return set
}

// stripeTransactedAtDate converts a transacted_at timestamp to a calendar date
// in loc. Using the local timezone avoids off-by-one-day errors for transactions
// that occur in the evening when the user is west of UTC.
func stripeTransactedAtDate(transactedAt time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	local := transactedAt.In(loc)
	y, m, d := local.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func stripeTransactionToHledger(t stripeClient.Transaction, hledgerAccount string, loc *time.Location) hledger.Transaction {
	amountDecimal := float64(t.AmountCents) / 100.0
	currency := strings.ToUpper(t.Currency)

	// Format() renders from DecimalMantissa, so it must be set alongside
	// FloatingPoint or the preview shows 0.00.
	qty := hledger.AmountQuantity{DecimalMantissa: t.AmountCents, DecimalPlaces: 2, FloatingPoint: amountDecimal}
	counterQty := hledger.AmountQuantity{DecimalMantissa: -t.AmountCents, DecimalPlaces: 2, FloatingPoint: -amountDecimal}

	return hledger.Transaction{
		Date:        stripeTransactedAtDate(t.TransactedAt, loc).Format("2006-01-02"),
		Description: t.Description,
		Postings: []hledger.Posting{
			{
				Account: hledgerAccount,
				Amounts: []hledger.Amount{{Commodity: currency, Quantity: qty}},
			},
			{
				Account: "expenses:unknown",
				Amounts: []hledger.Amount{{Commodity: currency, Quantity: counterQty}},
			},
		},
	}
}

func stripeTransactionToInput(t stripeClient.Transaction, hledgerAccount, importBatchID string, loc *time.Location) journal.TransactionInput {
	amountDecimal := float64(t.AmountCents) / 100.0
	currency := strings.ToUpper(t.Currency)
	amountStr := fmt.Sprintf("%.2f", amountDecimal)
	counterAmountStr := fmt.Sprintf("%.2f", -amountDecimal)

	return journal.TransactionInput{
		Date:        stripeTransactedAtDate(t.TransactedAt, loc),
		Description: t.Description,
		FloatMeta: map[string]string{
			"float-import":     importBatchID,
			"float-stripe-txn": t.ID,
		},
		Postings: []journal.PostingInput{
			{Account: hledgerAccount, Commodity: currency, Quantity: amountStr},
			{Account: "expenses:unknown", Commodity: currency, Quantity: counterAmountStr},
		},
	}
}

func stripeAccountSlug(accountID string) string {
	return strings.ToLower(strings.ReplaceAll(accountID, "_", "-"))
}
