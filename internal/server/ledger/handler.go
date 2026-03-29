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
const tagsKey = "tags"

func netWorthKey(begin, end string) string {
	return fmt.Sprintf("networth:%s:%s", begin, end)
}

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

// cachedNetWorth fetches a balance sheet timeseries from cache or hledger.
func cachedNetWorth(ctx context.Context, c *cache.Cache[any], hl *hledger.Client, begin, end string) (*hledger.BalanceSheetTimeseries, error) {
	if c == nil {
		return hl.BalanceSheetTimeseries(ctx, begin, end)
	}
	val, err := c.Get(ctx, netWorthKey(begin, end), func(ctx context.Context) (any, error) {
		return hl.BalanceSheetTimeseries(ctx, begin, end)
	})
	if err != nil {
		return nil, err
	}
	return val.(*hledger.BalanceSheetTimeseries), nil
}

// cachedTags fetches tag names from cache or hledger.
func cachedTags(ctx context.Context, c *cache.Cache[any], hl *hledger.Client) ([]string, error) {
	if c == nil {
		return hl.Tags(ctx)
	}
	val, err := c.Get(ctx, tagsKey, func(ctx context.Context) (any, error) {
		return hl.Tags(ctx)
	})
	if err != nil {
		return nil, err
	}
	return val.([]string), nil
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
	total := int32(len(txns))
	if req.Msg.Offset > 0 {
		if int(req.Msg.Offset) >= len(txns) {
			txns = nil
		} else {
			txns = txns[req.Msg.Offset:]
		}
	}
	if req.Msg.Limit > 0 && int(req.Msg.Limit) < len(txns) {
		txns = txns[:req.Msg.Limit]
	}
	proto := make([]*floatv1.Transaction, len(txns))
	for i, t := range txns {
		proto[i] = toProtoTransaction(t)
	}
	return connect.NewResponse(&floatv1.ListTransactionsResponse{Transactions: proto, Total: total}), nil
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

func (h *Handler) GetNetWorthTimeseries(ctx context.Context, req *connect.Request[floatv1.GetNetWorthTimeseriesRequest]) (*connect.Response[floatv1.GetNetWorthTimeseriesResponse], error) {
	logger := slogctx.FromContext(ctx)
	ts, err := cachedNetWorth(ctx, h.cache, h.hl, req.Msg.Begin, req.Msg.End)
	if err != nil {
		logger.ErrorContext(ctx, "hledger balance sheet timeseries failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	snapshots := make([]*floatv1.NetWorthSnapshot, len(ts.Periods))
	for i, date := range ts.Periods {
		snap := &floatv1.NetWorthSnapshot{Date: date}
		for _, sub := range ts.Subreports {
			switch sub.Name {
			case "Assets":
				snap.Assets = toProtoAmounts(sub.Totals[i])
			case "Liabilities":
				snap.Liabilities = toProtoAmounts(sub.Totals[i])
			}
		}
		snap.NetWorth = toProtoAmounts(ts.NetWorth[i])
		snapshots[i] = snap
	}
	return connect.NewResponse(&floatv1.GetNetWorthTimeseriesResponse{Snapshots: snapshots}), nil
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

func (h *Handler) ListTags(ctx context.Context, req *connect.Request[floatv1.ListTagsRequest]) (*connect.Response[floatv1.ListTagsResponse], error) {
	logger := slogctx.FromContext(ctx)
	tags, err := cachedTags(ctx, h.cache, h.hl)
	if err != nil {
		logger.ErrorContext(ctx, "hledger tags failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&floatv1.ListTagsResponse{Tags: tags}), nil
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
		Status:      "Pending",
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

func (h *Handler) UpdateTransaction(ctx context.Context, req *connect.Request[floatv1.UpdateTransactionRequest]) (*connect.Response[floatv1.UpdateTransactionResponse], error) {
	logger := slogctx.FromContext(ctx)
	fid := req.Msg.Fid
	if fid == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("fid is required"))
	}
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

	postings := make([]journal.PostingInput, len(req.Msg.Postings))
	for i, p := range req.Msg.Postings {
		postings[i] = journal.PostingInput{
			Account: p.Account,
			Amount:  p.Amount,
			Comment: p.Comment,
		}
	}

	var updated hledger.Transaction
	err := h.lock.Do(ctx, func() error {
		var e error
		updated, e = journal.UpdateTransaction(ctx, h.hl, h.dataDir, fid, req.Msg.Description, req.Msg.Date, req.Msg.Comment, postings)
		return e
	})
	if err != nil {
		if strings.Contains(err.Error(), "no transaction found") {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		if strings.Contains(err.Error(), "invalid date") {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		logger.ErrorContext(ctx, "update transaction failed", "fid", fid, "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&floatv1.UpdateTransactionResponse{
		Transaction: toProtoTransaction(updated),
	}), nil
}

func (h *Handler) UpdateTransactionStatus(ctx context.Context, req *connect.Request[floatv1.UpdateTransactionStatusRequest]) (*connect.Response[floatv1.UpdateTransactionStatusResponse], error) {
	logger := slogctx.FromContext(ctx)
	fid := req.Msg.Fid
	if fid == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("fid is required"))
	}
	switch req.Msg.Status {
	case "", "Pending", "Cleared":
		// valid
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid status %q: must be \"\", \"Pending\", or \"Cleared\"", req.Msg.Status))
	}
	err := h.lock.Do(ctx, func() error {
		return journal.UpdateTransactionStatus(ctx, h.hl, h.dataDir, fid, req.Msg.Status)
	})
	if err != nil {
		if strings.Contains(err.Error(), "no transaction found") {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		logger.ErrorContext(ctx, "update transaction status failed", "fid", fid, "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	txns, err := h.hl.Transactions(ctx, "tag:fid="+fid)
	if err != nil {
		logger.ErrorContext(ctx, "fetch transaction after status update failed", "fid", fid, "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if len(txns) == 0 {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("transaction %s not found after status update", fid))
	}
	return connect.NewResponse(&floatv1.UpdateTransactionStatusResponse{
		Transaction: toProtoTransaction(txns[0]),
	}), nil
}

func toProtoTransaction(t hledger.Transaction) *floatv1.Transaction {
	postings := make([]*floatv1.Posting, len(t.Postings))
	for i, p := range t.Postings {
		postings[i] = toProtoPosting(p)
	}
	// Normalize hledger's "Unmarked" to "" for consistency with the proto contract.
	status := t.Status
	if status == "Unmarked" {
		status = ""
	}
	return &floatv1.Transaction{
		Fid:         t.FID,
		Date:        t.Date,
		Description: t.Description,
		Comment:     t.Comment,
		Postings:    postings,
		Status:      status,
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

func toProtoAmounts(amounts []hledger.Amount) []*floatv1.Amount {
	result := make([]*floatv1.Amount, len(amounts))
	for i, a := range amounts {
		result[i] = toProtoAmount(a)
	}
	return result
}

func toProtoPriceDirective(p journal.Price) *floatv1.PriceDirective {
	return &floatv1.PriceDirective{
		Pid:       p.PID,
		Date:      p.Date,
		Commodity: p.Commodity,
		Price: &floatv1.Amount{
			Commodity: p.Currency,
			Quantity:  p.Quantity,
		},
	}
}

func (h *Handler) ListPrices(ctx context.Context, _ *connect.Request[floatv1.ListPricesRequest]) (*connect.Response[floatv1.ListPricesResponse], error) {
	prices, err := journal.ListPrices(h.dataDir)
	if err != nil {
		slogctx.FromContext(ctx).ErrorContext(ctx, "list prices failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	out := make([]*floatv1.PriceDirective, len(prices))
	for i, p := range prices {
		out[i] = toProtoPriceDirective(p)
	}
	return connect.NewResponse(&floatv1.ListPricesResponse{Prices: out}), nil
}

func (h *Handler) AddPrice(ctx context.Context, req *connect.Request[floatv1.AddPriceRequest]) (*connect.Response[floatv1.AddPriceResponse], error) {
	logger := slogctx.FromContext(ctx)
	if req.Msg.Commodity == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("commodity is required"))
	}
	if req.Msg.Quantity == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("quantity is required"))
	}
	if req.Msg.Currency == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("currency is required"))
	}
	date := req.Msg.Date
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	var pid string
	err := h.lock.Do(ctx, func() error {
		var e error
		pid, e = journal.AppendPrice(h.dataDir, date, req.Msg.Commodity, req.Msg.Quantity, req.Msg.Currency)
		return e
	})
	if err != nil {
		logger.ErrorContext(ctx, "add price failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	price := journal.Price{
		PID:       pid,
		Date:      date,
		Commodity: req.Msg.Commodity,
		Quantity:  req.Msg.Quantity,
		Currency:  req.Msg.Currency,
	}
	return connect.NewResponse(&floatv1.AddPriceResponse{Price: toProtoPriceDirective(price)}), nil
}

func (h *Handler) DeletePrice(ctx context.Context, req *connect.Request[floatv1.DeletePriceRequest]) (*connect.Response[floatv1.DeletePriceResponse], error) {
	logger := slogctx.FromContext(ctx)
	if req.Msg.Pid == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("pid is required"))
	}
	err := h.lock.Do(ctx, func() error {
		return journal.DeletePrice(h.dataDir, req.Msg.Pid)
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		logger.ErrorContext(ctx, "delete price failed", "pid", req.Msg.Pid, "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&floatv1.DeletePriceResponse{}), nil
}
