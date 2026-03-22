package ledger

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/gen/float/v1/floatv1connect"
	"github.com/brendanv/float/internal/cache"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/slogctx"
	"github.com/brendanv/float/internal/txlock"
)

// Handler implements LedgerService RPCs by delegating to the hledger wrapper.
type Handler struct {
	floatv1connect.UnimplementedLedgerServiceHandler
	hl      *hledger.Client
	lock    *txlock.TxLock
	dataDir string
	cache   *cache.Cache[any] // nil = bypass cache
}

// NewHandler creates a Handler. c may be nil to disable caching (useful in tests).
func NewHandler(hl *hledger.Client, lock *txlock.TxLock, dataDir string, c *cache.Cache[any]) *Handler {
	return &Handler{hl: hl, lock: lock, dataDir: dataDir, cache: c}
}

// cacheKey helpers produce deterministic, namespaced keys from RPC parameters.
// Query args are sorted so that ["b","a"] and ["a","b"] produce the same key.

func transactionsKey(query []string) string {
	sorted := append([]string(nil), query...)
	sort.Strings(sorted)
	return "transactions:" + strings.Join(sorted, "|")
}

func balancesKey(depth int, query []string) string {
	sorted := append([]string(nil), query...)
	sort.Strings(sorted)
	return fmt.Sprintf("balances:%d:%s", depth, strings.Join(sorted, "|"))
}

const accountsKey = "accounts"

// cachedTransactions fetches transactions from cache or hledger.
func cachedTransactions(ctx context.Context, c *cache.Cache[any], hl *hledger.Client, query []string) ([]hledger.Transaction, error) {
	if c == nil {
		return hl.Transactions(ctx, query...)
	}
	val, err := c.Get(ctx, transactionsKey(query), func(ctx context.Context) (any, error) {
		return hl.Transactions(ctx, query...)
	})
	if err != nil {
		return nil, err
	}
	return val.([]hledger.Transaction), nil
}

// cachedBalances fetches balances from cache or hledger.
func cachedBalances(ctx context.Context, c *cache.Cache[any], hl *hledger.Client, depth int, query []string) (*hledger.BalanceReport, error) {
	if c == nil {
		return hl.Balances(ctx, depth, query...)
	}
	val, err := c.Get(ctx, balancesKey(depth, query), func(ctx context.Context) (any, error) {
		return hl.Balances(ctx, depth, query...)
	})
	if err != nil {
		return nil, err
	}
	return val.(*hledger.BalanceReport), nil
}

// cachedAccounts fetches accounts from cache or hledger.
func cachedAccounts(ctx context.Context, c *cache.Cache[any], hl *hledger.Client) ([]*hledger.AccountNode, error) {
	if c == nil {
		return hl.Accounts(ctx, false)
	}
	val, err := c.Get(ctx, accountsKey, func(ctx context.Context) (any, error) {
		return hl.Accounts(ctx, false)
	})
	if err != nil {
		return nil, err
	}
	return val.([]*hledger.AccountNode), nil
}

func (h *Handler) ListTransactions(ctx context.Context, req *connect.Request[floatv1.ListTransactionsRequest]) (*connect.Response[floatv1.ListTransactionsResponse], error) {
	logger := slogctx.FromContext(ctx)
	txns, err := cachedTransactions(ctx, h.cache, h.hl, req.Msg.Query)
	if err != nil {
		logger.ErrorContext(ctx, "hledger transactions failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	proto := make([]*floatv1.Transaction, len(txns))
	for i, t := range txns {
		proto[i] = toProtoTransaction(t)
	}
	return connect.NewResponse(&floatv1.ListTransactionsResponse{Transactions: proto}), nil
}

func (h *Handler) GetBalances(ctx context.Context, req *connect.Request[floatv1.GetBalancesRequest]) (*connect.Response[floatv1.GetBalancesResponse], error) {
	logger := slogctx.FromContext(ctx)
	report, err := cachedBalances(ctx, h.cache, h.hl, int(req.Msg.Depth), req.Msg.Query)
	if err != nil {
		logger.ErrorContext(ctx, "hledger balances failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	rows := make([]*floatv1.BalanceRow, len(report.Rows))
	for i, r := range report.Rows {
		rows[i] = toProtoBalanceRow(r)
	}
	total := make([]*floatv1.Amount, len(report.Total))
	for i, a := range report.Total {
		total[i] = toProtoAmount(a)
	}
	return connect.NewResponse(&floatv1.GetBalancesResponse{
		Report: &floatv1.BalanceReport{Rows: rows, Total: total},
	}), nil
}

func (h *Handler) ListAccounts(ctx context.Context, req *connect.Request[floatv1.ListAccountsRequest]) (*connect.Response[floatv1.ListAccountsResponse], error) {
	logger := slogctx.FromContext(ctx)
	nodes, err := cachedAccounts(ctx, h.cache, h.hl)
	if err != nil {
		logger.ErrorContext(ctx, "hledger accounts failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	accounts := make([]*floatv1.Account, len(nodes))
	for i, n := range nodes {
		accounts[i] = toProtoAccount(n)
	}
	return connect.NewResponse(&floatv1.ListAccountsResponse{Accounts: accounts}), nil
}

func (h *Handler) DeleteTransaction(ctx context.Context, req *connect.Request[floatv1.DeleteTransactionRequest]) (*connect.Response[floatv1.DeleteTransactionResponse], error) {
	logger := slogctx.FromContext(ctx)
	fid := req.Msg.Fid
	if fid == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("fid is required"))
	}
	err := h.lock.Do(ctx, func() error {
		return journal.DeleteTransaction(ctx, h.hl, h.dataDir, fid)
	})
	if err != nil {
		if strings.Contains(err.Error(), "no transaction found") {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		logger.ErrorContext(ctx, "delete transaction failed", "fid", fid, "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&floatv1.DeleteTransactionResponse{}), nil
}

func (h *Handler) ModifyTags(ctx context.Context, req *connect.Request[floatv1.ModifyTagsRequest]) (*connect.Response[floatv1.ModifyTagsResponse], error) {
	logger := slogctx.FromContext(ctx)
	fid := req.Msg.Fid
	if fid == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("fid is required"))
	}
	err := h.lock.Do(ctx, func() error {
		return journal.ModifyTags(ctx, h.hl, h.dataDir, fid, req.Msg.Tags)
	})
	if err != nil {
		if strings.Contains(err.Error(), "no transaction found") {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		logger.ErrorContext(ctx, "modify tags failed", "fid", fid, "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&floatv1.ModifyTagsResponse{}), nil
}

func (h *Handler) UpdateTransactionDate(ctx context.Context, req *connect.Request[floatv1.UpdateTransactionDateRequest]) (*connect.Response[floatv1.UpdateTransactionDateResponse], error) {
	logger := slogctx.FromContext(ctx)
	fid := req.Msg.Fid
	if fid == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("fid is required"))
	}
	if req.Msg.NewDate == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("new_date is required"))
	}
	var updated hledger.Transaction
	err := h.lock.Do(ctx, func() error {
		var e error
		updated, e = journal.UpdateTransactionDate(ctx, h.hl, h.dataDir, fid, req.Msg.NewDate)
		return e
	})
	if err != nil {
		if strings.Contains(err.Error(), "no transaction found") {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		if strings.Contains(err.Error(), "invalid date") {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		logger.ErrorContext(ctx, "update transaction date failed", "fid", fid, "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&floatv1.UpdateTransactionDateResponse{
		Transaction: toProtoTransaction(updated),
	}), nil
}

func (h *Handler) AddTransaction(ctx context.Context, req *connect.Request[floatv1.AddTransactionRequest]) (*connect.Response[floatv1.AddTransactionResponse], error) {
	logger := slogctx.FromContext(ctx)
	if req.Msg.Description == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("description is required"))
	}
	if len(req.Msg.Postings) < 2 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("at least 2 postings are required"))
	}
	for i, p := range req.Msg.Postings {
		if p.Account == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("posting %d: account is required", i))
		}
	}

	var date time.Time
	if req.Msg.Date == "" {
		date = time.Now().UTC().Truncate(24 * time.Hour)
	} else {
		var err error
		date, err = time.Parse("2006-01-02", req.Msg.Date)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid date %q: must be YYYY-MM-DD", req.Msg.Date))
		}
	}

	postings := make([]journal.PostingInput, len(req.Msg.Postings))
	for i, p := range req.Msg.Postings {
		postings[i] = journal.PostingInput{
			Account: p.Account,
			Amount:  p.Amount,
			Comment: p.Comment,
		}
	}
	tx := journal.TransactionInput{
		Date:        date,
		Description: req.Msg.Description,
		Comment:     req.Msg.Comment,
		Postings:    postings,
	}

	var fid string
	err := h.lock.Do(ctx, func() error {
		var e error
		fid, e = journal.AppendTransaction(ctx, h.hl, h.dataDir, tx)
		return e
	})
	if err != nil {
		logger.ErrorContext(ctx, "add transaction failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	txns, err := h.hl.Transactions(ctx, "tag:fid="+fid)
	if err != nil {
		logger.ErrorContext(ctx, "fetch new transaction failed", "fid", fid, "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if len(txns) == 0 {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("transaction %s not found after add", fid))
	}
	return connect.NewResponse(&floatv1.AddTransactionResponse{
		Transaction: toProtoTransaction(txns[0]),
	}), nil
}

func toProtoTransaction(t hledger.Transaction) *floatv1.Transaction {
	postings := make([]*floatv1.Posting, len(t.Postings))
	for i, p := range t.Postings {
		postings[i] = toProtoPosting(p)
	}
	return &floatv1.Transaction{
		Fid:         t.FID,
		Date:        t.Date,
		Description: t.Description,
		Comment:     t.Comment,
		Postings:    postings,
	}
}

func toProtoPosting(p hledger.Posting) *floatv1.Posting {
	amounts := make([]*floatv1.Amount, len(p.Amounts))
	for i, a := range p.Amounts {
		amounts[i] = toProtoAmount(a)
	}
	return &floatv1.Posting{
		Account: p.Account,
		Amounts: amounts,
		Comment: p.Comment,
	}
}

func toProtoAmount(a hledger.Amount) *floatv1.Amount {
	quantity := fmt.Sprintf("%.*f", a.Quantity.DecimalPlaces, a.Quantity.FloatingPoint)
	return &floatv1.Amount{
		Commodity: a.Commodity,
		Quantity:  quantity,
	}
}

func toProtoBalanceRow(r hledger.BalanceRow) *floatv1.BalanceRow {
	amounts := make([]*floatv1.Amount, len(r.Amounts))
	for i, a := range r.Amounts {
		amounts[i] = toProtoAmount(a)
	}
	return &floatv1.BalanceRow{
		DisplayName: r.DisplayName,
		FullName:    r.FullName,
		Indent:      int32(r.Indent),
		Amounts:     amounts,
	}
}

func toProtoAccount(n *hledger.AccountNode) *floatv1.Account {
	return &floatv1.Account{
		Name:     n.Name,
		FullName: n.FullName,
		Type:     string(n.Type),
	}
}
