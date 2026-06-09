package ledger

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/gen/float/v1/floatv1connect"
	"github.com/brendanv/float/internal/alphavantage"
	"github.com/brendanv/float/internal/cache"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/gitsnap"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/logstream"
	"github.com/brendanv/float/internal/rules"
	"github.com/brendanv/float/internal/slogctx"
	"github.com/brendanv/float/internal/templates"
	"github.com/brendanv/float/internal/txlock"
)

// Handler implements LedgerService RPCs by delegating to the hledger wrapper.
type Handler struct {
	floatv1connect.UnimplementedLedgerServiceHandler
	hl             *hledger.Client
	lock           *txlock.TxLock
	dataDir        string
	configPath     string
	cache          *cache.Cache[any] // nil = bypass cache
	snap           *gitsnap.Repo
	cfg            *config.Config
	logBroadcaster *logstream.Broadcaster
	// AIBaseURL overrides the OpenRouter API endpoint. Set in tests only.
	AIBaseURL string
	// afterImportAllPreFetch is called between the pre-fetch phase and lock acquisition
	// in ImportAllStripeTransactions. Used in tests to simulate a concurrent import (e.g.,
	// daily auto-import writing the same transactions). Nil in production.
	afterImportAllPreFetch func()
	// afterImportPreFetch is called between the pre-fetch phase and lock acquisition
	// in ImportStripeTransactions. Used in tests to simulate a concurrent import.
	// Nil in production.
	afterImportPreFetch func()
	// afterDailyImportPreFetch is called between the pre-lock dedup phase and lock
	// acquisition in runDailyStripeImport. Used in tests to simulate a concurrent
	// import landing in the race window. Nil in production.
	afterDailyImportPreFetch func()
}

func NewHandler(hl *hledger.Client, lock *txlock.TxLock, dataDir string, configPath string, c *cache.Cache[any], snap *gitsnap.Repo, cfg *config.Config, broadcaster *logstream.Broadcaster) *Handler {
	return &Handler{hl: hl, lock: lock, dataDir: dataDir, configPath: configPath, cache: c, snap: snap, cfg: cfg, logBroadcaster: broadcaster}
}

func (h *Handler) RunHledgerQuery(ctx context.Context, req *connect.Request[floatv1.RunHledgerQueryRequest]) (*connect.Response[floatv1.RunHledgerQueryResponse], error) {
	logger := slogctx.FromContext(ctx)
	if strings.TrimSpace(req.Msg.Args) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("args is required"))
	}
	stdout, stderr, cmdLine, err := h.hl.RunQuery(ctx, req.Msg.Args)
	success := err == nil
	if err != nil {
		logger.InfoContext(ctx, "hledger query returned non-zero", "args", req.Msg.Args, "error", err)
	}
	return connect.NewResponse(&floatv1.RunHledgerQueryResponse{
		Stdout:      string(stdout),
		Stderr:      string(stderr),
		Success:     success,
		CommandLine: cmdLine,
	}), nil
}

// cacheKey helpers produce deterministic, namespaced keys from RPC parameters.
// Query args are sorted so that ["b","a"] and ["a","b"] produce the same key.

func transactionsKey(query []string) string {
	return "transactions:" + normalizedQueryKeyPart(query)
}

func balancesKey(depth int, query []string) string {
	return fmt.Sprintf("balances:%d:%s", depth, normalizedQueryKeyPart(query))
}

func accountRegisterKey(account string, query []string) string {
	return fmt.Sprintf("aregister:%s:%s", account, normalizedQueryKeyPart(query))
}

const accountsKey = "accounts"
const tagsKey = "tags"
const payeesKey = "payees"

func netWorthKey(begin, end string) string {
	return fmt.Sprintf("networth:%s:%s", begin, end)
}

func incomeStatementKey(begin, end string) string {
	return fmt.Sprintf("incomestmt:%s:%s", begin, end)
}

func balancesValuedKey(valueSpec string, depth int, query []string) string {
	return fmt.Sprintf("balancesvalued:%s:%d:%s", valueSpec, depth, normalizedQueryKeyPart(query))
}

func normalizedQueryKeyPart(query []string) string {
	sorted := append([]string(nil), query...)
	sort.Strings(sorted)
	return strings.Join(sorted, "|")
}

func cachedGet[T any](ctx context.Context, c *cache.Cache[any], key string, load func(context.Context) (T, error)) (T, error) {
	var zero T
	if c == nil {
		return load(ctx)
	}
	val, err := c.Get(ctx, key, func(ctx context.Context) (any, error) {
		return load(ctx)
	})
	if err != nil {
		return zero, err
	}
	typed, ok := val.(T)
	if !ok {
		return zero, fmt.Errorf("cache value for key %q had unexpected type %T", key, val)
	}
	return typed, nil
}

// cachedTransactions fetches transactions from cache or hledger.
func cachedTransactions(ctx context.Context, c *cache.Cache[any], hl *hledger.Client, query []string) ([]hledger.Transaction, error) {
	return cachedGet(ctx, c, transactionsKey(query), func(ctx context.Context) ([]hledger.Transaction, error) {
		return hl.Transactions(ctx, query...)
	})
}

// cachedBalances fetches balances from cache or hledger.
func cachedBalances(ctx context.Context, c *cache.Cache[any], hl *hledger.Client, depth int, query []string) (*hledger.BalanceReport, error) {
	return cachedGet(ctx, c, balancesKey(depth, query), func(ctx context.Context) (*hledger.BalanceReport, error) {
		return hl.Balances(ctx, depth, query...)
	})
}

// cachedBalancesValued fetches market-valued balances from cache or hledger.
func cachedBalancesValued(ctx context.Context, c *cache.Cache[any], hl *hledger.Client, valueSpec string, depth int, query []string) (*hledger.BalanceReport, error) {
	return cachedGet(ctx, c, balancesValuedKey(valueSpec, depth, query), func(ctx context.Context) (*hledger.BalanceReport, error) {
		return hl.BalancesValued(ctx, valueSpec, depth, query...)
	})
}

// cachedAregister fetches account register rows from cache or hledger.
func cachedAregister(ctx context.Context, c *cache.Cache[any], hl *hledger.Client, account string, query []string) ([]hledger.AregisterRow, error) {
	return cachedGet(ctx, c, accountRegisterKey(account, query), func(ctx context.Context) ([]hledger.AregisterRow, error) {
		return hl.Aregister(ctx, account, query...)
	})
}

// cachedNetWorth fetches a balance sheet timeseries from cache or hledger.
func cachedNetWorth(ctx context.Context, c *cache.Cache[any], hl *hledger.Client, begin, end string) (*hledger.BalanceSheetTimeseries, error) {
	return cachedGet(ctx, c, netWorthKey(begin, end), func(ctx context.Context) (*hledger.BalanceSheetTimeseries, error) {
		return hl.BalanceSheetTimeseries(ctx, begin, end)
	})
}

func portfolioTimeseriesKey(accounts []string, begin string) string {
	sorted := make([]string, len(accounts))
	copy(sorted, accounts)
	sort.Strings(sorted)
	return fmt.Sprintf("portfoliotimeseries:%s:%s", strings.Join(sorted, "|"), begin)
}

// cachedPortfolioTimeseries fetches a portfolio value timeseries from cache or hledger.
func cachedPortfolioTimeseries(ctx context.Context, c *cache.Cache[any], hl *hledger.Client, accounts []string, begin string) (*hledger.BalanceSheetTimeseries, error) {
	return cachedGet(ctx, c, portfolioTimeseriesKey(accounts, begin), func(ctx context.Context) (*hledger.BalanceSheetTimeseries, error) {
		return hl.PortfolioTimeseries(ctx, accounts, begin)
	})
}

func portfolioCostBasisKey(accounts []string, begin string) string {
	sorted := make([]string, len(accounts))
	copy(sorted, accounts)
	sort.Strings(sorted)
	return fmt.Sprintf("portfoliocostbasis:%s:%s", strings.Join(sorted, "|"), begin)
}

// cachedPortfolioCostBasis fetches a portfolio cost-basis timeseries from cache or hledger.
func cachedPortfolioCostBasis(ctx context.Context, c *cache.Cache[any], hl *hledger.Client, accounts []string, begin string) (*hledger.BalanceSheetTimeseries, error) {
	return cachedGet(ctx, c, portfolioCostBasisKey(accounts, begin), func(ctx context.Context) (*hledger.BalanceSheetTimeseries, error) {
		return hl.PortfolioCostBasisTimeseries(ctx, accounts, begin)
	})
}

// cachedIncomeStatement fetches an income statement timeseries from cache or hledger.
func cachedIncomeStatement(ctx context.Context, c *cache.Cache[any], hl *hledger.Client, begin, end string) (*hledger.IncomeStatementTimeseries, error) {
	return cachedGet(ctx, c, incomeStatementKey(begin, end), func(ctx context.Context) (*hledger.IncomeStatementTimeseries, error) {
		return hl.IncomeStatementTimeseries(ctx, begin, end)
	})
}

// cachedTags fetches tag names from cache or hledger.
func cachedTags(ctx context.Context, c *cache.Cache[any], hl *hledger.Client) ([]string, error) {
	return cachedGet(ctx, c, tagsKey, func(ctx context.Context) ([]string, error) {
		return hl.Tags(ctx)
	})
}

// cachedPayees fetches payee names from cache or hledger.
func cachedPayees(ctx context.Context, c *cache.Cache[any], hl *hledger.Client) ([]string, error) {
	return cachedGet(ctx, c, payeesKey, func(ctx context.Context) ([]string, error) {
		return hl.Payees(ctx)
	})
}

// cachedAccounts fetches accounts from cache or hledger.
func cachedAccounts(ctx context.Context, c *cache.Cache[any], hl *hledger.Client) ([]*hledger.AccountNode, error) {
	return cachedGet(ctx, c, accountsKey, func(ctx context.Context) ([]*hledger.AccountNode, error) {
		return hl.Accounts(ctx, false)
	})
}

const unusedAccountsKey = "unusedaccounts"

// cachedUnusedAccounts fetches unused account names from cache or hledger.
func cachedUnusedAccounts(ctx context.Context, c *cache.Cache[any], hl *hledger.Client) ([]string, error) {
	return cachedGet(ctx, c, unusedAccountsKey, func(ctx context.Context) ([]string, error) {
		return hl.UnusedAccounts(ctx)
	})
}

func paginate[T any](items []T, offset, limit int32) ([]T, int32, bool) {
	total := int32(len(items))
	if offset > 0 {
		if int(offset) >= len(items) {
			items = nil
		} else {
			items = items[offset:]
		}
	}
	hasNext := false
	if limit > 0 && int(limit) < len(items) {
		items = items[:limit]
		hasNext = true
	}
	return items, total, hasNext
}

func (h *Handler) ListTransactions(ctx context.Context, req *connect.Request[floatv1.ListTransactionsRequest]) (*connect.Response[floatv1.ListTransactionsResponse], error) {
	txns, err := cachedTransactions(ctx, h.cache, h.hl, req.Msg.Query)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger transactions failed")
	}
	txns, total, hasNext := paginate(txns, req.Msg.Offset, req.Msg.Limit)
	proto := make([]*floatv1.Transaction, len(txns))
	for i, t := range txns {
		proto[i] = toProtoTransaction(t)
	}
	return connect.NewResponse(&floatv1.ListTransactionsResponse{Transactions: proto, Total: total, HasNext: hasNext}), nil
}

func (h *Handler) GetAccountRegister(ctx context.Context, req *connect.Request[floatv1.GetAccountRegisterRequest]) (*connect.Response[floatv1.GetAccountRegisterResponse], error) {
	account := strings.TrimSpace(req.Msg.Account)
	if account == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("account is required"))
	}
	rows, err := cachedAregister(ctx, h.cache, h.hl, account, req.Msg.Query)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger aregister failed")
	}
	rows, total, hasNext := paginate(rows, req.Msg.Offset, req.Msg.Limit)
	proto := make([]*floatv1.AccountRegisterRow, len(rows))
	for i, r := range rows {
		proto[i] = toProtoAccountRegisterRow(r)
	}
	return connect.NewResponse(&floatv1.GetAccountRegisterResponse{
		Rows: proto, Total: total, HasNext: hasNext,
	}), nil
}

func (h *Handler) GetBalances(ctx context.Context, req *connect.Request[floatv1.GetBalancesRequest]) (*connect.Response[floatv1.GetBalancesResponse], error) {
	var report *hledger.BalanceReport
	var err error
	if req.Msg.Value != "" {
		report, err = cachedBalancesValued(ctx, h.cache, h.hl, req.Msg.Value, int(req.Msg.Depth), req.Msg.Query)
	} else {
		report, err = cachedBalances(ctx, h.cache, h.hl, int(req.Msg.Depth), req.Msg.Query)
	}
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger balances failed")
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

// exactQty accumulates a commodity quantity using exact integer arithmetic
// (mantissa / 10^scale) to avoid float64 rounding errors that produce
// near-zero residuals for fully-liquidated positions (e.g. 6e-14 instead of 0).
type exactQty struct {
	mantissa int64
	scale    int
}

func (q *exactQty) add(mantissa int64, decimalPlaces int) {
	m := mantissa
	p := decimalPlaces
	if p > q.scale {
		factor := int64(1)
		for i := 0; i < p-q.scale; i++ {
			factor *= 10
		}
		q.mantissa *= factor
		q.scale = p
	} else if p < q.scale {
		factor := int64(1)
		for i := 0; i < q.scale-p; i++ {
			factor *= 10
		}
		m *= factor
	}
	q.mantissa += m
}

func (q exactQty) float() float64 {
	v := float64(q.mantissa)
	if q.scale > 0 {
		divisor := float64(1)
		for i := 0; i < q.scale; i++ {
			divisor *= 10
		}
		v /= divisor
	}
	return v
}

// currencySymbols is the set of commodity codes treated as plain fiat currency.
// Accounts whose amounts consist only of these commodities are excluded from the
// portfolio view — they are cash positions, not equity holdings.
var currencySymbols = map[string]bool{
	"$": true, "USD": true, "EUR": true, "GBP": true, "JPY": true,
	"CAD": true, "AUD": true, "CHF": true, "NZD": true,
	"HKD": true, "SGD": true, "SEK": true, "NOK": true,
	"DKK": true, "MXN": true, "INR": true, "CNY": true,
}

func (h *Handler) GetPortfolioHoldings(ctx context.Context, req *connect.Request[floatv1.GetPortfolioHoldingsRequest]) (*connect.Response[floatv1.GetPortfolioHoldingsResponse], error) {
	logger := slogctx.FromContext(ctx)

	prefix := req.Msg.AccountPrefix
	if prefix == "" {
		prefix = "assets"
	}
	query := []string{prefix}

	// Raw balances provide the original commodity quantities.
	raw, err := cachedBalances(ctx, h.cache, h.hl, 0, query)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger balances failed for portfolio")
	}

	// Build symbol→latestPriceInfo from the prices journal.
	// priceInfo holds the most recent price amount and currency for each commodity,
	// used to compute CurrentValue and LatestPrice per holding.
	priceList, err := journal.ListPrices(h.dataDir)
	if err != nil {
		logger.WarnContext(ctx, "could not read prices for price dates", "error", err)
	}
	type priceInfo struct {
		date     string
		quantity float64
		currency string
	}
	latestPriceInfo := make(map[string]priceInfo)
	for _, p := range priceList {
		qty, _ := strconv.ParseFloat(p.Quantity, 64)
		if existing := latestPriceInfo[p.Commodity]; p.Date > existing.date {
			latestPriceInfo[p.Commodity] = priceInfo{date: p.Date, quantity: qty, currency: p.Currency}
		}
	}

	// Build symbol→lastTxDate from transactions in the portfolio accounts.
	// A transaction date is a valid proxy for price: you know the price on any
	// day you bought or sold the commodity. This fills in price dates for
	// commodities (e.g. 401k funds) that have no P directives.
	lastTxDateByCommodity := make(map[string]string)
	if txns, txErr := cachedTransactions(ctx, h.cache, h.hl, query); txErr != nil {
		logger.WarnContext(ctx, "could not load transactions for last purchase dates", "error", txErr)
	} else {
		for _, txn := range txns {
			for _, p := range txn.Postings {
				for _, amt := range p.Amounts {
					if currencySymbols[amt.Commodity] {
						continue
					}
					date := txn.Date
					if p.Date != nil && *p.Date != "" {
						date = *p.Date
					}
					if date > lastTxDateByCommodity[amt.Commodity] {
						lastTxDateByCommodity[amt.Commodity] = date
					}
				}
			}
		}
	}

	// Aggregate raw amounts by (account, commodity), summing quantities across
	// lots. hledger returns a separate Amount per purchase lot when positions
	// are built up at different prices (each lot has a distinct acost), so
	// without aggregation every lot would appear as its own holding row.
	type holdingKey struct{ account, symbol string }
	type costBasis struct {
		total    float64
		currency string
	}
	aggregated := make(map[holdingKey]exactQty)
	costByHolding := make(map[holdingKey]costBasis)
	var holdingOrder []holdingKey
	for _, row := range raw.Rows {
		for _, amt := range row.Amounts {
			if currencySymbols[amt.Commodity] {
				continue
			}
			k := holdingKey{row.FullName, amt.Commodity}
			if _, seen := aggregated[k]; !seen {
				holdingOrder = append(holdingOrder, k)
			}
			cur := aggregated[k]
			cur.add(amt.Quantity.DecimalMantissa, amt.Quantity.DecimalPlaces)
			aggregated[k] = cur

			if c, err := amt.ParseCost(); err == nil && c != nil {
				var lotCost float64
				switch c.Tag {
				case "TotalCost":
					lotCost = c.Contents.Quantity.FloatingPoint
				case "UnitCost":
					lotCost = c.Contents.Quantity.FloatingPoint * amt.Quantity.FloatingPoint
				}
				cb := costByHolding[k]
				cb.total += lotCost
				cb.currency = c.Contents.Commodity
				costByHolding[k] = cb
			}
		}
	}

	// Build holdings from aggregated (account, symbol) pairs.
	var holdings []*floatv1.Holding
	for _, k := range holdingOrder {
		eq := aggregated[k]
		if eq.mantissa == 0 {
			continue
		}
		qty := eq.float()
		priceDate := latestPriceInfo[k.symbol].date
		if txDate := lastTxDateByCommodity[k.symbol]; txDate > priceDate {
			priceDate = txDate
		}
		holding := &floatv1.Holding{
			Account:  k.account,
			Symbol:   k.symbol,
			Quantity: fmt.Sprintf("%g", qty),
			PriceDate: priceDate,
		}

		// Compute CurrentValue and LatestPrice from the price list.
		// Using the price list directly (rather than a hledger --value balance) gives
		// per-commodity values even when multiple commodities share the same account.
		if info, ok := latestPriceInfo[k.symbol]; ok {
			value := qty * info.quantity
			holding.CurrentValue = &floatv1.Amount{
				Commodity: info.currency,
				Quantity:  fmt.Sprintf("%.2f", value),
			}
			if qty != 0 {
				holding.LatestPrice = &floatv1.Amount{
					Commodity: info.currency,
					Quantity:  fmt.Sprintf("%.2f", info.quantity),
				}
			}
		}

		// Populate cost basis and unrealized gain from per-lot acost annotations
		// aggregated during the raw balance pass above.
		if cb, ok := costByHolding[k]; ok && cb.currency != "" {
			holding.BookValue = &floatv1.Amount{
				Commodity: cb.currency,
				Quantity:  fmt.Sprintf("%.2f", cb.total),
			}
			if holding.CurrentValue != nil {
				currentVal, _ := strconv.ParseFloat(holding.CurrentValue.Quantity, 64)
				gain := currentVal - cb.total
				holding.UnrealizedGain = &floatv1.Amount{
					Commodity: cb.currency,
					Quantity:  fmt.Sprintf("%.2f", gain),
				}
				if cb.total != 0 {
					holding.UnrealizedGainPct = gain / cb.total * 100
				}
			}
		}

		holdings = append(holdings, holding)
	}

	// Sum total value, compute allocation percentages, and track as_of_date.
	var totalValue float64
	var baseCurrency string
	var asOfDate string
	for _, h := range holdings {
		if h.CurrentValue != nil {
			v, _ := strconv.ParseFloat(h.CurrentValue.Quantity, 64)
			totalValue += v
			baseCurrency = h.CurrentValue.Commodity
		}
		if h.PriceDate > asOfDate {
			asOfDate = h.PriceDate
		}
	}

	if totalValue > 0 {
		for _, h := range holdings {
			if h.CurrentValue != nil {
				v, _ := strconv.ParseFloat(h.CurrentValue.Quantity, 64)
				h.PortfolioPct = v / totalValue * 100
			}
		}
		sort.Slice(holdings, func(i, j int) bool {
			var vi, vj float64
			if holdings[i].CurrentValue != nil {
				vi, _ = strconv.ParseFloat(holdings[i].CurrentValue.Quantity, 64)
			}
			if holdings[j].CurrentValue != nil {
				vj, _ = strconv.ParseFloat(holdings[j].CurrentValue.Quantity, 64)
			}
			return vi > vj
		})
	}

	resp := &floatv1.GetPortfolioHoldingsResponse{Holdings: holdings, AsOfDate: asOfDate}
	if totalValue > 0 {
		resp.TotalValue = &floatv1.Amount{Commodity: baseCurrency, Quantity: fmt.Sprintf("%.2f", totalValue)}
	}
	return connect.NewResponse(resp), nil
}

func (h *Handler) GetPortfolioTimeseries(ctx context.Context, req *connect.Request[floatv1.GetPortfolioTimeseriesRequest]) (*connect.Response[floatv1.GetPortfolioTimeseriesResponse], error) {

	prefix := req.Msg.AccountPrefix
	if prefix == "" {
		prefix = "assets"
	}

	// Determine which accounts under the prefix actually hold non-currency
	// commodities (equities, funds, etc.). This mirrors the GetPortfolioHoldings
	// filter so that cash accounts like checking/savings are excluded from the
	// chart total, matching what the Total Value card shows.
	raw, err := cachedBalances(ctx, h.cache, h.hl, 0, []string{prefix})
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger balances failed for portfolio timeseries")
	}
	seen := make(map[string]bool)
	var investmentAccounts []string
	for _, row := range raw.Rows {
		for _, amt := range row.Amounts {
			if !currencySymbols[amt.Commodity] && !seen[row.FullName] {
				investmentAccounts = append(investmentAccounts, row.FullName)
				seen[row.FullName] = true
			}
		}
	}
	if len(investmentAccounts) == 0 {
		return connect.NewResponse(&floatv1.GetPortfolioTimeseriesResponse{}), nil
	}

	ts, err := cachedPortfolioTimeseries(ctx, h.cache, h.hl, investmentAccounts, req.Msg.Begin)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger portfolio timeseries failed")
	}

	cb, err := cachedPortfolioCostBasis(ctx, h.cache, h.hl, investmentAccounts, req.Msg.Begin)
	if err != nil {
		// Cost basis is best-effort: log and continue without it.
		cb = nil
	}

	// Build a date→cost-basis index for easy lookup.
	cbByDate := map[string]*floatv1.Amount{}
	if cb != nil {
		for i, date := range cb.Periods {
			for _, sub := range cb.Subreports {
				if sub.Name == "Assets" && len(sub.Totals[i]) > 0 {
					a := sub.Totals[i][0]
					cbByDate[date] = &floatv1.Amount{
						Commodity: a.Commodity,
						Quantity:  fmt.Sprintf("%.2f", a.Quantity.FloatingPoint),
					}
				}
			}
		}
	}

	snapshots := make([]*floatv1.PortfolioTimeseriesSnapshot, len(ts.Periods))
	for i, date := range ts.Periods {
		snap := &floatv1.PortfolioTimeseriesSnapshot{Date: date}
		for _, sub := range ts.Subreports {
			if sub.Name == "Assets" && len(sub.Totals[i]) > 0 {
				a := sub.Totals[i][0]
				snap.TotalValue = &floatv1.Amount{
					Commodity: a.Commodity,
					Quantity:  fmt.Sprintf("%.2f", a.Quantity.FloatingPoint),
				}
			}
		}
		snap.CostBasis = cbByDate[date]
		snapshots[i] = snap
	}
	return connect.NewResponse(&floatv1.GetPortfolioTimeseriesResponse{Snapshots: snapshots}), nil
}

func (h *Handler) GetNetWorthTimeseries(ctx context.Context, req *connect.Request[floatv1.GetNetWorthTimeseriesRequest]) (*connect.Response[floatv1.GetNetWorthTimeseriesResponse], error) {
	ts, err := cachedNetWorth(ctx, h.cache, h.hl, req.Msg.Begin, req.Msg.End)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger balance sheet timeseries failed")
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

func (h *Handler) GetIncomeStatementTimeseries(ctx context.Context, req *connect.Request[floatv1.GetIncomeStatementTimeseriesRequest]) (*connect.Response[floatv1.GetIncomeStatementTimeseriesResponse], error) {
	ts, err := cachedIncomeStatement(ctx, h.cache, h.hl, req.Msg.Begin, req.Msg.End)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger income statement timeseries failed")
	}

	var rows []*floatv1.IncomeStatementRow
	for _, sub := range ts.Subreports {
		for _, r := range sub.Rows {
			rows = append(rows, toProtoISRow(r, false))
		}
		// Append a synthetic section-total row using the section totals.
		totalRow := hledger.ISRow{
			DisplayName:      "Total " + sub.Name,
			FullName:         "",
			Indent:           0,
			Section:          sub.Name,
			PerPeriodAmounts: sub.Totals,
		}
		rows = append(rows, toProtoISRow(totalRow, true))
	}

	netAmounts := make([]*floatv1.AmountList, len(ts.NetAmounts))
	for i, amounts := range ts.NetAmounts {
		netAmounts[i] = &floatv1.AmountList{Amounts: toProtoAmounts(amounts)}
	}

	return connect.NewResponse(&floatv1.GetIncomeStatementTimeseriesResponse{
		Periods:    ts.Periods,
		Rows:       rows,
		NetAmounts: netAmounts,
	}), nil
}

func toProtoISRow(r hledger.ISRow, isTotal bool) *floatv1.IncomeStatementRow {
	perPeriod := make([]*floatv1.AmountList, len(r.PerPeriodAmounts))
	for i, amounts := range r.PerPeriodAmounts {
		perPeriod[i] = &floatv1.AmountList{Amounts: toProtoAmounts(amounts)}
	}
	return &floatv1.IncomeStatementRow{
		DisplayName:      r.DisplayName,
		FullName:         r.FullName,
		Indent:           int32(r.Indent),
		Section:          r.Section,
		PerPeriodAmounts: perPeriod,
		TotalAmounts:     toProtoAmounts(r.TotalAmounts),
		IsTotal:          isTotal,
	}
}

func (h *Handler) ListAccounts(ctx context.Context, req *connect.Request[floatv1.ListAccountsRequest]) (*connect.Response[floatv1.ListAccountsResponse], error) {
	nodes, err := cachedAccounts(ctx, h.cache, h.hl)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger accounts failed")
	}
	accounts := make([]*floatv1.Account, len(nodes))
	for i, n := range nodes {
		accounts[i] = toProtoAccount(n)
	}
	return connect.NewResponse(&floatv1.ListAccountsResponse{Accounts: accounts}), nil
}

// GetBalanceAssertionStatus reports, for every asset and liability account that
// has postings, the date of its most recent balance assertion, the number of
// transactions since that assertion, and its most recent transaction (so the UI
// can edit it to add a fresh assertion). It does a single transactions pass and
// groups in Go rather than running hledger once per account. Accounts are sorted
// by the transaction count since the last assertion so the busiest drift risks
// surface first.
func (h *Handler) GetBalanceAssertionStatus(ctx context.Context, req *connect.Request[floatv1.GetBalanceAssertionStatusRequest]) (*connect.Response[floatv1.GetBalanceAssertionStatusResponse], error) {
	nodes, err := cachedAccounts(ctx, h.cache, h.hl)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger accounts failed")
	}
	// Cash (C) is an hledger subtype of Asset, so treat it as an asset too.
	accountType := make(map[string]hledger.AccountType, len(nodes))
	for _, n := range nodes {
		switch n.Type {
		case hledger.AccountTypeAsset, hledger.AccountTypeCash:
			accountType[n.FullName] = hledger.AccountTypeAsset
		case hledger.AccountTypeLiability:
			accountType[n.FullName] = hledger.AccountTypeLiability
		}
	}

	txns, err := cachedTransactions(ctx, h.cache, h.hl, nil)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger transactions failed")
	}

	balReport, err := cachedBalances(ctx, h.cache, h.hl, 0, nil)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger balances failed")
	}
	balByAccount := make(map[string][]hledger.Amount, len(balReport.Rows))
	for _, row := range balReport.Rows {
		balByAccount[row.FullName] = row.Amounts
	}

	type agg struct {
		lastTxn               *hledger.Transaction
		lastAssertion         string // "YYYY-MM-DD"; "" if never asserted
		transactionCount      int32
		lastAssertionTxnCount int32
	}
	aggs := make(map[string]*agg)
	for i := range txns {
		t := &txns[i]
		seen := make(map[string]bool)
		for _, p := range t.Postings {
			if _, ok := accountType[p.Account]; !ok {
				continue
			}
			a := aggs[p.Account]
			if a == nil {
				a = &agg{}
				aggs[p.Account] = a
			}
			// Transactions arrive in ascending date/file order; track the most
			// recent transaction touching each account (compare dates so we are
			// resilient to unsorted input).
			if !seen[p.Account] {
				a.transactionCount++
				if a.lastTxn == nil || t.Date >= a.lastTxn.Date {
					a.lastTxn = t
				}
				seen[p.Account] = true
			}
			// Any assertion form (=, =*, ==, ==*) counts as "asserted".
			if p.BalanceAssertion != nil && t.Date >= a.lastAssertion {
				a.lastAssertion = t.Date
				a.lastAssertionTxnCount = a.transactionCount
			}
		}
	}

	statuses := make([]*floatv1.AccountAssertionStatus, 0, len(aggs))
	for account, a := range aggs {
		if a.lastTxn == nil {
			continue
		}
		transactionsSinceLastAssertion := a.transactionCount
		if a.lastAssertion != "" {
			transactionsSinceLastAssertion -= a.lastAssertionTxnCount
		}
		s := &floatv1.AccountAssertionStatus{
			Account:                        account,
			Type:                           string(accountType[account]),
			Balance:                        aggregateAmountsByCommodity(balByAccount[account]),
			LastTransaction:                toProtoTransaction(*a.lastTxn),
			TransactionsSinceLastAssertion: transactionsSinceLastAssertion,
		}
		if a.lastAssertion != "" {
			s.LastAssertionDate = &a.lastAssertion
		}
		statuses = append(statuses, s)
	}

	sort.Slice(statuses, func(i, j int) bool {
		ti := statuses[i].GetTransactionsSinceLastAssertion()
		tj := statuses[j].GetTransactionsSinceLastAssertion()
		if ti != tj {
			return ti > tj
		}
		return statuses[i].Account < statuses[j].Account
	})

	return connect.NewResponse(&floatv1.GetBalanceAssertionStatusResponse{Accounts: statuses}), nil
}

func (h *Handler) ListTags(ctx context.Context, req *connect.Request[floatv1.ListTagsRequest]) (*connect.Response[floatv1.ListTagsResponse], error) {
	all, err := cachedTags(ctx, h.cache, h.hl)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger tags failed")
	}
	tags := make([]string, 0, len(all))
	for _, t := range all {
		if !strings.HasPrefix(t, hledger.HiddenMetaPrefix) {
			tags = append(tags, t)
		}
	}
	return connect.NewResponse(&floatv1.ListTagsResponse{Tags: tags}), nil
}

func (h *Handler) ListPayees(ctx context.Context, req *connect.Request[floatv1.ListPayeesRequest]) (*connect.Response[floatv1.ListPayeesResponse], error) {
	payees, err := cachedPayees(ctx, h.cache, h.hl)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger payees failed")
	}
	return connect.NewResponse(&floatv1.ListPayeesResponse{Payees: payees}), nil
}

func (h *Handler) DeleteTransaction(ctx context.Context, req *connect.Request[floatv1.DeleteTransactionRequest]) (*connect.Response[floatv1.DeleteTransactionResponse], error) {
	fid := req.Msg.Fid
	if fid == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("fid is required"))
	}
	err := h.lock.Do(ctx, fmt.Sprintf("delete transaction %s", fid), func() error {
		return journal.DeleteTransaction(ctx, h.hl, h.dataDir, fid)
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, rpcErr(ctx, err, "delete transaction failed", "fid", fid)
	}
	return connect.NewResponse(&floatv1.DeleteTransactionResponse{}), nil
}

func (h *Handler) ModifyTags(ctx context.Context, req *connect.Request[floatv1.ModifyTagsRequest]) (*connect.Response[floatv1.ModifyTagsResponse], error) {
	fid := req.Msg.Fid
	if fid == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("fid is required"))
	}
	err := h.lock.Do(ctx, fmt.Sprintf("modify tags on transaction %s", fid), func() error {
		return journal.ModifyTags(ctx, h.hl, h.dataDir, fid, req.Msg.Tags)
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, rpcErr(ctx, err, "modify tags failed", "fid", fid)
	}
	return connect.NewResponse(&floatv1.ModifyTagsResponse{}), nil
}

func (h *Handler) UpdateTransactionDate(ctx context.Context, req *connect.Request[floatv1.UpdateTransactionDateRequest]) (*connect.Response[floatv1.UpdateTransactionDateResponse], error) {
	fid := req.Msg.Fid
	if fid == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("fid is required"))
	}
	if req.Msg.NewDate == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("new_date is required"))
	}
	var updated hledger.Transaction
	err := h.lock.Do(ctx, fmt.Sprintf("update date on transaction %s", fid), func() error {
		var e error
		updated, e = journal.UpdateTransactionDate(ctx, h.hl, h.dataDir, fid, req.Msg.NewDate)
		return e
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		if errors.Is(err, journal.ErrInvalidDate) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, rpcErr(ctx, err, "update transaction date failed", "fid", fid)
	}
	return connect.NewResponse(&floatv1.UpdateTransactionDateResponse{
		Transaction: toProtoTransaction(updated),
	}), nil
}

func (h *Handler) AddTransaction(ctx context.Context, req *connect.Request[floatv1.AddTransactionRequest]) (*connect.Response[floatv1.AddTransactionResponse], error) {
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
			Account:          p.Account,
			Commodity:        p.Commodity,
			Quantity:         p.Quantity,
			Comment:          p.Comment,
			Cost:             protoToJournalCost(p.Cost),
			BalanceAssertion: protoToJournalAssertion(p.BalanceAssertion),
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
		Tags:        req.Msg.Tags,
		Postings:    postings,
		Status:      "Cleared",
	}

	var fid string
	err := h.lock.Do(ctx, fmt.Sprintf("add transaction: %s", desc), func() error {
		var e error
		fid, e = journal.AppendTransaction(ctx, h.hl, h.dataDir, tx)
		return e
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "add transaction failed")
	}

	txns, err := h.hl.Transactions(ctx, "code:"+fid)
	if err != nil {
		return nil, rpcErr(ctx, err, "fetch new transaction failed", "fid", fid)
	}
	if len(txns) == 0 {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("transaction %s not found after add", fid))
	}
	return connect.NewResponse(&floatv1.AddTransactionResponse{
		Transaction: toProtoTransaction(txns[0]),
	}), nil
}

func (h *Handler) AddTransactions(ctx context.Context, req *connect.Request[floatv1.AddTransactionsRequest]) (*connect.Response[floatv1.AddTransactionsResponse], error) {
	if len(req.Msg.Transactions) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("at least one transaction is required"))
	}

	inputs := make([]journal.TransactionInput, 0, len(req.Msg.Transactions))
	for i, t := range req.Msg.Transactions {
		if t.Description == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("transaction %d: description is required", i))
		}
		if len(t.Postings) < 2 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("transaction %d: at least 2 postings are required", i))
		}
		for j, p := range t.Postings {
			if p.Account == "" {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("transaction %d posting %d: account is required", i, j))
			}
		}

		var date time.Time
		if t.Date == "" {
			date = time.Now().UTC().Truncate(24 * time.Hour)
		} else {
			var err error
			date, err = time.Parse("2006-01-02", t.Date)
			if err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("transaction %d: invalid date %q: must be YYYY-MM-DD", i, t.Date))
			}
		}

		postings := make([]journal.PostingInput, len(t.Postings))
		for j, p := range t.Postings {
			postings[j] = journal.PostingInput{
				Account:          p.Account,
				Commodity:        p.Commodity,
				Quantity:         p.Quantity,
				Comment:          p.Comment,
				Cost:             protoToJournalCost(p.Cost),
				BalanceAssertion: protoToJournalAssertion(p.BalanceAssertion),
			}
		}
		desc := t.Description
		if t.Payee != "" {
			desc = t.Payee + " | " + desc
		}
		inputs = append(inputs, journal.TransactionInput{
			Date:        date,
			Description: desc,
			Comment:     t.Comment,
			Tags:        t.Tags,
			Postings:    postings,
			Status:      "Cleared",
		})
	}

	fids := make([]string, 0, len(inputs))
	err := h.lock.Do(ctx, fmt.Sprintf("add %d transactions", len(inputs)), func() error {
		for _, input := range inputs {
			fid, e := journal.AppendTransaction(ctx, h.hl, h.dataDir, input)
			if e != nil {
				return e
			}
			fids = append(fids, fid)
		}
		return nil
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "add transactions failed")
	}

	txns, err := h.hl.Transactions(ctx, journal.BuildFIDQuery(fids))
	if err != nil {
		return nil, rpcErr(ctx, err, "fetch new transactions failed")
	}
	byFID := make(map[string]hledger.Transaction, len(txns))
	for _, t := range txns {
		byFID[t.FID] = t
	}
	result := make([]*floatv1.Transaction, 0, len(fids))
	for _, fid := range fids {
		if t, ok := byFID[fid]; ok {
			result = append(result, toProtoTransaction(t))
		}
	}
	return connect.NewResponse(&floatv1.AddTransactionsResponse{
		Transactions: result,
	}), nil
}

func (h *Handler) UpdateTransaction(ctx context.Context, req *connect.Request[floatv1.UpdateTransactionRequest]) (*connect.Response[floatv1.UpdateTransactionResponse], error) {
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
			Account:          p.Account,
			Commodity:        p.Commodity,
			Quantity:         p.Quantity,
			Comment:          p.Comment,
			Cost:             protoToJournalCost(p.Cost),
			BalanceAssertion: protoToJournalAssertion(p.BalanceAssertion),
		}
	}

	desc := req.Msg.Description
	if req.Msg.Payee != "" {
		desc = req.Msg.Payee + " | " + desc
	}

	var updated hledger.Transaction
	err := h.lock.Do(ctx, fmt.Sprintf("update transaction %s", fid), func() error {
		var e error
		updated, e = journal.UpdateTransaction(ctx, h.hl, h.dataDir, fid, desc, req.Msg.Date, req.Msg.Comment, req.Msg.Tags, postings, req.Msg.Status)
		return e
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		if errors.Is(err, journal.ErrInvalidDate) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, rpcErr(ctx, err, "update transaction failed", "fid", fid)
	}
	return connect.NewResponse(&floatv1.UpdateTransactionResponse{
		Transaction: toProtoTransaction(updated),
	}), nil
}

func (h *Handler) UpdateTransactionStatus(ctx context.Context, req *connect.Request[floatv1.UpdateTransactionStatusRequest]) (*connect.Response[floatv1.UpdateTransactionStatusResponse], error) {
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
	var statusMsg string
	switch req.Msg.Status {
	case "Cleared":
		statusMsg = "mark transaction " + fid + " as cleared"
	case "Pending":
		statusMsg = "mark transaction " + fid + " as pending"
	case "":
		statusMsg = "unmark transaction " + fid
	}
	err := h.lock.Do(ctx, statusMsg, func() error {
		return journal.UpdateTransactionStatus(ctx, h.hl, h.dataDir, fid, req.Msg.Status)
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, rpcErr(ctx, err, "update transaction status failed", "fid", fid)
	}
	txns, err := h.hl.Transactions(ctx, "code:"+fid)
	if err != nil {
		return nil, rpcErr(ctx, err, "fetch transaction after status update failed", "fid", fid)
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
	var importBatchID *string
	if v, ok := t.FloatMeta["float-import"]; ok {
		importBatchID = &v
	}
	var stripeTxnID *string
	if v, ok := t.FloatMeta["float-stripe-txn"]; ok {
		stripeTxnID = &v
	}
	return &floatv1.Transaction{
		Fid:                 t.FID,
		Date:                t.Date,
		Description:         t.Description,
		Comment:             t.Comment,
		Postings:            postings,
		Status:              status,
		Tags:                tags,
		Payee:               t.Payee,
		Note:                t.Note,
		ImportBatchId:       importBatchID,
		StripeTransactionId: stripeTxnID,
	}
}

func toProtoPosting(p hledger.Posting) *floatv1.Posting {
	amounts := make([]*floatv1.Amount, len(p.Amounts))
	for i, a := range p.Amounts {
		amounts[i] = toProtoAmount(a)
	}
	return &floatv1.Posting{
		Account:          p.Account,
		Amounts:          amounts,
		Comment:          p.Comment,
		BalanceAssertion: toProtoBalanceAssertion(p.BalanceAssertion),
	}
}

// toProtoBalanceAssertion exposes only the simple = form via gRPC.
// Non-`=` variants (=*, ==, ==*) are preserved on disk by the journal
// package but hidden from API responses.
func toProtoBalanceAssertion(ba *hledger.BalanceAssertion) *floatv1.BalanceAssertion {
	if ba == nil || ba.Inclusive || ba.Total {
		return nil
	}
	return &floatv1.BalanceAssertion{
		Amount: toProtoAmount(ba.Amount),
	}
}

func toProtoAmount(a hledger.Amount) *floatv1.Amount {
	quantity := fmt.Sprintf("%.*f", a.Quantity.DecimalPlaces, a.Quantity.FloatingPoint)
	out := &floatv1.Amount{
		Commodity: a.Commodity,
		Quantity:  quantity,
	}
	if cost, err := a.ParseCost(); err == nil && cost != nil {
		out.Cost = &floatv1.Cost{
			Commodity: cost.Contents.Commodity,
			Quantity:  fmt.Sprintf("%.*f", cost.Contents.Quantity.DecimalPlaces, cost.Contents.Quantity.FloatingPoint),
			IsTotal:   cost.Tag == "TotalCost",
		}
	}
	return out
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
	tags := make(map[string]string, len(r.Transaction.Tags))
	for _, kv := range r.Transaction.Tags {
		if !strings.HasPrefix(kv[0], hledger.HiddenMetaPrefix) {
			tags[kv[0]] = kv[1]
		}
	}
	var stripeTxnID *string
	if v, ok := r.Transaction.FloatMeta["float-stripe-txn"]; ok {
		stripeTxnID = &v
	}
	row := &floatv1.AccountRegisterRow{
		Fid:                 r.Transaction.FID,
		Date:                r.Transaction.Date,
		Description:         r.Transaction.Description,
		Status:              status,
		OtherAccounts:       append([]string(nil), r.OtherAccounts...),
		Change:              change,
		RunningTotal:        balance,
		Tags:                tags,
		StripeTransactionId: stripeTxnID,
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

// aggregateAmountsByCommodity sums amounts that share the same commodity.
// Zero-balance commodities are omitted from the result.
func aggregateAmountsByCommodity(amounts []hledger.Amount) []*floatv1.Amount {
	totals := make(map[string]exactQty)
	var order []string
	for _, a := range amounts {
		if _, seen := totals[a.Commodity]; !seen {
			order = append(order, a.Commodity)
		}
		cur := totals[a.Commodity]
		cur.add(a.Quantity.DecimalMantissa, a.Quantity.DecimalPlaces)
		totals[a.Commodity] = cur
	}
	var result []*floatv1.Amount
	for _, commodity := range order {
		eq := totals[commodity]
		if eq.mantissa == 0 {
			continue
		}
		result = append(result, &floatv1.Amount{
			Commodity: commodity,
			Quantity:  fmt.Sprintf("%g", eq.float()),
		})
	}
	return result
}

func toProtoAccountDeclaration(d journal.AccountDeclaration) *floatv1.AccountDeclaration {
	return &floatv1.AccountDeclaration{
		Name: d.Name,
	}
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
		return nil, rpcErr(ctx, err, "list prices failed")
	}
	out := make([]*floatv1.PriceDirective, len(prices))
	for i, p := range prices {
		out[i] = toProtoPriceDirective(p)
	}
	return connect.NewResponse(&floatv1.ListPricesResponse{Prices: out}), nil
}

func (h *Handler) AddPrice(ctx context.Context, req *connect.Request[floatv1.AddPriceRequest]) (*connect.Response[floatv1.AddPriceResponse], error) {
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
	err := h.lock.Do(ctx, fmt.Sprintf("add price: %s %s @ %s %s", date, req.Msg.Commodity, req.Msg.Quantity, req.Msg.Currency), func() error {
		var e error
		pid, e = journal.AppendPrice(h.dataDir, date, req.Msg.Commodity, req.Msg.Quantity, req.Msg.Currency)
		return e
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "add price failed")
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
	if req.Msg.Pid == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("pid is required"))
	}
	err := h.lock.Do(ctx, fmt.Sprintf("delete price %s", req.Msg.Pid), func() error {
		return journal.DeletePrice(h.dataDir, req.Msg.Pid)
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, rpcErr(ctx, err, "delete price failed", "pid", req.Msg.Pid)
	}
	return connect.NewResponse(&floatv1.DeletePriceResponse{}), nil
}

func (h *Handler) BackfillPrices(ctx context.Context, req *connect.Request[floatv1.BackfillPricesRequest]) (*connect.Response[floatv1.BackfillPricesResponse], error) {

	commodity := req.Msg.Commodity
	startDate := req.Msg.StartDate
	endDate := req.Msg.EndDate
	if commodity == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("commodity is required"))
	}
	if startDate == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("start_date is required"))
	}
	if endDate == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("end_date is required"))
	}
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid start_date %q: %w", startDate, err))
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid end_date %q: %w", endDate, err))
	}
	if end.Before(start) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("end_date must not be before start_date"))
	}

	if h.cfg == nil || h.cfg.AlphaVantage.APIKey == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("alpha_vantage.api_key is not configured"))
	}

	av := alphavantage.NewClient(h.cfg.AlphaVantage.APIKey)
	weeklyPrices, err := av.FetchWeeklyPrices(ctx, commodity, startDate, endDate)
	if err != nil {
		return nil, rpcErr(ctx, err, "backfill prices: fetch failed", "commodity", commodity)
	}

	existing, err := journal.ListPrices(h.dataDir)
	if err != nil {
		return nil, rpcErr(ctx, err, "backfill prices: list existing failed")
	}
	skip := make(map[string]struct{}, len(existing))
	for _, p := range existing {
		if p.Commodity == commodity {
			skip[p.Date] = struct{}{}
		}
	}

	var toWrite []alphavantage.WeeklyPrice
	for _, wp := range weeklyPrices {
		if _, exists := skip[wp.Date]; !exists {
			toWrite = append(toWrite, wp)
		}
	}

	skippedCount := int32(len(weeklyPrices) - len(toWrite))
	if len(toWrite) == 0 {
		return connect.NewResponse(&floatv1.BackfillPricesResponse{SkippedCount: skippedCount}), nil
	}

	var added []journal.Price
	if err := h.lock.Do(ctx, fmt.Sprintf("backfill prices: %s %s to %s", commodity, startDate, endDate), func() error {
		for _, wp := range toWrite {
			pid, e := journal.AppendPrice(h.dataDir, wp.Date, commodity, wp.Close, wp.Currency)
			if e != nil {
				return e
			}
			added = append(added, journal.Price{PID: pid, Date: wp.Date, Commodity: commodity, Quantity: wp.Close, Currency: wp.Currency})
		}
		return nil
	}); err != nil {
		return nil, rpcErr(ctx, err, "backfill prices: write failed", "commodity", commodity)
	}

	out := make([]*floatv1.PriceDirective, len(added))
	for i, p := range added {
		out[i] = toProtoPriceDirective(p)
	}
	return connect.NewResponse(&floatv1.BackfillPricesResponse{
		Prices:       out,
		SkippedCount: skippedCount,
	}), nil
}

func (h *Handler) GeneratePricesFromCost(ctx context.Context, req *connect.Request[floatv1.GeneratePricesFromCostRequest]) (*connect.Response[floatv1.GeneratePricesFromCostResponse], error) {
	prefix := req.Msg.AccountPrefix
	if prefix == "" {
		prefix = "assets"
	}

	existing, err := journal.ListPrices(h.dataDir)
	if err != nil {
		return nil, rpcErr(ctx, err, "generate prices from cost: list existing failed")
	}
	// Track the most recent price date per commodity. An empty string means no
	// existing prices, so all purchase transactions qualify. A non-empty date
	// means only transactions strictly newer than that date qualify (catching up
	// after prices have gone stale).
	latestPriceDateByCommodity := make(map[string]string, len(existing))
	for _, p := range existing {
		if p.Date > latestPriceDateByCommodity[p.Commodity] {
			latestPriceDateByCommodity[p.Commodity] = p.Date
		}
	}

	txns, err := cachedTransactions(ctx, h.cache, h.hl, []string{prefix})
	if err != nil {
		return nil, rpcErr(ctx, err, "generate prices from cost: load transactions failed")
	}

	seen := make(map[string]map[string]bool)
	var toWrite []struct {
		date      string
		commodity string
		unitPrice float64
		currency  string
	}

	for _, txn := range txns {
		for _, p := range txn.Postings {
			for _, amt := range p.Amounts {
				if currencySymbols[amt.Commodity] {
					continue
				}
				c, cerr := amt.ParseCost()
				if cerr != nil || c == nil {
					continue
				}
				var unitPrice float64
				switch c.Tag {
				case "UnitCost":
					unitPrice = c.Contents.Quantity.FloatingPoint
				case "TotalCost":
					if q := amt.Quantity.FloatingPoint; q != 0 {
						unitPrice = c.Contents.Quantity.FloatingPoint / math.Abs(q)
					}
				}
				if unitPrice <= 0 {
					continue
				}
				date := txn.Date
				if p.Date != nil && *p.Date != "" {
					date = *p.Date
				}
				if date <= latestPriceDateByCommodity[amt.Commodity] {
					continue
				}
				if _, ok := seen[amt.Commodity]; !ok {
					seen[amt.Commodity] = make(map[string]bool)
				}
				if seen[amt.Commodity][date] {
					continue
				}
				seen[amt.Commodity][date] = true
				toWrite = append(toWrite, struct {
					date      string
					commodity string
					unitPrice float64
					currency  string
				}{date, amt.Commodity, unitPrice, c.Contents.Commodity})
			}
		}
	}

	// Count commodities with existing prices that produced no new entries
	// (their prices are already up to date relative to recent purchases).
	generatedCommodities := make(map[string]bool)
	for _, pw := range toWrite {
		generatedCommodities[pw.commodity] = true
	}
	var skippedCommodities int32
	for commodity, latestDate := range latestPriceDateByCommodity {
		if latestDate != "" && !generatedCommodities[commodity] {
			skippedCommodities++
		}
	}

	if len(toWrite) == 0 {
		return connect.NewResponse(&floatv1.GeneratePricesFromCostResponse{
			SkippedCommodities: skippedCommodities,
		}), nil
	}

	sort.Slice(toWrite, func(i, j int) bool {
		if toWrite[i].commodity != toWrite[j].commodity {
			return toWrite[i].commodity < toWrite[j].commodity
		}
		return toWrite[i].date < toWrite[j].date
	})

	var added []journal.Price
	if err := h.lock.Do(ctx, "generate prices from cost", func() error {
		for _, pw := range toWrite {
			pid, e := journal.AppendPrice(h.dataDir, pw.date, pw.commodity, fmt.Sprintf("%.2f", pw.unitPrice), pw.currency)
			if e != nil {
				return e
			}
			added = append(added, journal.Price{PID: pid, Date: pw.date, Commodity: pw.commodity, Quantity: fmt.Sprintf("%.2f", pw.unitPrice), Currency: pw.currency})
		}
		return nil
	}); err != nil {
		return nil, rpcErr(ctx, err, "generate prices from cost: write failed")
	}

	out := make([]*floatv1.PriceDirective, len(added))
	for i, p := range added {
		out[i] = toProtoPriceDirective(p)
	}
	return connect.NewResponse(&floatv1.GeneratePricesFromCostResponse{
		AddedPrices:        out,
		SkippedCommodities: skippedCommodities,
	}), nil
}

func (h *Handler) ListAccountDeclarations(ctx context.Context, _ *connect.Request[floatv1.ListAccountDeclarationsRequest]) (*connect.Response[floatv1.ListAccountDeclarationsResponse], error) {
	decls, err := journal.ListAccountDeclarations(h.dataDir)
	if err != nil {
		return nil, rpcErr(ctx, err, "list account declarations failed")
	}
	unused, err := cachedUnusedAccounts(ctx, h.cache, h.hl)
	if err != nil {
		return nil, rpcErr(ctx, err, "unused accounts failed")
	}
	unusedSet := make(map[string]bool, len(unused))
	for _, name := range unused {
		unusedSet[name] = true
	}
	out := make([]*floatv1.AccountDeclaration, len(decls))
	for i, d := range decls {
		pd := toProtoAccountDeclaration(d)
		pd.HasPostings = !unusedSet[d.Name]
		out[i] = pd
	}
	return connect.NewResponse(&floatv1.ListAccountDeclarationsResponse{Declarations: out}), nil
}

func (h *Handler) DeclareAccount(ctx context.Context, req *connect.Request[floatv1.DeclareAccountRequest]) (*connect.Response[floatv1.DeclareAccountResponse], error) {
	name := strings.TrimSpace(req.Msg.Name)
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	err := h.lock.Do(ctx, fmt.Sprintf("declare account: %s", name), func() error {
		return journal.AppendAccountDeclaration(h.dataDir, name)
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "declare account failed", "name", name)
	}
	return connect.NewResponse(&floatv1.DeclareAccountResponse{
		Declaration: toProtoAccountDeclaration(journal.AccountDeclaration{Name: name}),
	}), nil
}

func (h *Handler) DeleteAccountDeclaration(ctx context.Context, req *connect.Request[floatv1.DeleteAccountDeclarationRequest]) (*connect.Response[floatv1.DeleteAccountDeclarationResponse], error) {
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	err := h.lock.Do(ctx, fmt.Sprintf("delete account declaration %s", req.Msg.Name), func() error {
		return journal.DeleteAccountDeclaration(h.dataDir, req.Msg.Name)
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, rpcErr(ctx, err, "delete account declaration failed", "name", req.Msg.Name)
	}
	return connect.NewResponse(&floatv1.DeleteAccountDeclarationResponse{}), nil
}

func (h *Handler) RenameAccount(ctx context.Context, req *connect.Request[floatv1.RenameAccountRequest]) (*connect.Response[floatv1.RenameAccountResponse], error) {
	oldName := strings.TrimSpace(req.Msg.OldName)
	newName := strings.TrimSpace(req.Msg.NewName)
	if oldName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("old_name is required"))
	}
	if newName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("new_name is required"))
	}
	if oldName == newName {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("old_name and new_name must differ"))
	}

	var renamed int
	err := h.lock.Do(ctx, fmt.Sprintf("rename account: %s → %s", oldName, newName), func() error {
		// Rename in accounts.journal if the declaration exists there.
		if declErr := journal.RenameAccountDeclaration(h.dataDir, oldName, newName); declErr != nil {
			if !errors.Is(declErr, journal.ErrNotFound) {
				return declErr
			}
		}
		// Rename in all transaction (YYYY/MM) journal files.
		n, err := journal.RenameAccountInJournalFiles(h.dataDir, oldName, newName)
		if err != nil {
			return err
		}
		renamed = n
		return nil
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "rename account failed", "old", oldName, "new", newName)
	}
	return connect.NewResponse(&floatv1.RenameAccountResponse{PostingsRenamed: int32(renamed)}), nil
}

func (h *Handler) BulkEditTransactions(ctx context.Context, req *connect.Request[floatv1.BulkEditTransactionsRequest]) (*connect.Response[floatv1.BulkEditTransactionsResponse], error) {

	if len(req.Msg.Fids) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("fids must not be empty"))
	}
	if len(req.Msg.Operations) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("operations must not be empty"))
	}
	deleteOp := false
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
		case *floatv1.BulkEditOperation_UpdateUnknownAccount:
			if v.UpdateUnknownAccount.Account == "" {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("operation %d: update_unknown_account account must not be empty", i))
			}
		case *floatv1.BulkEditOperation_Delete:
			deleteOp = true
		default:
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("operation %d: unrecognized or missing operation type", i))
		}
	}
	if deleteOp {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("use BulkDeleteTransactions for bulk delete operations"))
	}

	err := h.lock.Do(ctx, bulkEditMessage(req.Msg.Fids, req.Msg.Operations), func() error {
		for _, fid := range req.Msg.Fids {

			txns, err := h.hl.Transactions(ctx, "code:"+fid)
			if err != nil {
				return fmt.Errorf("bulk-edit: lookup fid %q: %w", fid, err)
			}
			if len(txns) == 0 {
				return fmt.Errorf("bulk-edit: no transaction found with fid %q: %w", fid, journal.ErrNotFound)
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
					note := t.Description // no "|": preserve full description as note
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
				case *floatv1.BulkEditOperation_UpdateUnknownAccount:
					newAcct := v.UpdateUnknownAccount.Account
					for i, p := range input.Postings {
						if strings.Contains(strings.ToLower(p.Account), "unknown") {
							input.Postings[i].Account = newAcct
						}
					}
				}
			}

			if _, err := journal.WriteTransaction(ctx, h.hl, h.dataDir, input, src); err != nil {
				return fmt.Errorf("bulk-edit: fid %q: write: %w", fid, err)
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, rpcErr(ctx, err, "bulk edit transactions failed")
	}

	editedTxns, err := h.hl.Transactions(ctx, journal.BuildFIDQuery(req.Msg.Fids))
	if err != nil {
		return nil, rpcErr(ctx, err, "bulk edit: fetch after update failed")
	}
	editedByFID := make(map[string]*floatv1.Transaction, len(editedTxns))
	for _, t := range editedTxns {
		editedByFID[t.FID] = toProtoTransaction(t)
	}
	results := make([]*floatv1.Transaction, 0, len(req.Msg.Fids))
	for _, fid := range req.Msg.Fids {
		t, ok := editedByFID[fid]
		if !ok {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("transaction %s not found after bulk edit", fid))
		}
		results = append(results, t)
	}
	return connect.NewResponse(&floatv1.BulkEditTransactionsResponse{Transactions: results}), nil
}

func (h *Handler) BulkDeleteTransactions(
	ctx context.Context,
	req *connect.Request[floatv1.BulkDeleteTransactionsRequest],
	stream *connect.ServerStream[floatv1.BulkDeleteTransactionsResponse],
) error {
	if len(req.Msg.Fids) == 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("fids must not be empty"))
	}

	total := int32(len(req.Msg.Fids))
	var deleted int32

	err := h.lock.Do(ctx, fmt.Sprintf("delete %d transactions", len(req.Msg.Fids)), func() error {
		return journal.BatchDeleteTransactions(ctx, h.hl, req.Msg.Fids, func(d, _ int32) {
			deleted = d
			_ = stream.Send(&floatv1.BulkDeleteTransactionsResponse{
				Payload: &floatv1.BulkDeleteTransactionsResponse_Progress{
					Progress: &floatv1.BulkDeleteTransactionsProgress{
						Deleted: deleted,
						Total:   total,
					},
				},
			})
		})
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return connect.NewError(connect.CodeNotFound, err)
		}
		return rpcErr(ctx, err, "bulk delete transactions failed")
	}

	return stream.Send(&floatv1.BulkDeleteTransactionsResponse{
		Payload: &floatv1.BulkDeleteTransactionsResponse_Result{
			Result: &floatv1.BulkDeleteTransactionsResult{DeletedCount: deleted},
		},
	})
}

func (h *Handler) ListSnapshots(ctx context.Context, req *connect.Request[floatv1.ListSnapshotsRequest]) (*connect.Response[floatv1.ListSnapshotsResponse], error) {
	if h.snap == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("snapshots not enabled"))
	}
	snaps, err := h.snap.List(ctx, int(req.Msg.Limit))
	if err != nil {
		return nil, rpcErr(ctx, err, "list snapshots failed")
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
		return nil, rpcErr(ctx, err, "restore snapshot failed", "hash", req.Msg.Hash)
	}
	h.lock.BumpGeneration()
	return connect.NewResponse(&floatv1.RestoreSnapshotResponse{}), nil
}

func (h *Handler) GetSnapshotDiff(ctx context.Context, req *connect.Request[floatv1.GetSnapshotDiffRequest]) (*connect.Response[floatv1.GetSnapshotDiffResponse], error) {
	if h.snap == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("snapshots not enabled"))
	}
	if req.Msg.Hash == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("hash is required"))
	}
	files, err := h.snap.Diff(ctx, req.Msg.Hash)
	if err != nil {
		return nil, rpcErr(ctx, err, "snapshot diff failed", "hash", req.Msg.Hash)
	}
	out := make([]*floatv1.FileDiff, len(files))
	for i, f := range files {
		out[i] = &floatv1.FileDiff{
			Path:       f.Path,
			OldPath:    f.OldPath,
			ChangeType: changeTypeString(f.Change),
			IsBinary:   f.IsBinary,
			Patch:      f.Patch,
		}
	}
	return connect.NewResponse(&floatv1.GetSnapshotDiffResponse{Hash: req.Msg.Hash, Files: out}), nil
}

func changeTypeString(c gitsnap.ChangeType) string {
	switch c {
	case gitsnap.ChangeAdded:
		return "added"
	case gitsnap.ChangeDeleted:
		return "deleted"
	case gitsnap.ChangeRenamed:
		return "renamed"
	default:
		return "modified"
	}
}

// ---- Import handlers ----

func (h *Handler) ListBankProfiles(_ context.Context, _ *connect.Request[floatv1.ListBankProfilesRequest]) (*connect.Response[floatv1.ListBankProfilesResponse], error) {
	if h.cfg == nil {
		return connect.NewResponse(&floatv1.ListBankProfilesResponse{}), nil
	}
	out := make([]*floatv1.BankProfile, len(h.cfg.BankProfiles))
	for i, p := range h.cfg.BankProfiles {
		out[i] = &floatv1.BankProfile{Name: p.Name, RulesFile: p.RulesFile, SkipRules: p.SkipRules}
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

	newProfile := config.BankProfile{Name: req.Msg.Name, RulesFile: cleaned, SkipRules: req.Msg.SkipRules}
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
		return nil, rpcErr(ctx, err, "create bank profile failed")
	}

	slogctx.FromContext(ctx).InfoContext(ctx, "created bank profile", "name", req.Msg.Name, "rules_file", cleaned)
	return connect.NewResponse(&floatv1.CreateBankProfileResponse{
		Profile: &floatv1.BankProfile{Name: newProfile.Name, RulesFile: newProfile.RulesFile, SkipRules: newProfile.SkipRules},
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
			return nil, rpcErr(ctx, err, "read rules file failed")
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
			return fmt.Errorf("bank profile %q: %w", req.Msg.Name, journal.ErrNotFound)
		}

		profile := h.cfg.BankProfiles[idx]

		if len(req.Msg.RulesContent) > 0 {
			rulesPath := filepath.Join(h.dataDir, profile.RulesFile)
			if err := os.WriteFile(rulesPath, req.Msg.RulesContent, 0o644); err != nil {
				return fmt.Errorf("write rules file: %w", err)
			}
		}

		h.cfg.BankProfiles[idx].Name = newName
		if req.Msg.SkipRules != nil {
			h.cfg.BankProfiles[idx].SkipRules = *req.Msg.SkipRules
		}
		updated = h.cfg.BankProfiles[idx]

		if err := config.Save(h.configPath, h.cfg); err != nil {
			h.cfg.BankProfiles[idx].Name = req.Msg.Name // rollback
			return fmt.Errorf("save config: %w", err)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, rpcErr(ctx, err, "update bank profile failed")
	}

	slogctx.FromContext(ctx).InfoContext(ctx, "updated bank profile", "name", updated.Name)
	return connect.NewResponse(&floatv1.UpdateBankProfileResponse{
		Profile: &floatv1.BankProfile{Name: updated.Name, RulesFile: updated.RulesFile, SkipRules: updated.SkipRules},
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
			return fmt.Errorf("bank profile %q: %w", req.Msg.Name, journal.ErrNotFound)
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
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, rpcErr(ctx, err, "delete bank profile failed")
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
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(req.Msg.CsvData); err != nil {
		_ = tmp.Close()
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("write temp file: %w", err))
	}
	_ = tmp.Close()

	// Parse CSV with hledger.
	candidates, err := h.hl.PrintCSV(ctx, tmp.Name(), rulesFile)
	if err != nil {
		logger.ErrorContext(ctx, "hledger PrintCSV failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("parse CSV: %w", err))
	}

	// Build fingerprint set from existing transactions.
	existing, err := h.hl.Transactions(ctx)
	if err != nil {
		return nil, rpcErr(ctx, err, "fetch existing transactions failed")
	}
	fpSet := make(map[string]bool, len(existing))
	for _, t := range existing {
		fpSet[journal.TxnFingerprint(t)] = true
	}

	// Load float rules for second-pass categorization.
	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		return nil, rpcErr(ctx, err, "load rules failed")
	}

	out := make([]*floatv1.ImportCandidate, len(candidates))
	for i, c := range candidates {
		candidate := &floatv1.ImportCandidate{}
		if !profile.SkipRules {
			if r := rules.Match(rulesList, c.Description, sourceAccountFromPostings(c.Postings)); r != nil {
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
		}
		// Fingerprint after rules so it matches the form written to disk by ImportTransactions.
		candidate.IsDuplicate = fpSet[journal.TxnFingerprint(c)]
		candidate.Transaction = toProtoTransaction(c)
		out[i] = candidate
	}
	return connect.NewResponse(&floatv1.PreviewImportResponse{Candidates: out}), nil
}

func (h *Handler) ImportTransactions(ctx context.Context, req *connect.Request[floatv1.ImportTransactionsRequest], stream *connect.ServerStream[floatv1.ImportTransactionsResponse]) error {
	logger := slogctx.FromContext(ctx)
	if len(req.Msg.CsvData) == 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("csv_data is required"))
	}
	if req.Msg.ProfileName == "" {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("profile_name is required"))
	}
	if len(req.Msg.CandidateIndices) == 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("candidate_indices must not be empty"))
	}

	profile, err := h.bankProfile(req.Msg.ProfileName)
	if err != nil {
		return connect.NewError(connect.CodeNotFound, err)
	}
	rulesFile := filepath.Join(h.dataDir, profile.RulesFile)

	// Write CSV to temp file.
	tmp, err := os.CreateTemp("", "float-import-*.csv")
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("create temp file: %w", err))
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(req.Msg.CsvData); err != nil {
		_ = tmp.Close()
		return connect.NewError(connect.CodeInternal, fmt.Errorf("write temp file: %w", err))
	}
	_ = tmp.Close()

	candidates, err := h.hl.PrintCSV(ctx, tmp.Name(), rulesFile)
	if err != nil {
		logger.ErrorContext(ctx, "hledger PrintCSV failed", "error", err)
		return connect.NewError(connect.CodeInternal, fmt.Errorf("parse CSV: %w", err))
	}

	// Load rules for categorization during import.
	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		return rpcErr(ctx, err, "load rules failed")
	}

	// Build selected indices set.
	selectedSet := make(map[int32]bool, len(req.Msg.CandidateIndices))
	for _, idx := range req.Msg.CandidateIndices {
		selectedSet[idx] = true
	}

	importBatchID := profileToSlug(req.Msg.ProfileName) + "/" + time.Now().Format("2006-01-02") + "-" + journal.MintFID()
	total := int32(len(req.Msg.CandidateIndices))

	var importedFIDs []string
	err = h.lock.Do(ctx, fmt.Sprintf("import %d transactions (batch %s)", len(req.Msg.CandidateIndices), importBatchID), func() error {
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
			if !profile.SkipRules {
				if r := rules.Match(rulesList, c.Description, sourceAccountFromPostings(c.Postings)); r != nil {
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

		// Save a copy of the uploaded CSV inside lock.Do so it is included in the git commit.
		// importBatchID may contain a "/" (profile-slug/date-fid), so we create the subdirectory.
		uploadsDir := filepath.Join(h.dataDir, "uploads")
		uploadFilePath := filepath.Join(uploadsDir, filepath.FromSlash(importBatchID+".csv"))
		if mkErr := os.MkdirAll(filepath.Dir(uploadFilePath), 0o755); mkErr != nil {
			logger.ErrorContext(ctx, "create uploads dir failed", "error", mkErr)
		} else if wErr := os.WriteFile(uploadFilePath, req.Msg.CsvData, 0o644); wErr != nil {
			logger.ErrorContext(ctx, "save uploaded CSV failed", "error", wErr)
		}
		return nil
	})
	if err != nil {
		return rpcErr(ctx, err, "import transactions failed")
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

func (h *Handler) GetImportedTransactions(ctx context.Context, req *connect.Request[floatv1.GetImportedTransactionsRequest]) (*connect.Response[floatv1.ListTransactionsResponse], error) {
	if req.Msg.ImportBatchId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("import_batch_id is required"))
	}
	query := []string{"tag:float-import=" + req.Msg.ImportBatchId}
	txns, err := cachedTransactions(ctx, h.cache, h.hl, query)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger transactions failed")
	}
	txns, total, hasNext := paginate(txns, req.Msg.Offset, req.Msg.Limit)
	proto := make([]*floatv1.Transaction, len(txns))
	for i, t := range txns {
		proto[i] = toProtoTransaction(t)
	}
	return connect.NewResponse(&floatv1.ListTransactionsResponse{Transactions: proto, Total: total, HasNext: hasNext}), nil
}

func (h *Handler) ListImports(ctx context.Context, _ *connect.Request[floatv1.ListImportsRequest]) (*connect.Response[floatv1.ListImportsResponse], error) {
	txns, err := cachedTransactions(ctx, h.cache, h.hl, []string{"tag:float-import"})
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger transactions failed")
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
			// Extract date from the batch ID. New format is "profile-slug/YYYY-MM-DD-fid";
			// legacy format is "YYYY-MM-DD-fid". In both cases the date is the first 10
			// characters of the last path segment.
			datePart := batchID
			if slashIdx := strings.LastIndex(batchID, "/"); slashIdx >= 0 {
				datePart = batchID[slashIdx+1:]
			}
			date := ""
			if len(datePart) >= 10 {
				date = datePart[:10]
			}
			seen[batchID] = len(batches)
			batches = append(batches, batchEntry{batchID: batchID, date: date, count: 1})
		}
	}

	// Sort descending by date, then by batch ID for stable ordering within the same date.
	sort.Slice(batches, func(i, j int) bool {
		if batches[i].date != batches[j].date {
			return batches[i].date > batches[j].date
		}
		return batches[i].batchID > batches[j].batchID
	})

	out := make([]*floatv1.ImportSummary, len(batches))
	for i, b := range batches {
		source := "csv"
		if strings.HasPrefix(b.batchID, "stripe-") {
			source = "stripe"
		}
		out[i] = &floatv1.ImportSummary{
			ImportBatchId:    b.batchID,
			Date:             b.date,
			TransactionCount: b.count,
			Source:           source,
		}
	}
	return connect.NewResponse(&floatv1.ListImportsResponse{Imports: out}), nil
}

func (h *Handler) GetImportFile(ctx context.Context, req *connect.Request[floatv1.GetImportFileRequest]) (*connect.Response[floatv1.GetImportFileResponse], error) {
	if req.Msg.ImportBatchId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("import_batch_id is required"))
	}
	uploadsDir := filepath.Clean(filepath.Join(h.dataDir, "uploads"))
	filePath := filepath.Join(uploadsDir, filepath.FromSlash(req.Msg.ImportBatchId+".csv"))
	if !strings.HasPrefix(filepath.Clean(filePath), uploadsDir+string(filepath.Separator)) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid import_batch_id"))
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("import file not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read import file: %w", err))
	}
	return connect.NewResponse(&floatv1.GetImportFileResponse{
		CsvContent: data,
		Filename:   req.Msg.ImportBatchId + ".csv",
	}), nil
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

// profileToSlug converts a bank profile name to a URL/path-safe slug.
// e.g. "Chase Checking" → "chase-checking", "My Bank (US)" → "my-bank-us"
func profileToSlug(name string) string {
	var b strings.Builder
	prevHyphen := true // skip leading hyphens
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevHyphen = false
		} else if !prevHyphen {
			b.WriteByte('-')
			prevHyphen = true
		}
	}
	s := b.String()
	return strings.TrimRight(s, "-")
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
		return nil, rpcErr(ctx, err, "list rules failed")
	}
	out := make([]*floatv1.TransactionRule, len(rulesList))
	for i, r := range rulesList {
		out[i] = toProtoRule(r)
	}
	return connect.NewResponse(&floatv1.ListRulesResponse{Rules: out}), nil
}

func (h *Handler) AddRule(ctx context.Context, req *connect.Request[floatv1.AddRuleRequest]) (*connect.Response[floatv1.AddRuleResponse], error) {
	if len(req.Msg.Rules) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("at least one rule is required"))
	}
	patterns := make([]string, len(req.Msg.Rules))
	for i, r := range req.Msg.Rules {
		if r.Pattern == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("pattern is required"))
		}
		if _, err := rules.CompilePattern(r.Pattern); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid pattern %q: %w", r.Pattern, err))
		}
		patterns[i] = r.Pattern
	}

	var newRules []rules.Rule
	err := h.lock.Do(ctx, addRuleMessage(patterns), func() error {
		rulesList, loadErr := rules.Load(h.dataDir)
		if loadErr != nil {
			return loadErr
		}
		newRules = make([]rules.Rule, len(req.Msg.Rules))
		for i, r := range req.Msg.Rules {
			newRules[i] = rules.Rule{
				ID:           journal.MintFID(),
				Pattern:      r.Pattern,
				Payee:        r.Payee,
				Account:      r.Account,
				Tags:         r.Tags,
				Priority:     int(r.Priority),
				AutoReviewed: r.AutoReviewed,
				MatchAccount: r.MatchAccount,
			}
		}
		rulesList = append(rulesList, newRules...)
		return rules.Save(h.dataDir, rulesList)
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "add rule failed")
	}
	out := make([]*floatv1.TransactionRule, len(newRules))
	for i, r := range newRules {
		out[i] = toProtoRule(r)
	}
	return connect.NewResponse(&floatv1.AddRuleResponse{Rules: out}), nil
}

const maxSnapshotDescriptionLen = 180

func addRuleMessage(patterns []string) string {
	prefix := fmt.Sprintf("add %d rule(s)", len(patterns))
	if len(patterns) == 0 {
		return prefix
	}
	full := prefix + ": " + strings.Join(patterns, ", ")
	if len(full) <= maxSnapshotDescriptionLen {
		return full
	}

	base := prefix + ": "
	remaining := maxSnapshotDescriptionLen - len(base)
	if remaining <= 0 {
		return prefix
	}

	var b strings.Builder
	used := 0
	appended := 0
	for i, pattern := range patterns {
		part := pattern
		if appended > 0 {
			part = ", " + pattern
		}
		if used+len(part) > remaining {
			left := len(patterns) - i
			if left > 0 {
				suffix := fmt.Sprintf(" ... +%d more", left)
				for appended > 0 && used+len(suffix) > remaining {
					text := b.String()
					last := strings.LastIndex(text, ", ")
					if last < 0 {
						b.Reset()
						used = 0
						break
					}
					b.Reset()
					b.WriteString(text[:last])
					used = len(text[:last])
					appended--
				}
				if used+len(suffix) <= remaining {
					b.WriteString(suffix)
				}
			}
			break
		}
		b.WriteString(part)
		used += len(part)
		appended++
	}
	if b.Len() == 0 {
		return prefix
	}
	return base + b.String()
}

func (h *Handler) UpdateRule(ctx context.Context, req *connect.Request[floatv1.UpdateRuleRequest]) (*connect.Response[floatv1.UpdateRuleResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	if req.Msg.Pattern == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("pattern is required"))
	}
	if _, err := rules.CompilePattern(req.Msg.Pattern); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid pattern %q: %w", req.Msg.Pattern, err))
	}

	var updated rules.Rule
	err := h.lock.Do(ctx, fmt.Sprintf("update rule %s", req.Msg.Id), func() error {
		rulesList, loadErr := rules.Load(h.dataDir)
		if loadErr != nil {
			return loadErr
		}
		found := false
		for i, r := range rulesList {
			if r.ID == req.Msg.Id {
				rulesList[i] = rules.Rule{
					ID:           req.Msg.Id,
					Pattern:      req.Msg.Pattern,
					Payee:        req.Msg.Payee,
					Account:      req.Msg.Account,
					Tags:         req.Msg.Tags,
					Priority:     int(req.Msg.Priority),
					AutoReviewed: req.Msg.AutoReviewed,
					MatchAccount: req.Msg.MatchAccount,
				}
				updated = rulesList[i]
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("rule %q: %w", req.Msg.Id, journal.ErrNotFound)
		}
		return rules.Save(h.dataDir, rulesList)
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, rpcErr(ctx, err, "update rule failed", "id", req.Msg.Id)
	}
	return connect.NewResponse(&floatv1.UpdateRuleResponse{Rule: toProtoRule(updated)}), nil
}

func (h *Handler) DeleteRule(ctx context.Context, req *connect.Request[floatv1.DeleteRuleRequest]) (*connect.Response[floatv1.DeleteRuleResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	err := h.lock.Do(ctx, fmt.Sprintf("delete rule %s", req.Msg.Id), func() error {
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
			return fmt.Errorf("rule %q: %w", req.Msg.Id, journal.ErrNotFound)
		}
		return rules.Save(h.dataDir, filtered)
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, rpcErr(ctx, err, "delete rule failed", "id", req.Msg.Id)
	}
	return connect.NewResponse(&floatv1.DeleteRuleResponse{}), nil
}

func (h *Handler) PreviewApplyRules(ctx context.Context, req *connect.Request[floatv1.PreviewApplyRulesRequest]) (*connect.Response[floatv1.PreviewApplyRulesResponse], error) {

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
		return nil, rpcErr(ctx, err, "fetch transactions failed")
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
		if m.Changes.MarkReviewed != nil && *m.Changes.MarkReviewed {
			p.WillMarkReviewed = true
		}
		previews[i] = p
	}
	return connect.NewResponse(&floatv1.PreviewApplyRulesResponse{Previews: previews}), nil
}

func (h *Handler) ApplyRules(ctx context.Context, req *connect.Request[floatv1.ApplyRulesRequest], stream *connect.ServerStream[floatv1.ApplyRulesResponse]) error {

	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	if len(req.Msg.RuleIds) > 0 {
		rulesList = filterRules(rulesList, req.Msg.RuleIds)
	}

	txns, err := cachedTransactions(ctx, h.cache, h.hl, req.Msg.Query)
	if err != nil {
		return rpcErr(ctx, err, "fetch transactions failed")
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

	total := int32(len(matches))
	var applied int32
	err = h.lock.Do(ctx, fmt.Sprintf("apply rules to %d transactions", len(matches)), func() error {
		return rules.ApplyBatch(ctx, h.hl, h.dataDir, matches, func(a, _ int32) {
			applied = a
			_ = stream.Send(&floatv1.ApplyRulesResponse{
				Payload: &floatv1.ApplyRulesResponse_Progress{
					Progress: &floatv1.ApplyRulesProgress{
						Applied: applied,
						Total:   total,
					},
				},
			})
		})
	})
	if err != nil {
		return rpcErr(ctx, err, "apply rules failed")
	}

	return stream.Send(&floatv1.ApplyRulesResponse{
		Payload: &floatv1.ApplyRulesResponse_Result{
			Result: &floatv1.ApplyRulesResult{AppliedCount: applied},
		},
	})
}

// sourceAccountFromPostings returns the first asset/liability account from a
// slice of postings, or "" if none is found. Used to determine the source
// (bank) account for rule account-scoping.
func sourceAccountFromPostings(postings []hledger.Posting) string {
	for _, p := range postings {
		if isAssetOrLiabilityAccount(p.Account) {
			return p.Account
		}
	}
	return ""
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

func bulkEditMessage(fids []string, ops []*floatv1.BulkEditOperation) string {
	if len(fids) == 1 && len(ops) == 1 {
		fid := fids[0]
		switch v := ops[0].Operation.(type) {
		case *floatv1.BulkEditOperation_MarkReviewed:
			if v.MarkReviewed.Reviewed {
				return "mark transaction " + fid + " as reviewed"
			}
			return "unmark transaction " + fid + " as reviewed"
		case *floatv1.BulkEditOperation_AddTag:
			return fmt.Sprintf("add tag %q to transaction %s", v.AddTag.Key, fid)
		case *floatv1.BulkEditOperation_RemoveTag:
			return fmt.Sprintf("remove tag %q from transaction %s", v.RemoveTag.Key, fid)
		case *floatv1.BulkEditOperation_SetPayee:
			return fmt.Sprintf("set payee on transaction %s", fid)
		case *floatv1.BulkEditOperation_ClearPayee:
			return "clear payee on transaction " + fid
		case *floatv1.BulkEditOperation_Delete:
			return "delete transaction " + fid
		case *floatv1.BulkEditOperation_UpdateUnknownAccount:
			return fmt.Sprintf("update unknown account to %q in transaction %s", v.UpdateUnknownAccount.Account, fid)
		}
	}
	if len(ops) == 1 {
		if _, ok := ops[0].Operation.(*floatv1.BulkEditOperation_Delete); ok {
			return fmt.Sprintf("delete %d transactions", len(fids))
		}
		if v, ok := ops[0].Operation.(*floatv1.BulkEditOperation_UpdateUnknownAccount); ok {
			return fmt.Sprintf("update unknown account to %q in %d transactions", v.UpdateUnknownAccount.Account, len(fids))
		}
	}
	return fmt.Sprintf("bulk edit %d transactions", len(fids))
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

func protoToJournalCost(c *floatv1.Cost) *journal.CostInput {
	if c == nil {
		return nil
	}
	return &journal.CostInput{
		Commodity: c.Commodity,
		Quantity:  c.Quantity,
		IsTotal:   c.IsTotal,
	}
}

// protoToJournalAssertion converts a proto BalanceAssertion to the journal
// input form. The gRPC API only exposes the simple = form, so Inclusive
// and Total are always false here.
func protoToJournalAssertion(ba *floatv1.BalanceAssertion) *journal.BalanceAssertionInput {
	if ba == nil || ba.Amount == nil {
		return nil
	}
	return &journal.BalanceAssertionInput{
		Commodity: ba.Amount.Commodity,
		Quantity:  ba.Amount.Quantity,
	}
}

func toProtoRule(r rules.Rule) *floatv1.TransactionRule {
	return &floatv1.TransactionRule{
		Id:           r.ID,
		Pattern:      r.Pattern,
		Payee:        r.Payee,
		Account:      r.Account,
		Tags:         r.Tags,
		Priority:     int32(r.Priority),
		AutoReviewed: r.AutoReviewed,
		MatchAccount: r.MatchAccount,
	}
}

// ---- Template handlers ----

func toProtoTemplate(t templates.Template) *floatv1.TransactionTemplate {
	postings := make([]*floatv1.TemplatePosting, len(t.Postings))
	for i, p := range t.Postings {
		postings[i] = &floatv1.TemplatePosting{
			Account:         p.Account,
			Commodity:       p.Commodity,
			DefaultQuantity: p.DefaultQuantity,
			Comment:         p.Comment,
		}
	}
	return &floatv1.TransactionTemplate{
		Id:       t.ID,
		Name:     t.Name,
		Payee:    t.Payee,
		Note:     t.Note,
		Postings: postings,
		Tags:     t.Tags,
	}
}

func (h *Handler) ListTemplates(ctx context.Context, _ *connect.Request[floatv1.ListTemplatesRequest]) (*connect.Response[floatv1.ListTemplatesResponse], error) {
	ts, err := templates.Load(h.dataDir)
	if err != nil {
		return nil, rpcErr(ctx, err, "list templates failed")
	}
	out := make([]*floatv1.TransactionTemplate, len(ts))
	for i, t := range ts {
		out[i] = toProtoTemplate(t)
	}
	return connect.NewResponse(&floatv1.ListTemplatesResponse{Templates: out}), nil
}

func (h *Handler) AddTemplate(ctx context.Context, req *connect.Request[floatv1.AddTemplateRequest]) (*connect.Response[floatv1.TransactionTemplate], error) {
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	if len(req.Msg.Postings) < 2 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("at least 2 postings are required"))
	}
	for _, p := range req.Msg.Postings {
		if p.Account == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("each posting must have an account"))
		}
	}

	var added templates.Template
	err := h.lock.Do(ctx, fmt.Sprintf("add template: %s", req.Msg.Name), func() error {
		ts, loadErr := templates.Load(h.dataDir)
		if loadErr != nil {
			return loadErr
		}
		postings := make([]templates.TemplatePosting, len(req.Msg.Postings))
		for i, p := range req.Msg.Postings {
			postings[i] = templates.TemplatePosting{
				Account:         p.Account,
				Commodity:       p.Commodity,
				DefaultQuantity: p.DefaultQuantity,
				Comment:         p.Comment,
			}
		}
		added = templates.Template{
			ID:       journal.MintFID(),
			Name:     req.Msg.Name,
			Payee:    req.Msg.Payee,
			Note:     req.Msg.Note,
			Postings: postings,
			Tags:     req.Msg.Tags,
		}
		ts = append(ts, added)
		return templates.Save(h.dataDir, ts)
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "add template failed")
	}
	return connect.NewResponse(toProtoTemplate(added)), nil
}

func (h *Handler) UpdateTemplate(ctx context.Context, req *connect.Request[floatv1.UpdateTemplateRequest]) (*connect.Response[floatv1.TransactionTemplate], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	if len(req.Msg.Postings) < 2 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("at least 2 postings are required"))
	}
	for _, p := range req.Msg.Postings {
		if p.Account == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("each posting must have an account"))
		}
	}

	var updated templates.Template
	err := h.lock.Do(ctx, fmt.Sprintf("update template %s", req.Msg.Id), func() error {
		ts, loadErr := templates.Load(h.dataDir)
		if loadErr != nil {
			return loadErr
		}
		found := false
		for i, t := range ts {
			if t.ID == req.Msg.Id {
				postings := make([]templates.TemplatePosting, len(req.Msg.Postings))
				for j, p := range req.Msg.Postings {
					postings[j] = templates.TemplatePosting{
						Account:         p.Account,
						Commodity:       p.Commodity,
						DefaultQuantity: p.DefaultQuantity,
						Comment:         p.Comment,
					}
				}
				ts[i] = templates.Template{
					ID:       req.Msg.Id,
					Name:     req.Msg.Name,
					Payee:    req.Msg.Payee,
					Note:     req.Msg.Note,
					Postings: postings,
					Tags:     req.Msg.Tags,
				}
				updated = ts[i]
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("template %q: %w", req.Msg.Id, journal.ErrNotFound)
		}
		return templates.Save(h.dataDir, ts)
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, rpcErr(ctx, err, "update template failed", "id", req.Msg.Id)
	}
	return connect.NewResponse(toProtoTemplate(updated)), nil
}

func (h *Handler) DeleteTemplate(ctx context.Context, req *connect.Request[floatv1.DeleteTemplateRequest]) (*connect.Response[floatv1.DeleteTemplateResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	err := h.lock.Do(ctx, fmt.Sprintf("delete template %s", req.Msg.Id), func() error {
		ts, loadErr := templates.Load(h.dataDir)
		if loadErr != nil {
			return loadErr
		}
		filtered := ts[:0]
		found := false
		for _, t := range ts {
			if t.ID == req.Msg.Id {
				found = true
				continue
			}
			filtered = append(filtered, t)
		}
		if !found {
			return fmt.Errorf("template %q: %w", req.Msg.Id, journal.ErrNotFound)
		}
		return templates.Save(h.dataDir, filtered)
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, rpcErr(ctx, err, "delete template failed", "id", req.Msg.Id)
	}
	return connect.NewResponse(&floatv1.DeleteTemplateResponse{}), nil
}

func (h *Handler) GetAlphaVantageConfig(ctx context.Context, req *connect.Request[floatv1.GetAlphaVantageConfigRequest]) (*connect.Response[floatv1.GetAlphaVantageConfigResponse], error) {
	if h.cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	key := h.cfg.AlphaVantage.APIKey
	resp := &floatv1.GetAlphaVantageConfigResponse{}
	if key != "" {
		resp.ApiKeyConfigured = true
		if len(key) > 4 {
			resp.ApiKeyPreview = key[:4] + "..."
		} else {
			resp.ApiKeyPreview = "..."
		}
	}
	return connect.NewResponse(resp), nil
}

func (h *Handler) SetAlphaVantageApiKey(ctx context.Context, req *connect.Request[floatv1.SetAlphaVantageApiKeyRequest]) (*connect.Response[floatv1.SetAlphaVantageApiKeyResponse], error) {
	if h.cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	if h.configPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server config path not set"))
	}

	oldKey := h.cfg.AlphaVantage.APIKey
	err := h.lock.Do(ctx, "set alphavantage api key", func() error {
		h.cfg.AlphaVantage.APIKey = req.Msg.ApiKey
		if err := config.Save(h.configPath, h.cfg); err != nil {
			h.cfg.AlphaVantage.APIKey = oldKey
			return fmt.Errorf("save config: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "set alphavantage api key failed")
	}

	slogctx.FromContext(ctx).InfoContext(ctx, "updated alphavantage api key")
	return connect.NewResponse(&floatv1.SetAlphaVantageApiKeyResponse{}), nil
}
