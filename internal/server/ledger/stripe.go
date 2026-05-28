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
	if h.cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	secretKey := stripeSecretKey()
	resp := &floatv1.GetStripeConfigResponse{
		Enabled:            secretKey != "",
		PublishableKey:     os.Getenv("STRIPE_PUBLISHABLE_KEY"),
		LinkedAccountCount: int32(len(h.cfg.Stripe.LinkedAccounts)),
		CustomerId:         h.cfg.Stripe.CustomerID,
		DailyImportEnabled: h.cfg.Stripe.DailyImportEnabled,
		LastDailyImportAt:  h.cfg.Stripe.LastDailyImportAt,
	}
	return connect.NewResponse(resp), nil
}

func (h *Handler) SetStripeDailyImportEnabled(ctx context.Context, req *connect.Request[floatv1.SetStripeDailyImportEnabledRequest]) (*connect.Response[floatv1.SetStripeDailyImportEnabledResponse], error) {
	if h.cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	if err := h.lock.Do(ctx, "set stripe daily import enabled", func() error {
		h.cfg.Stripe.DailyImportEnabled = req.Msg.Enabled
		return config.Save(h.configPath, h.cfg)
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save stripe daily import setting: %w", err))
	}
	return connect.NewResponse(&floatv1.SetStripeDailyImportEnabledResponse{}), nil
}

func (h *Handler) SetStripeCustomerId(ctx context.Context, req *connect.Request[floatv1.SetStripeCustomerIdRequest]) (*connect.Response[floatv1.SetStripeCustomerIdResponse], error) {
	if h.cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	newID := strings.TrimSpace(req.Msg.CustomerId)
	if err := h.lock.Do(ctx, "set stripe customer id", func() error {
		h.cfg.Stripe.CustomerID = newID
		return config.Save(h.configPath, h.cfg)
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save stripe customer id: %w", err))
	}
	return connect.NewResponse(&floatv1.SetStripeCustomerIdResponse{}), nil
}

func (h *Handler) CreateStripeLinkSession(ctx context.Context, _ *connect.Request[floatv1.CreateStripeLinkSessionRequest]) (*connect.Response[floatv1.CreateStripeLinkSessionResponse], error) {
	logger := slogctx.FromContext(ctx)
	if h.cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	secretKey := stripeSecretKey()
	if secretKey == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("STRIPE_SECRET_KEY is not set"))
	}
	customerID := h.cfg.Stripe.CustomerID
	if customerID == "" {
		newCustomerID, err := stripeClient.CreateCustomer(ctx, secretKey)
		if err != nil {
			logger.ErrorContext(ctx, "create stripe customer failed", "error", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create Stripe customer"))
		}
		if err := h.lock.Do(ctx, "save stripe customer id", func() error {
			h.cfg.Stripe.CustomerID = newCustomerID
			return config.Save(h.configPath, h.cfg)
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
	if h.cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	secretKey := stripeSecretKey()
	if secretKey == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("STRIPE_SECRET_KEY is not set"))
	}
	if len(req.Msg.Accounts) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("accounts must not be empty"))
	}

	for _, a := range req.Msg.Accounts {
		if a.StripeAccountId == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("stripe_account_id is required for each account"))
		}
		if a.HledgerAccount == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("hledger_account is required for each account"))
		}
		if err := stripeClient.SubscribeTransactions(ctx, secretKey, a.StripeAccountId); err != nil {
			logger.ErrorContext(ctx, "subscribe transactions failed", "account", a.StripeAccountId, "error", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to subscribe to transaction updates"))
		}
	}

	err := h.lock.Do(ctx, "link stripe accounts", func() error {
		for _, a := range req.Msg.Accounts {
			found := false
			for i, existing := range h.cfg.Stripe.LinkedAccounts {
				if existing.StripeAccountID == a.StripeAccountId {
					h.cfg.Stripe.LinkedAccounts[i].HledgerAccount = a.HledgerAccount
					h.cfg.Stripe.LinkedAccounts[i].DisplayName = a.DisplayName
					found = true
					break
				}
			}
			if !found {
				h.cfg.Stripe.LinkedAccounts = append(h.cfg.Stripe.LinkedAccounts, config.StripeLinkedAccount{
					StripeAccountID: a.StripeAccountId,
					HledgerAccount:  a.HledgerAccount,
					DisplayName:     a.DisplayName,
				})
			}
		}
		return config.Save(h.configPath, h.cfg)
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "link stripe accounts failed")
	}

	out := make([]*floatv1.StripeLinkedAccount, len(h.cfg.Stripe.LinkedAccounts))
	for i, a := range h.cfg.Stripe.LinkedAccounts {
		out[i] = configToProtoLinkedAccount(a)
	}
	return connect.NewResponse(&floatv1.CompleteStripeLinkingResponse{LinkedAccounts: out}), nil
}

func (h *Handler) ListStripeLinkedAccounts(ctx context.Context, _ *connect.Request[floatv1.ListStripeLinkedAccountsRequest]) (*connect.Response[floatv1.ListStripeLinkedAccountsResponse], error) {
	logger := slogctx.FromContext(ctx)
	if h.cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	secretKey := stripeSecretKey()
	if secretKey == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("STRIPE_SECRET_KEY is not set"))
	}
	stripeAccounts, err := stripeClient.ListAccounts(ctx, secretKey, h.cfg.Stripe.CustomerID)
	if err != nil {
		logger.ErrorContext(ctx, "list stripe accounts failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list Stripe accounts"))
	}
	cfgMap := make(map[string]config.StripeLinkedAccount, len(h.cfg.Stripe.LinkedAccounts))
	for _, a := range h.cfg.Stripe.LinkedAccounts {
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
	if h.cfg == nil {
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

	err := h.lock.Do(ctx, fmt.Sprintf("unlink stripe account %s", req.Msg.StripeAccountId), func() error {
		updated := make([]config.StripeLinkedAccount, 0, len(h.cfg.Stripe.LinkedAccounts))
		for _, a := range h.cfg.Stripe.LinkedAccounts {
			if a.StripeAccountID != req.Msg.StripeAccountId {
				updated = append(updated, a)
			}
		}
		h.cfg.Stripe.LinkedAccounts = updated
		return config.Save(h.configPath, h.cfg)
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "unlink stripe account failed")
	}
	return connect.NewResponse(&floatv1.UnlinkStripeAccountResponse{}), nil
}

func (h *Handler) FetchStripeTransactions(ctx context.Context, req *connect.Request[floatv1.FetchStripeTransactionsRequest]) (*connect.Response[floatv1.FetchStripeTransactionsResponse], error) {
	logger := slogctx.FromContext(ctx)
	if h.cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	secretKey := stripeSecretKey()
	if secretKey == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("STRIPE_SECRET_KEY is not set"))
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

	existing, err := h.hl.Transactions(ctx)
	if err != nil {
		logger.ErrorContext(ctx, "fetch existing transactions failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to fetch existing transactions"))
	}
	stripeTxnSet := importedStripeTxnIDs(existing)

	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		logger.ErrorContext(ctx, "load rules failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load rules"))
	}

	candidates := make([]*floatv1.ImportCandidate, 0, len(stripeTxns))
	for _, st := range stripeTxns {
		// Only settled transactions are importable; duplicates (by Stripe txn id) are
		// hidden so the UI never shows them.
		if !stripeTxnSettled(st) || stripeTxnSet[st.ID] {
			continue
		}
		ht := stripeTransactionToHledger(st, linked.HledgerAccount)
		candidate := &floatv1.ImportCandidate{
			SourceId: st.ID,
		}
		if r := rules.Match(rulesList, ht.Description, linked.HledgerAccount); r != nil {
			candidate.MatchedRuleId = r.ID
			if r.Payee != "" {
				ht.Description = r.Payee + " | " + ht.Description
			}
			if r.Account != "" && len(ht.Postings) == 2 {
				for j, p := range ht.Postings {
					if !isAssetOrLiabilityAccount(p.Account) {
						ht.Postings[j].Account = r.Account
					}
				}
			}
		}
		candidate.Transaction = toProtoTransaction(ht)
		candidates = append(candidates, candidate)
	}
	return connect.NewResponse(&floatv1.FetchStripeTransactionsResponse{Candidates: candidates}), nil
}

func (h *Handler) ImportStripeTransactions(ctx context.Context, req *connect.Request[floatv1.ImportStripeTransactionsRequest], stream *connect.ServerStream[floatv1.ImportTransactionsResponse]) error {
	logger := slogctx.FromContext(ctx)
	if h.cfg == nil {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	secretKey := stripeSecretKey()
	if secretKey == "" {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("STRIPE_SECRET_KEY is not set"))
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

	stripeTxns, err := stripeClient.ListTransactions(ctx, secretKey, linked.StripeAccountID)
	if err != nil {
		logger.ErrorContext(ctx, "list stripe transactions failed", "account", linked.StripeAccountID, "error", err)
		return connect.NewError(connect.CodeInternal, errors.New("failed to list Stripe transactions"))
	}

	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		logger.ErrorContext(ctx, "load rules failed", "error", err)
		return connect.NewError(connect.CodeInternal, errors.New("failed to load rules"))
	}

	txnByID := make(map[string]stripeClient.Transaction, len(stripeTxns))
	for _, st := range stripeTxns {
		txnByID[st.ID] = st
	}

	existing, err := h.hl.Transactions(ctx)
	if err != nil {
		logger.ErrorContext(ctx, "fetch existing transactions failed", "error", err)
		return connect.NewError(connect.CodeInternal, errors.New("failed to fetch existing transactions"))
	}
	importedStripeTxnSet := importedStripeTxnIDs(existing)

	if h.afterImportPreFetch != nil {
		h.afterImportPreFetch()
	}

	selectedIDs := req.Msg.StripeTransactionIds
	importBatchID := "stripe-" + stripeAccountSlug(linked.StripeAccountID) + "/" + time.Now().Format("2006-01-02") + "-" + journal.MintFID()
	total := int32(len(selectedIDs))
	fetchedAt := time.Now().UTC().Format(time.RFC3339)

	var importedFIDs []string
	err = h.lock.Do(ctx, fmt.Sprintf("import %d stripe transactions (batch %s)", len(selectedIDs), importBatchID), func() error {
		// Refresh importedStripeTxnSet inside the lock so that transactions written by a
		// concurrent operation (e.g. daily auto-import) between our pre-fetch and lock
		// acquisition are not written a second time.
		freshExisting, freshErr := h.hl.Transactions(ctx)
		if freshErr != nil {
			return fmt.Errorf("refresh existing transactions: %w", freshErr)
		}
		for id := range importedStripeTxnIDs(freshExisting) {
			importedStripeTxnSet[id] = true
		}

		for _, txnID := range selectedIDs {
			if importedStripeTxnSet[txnID] {
				continue
			}
			st, ok := txnByID[txnID]
			if !ok {
				return fmt.Errorf("stripe transaction %q not found in current fetch results", txnID)
			}
			if !stripeTxnSettled(st) {
				continue
			}
			txInput := stripeTransactionToInput(st, linked.HledgerAccount, importBatchID)

			applyRuleToInput(&txInput, rules.Match(rulesList, st.Description, linked.HledgerAccount))

			fid, writeErr := journal.AppendTransaction(ctx, h.hl, h.dataDir, txInput)
			if writeErr != nil {
				return fmt.Errorf("write transaction %s: %w", txnID, writeErr)
			}
			importedFIDs = append(importedFIDs, fid)
			importedStripeTxnSet[txnID] = true

			if sendErr := stream.Send(&floatv1.ImportTransactionsResponse{
				Payload: &floatv1.ImportTransactionsResponse_Progress{
					Progress: &floatv1.ImportProgress{
						Imported: int32(len(importedFIDs)),
						Total:    total,
					},
				},
			}); sendErr != nil {
				return sendErr
			}
		}

		for i, la := range h.cfg.Stripe.LinkedAccounts {
			if la.StripeAccountID == linked.StripeAccountID {
				h.cfg.Stripe.LinkedAccounts[i].LastFetchedAt = fetchedAt
				break
			}
		}
		return config.Save(h.configPath, h.cfg)
	})
	if err != nil {
		logger.ErrorContext(ctx, "import stripe transactions failed", "error", err)
		return connect.NewError(connect.CodeInternal, errors.New("failed to import Stripe transactions"))
	}

	var txnProtos []*floatv1.Transaction
	for _, fid := range importedFIDs {
		txns, fetchErr := h.hl.Transactions(ctx, "code:"+fid)
		if fetchErr != nil || len(txns) == 0 {
			continue
		}
		txnProtos = append(txnProtos, toProtoTransaction(txns[0]))
	}

	return stream.Send(&floatv1.ImportTransactionsResponse{
		Payload: &floatv1.ImportTransactionsResponse_Result{
			Result: &floatv1.ImportTransactionsResult{
				ImportedCount: int32(len(importedFIDs)),
				Transactions:  txnProtos,
				ImportBatchId: importBatchID,
			},
		},
	})
}

func (h *Handler) FetchAllStripeTransactions(ctx context.Context, _ *connect.Request[floatv1.FetchAllStripeTransactionsRequest]) (*connect.Response[floatv1.FetchAllStripeTransactionsResponse], error) {
	logger := slogctx.FromContext(ctx)
	if h.cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	secretKey := stripeSecretKey()
	if secretKey == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("STRIPE_SECRET_KEY is not set"))
	}

	existing, err := h.hl.Transactions(ctx)
	if err != nil {
		logger.ErrorContext(ctx, "fetch existing transactions failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to fetch existing transactions"))
	}
	stripeTxnSet := importedStripeTxnIDs(existing)

	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		logger.ErrorContext(ctx, "load rules failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load rules"))
	}

	var eligible []config.StripeLinkedAccount
	for _, linked := range h.cfg.Stripe.LinkedAccounts {
		if linked.HledgerAccount != "" {
			eligible = append(eligible, linked)
		}
	}

	type accountResult struct {
		candidates []*floatv1.ImportCandidate
	}
	results := make([]accountResult, len(eligible))

	g, gctx := errgroup.WithContext(ctx)
	for i, linked := range eligible {
		i, linked := i, linked
		g.Go(func() error {
			logger.InfoContext(gctx, "fetch all: list stripe transactions", "account", linked.StripeAccountID)
			stripeTxns, err := stripeClient.ListTransactions(gctx, secretKey, linked.StripeAccountID)
			if err != nil {
				logger.ErrorContext(gctx, "list stripe transactions failed", "account", linked.StripeAccountID, "error", err)
				return fmt.Errorf("failed to list transactions for account %s", linked.StripeAccountID)
			}

			candidates := make([]*floatv1.ImportCandidate, 0, len(stripeTxns))
			for _, st := range stripeTxns {
				// Only settled transactions are importable; duplicates (by Stripe txn id)
				// are hidden so the UI never shows them.
				if !stripeTxnSettled(st) || stripeTxnSet[st.ID] {
					continue
				}
				ht := stripeTransactionToHledger(st, linked.HledgerAccount)
				candidate := &floatv1.ImportCandidate{SourceId: st.ID}
				if r := rules.Match(rulesList, ht.Description, linked.HledgerAccount); r != nil {
					candidate.MatchedRuleId = r.ID
					if r.Payee != "" {
						ht.Description = r.Payee + " | " + ht.Description
					}
					if r.Account != "" && len(ht.Postings) == 2 {
						for j, p := range ht.Postings {
							if !isAssetOrLiabilityAccount(p.Account) {
								ht.Postings[j].Account = r.Account
							}
						}
					}
				}
				candidate.Transaction = toProtoTransaction(ht)
				candidates = append(candidates, candidate)
			}

			results[i].candidates = candidates
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
			Candidates: results[i].candidates,
		})
	}

	return connect.NewResponse(&floatv1.FetchAllStripeTransactionsResponse{
		AccountCandidates: accountCandidates,
	}), nil
}

func (h *Handler) ImportAllStripeTransactions(ctx context.Context, req *connect.Request[floatv1.ImportAllStripeTransactionsRequest], stream *connect.ServerStream[floatv1.ImportTransactionsResponse]) error {
	logger := slogctx.FromContext(ctx)
	if h.cfg == nil {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	secretKey := stripeSecretKey()
	if secretKey == "" {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("STRIPE_SECRET_KEY is not set"))
	}
	if len(req.Msg.Selections) == 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("selections must not be empty"))
	}

	total := int32(0)
	for _, sel := range req.Msg.Selections {
		total += int32(len(sel.StripeTransactionIds))
	}
	if total == 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("no transactions selected"))
	}

	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		logger.ErrorContext(ctx, "load rules failed", "error", err)
		return connect.NewError(connect.CodeInternal, errors.New("failed to load rules"))
	}

	existing, err := h.hl.Transactions(ctx)
	if err != nil {
		logger.ErrorContext(ctx, "fetch existing transactions failed", "error", err)
		return connect.NewError(connect.CodeInternal, errors.New("failed to fetch existing transactions"))
	}
	importedStripeTxnSet := importedStripeTxnIDs(existing)

	// Phase 1: pre-fetch all Stripe API data outside the lock so the lock holds no
	// network calls. The map is keyed by the canonical linked.StripeAccountID — never
	// the raw sel.StripeAccountId — so any future tolerance in findLinkedAccount
	// matching can't cause the prefetch and lock phases to disagree on which fetch
	// belongs to which selection.
	type acctFetch struct {
		txns    []stripeClient.Transaction
		txnByID map[string]stripeClient.Transaction
		err     error
	}
	acctFetches := make(map[string]*acctFetch)
	for _, sel := range req.Msg.Selections {
		if len(sel.StripeTransactionIds) == 0 {
			continue
		}
		linked, findErr := h.findLinkedAccount(sel.StripeAccountId)
		if findErr != nil {
			// Defer to the lock phase, which re-resolves and surfaces a NotFound-style error.
			continue
		}
		if _, seen := acctFetches[linked.StripeAccountID]; seen {
			continue
		}
		af := &acctFetch{}
		txns, listErr := stripeClient.ListTransactions(ctx, secretKey, linked.StripeAccountID)
		if listErr != nil {
			af.err = fmt.Errorf("list transactions for %s: %w", linked.StripeAccountID, listErr)
			acctFetches[linked.StripeAccountID] = af
			continue
		}
		af.txns = txns
		af.txnByID = make(map[string]stripeClient.Transaction, len(txns))
		for _, st := range txns {
			af.txnByID[st.ID] = st
		}
		acctFetches[linked.StripeAccountID] = af
	}

	fetchedAt := time.Now().UTC().Format(time.RFC3339)
	var importedFIDs []string
	importedAccounts := make(map[string]bool)

	if h.afterImportAllPreFetch != nil {
		h.afterImportAllPreFetch()
	}

	err = h.lock.Do(ctx, fmt.Sprintf("import all stripe transactions (%d selections)", len(req.Msg.Selections)), func() error {
		// Refresh importedStripeTxnSet inside the lock so that transactions written by a
		// concurrent operation (e.g. daily auto-import) between our pre-fetch and lock
		// acquisition are not written a second time.
		freshExisting, freshErr := h.hl.Transactions(ctx)
		if freshErr != nil {
			return fmt.Errorf("refresh existing transactions: %w", freshErr)
		}
		for id := range importedStripeTxnIDs(freshExisting) {
			importedStripeTxnSet[id] = true
		}

		for _, sel := range req.Msg.Selections {
			if len(sel.StripeTransactionIds) == 0 {
				continue
			}

			linked, findErr := h.findLinkedAccount(sel.StripeAccountId)
			if findErr != nil {
				return fmt.Errorf("account %s: %w", sel.StripeAccountId, findErr)
			}

			af := acctFetches[linked.StripeAccountID]
			if af == nil || af.err != nil {
				var errMsg error
				if af != nil {
					errMsg = af.err
				} else {
					errMsg = fmt.Errorf("no pre-fetched data for account %s", linked.StripeAccountID)
				}
				return errMsg
			}

			importBatchID := "stripe-" + stripeAccountSlug(linked.StripeAccountID) + "/" + time.Now().Format("2006-01-02") + "-" + journal.MintFID()

			for _, txnID := range sel.StripeTransactionIds {
				if importedStripeTxnSet[txnID] {
					continue
				}
				st, ok := af.txnByID[txnID]
				if !ok {
					return fmt.Errorf("stripe transaction %q not found in current fetch results", txnID)
				}
				if !stripeTxnSettled(st) {
					continue
				}
				txInput := stripeTransactionToInput(st, linked.HledgerAccount, importBatchID)

				applyRuleToInput(&txInput, rules.Match(rulesList, st.Description, linked.HledgerAccount))

				fid, writeErr := journal.AppendTransaction(ctx, h.hl, h.dataDir, txInput)
				if writeErr != nil {
					return fmt.Errorf("write transaction %s: %w", txnID, writeErr)
				}
				importedFIDs = append(importedFIDs, fid)
				importedStripeTxnSet[txnID] = true
				importedAccounts[linked.StripeAccountID] = true

				if sendErr := stream.Send(&floatv1.ImportTransactionsResponse{
					Payload: &floatv1.ImportTransactionsResponse_Progress{
						Progress: &floatv1.ImportProgress{
							Imported: int32(len(importedFIDs)),
							Total:    total,
						},
					},
				}); sendErr != nil {
					return sendErr
				}
			}
		}

		for i, la := range h.cfg.Stripe.LinkedAccounts {
			if importedAccounts[la.StripeAccountID] {
				h.cfg.Stripe.LinkedAccounts[i].LastFetchedAt = fetchedAt
			}
		}
		return config.Save(h.configPath, h.cfg)
	})
	if err != nil {
		logger.ErrorContext(ctx, "import all stripe transactions failed", "error", err)
		return connect.NewError(connect.CodeInternal, errors.New("failed to import Stripe transactions"))
	}

	var txnProtos []*floatv1.Transaction
	for _, fid := range importedFIDs {
		txns, fetchErr := h.hl.Transactions(ctx, "code:"+fid)
		if fetchErr != nil || len(txns) == 0 {
			continue
		}
		txnProtos = append(txnProtos, toProtoTransaction(txns[0]))
	}

	return stream.Send(&floatv1.ImportTransactionsResponse{
		Payload: &floatv1.ImportTransactionsResponse_Result{
			Result: &floatv1.ImportTransactionsResult{
				ImportedCount: int32(len(importedFIDs)),
				Transactions:  txnProtos,
			},
		},
	})
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
	if h.cfg == nil {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	secretKey := stripeSecretKey()
	if secretKey == "" {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("STRIPE_SECRET_KEY is not set"))
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
	if h.cfg == nil {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	secretKey := stripeSecretKey()
	if secretKey == "" {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("STRIPE_SECRET_KEY is not set"))
	}

	var eligible []config.StripeLinkedAccount
	for _, la := range h.cfg.Stripe.LinkedAccounts {
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
	for _, a := range h.cfg.Stripe.LinkedAccounts {
		if a.StripeAccountID == stripeAccountID {
			return a, nil
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
	}
}

// stripeTxnSettled reports whether a Stripe transaction is settled ("posted")
// and therefore eligible for import. Pending and void transactions are ignored.
func stripeTxnSettled(t stripeClient.Transaction) bool {
	return t.Status == "posted"
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

func stripeTransactionToHledger(t stripeClient.Transaction, hledgerAccount string) hledger.Transaction {
	amountDecimal := float64(t.AmountCents) / 100.0
	currency := strings.ToUpper(t.Currency)

	qty := hledger.AmountQuantity{FloatingPoint: amountDecimal, DecimalPlaces: 2}
	counterQty := hledger.AmountQuantity{FloatingPoint: -amountDecimal, DecimalPlaces: 2}

	return hledger.Transaction{
		Date:        t.TransactedAt.Format("2006-01-02"),
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

func stripeTransactionToInput(t stripeClient.Transaction, hledgerAccount, importBatchID string) journal.TransactionInput {
	amountDecimal := float64(t.AmountCents) / 100.0
	currency := strings.ToUpper(t.Currency)
	amountStr := fmt.Sprintf("%.2f", amountDecimal)
	counterAmountStr := fmt.Sprintf("%.2f", -amountDecimal)

	return journal.TransactionInput{
		Date:        t.TransactedAt.UTC().Truncate(24 * time.Hour),
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

func applyRuleToInput(txInput *journal.TransactionInput, r *rules.Rule) {
	if r == nil {
		return
	}
	if r.Payee != "" {
		txInput.Description = r.Payee + " | " + txInput.Description
	}
	if r.Account != "" && len(txInput.Postings) == 2 {
		for j, p := range txInput.Postings {
			if !isAssetOrLiabilityAccount(p.Account) {
				txInput.Postings[j].Account = r.Account
			}
		}
	}
	if len(r.Tags) > 0 {
		if txInput.Tags == nil {
			txInput.Tags = make(map[string]string)
		}
		for k, v := range r.Tags {
			txInput.Tags[k] = v
		}
	}
	if r.AutoReviewed {
		txInput.Status = "Cleared"
	}
}
