package ledger

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/rules"
	"github.com/brendanv/float/internal/slogctx"
	stripeClient "github.com/brendanv/float/internal/stripe"
)

func stripeSecretKey() string {
	return os.Getenv("STRIPE_SECRET_KEY")
}

func stripeAccountID() string {
	return os.Getenv("STRIPE_ACCOUNT_ID")
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
	}
	return connect.NewResponse(resp), nil
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
	accountID := stripeAccountID()
	if accountID == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("STRIPE_ACCOUNT_ID is not set"))
	}

	clientSecret, err := stripeClient.CreateFCSession(ctx, secretKey, accountID)
	if err != nil {
		logger.ErrorContext(ctx, "create stripe fc session failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
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
			return nil, connect.NewError(connect.CodeInternal, err)
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
		logger.ErrorContext(ctx, "link stripe accounts failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	out := make([]*floatv1.StripeLinkedAccount, len(h.cfg.Stripe.LinkedAccounts))
	for i, a := range h.cfg.Stripe.LinkedAccounts {
		out[i] = configToProtoLinkedAccount(a)
	}
	return connect.NewResponse(&floatv1.CompleteStripeLinkingResponse{LinkedAccounts: out}), nil
}

func (h *Handler) ListStripeLinkedAccounts(ctx context.Context, _ *connect.Request[floatv1.ListStripeLinkedAccountsRequest]) (*connect.Response[floatv1.ListStripeLinkedAccountsResponse], error) {
	if h.cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	secretKey := stripeSecretKey()
	if secretKey == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("STRIPE_SECRET_KEY is not set"))
	}
	stripeAccounts, err := stripeClient.ListAccounts(ctx, secretKey, stripeAccountID())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	cfgMap := make(map[string]config.StripeLinkedAccount, len(h.cfg.Stripe.LinkedAccounts))
	for _, a := range h.cfg.Stripe.LinkedAccounts {
		cfgMap[a.StripeAccountID] = a
	}
	out := make([]*floatv1.StripeLinkedAccount, 0, len(stripeAccounts))
	for _, sa := range stripeAccounts {
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

	err := h.lock.Do(ctx, fmt.Sprintf("unlink stripe account %s", req.Msg.StripeAccountId), func() error {
		updated := h.cfg.Stripe.LinkedAccounts[:0]
		for _, a := range h.cfg.Stripe.LinkedAccounts {
			if a.StripeAccountID != req.Msg.StripeAccountId {
				updated = append(updated, a)
			}
		}
		h.cfg.Stripe.LinkedAccounts = updated
		return config.Save(h.configPath, h.cfg)
	})
	if err != nil {
		logger.ErrorContext(ctx, "unlink stripe account failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
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

	if err := stripeClient.RefreshTransactions(ctx, secretKey, linked.StripeAccountID); err != nil {
		logger.ErrorContext(ctx, "refresh stripe transactions failed", "account", linked.StripeAccountID, "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var since time.Time
	if linked.LastFetchedAt != "" {
		since, _ = time.Parse(time.RFC3339, linked.LastFetchedAt)
	}

	stripeTxns, err := stripeClient.ListTransactions(ctx, secretKey, linked.StripeAccountID, since)
	if err != nil {
		logger.ErrorContext(ctx, "list stripe transactions failed", "account", linked.StripeAccountID, "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	existing, err := h.hl.Transactions(ctx)
	if err != nil {
		logger.ErrorContext(ctx, "fetch existing transactions failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	fpSet := make(map[string]bool, len(existing))
	for _, t := range existing {
		fpSet[journal.TxnFingerprint(t)] = true
	}

	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		logger.ErrorContext(ctx, "load rules failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	candidates := make([]*floatv1.ImportCandidate, 0, len(stripeTxns))
	for _, st := range stripeTxns {
		ht := stripeTransactionToHledger(st, linked.HledgerAccount)
		candidate := &floatv1.ImportCandidate{
			IsDuplicate: fpSet[journal.TxnFingerprint(ht)],
		}
		if r := rules.Match(rulesList, ht.Description); r != nil {
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
	if len(req.Msg.CandidateIndices) == 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("candidate_indices must not be empty"))
	}

	linked, err := h.findLinkedAccount(req.Msg.StripeAccountId)
	if err != nil {
		return connect.NewError(connect.CodeNotFound, err)
	}

	var since time.Time
	if linked.LastFetchedAt != "" {
		since, _ = time.Parse(time.RFC3339, linked.LastFetchedAt)
	}

	stripeTxns, err := stripeClient.ListTransactions(ctx, secretKey, linked.StripeAccountID, since)
	if err != nil {
		logger.ErrorContext(ctx, "list stripe transactions failed", "account", linked.StripeAccountID, "error", err)
		return connect.NewError(connect.CodeInternal, err)
	}

	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		logger.ErrorContext(ctx, "load rules failed", "error", err)
		return connect.NewError(connect.CodeInternal, err)
	}

	selectedSet := make(map[int32]bool, len(req.Msg.CandidateIndices))
	for _, idx := range req.Msg.CandidateIndices {
		selectedSet[idx] = true
	}

	importBatchID := "stripe-" + stripeAccountSlug(linked.StripeAccountID) + "/" + time.Now().Format("2006-01-02") + "-" + journal.MintFID()
	total := int32(len(req.Msg.CandidateIndices))
	fetchedAt := time.Now().UTC().Format(time.RFC3339)

	var importedFIDs []string
	err = h.lock.Do(ctx, fmt.Sprintf("import %d stripe transactions (batch %s)", len(req.Msg.CandidateIndices), importBatchID), func() error {
		for i, st := range stripeTxns {
			if !selectedSet[int32(i)] {
				continue
			}
			txInput := stripeTransactionToInput(st, linked.HledgerAccount, importBatchID)

			if r := rules.Match(rulesList, st.Description); r != nil {
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

			fid, writeErr := journal.AppendTransaction(ctx, h.hl, h.dataDir, txInput)
			if writeErr != nil {
				return fmt.Errorf("write transaction %d: %w", i, writeErr)
			}
			importedFIDs = append(importedFIDs, fid)

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
		return connect.NewError(connect.CodeInternal, err)
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

func stripeTransactionToHledger(t stripeClient.Transaction, hledgerAccount string) hledger.Transaction {
	amountDecimal := float64(t.AmountCents) / 100.0
	currency := strings.ToUpper(t.Currency)

	qty := hledger.AmountQuantity{FloatingPoint: amountDecimal, DecimalPlaces: 2}
	counterQty := hledger.AmountQuantity{FloatingPoint: -amountDecimal, DecimalPlaces: 2}

	status := ""
	if t.Status == "pending" {
		status = "Pending"
	}

	return hledger.Transaction{
		Date:        t.TransactedAt.Format("2006-01-02"),
		Description: t.Description,
		Status:      status,
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

	status := ""
	if t.Status == "pending" {
		status = "Pending"
	}

	return journal.TransactionInput{
		Date:        t.TransactedAt.UTC().Truncate(24 * time.Hour),
		Description: t.Description,
		Status:      status,
		Tags:        map[string]string{"stripe-txn": t.ID},
		FloatMeta:   map[string]string{"float-import": importBatchID},
		Postings: []journal.PostingInput{
			{Account: hledgerAccount, Commodity: currency, Quantity: amountStr},
			{Account: "expenses:unknown", Commodity: currency, Quantity: counterAmountStr},
		},
	}
}

func stripeAccountSlug(accountID string) string {
	return strings.ToLower(strings.ReplaceAll(accountID, "_", "-"))
}
