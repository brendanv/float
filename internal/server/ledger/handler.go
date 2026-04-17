package ledger

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/gen/float/v1/floatv1connect"
	"github.com/brendanv/float/internal/cache"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/gitsnap"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/rules"
	"github.com/brendanv/float/internal/slogctx"
	"github.com/brendanv/float/internal/txlock"
)

// Handler implements LedgerService RPCs by delegating to the hledger wrapper.
type Handler struct {
	floatv1connect.UnimplementedLedgerServiceHandler
	hl         *hledger.Client
	lock       *txlock.TxLock
	dataDir    string
	configPath string
	cache      *cache.Cache[any] // nil = bypass cache
	snap       *gitsnap.Repo
	cfg        *config.Config
}

func NewHandler(hl *hledger.Client, lock *txlock.TxLock, dataDir string, configPath string, c *cache.Cache[any], snap *gitsnap.Repo, cfg *config.Config) *Handler {
	return &Handler{hl: hl, lock: lock, dataDir: dataDir, configPath: configPath, cache: c, snap: snap, cfg: cfg}
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

func accountRegisterKey(account string, query []string) string {
	sorted := append([]string(nil), query...)
	sort.Strings(sorted)
	return fmt.Sprintf("aregister:%s:%s", account, strings.Join(sorted, "|"))
}

const accountsKey = "accounts"
const tagsKey = "tags"
const payeesKey = "payees"

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

// cachedAregister fetches account register rows from cache or hledger.
func cachedAregister(ctx context.Context, c *cache.Cache[any], hl *hledger.Client, account string, query []string) ([]hledger.AregisterRow, error) {
	if c == nil {
		return hl.Aregister(ctx, account, query...)
	}
	val, err := c.Get(ctx, accountRegisterKey(account, query), func(ctx context.Context) (any, error) {
		return hl.Aregister(ctx, account, query...)
	})
	if err != nil {
		return nil, err
	}
	return val.([]hledger.AregisterRow), nil
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

// cachedPayees fetches payee names from cache or hledger.
func cachedPayees(ctx context.Context, c *cache.Cache[any], hl *hledger.Client) ([]string, error) {
	if c == nil {
		return hl.Payees(ctx)
	}
	val, err := c.Get(ctx, payeesKey, func(ctx context.Context) (any, error) {
		return hl.Payees(ctx)
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
	hasNext := false
	if req.Msg.Limit > 0 && int(req.Msg.Limit) < len(txns) {
		txns = txns[:req.Msg.Limit]
		hasNext = true
	}
	proto := make([]*floatv1.Transaction, len(txns))
	for i, t := range txns {
		proto[i] = toProtoTransaction(t)
	}
	return connect.NewResponse(&floatv1.ListTransactionsResponse{Transactions: proto, Total: total, HasNext: hasNext}), nil
}

func (h *Handler) GetAccountRegister(ctx context.Context, req *connect.Request[floatv1.GetAccountRegisterRequest]) (*connect.Response[floatv1.GetAccountRegisterResponse], error) {
	logger := slogctx.FromContext(ctx)
	account := strings.TrimSpace(req.Msg.Account)
	if account == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("account is required"))
	}
	rows, err := cachedAregister(ctx, h.cache, h.hl, account, req.Msg.Query)
	if err != nil {
		logger.ErrorContext(ctx, "hledger aregister failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	total := int32(len(rows))
	if req.Msg.Offset > 0 {
		if int(req.Msg.Offset) >= len(rows) {
			rows = nil
		} else {
			rows = rows[req.Msg.Offset:]
		}
	}
	hasNext := false
	if req.Msg.Limit > 0 && int(req.Msg.Limit) < len(rows) {
		rows = rows[:req.Msg.Limit]
		hasNext = true
	}
	proto := make([]*floatv1.AccountRegisterRow, len(rows))
	for i, r := range rows {
		proto[i] = toProtoAccountRegisterRow(r)
	}
	return connect.NewResponse(&floatv1.GetAccountRegisterResponse{
		Rows: proto, Total: total, HasNext: hasNext,
	}), nil
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

func (h *Handler) ListPayees(ctx context.Context, req *connect.Request[floatv1.ListPayeesRequest]) (*connect.Response[floatv1.ListPayeesResponse], error) {
	logger := slogctx.FromContext(ctx)
	payees, err := cachedPayees(ctx, h.cache, h.hl)
	if err != nil {
		logger.ErrorContext(ctx, "hledger payees failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&floatv1.ListPayeesResponse{Payees: payees}), nil
}

func (h *Handler) DeleteTransaction(ctx context.Context, req *connect.Request[floatv1.DeleteTransactionRequest]) (*connect.Response[floatv1.DeleteTransactionResponse], error) {
	logger := slogctx.FromContext(ctx)
	fid := req.Msg.Fid
	if fid == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("fid is required"))
	}
	err := h.lock.Do(ctx, "delete transaction", func() error {
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
	err := h.lock.Do(ctx, "modify transaction tags", func() error {
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
	err := h.lock.Do(ctx, "update transaction date", func() error {
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
	desc := req.Msg.Description
	if req.Msg.Payee != "" {
		desc = req.Msg.Payee + " | " + desc
	}
	tx := journal.TransactionInput{
		Date:        date,
		Description: desc,
		Comment:     req.Msg.Comment,
		Postings:    postings,
		Status:      "Pending",
	}

	var fid string
	err := h.lock.Do(ctx, "add transaction", func() error {
		var e error
		fid, e = journal.AppendTransaction(ctx, h.hl, h.dataDir, tx)
		return e
	})
	if err != nil {
		logger.ErrorContext(ctx, "add transaction failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	txns, err := h.hl.Transactions(ctx, "code:"+fid)
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

	desc := req.Msg.Description
	if req.Msg.Payee != "" {
		desc = req.Msg.Payee + " | " + desc
	}

	var updated hledger.Transaction
	err := h.lock.Do(ctx, "update transaction", func() error {
		var e error
		updated, e = journal.UpdateTransaction(ctx, h.hl, h.dataDir, fid, desc, req.Msg.Date, req.Msg.Comment, postings)
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
	err := h.lock.Do(ctx, "update transaction status", func() error {
		return journal.UpdateTransactionStatus(ctx, h.hl, h.dataDir, fid, req.Msg.Status)
	})
	if err != nil {
		if strings.Contains(err.Error(), "no transaction found") {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		logger.ErrorContext(ctx, "update transaction status failed", "fid", fid, "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	txns, err := h.hl.Transactions(ctx, "code:"+fid)
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
	tags := make(map[string]string, len(t.Tags))
	for _, kv := range t.Tags {
		if !strings.HasPrefix(kv[0], hledger.HiddenMetaPrefix) {
			tags[kv[0]] = kv[1]
		}
	}
	return &floatv1.Transaction{
		Fid:         t.FID,
		Date:        t.Date,
		Description: t.Description,
		Comment:     t.Comment,
		Postings:    postings,
		Status:      status,
		Tags:        tags,
		Payee:       t.Payee,
		Note:        t.Note,

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

func toProtoAccountRegisterRow(r hledger.AregisterRow) *floatv1.AccountRegisterRow {
	change := make([]*floatv1.Amount, len(r.Change))
	for i, a := range r.Change {
		change[i] = toProtoAmount(a)
	}
	balance := make([]*floatv1.Amount, len(r.Balance))
	for i, a := range r.Balance {
		balance[i] = toProtoAmount(a)
	}
	// Normalize "Unmarked" to "" for proto contract, matching toProtoTransaction.
	status := r.Transaction.Status
	if status == "Unmarked" {
		status = ""
	}
	row := &floatv1.AccountRegisterRow{
		Fid:           r.Transaction.FID,
		Date:          r.Transaction.Date,
		Description:   r.Transaction.Description,
		Status:        status,
		OtherAccounts: append([]string(nil), r.OtherAccounts...),
		Change:        change,
		RunningTotal:  balance,
	}
	row.Payee = r.Transaction.Payee
	row.Note = r.Transaction.Note
	return row
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
	err := h.lock.Do(ctx, "add price directive", func() error {
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
	err := h.lock.Do(ctx, "delete price directive", func() error {
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

func (h *Handler) BulkEditTransactions(ctx context.Context, req *connect.Request[floatv1.BulkEditTransactionsRequest]) (*connect.Response[floatv1.BulkEditTransactionsResponse], error) {
	logger := slogctx.FromContext(ctx)

	if len(req.Msg.Fids) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("fids must not be empty"))
	}
	if len(req.Msg.Operations) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("operations must not be empty"))
	}
	for i, op := range req.Msg.Operations {
		switch v := op.Operation.(type) {
		case *floatv1.BulkEditOperation_MarkReviewed:
			// no additional validation needed
		case *floatv1.BulkEditOperation_AddTag:
			if v.AddTag.Key == "" {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("operation %d: add_tag key must not be empty", i))
			}
			if strings.HasPrefix(v.AddTag.Key, hledger.HiddenMetaPrefix) {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("operation %d: add_tag key must not use reserved prefix %q", i, hledger.HiddenMetaPrefix))
			}
		case *floatv1.BulkEditOperation_RemoveTag:
			if v.RemoveTag.Key == "" {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("operation %d: remove_tag key must not be empty", i))
			}
		case *floatv1.BulkEditOperation_SetPayee:
			if v.SetPayee.Payee == "" {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("operation %d: set_payee payee must not be empty", i))
			}
		case *floatv1.BulkEditOperation_ClearPayee:
			// no additional validation needed
		default:
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("operation %d: unrecognized or missing operation type", i))
		}
	}

	err := h.lock.Do(ctx, "bulk edit transactions", func() error {
		for _, fid := range req.Msg.Fids {
			txns, err := h.hl.Transactions(ctx, "code:"+fid)
			if err != nil {
				return fmt.Errorf("bulk-edit: lookup fid %q: %w", fid, err)
			}
			if len(txns) == 0 {
				return fmt.Errorf("bulk-edit: no transaction found with fid %q", fid)
			}
			if len(txns) > 1 {
				return fmt.Errorf("bulk-edit: fid %q matched %d transactions (corrupt journal — run audit)", fid, len(txns))
			}
			t := txns[0]
			src := &journal.SourceLocation{File: t.SourcePos[0].File, Line: t.SourcePos[0].Line}
			input, err := journal.InputFromTransaction(t)
			if err != nil {
				return fmt.Errorf("bulk-edit: fid %q: %w", fid, err)
			}

			for _, op := range req.Msg.Operations {
				switch v := op.Operation.(type) {
				case *floatv1.BulkEditOperation_MarkReviewed:
					if v.MarkReviewed.Reviewed {
						input.Status = "Cleared"
					} else {
						input.Status = ""
					}
				case *floatv1.BulkEditOperation_AddTag:
					if input.Tags == nil {
						input.Tags = make(map[string]string)
					}
					input.Tags[v.AddTag.Key] = v.AddTag.Value
				case *floatv1.BulkEditOperation_RemoveTag:
					delete(input.Tags, v.RemoveTag.Key)
				case *floatv1.BulkEditOperation_SetPayee:
					note := ""
					if t.Note != nil {
						note = *t.Note
					}
					input.Description = v.SetPayee.Payee + " | " + note
				case *floatv1.BulkEditOperation_ClearPayee:
					note := ""
					if t.Note != nil {
						note = *t.Note
					}
					input.Description = note
				}
			}

			if _, err := journal.WriteTransaction(ctx, h.hl, h.dataDir, input, src); err != nil {
				return fmt.Errorf("bulk-edit: fid %q: write: %w", fid, err)
			}
		}
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "no transaction found") {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		logger.ErrorContext(ctx, "bulk edit transactions failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	results := make([]*floatv1.Transaction, 0, len(req.Msg.Fids))
	for _, fid := range req.Msg.Fids {
		txns, err := h.hl.Transactions(ctx, "code:"+fid)
		if err != nil {
			logger.ErrorContext(ctx, "bulk edit: fetch after update failed", "fid", fid, "error", err)
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		if len(txns) == 0 {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("transaction %s not found after bulk edit", fid))
		}
		results = append(results, toProtoTransaction(txns[0]))
	}
	return connect.NewResponse(&floatv1.BulkEditTransactionsResponse{Transactions: results}), nil
}

func (h *Handler) ListSnapshots(ctx context.Context, req *connect.Request[floatv1.ListSnapshotsRequest]) (*connect.Response[floatv1.ListSnapshotsResponse], error) {
	if h.snap == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("snapshots not enabled"))
	}
	snaps, err := h.snap.List(ctx, int(req.Msg.Limit))
	if err != nil {
		slogctx.FromContext(ctx).ErrorContext(ctx, "list snapshots failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	out := make([]*floatv1.Snapshot, len(snaps))
	for i, s := range snaps {
		out[i] = &floatv1.Snapshot{
			Hash:      s.Hash,
			Message:   s.Message,
			Timestamp: s.Timestamp.Format(time.RFC3339),
		}
	}
	return connect.NewResponse(&floatv1.ListSnapshotsResponse{Snapshots: out}), nil
}

func (h *Handler) RestoreSnapshot(ctx context.Context, req *connect.Request[floatv1.RestoreSnapshotRequest]) (*connect.Response[floatv1.RestoreSnapshotResponse], error) {
	if h.snap == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("snapshots not enabled"))
	}
	if req.Msg.Hash == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("hash is required"))
	}
	if err := h.snap.Restore(ctx, req.Msg.Hash); err != nil {
		slogctx.FromContext(ctx).ErrorContext(ctx, "restore snapshot failed", "hash", req.Msg.Hash, "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	h.lock.BumpGeneration()
	return connect.NewResponse(&floatv1.RestoreSnapshotResponse{}), nil
}

// ---- Import handlers ----

func (h *Handler) ListBankProfiles(_ context.Context, _ *connect.Request[floatv1.ListBankProfilesRequest]) (*connect.Response[floatv1.ListBankProfilesResponse], error) {
	if h.cfg == nil {
		return connect.NewResponse(&floatv1.ListBankProfilesResponse{}), nil
	}
	out := make([]*floatv1.BankProfile, len(h.cfg.BankProfiles))
	for i, p := range h.cfg.BankProfiles {
		out[i] = &floatv1.BankProfile{Name: p.Name, RulesFile: p.RulesFile}
	}
	return connect.NewResponse(&floatv1.ListBankProfilesResponse{Profiles: out}), nil
}

func (h *Handler) CreateBankProfile(ctx context.Context, req *connect.Request[floatv1.CreateBankProfileRequest]) (*connect.Response[floatv1.CreateBankProfileResponse], error) {
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	if req.Msg.RulesFile == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("rules_file is required"))
	}

	// Reject path traversal attempts.
	cleaned := filepath.Clean(req.Msg.RulesFile)
	if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, "..") {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("rules_file must be a relative path within the data directory"))
	}

	if h.cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	if h.configPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server config path not set"))
	}

	// Check for duplicate name.
	for _, p := range h.cfg.BankProfiles {
		if p.Name == req.Msg.Name {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("bank profile %q already exists", req.Msg.Name))
		}
	}

	newProfile := config.BankProfile{Name: req.Msg.Name, RulesFile: cleaned}
	err := h.lock.Do(ctx, fmt.Sprintf("create bank profile %q", req.Msg.Name), func() error {
		// Write rules file if content provided.
		if len(req.Msg.RulesContent) > 0 {
			rulesPath := filepath.Join(h.dataDir, cleaned)
			if err := os.MkdirAll(filepath.Dir(rulesPath), 0o755); err != nil {
				return fmt.Errorf("create rules dir: %w", err)
			}
			if err := os.WriteFile(rulesPath, req.Msg.RulesContent, 0o644); err != nil {
				return fmt.Errorf("write rules file: %w", err)
			}
		}

		// Append profile to config and save.
		h.cfg.BankProfiles = append(h.cfg.BankProfiles, newProfile)
		if err := config.Save(h.configPath, h.cfg); err != nil {
			h.cfg.BankProfiles = h.cfg.BankProfiles[:len(h.cfg.BankProfiles)-1]
			return fmt.Errorf("save config: %w", err)
		}
		return nil
	})
	if err != nil {
		slogctx.FromContext(ctx).ErrorContext(ctx, "create bank profile failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	slogctx.FromContext(ctx).InfoContext(ctx, "created bank profile", "name", req.Msg.Name, "rules_file", cleaned)
	return connect.NewResponse(&floatv1.CreateBankProfileResponse{
		Profile: &floatv1.BankProfile{Name: newProfile.Name, RulesFile: newProfile.RulesFile},
	}), nil
}

func (h *Handler) GetBankProfileContent(ctx context.Context, req *connect.Request[floatv1.GetBankProfileContentRequest]) (*connect.Response[floatv1.GetBankProfileContentResponse], error) {
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	profile, err := h.bankProfile(req.Msg.Name)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	rulesPath := filepath.Join(h.dataDir, profile.RulesFile)
	content, err := os.ReadFile(rulesPath)
	if err != nil {
		if os.IsNotExist(err) {
			content = []byte{}
		} else {
			slogctx.FromContext(ctx).ErrorContext(ctx, "read rules file failed", "error", err)
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}
	return connect.NewResponse(&floatv1.GetBankProfileContentResponse{
		RulesFile:    profile.RulesFile,
		RulesContent: content,
	}), nil
}

func (h *Handler) UpdateBankProfile(ctx context.Context, req *connect.Request[floatv1.UpdateBankProfileRequest]) (*connect.Response[floatv1.UpdateBankProfileResponse], error) {
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	if h.cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	if h.configPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server config path not set"))
	}

	newName := req.Msg.NewName
	if newName == "" {
		newName = req.Msg.Name
	}

	// Check new name isn't already taken (unless it's the same profile).
	if newName != req.Msg.Name {
		for _, p := range h.cfg.BankProfiles {
			if p.Name == newName {
				return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("bank profile %q already exists", newName))
			}
		}
	}

	var updated config.BankProfile
	err := h.lock.Do(ctx, fmt.Sprintf("update bank profile %q", req.Msg.Name), func() error {
		idx := -1
		for i, p := range h.cfg.BankProfiles {
			if p.Name == req.Msg.Name {
				idx = i
				break
			}
		}
		if idx < 0 {
			return fmt.Errorf("bank profile %q not found", req.Msg.Name)
		}

		profile := h.cfg.BankProfiles[idx]

		if len(req.Msg.RulesContent) > 0 {
			rulesPath := filepath.Join(h.dataDir, profile.RulesFile)
			if err := os.WriteFile(rulesPath, req.Msg.RulesContent, 0o644); err != nil {
				return fmt.Errorf("write rules file: %w", err)
			}
		}

		h.cfg.BankProfiles[idx].Name = newName
		updated = h.cfg.BankProfiles[idx]

		if err := config.Save(h.configPath, h.cfg); err != nil {
			h.cfg.BankProfiles[idx].Name = req.Msg.Name // rollback
			return fmt.Errorf("save config: %w", err)
		}
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		slogctx.FromContext(ctx).ErrorContext(ctx, "update bank profile failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	slogctx.FromContext(ctx).InfoContext(ctx, "updated bank profile", "name", updated.Name)
	return connect.NewResponse(&floatv1.UpdateBankProfileResponse{
		Profile: &floatv1.BankProfile{Name: updated.Name, RulesFile: updated.RulesFile},
	}), nil
}

func (h *Handler) DeleteBankProfile(ctx context.Context, req *connect.Request[floatv1.DeleteBankProfileRequest]) (*connect.Response[floatv1.DeleteBankProfileResponse], error) {
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	if h.cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	if h.configPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server config path not set"))
	}

	err := h.lock.Do(ctx, fmt.Sprintf("delete bank profile %q", req.Msg.Name), func() error {
		idx := -1
		for i, p := range h.cfg.BankProfiles {
			if p.Name == req.Msg.Name {
				idx = i
				break
			}
		}
		if idx < 0 {
			return fmt.Errorf("bank profile %q not found", req.Msg.Name)
		}

		rulesFile := h.cfg.BankProfiles[idx].RulesFile
		h.cfg.BankProfiles = append(h.cfg.BankProfiles[:idx], h.cfg.BankProfiles[idx+1:]...)

		if err := config.Save(h.configPath, h.cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}

		if req.Msg.DeleteRulesFile && rulesFile != "" {
			rulesPath := filepath.Join(h.dataDir, rulesFile)
			if err := os.Remove(rulesPath); err != nil && !os.IsNotExist(err) {
				slogctx.FromContext(ctx).WarnContext(ctx, "failed to delete rules file", "path", rulesPath, "error", err)
			}
		}
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		slogctx.FromContext(ctx).ErrorContext(ctx, "delete bank profile failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	slogctx.FromContext(ctx).InfoContext(ctx, "deleted bank profile", "name", req.Msg.Name)
	return connect.NewResponse(&floatv1.DeleteBankProfileResponse{}), nil
}

func (h *Handler) PreviewImport(ctx context.Context, req *connect.Request[floatv1.PreviewImportRequest]) (*connect.Response[floatv1.PreviewImportResponse], error) {
	logger := slogctx.FromContext(ctx)
	if len(req.Msg.CsvData) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("csv_data is required"))
	}
	if req.Msg.ProfileName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("profile_name is required"))
	}

	// Find bank profile.
	profile, err := h.bankProfile(req.Msg.ProfileName)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	rulesFile := filepath.Join(h.dataDir, profile.RulesFile)

	// Write CSV to temp file.
	tmp, err := os.CreateTemp("", "float-import-*.csv")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create temp file: %w", err))
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(req.Msg.CsvData); err != nil {
		tmp.Close()
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("write temp file: %w", err))
	}
	tmp.Close()

	// Parse CSV with hledger.
	candidates, err := h.hl.PrintCSV(ctx, tmp.Name(), rulesFile)
	if err != nil {
		logger.ErrorContext(ctx, "hledger PrintCSV failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("parse CSV: %w", err))
	}

	// Build fingerprint set from existing transactions.
	existing, err := h.hl.Transactions(ctx)
	if err != nil {
		logger.ErrorContext(ctx, "fetch existing transactions failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	fpSet := make(map[string]bool, len(existing))
	for _, t := range existing {
		fpSet[journal.TxnFingerprint(t)] = true
	}

	// Load float rules for second-pass categorization.
	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		logger.ErrorContext(ctx, "load rules failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	out := make([]*floatv1.ImportCandidate, len(candidates))
	for i, c := range candidates {
		candidate := &floatv1.ImportCandidate{
			IsDuplicate: fpSet[journal.TxnFingerprint(c)],
		}
		if r := rules.Match(rulesList, c.Description); r != nil {
			candidate.MatchedRuleId = r.ID
			// Apply rule transformations so the preview reflects what will actually be imported.
			if r.Payee != "" {
				c.Description = r.Payee + " | " + c.Description
			}
			if r.Account != "" && len(c.Postings) == 2 {
				for j, p := range c.Postings {
					if !isAssetOrLiabilityAccount(p.Account) {
						c.Postings[j].Account = r.Account
					}
				}
			}
		}
		candidate.Transaction = toProtoTransaction(c)
		out[i] = candidate
	}
	return connect.NewResponse(&floatv1.PreviewImportResponse{Candidates: out}), nil
}

func (h *Handler) ImportTransactions(ctx context.Context, req *connect.Request[floatv1.ImportTransactionsRequest]) (*connect.Response[floatv1.ImportTransactionsResponse], error) {
	logger := slogctx.FromContext(ctx)
	if len(req.Msg.CsvData) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("csv_data is required"))
	}
	if req.Msg.ProfileName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("profile_name is required"))
	}
	if len(req.Msg.CandidateIndices) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("candidate_indices must not be empty"))
	}

	profile, err := h.bankProfile(req.Msg.ProfileName)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	rulesFile := filepath.Join(h.dataDir, profile.RulesFile)

	// Write CSV to temp file.
	tmp, err := os.CreateTemp("", "float-import-*.csv")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create temp file: %w", err))
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(req.Msg.CsvData); err != nil {
		tmp.Close()
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("write temp file: %w", err))
	}
	tmp.Close()

	candidates, err := h.hl.PrintCSV(ctx, tmp.Name(), rulesFile)
	if err != nil {
		logger.ErrorContext(ctx, "hledger PrintCSV failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("parse CSV: %w", err))
	}

	// Load rules for categorization during import.
	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		logger.ErrorContext(ctx, "load rules failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Build selected indices set.
	selectedSet := make(map[int32]bool, len(req.Msg.CandidateIndices))
	for _, idx := range req.Msg.CandidateIndices {
		selectedSet[idx] = true
	}

	importBatchID := time.Now().Format("2006-01-02") + "-" + journal.MintFID()

	var importedFIDs []string
	err = h.lock.Do(ctx, "import transactions", func() error {
		for i, c := range candidates {
			if !selectedSet[int32(i)] {
				continue
			}
			txInput, convErr := journal.HledgerTxnToInput(c)
			if convErr != nil {
				return fmt.Errorf("convert transaction %d: %w", i, convErr)
			}

			// Stamp every transaction with the import batch ID as hidden metadata.
			if txInput.FloatMeta == nil {
				txInput.FloatMeta = make(map[string]string)
			}
			txInput.FloatMeta["float-import"] = importBatchID

			// Apply float rules during import.
			if r := rules.Match(rulesList, c.Description); r != nil {
				if r.Payee != "" {
					note := txInput.Description
					txInput.Description = r.Payee + " | " + note
				}
				if r.Account != "" && len(c.Postings) == 2 {
					for j, p := range txInput.Postings {
						if !isAssetOrLiabilityAccount(p.Account) {
							txInput.Postings[j].Account = r.Account
						}
					}
				}
				if len(r.Tags) > 0 {
					for k, v := range r.Tags {
						txInput.Tags[k] = v
					}
				}
			}

			fid, writeErr := journal.AppendTransaction(ctx, h.hl, h.dataDir, txInput)
			if writeErr != nil {
				return fmt.Errorf("write transaction %d: %w", i, writeErr)
			}
			importedFIDs = append(importedFIDs, fid)
		}
		return nil
	})
	if err != nil {
		logger.ErrorContext(ctx, "import transactions failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Fetch the imported transactions to return.
	var txnProtos []*floatv1.Transaction
	for _, fid := range importedFIDs {
		txns, fetchErr := h.hl.Transactions(ctx, "code:"+fid)
		if fetchErr != nil || len(txns) == 0 {
			continue
		}
		txnProtos = append(txnProtos, toProtoTransaction(txns[0]))
	}

	return connect.NewResponse(&floatv1.ImportTransactionsResponse{
		ImportedCount: int32(len(importedFIDs)),
		Transactions:  txnProtos,
		ImportBatchId: importBatchID,
	}), nil
}

func (h *Handler) GetImportedTransactions(ctx context.Context, req *connect.Request[floatv1.GetImportedTransactionsRequest]) (*connect.Response[floatv1.ListTransactionsResponse], error) {
	logger := slogctx.FromContext(ctx)
	if req.Msg.ImportBatchId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("import_batch_id is required"))
	}
	query := []string{"tag:float-import=" + req.Msg.ImportBatchId}
	txns, err := cachedTransactions(ctx, h.cache, h.hl, query)
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
	hasNext := false
	if req.Msg.Limit > 0 && int(req.Msg.Limit) < len(txns) {
		txns = txns[:req.Msg.Limit]
		hasNext = true
	}
	proto := make([]*floatv1.Transaction, len(txns))
	for i, t := range txns {
		proto[i] = toProtoTransaction(t)
	}
	return connect.NewResponse(&floatv1.ListTransactionsResponse{Transactions: proto, Total: total, HasNext: hasNext}), nil
}

func (h *Handler) ListImports(ctx context.Context, _ *connect.Request[floatv1.ListImportsRequest]) (*connect.Response[floatv1.ListImportsResponse], error) {
	logger := slogctx.FromContext(ctx)
	txns, err := cachedTransactions(ctx, h.cache, h.hl, []string{"tag:float-import"})
	if err != nil {
		logger.ErrorContext(ctx, "hledger transactions failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Group transactions by import batch ID, preserving newest-first order.
	type batchEntry struct {
		batchID string
		date    string
		count   int32
	}
	seen := make(map[string]int) // batchID -> index in batches
	var batches []batchEntry
	for _, t := range txns {
		batchID := t.FloatMeta["float-import"]
		if batchID == "" {
			continue
		}
		if idx, ok := seen[batchID]; ok {
			batches[idx].count++
		} else {
			date := ""
			if len(batchID) >= 10 {
				date = batchID[:10]
			}
			seen[batchID] = len(batches)
			batches = append(batches, batchEntry{batchID: batchID, date: date, count: 1})
		}
	}

	// Sort descending by batch ID (which starts with YYYY-MM-DD).
	sort.Slice(batches, func(i, j int) bool {
		return batches[i].batchID > batches[j].batchID
	})

	out := make([]*floatv1.ImportSummary, len(batches))
	for i, b := range batches {
		out[i] = &floatv1.ImportSummary{
			ImportBatchId:    b.batchID,
			Date:             b.date,
			TransactionCount: b.count,
		}
	}
	return connect.NewResponse(&floatv1.ListImportsResponse{Imports: out}), nil
}

// isAssetOrLiabilityAccount returns true if the account name looks like an
// asset or liability account based on common prefixes.
func isAssetOrLiabilityAccount(account string) bool {
	lower := strings.ToLower(account)
	return strings.HasPrefix(lower, "assets") ||
		strings.HasPrefix(lower, "liabilities") ||
		strings.HasPrefix(lower, "asset:") ||
		strings.HasPrefix(lower, "liability:")
}

// bankProfile finds a BankProfile by name in the config.
func (h *Handler) bankProfile(name string) (config.BankProfile, error) {
	if h.cfg != nil {
		for _, p := range h.cfg.BankProfiles {
			if p.Name == name {
				return p, nil
			}
		}
	}
	return config.BankProfile{}, fmt.Errorf("bank profile %q not found", name)
}

// ---- Rules handlers ----

func (h *Handler) ListRules(ctx context.Context, _ *connect.Request[floatv1.ListRulesRequest]) (*connect.Response[floatv1.ListRulesResponse], error) {
	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		slogctx.FromContext(ctx).ErrorContext(ctx, "list rules failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	out := make([]*floatv1.TransactionRule, len(rulesList))
	for i, r := range rulesList {
		out[i] = toProtoRule(r)
	}
	return connect.NewResponse(&floatv1.ListRulesResponse{Rules: out}), nil
}

func (h *Handler) AddRule(ctx context.Context, req *connect.Request[floatv1.AddRuleRequest]) (*connect.Response[floatv1.AddRuleResponse], error) {
	logger := slogctx.FromContext(ctx)
	if req.Msg.Pattern == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("pattern is required"))
	}

	var newRule rules.Rule
	err := h.lock.Do(ctx, "add rule", func() error {
		rulesList, loadErr := rules.Load(h.dataDir)
		if loadErr != nil {
			return loadErr
		}
		newRule = rules.Rule{
			ID:       journal.MintFID(),
			Pattern:  req.Msg.Pattern,
			Payee:    req.Msg.Payee,
			Account:  req.Msg.Account,
			Tags:     req.Msg.Tags,
			Priority: int(req.Msg.Priority),
		}
		rulesList = append(rulesList, newRule)
		return rules.Save(h.dataDir, rulesList)
	})
	if err != nil {
		logger.ErrorContext(ctx, "add rule failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&floatv1.AddRuleResponse{Rule: toProtoRule(newRule)}), nil
}

func (h *Handler) UpdateRule(ctx context.Context, req *connect.Request[floatv1.UpdateRuleRequest]) (*connect.Response[floatv1.UpdateRuleResponse], error) {
	logger := slogctx.FromContext(ctx)
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	if req.Msg.Pattern == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("pattern is required"))
	}

	var updated rules.Rule
	err := h.lock.Do(ctx, "update rule", func() error {
		rulesList, loadErr := rules.Load(h.dataDir)
		if loadErr != nil {
			return loadErr
		}
		found := false
		for i, r := range rulesList {
			if r.ID == req.Msg.Id {
				rulesList[i] = rules.Rule{
					ID:       req.Msg.Id,
					Pattern:  req.Msg.Pattern,
					Payee:    req.Msg.Payee,
					Account:  req.Msg.Account,
					Tags:     req.Msg.Tags,
					Priority: int(req.Msg.Priority),
				}
				updated = rulesList[i]
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("rule %q not found", req.Msg.Id)
		}
		return rules.Save(h.dataDir, rulesList)
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		logger.ErrorContext(ctx, "update rule failed", "id", req.Msg.Id, "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&floatv1.UpdateRuleResponse{Rule: toProtoRule(updated)}), nil
}

func (h *Handler) DeleteRule(ctx context.Context, req *connect.Request[floatv1.DeleteRuleRequest]) (*connect.Response[floatv1.DeleteRuleResponse], error) {
	logger := slogctx.FromContext(ctx)
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	err := h.lock.Do(ctx, "delete rule", func() error {
		rulesList, loadErr := rules.Load(h.dataDir)
		if loadErr != nil {
			return loadErr
		}
		filtered := rulesList[:0]
		found := false
		for _, r := range rulesList {
			if r.ID == req.Msg.Id {
				found = true
				continue
			}
			filtered = append(filtered, r)
		}
		if !found {
			return fmt.Errorf("rule %q not found", req.Msg.Id)
		}
		return rules.Save(h.dataDir, filtered)
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		logger.ErrorContext(ctx, "delete rule failed", "id", req.Msg.Id, "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&floatv1.DeleteRuleResponse{}), nil
}

func (h *Handler) PreviewApplyRules(ctx context.Context, req *connect.Request[floatv1.PreviewApplyRulesRequest]) (*connect.Response[floatv1.PreviewApplyRulesResponse], error) {
	logger := slogctx.FromContext(ctx)

	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Filter to requested rules if specified.
	if len(req.Msg.RuleIds) > 0 {
		rulesList = filterRules(rulesList, req.Msg.RuleIds)
	}

	txns, err := cachedTransactions(ctx, h.cache, h.hl, req.Msg.Query)
	if err != nil {
		logger.ErrorContext(ctx, "fetch transactions failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	matches := rules.Preview(rulesList, txns)
	previews := make([]*floatv1.RuleApplicationPreview, len(matches))
	for i, m := range matches {
		p := &floatv1.RuleApplicationPreview{
			Fid:           m.Transaction.FID,
			Description:   m.Transaction.Description,
			MatchedRuleId: m.Rule.ID,
			AddTags:       m.Changes.AddTags,
		}
		// Current category account.
		if idx := categoryPostingIndex(m.Transaction); idx >= 0 {
			p.CurrentAccount = m.Transaction.Postings[idx].Account
		}
		if m.Transaction.Payee != nil {
			p.CurrentPayee = *m.Transaction.Payee
		}
		if m.Changes.NewAccount != nil {
			p.NewAccount = *m.Changes.NewAccount
		}
		if m.Changes.NewPayee != nil {
			p.NewPayee = *m.Changes.NewPayee
		}
		previews[i] = p
	}
	return connect.NewResponse(&floatv1.PreviewApplyRulesResponse{Previews: previews}), nil
}

func (h *Handler) ApplyRules(ctx context.Context, req *connect.Request[floatv1.ApplyRulesRequest]) (*connect.Response[floatv1.ApplyRulesResponse], error) {
	logger := slogctx.FromContext(ctx)

	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if len(req.Msg.RuleIds) > 0 {
		rulesList = filterRules(rulesList, req.Msg.RuleIds)
	}

	txns, err := h.hl.Transactions(ctx, req.Msg.Query...)
	if err != nil {
		logger.ErrorContext(ctx, "fetch transactions failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	matches := rules.Preview(rulesList, txns)

	// Filter to requested FIDs if specified.
	if len(req.Msg.Fids) > 0 {
		fidSet := make(map[string]bool, len(req.Msg.Fids))
		for _, fid := range req.Msg.Fids {
			fidSet[fid] = true
		}
		filtered := matches[:0]
		for _, m := range matches {
			if fidSet[m.Transaction.FID] {
				filtered = append(filtered, m)
			}
		}
		matches = filtered
	}

	var applied int
	err = h.lock.Do(ctx, "apply rules", func() error {
		var applyErr error
		applied, applyErr = rules.Apply(ctx, h.hl, h.dataDir, matches)
		return applyErr
	})
	if err != nil {
		logger.ErrorContext(ctx, "apply rules failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&floatv1.ApplyRulesResponse{AppliedCount: int32(applied)}), nil
}

// filterRules returns only rules whose IDs are in the given set.
func filterRules(rulesList []rules.Rule, ids []string) []rules.Rule {
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	filtered := rulesList[:0]
	for _, r := range rulesList {
		if idSet[r.ID] {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// categoryPostingIndex returns the index of the non-asset/liability posting
// in a 2-posting transaction, or -1 if ambiguous.
func categoryPostingIndex(txn hledger.Transaction) int {
	if len(txn.Postings) != 2 {
		return -1
	}
	for i, p := range txn.Postings {
		if !isAssetOrLiabilityAccount(p.Account) {
			return i
		}
	}
	return -1
}

func toProtoRule(r rules.Rule) *floatv1.TransactionRule {
	return &floatv1.TransactionRule{
		Id:       r.ID,
		Pattern:  r.Pattern,
		Payee:    r.Payee,
		Account:  r.Account,
		Tags:     r.Tags,
		Priority: int32(r.Priority),
	}
}
