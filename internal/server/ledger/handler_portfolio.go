package ledger

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/slogctx"
)

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

	cfg := h.loadCfg()
	var excludedSymbols map[string]bool
	var excludedAccountPrefixes []string
	if cfg != nil {
		excludedSymbols = make(map[string]bool, len(cfg.Portfolio.ExcludedSymbols))
		for _, s := range cfg.Portfolio.ExcludedSymbols {
			excludedSymbols[s] = true
		}
		excludedAccountPrefixes = cfg.Portfolio.ExcludedAccountPrefixes
	}

	// Raw balances provide the original commodity quantities. Fetched
	// unfiltered and shared across every prefix/generation; the prefix is an
	// account-name filter applied in Go below, not accounting logic.
	raw, err := cachedBalances(ctx, h.cache, h.hl, hledger.BalanceOpts{}, nil)
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
	if txns, txErr := cachedTransactions(ctx, h.cache, h.hl, nil); txErr != nil {
		logger.WarnContext(ctx, "could not load transactions for last purchase dates", "error", txErr)
	} else {
		for _, txn := range txns {
			if !transactionTouchesPrefix(txn, prefix) {
				continue
			}
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
	aggregated := make(map[holdingKey]hledger.AmountQuantity)
	costByHolding := make(map[holdingKey]costBasis)
	var holdingOrder []holdingKey
	for _, row := range raw.Rows {
		if !accountMatchesPrefix(row.FullName, prefix) {
			continue
		}
		if isExcludedAccount(row.FullName, excludedAccountPrefixes) {
			continue
		}
		for _, amt := range row.Amounts {
			if currencySymbols[amt.Commodity] {
				continue
			}
			if excludedSymbols[amt.Commodity] {
				continue
			}
			k := holdingKey{row.FullName, amt.Commodity}
			if _, seen := aggregated[k]; !seen {
				holdingOrder = append(holdingOrder, k)
			}
			aggregated[k] = aggregated[k].Add(amt.Quantity)

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
		if eq.DecimalMantissa == 0 {
			continue
		}
		qty := eq.FloatingPoint
		priceDate := latestPriceInfo[k.symbol].date
		if txDate := lastTxDateByCommodity[k.symbol]; txDate > priceDate {
			priceDate = txDate
		}
		holding := &floatv1.Holding{
			Account:   k.account,
			Symbol:    k.symbol,
			Quantity:  fmt.Sprintf("%g", qty),
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

	// Sum total value per currency, compute allocation percentages, and track as_of_date.
	// Holdings can be priced in different currencies (e.g. USD funds alongside GBP or EUR
	// funds), so we must not add quantities across currencies.
	totalByCurrency := make(map[string]float64)
	var currencyOrder []string
	var asOfDate string
	for _, h := range holdings {
		if h.CurrentValue != nil {
			v, _ := strconv.ParseFloat(h.CurrentValue.Quantity, 64)
			curr := h.CurrentValue.Commodity
			if _, seen := totalByCurrency[curr]; !seen {
				currencyOrder = append(currencyOrder, curr)
			}
			totalByCurrency[curr] += v
		}
		if h.PriceDate > asOfDate {
			asOfDate = h.PriceDate
		}
	}

	// PortfolioPct is meaningful only within a single currency: each holding's share
	// of the total for its own currency.
	for _, h := range holdings {
		if h.CurrentValue != nil {
			curr := h.CurrentValue.Commodity
			if total := totalByCurrency[curr]; total > 0 {
				v, _ := strconv.ParseFloat(h.CurrentValue.Quantity, 64)
				h.PortfolioPct = v / total * 100
			}
		}
	}

	// Ordering by raw quantity compares values across currencies in mixed portfolios; approximate but consistent.
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

	resp := &floatv1.GetPortfolioHoldingsResponse{Holdings: holdings, AsOfDate: asOfDate}

	// Build per-currency subtotals in first-seen order.
	for _, curr := range currencyOrder {
		if total := totalByCurrency[curr]; total > 0 {
			resp.CurrencyTotals = append(resp.CurrencyTotals, &floatv1.Amount{
				Commodity: curr,
				Quantity:  fmt.Sprintf("%.2f", total),
			})
		}
	}
	// Only expose a single TotalValue when all valued holdings share one currency.
	if len(resp.CurrencyTotals) == 1 {
		resp.TotalValue = resp.CurrencyTotals[0]
	}

	return connect.NewResponse(resp), nil
}

func (h *Handler) GetPortfolioTimeseries(ctx context.Context, req *connect.Request[floatv1.GetPortfolioTimeseriesRequest]) (*connect.Response[floatv1.GetPortfolioTimeseriesResponse], error) {

	prefix := req.Msg.AccountPrefix
	if prefix == "" {
		prefix = "assets"
	}

	cfg := h.loadCfg()
	var tsExcludedSymbols map[string]bool
	var tsExcludedAccountPrefixes []string
	if cfg != nil {
		tsExcludedSymbols = make(map[string]bool, len(cfg.Portfolio.ExcludedSymbols))
		for _, s := range cfg.Portfolio.ExcludedSymbols {
			tsExcludedSymbols[s] = true
		}
		tsExcludedAccountPrefixes = cfg.Portfolio.ExcludedAccountPrefixes
	}

	// Determine which accounts under the prefix actually hold non-currency
	// commodities (equities, funds, etc.). This mirrors the GetPortfolioHoldings
	// filter so that cash accounts like checking/savings are excluded from the
	// chart total, matching what the Total Value card shows.
	raw, err := cachedBalances(ctx, h.cache, h.hl, hledger.BalanceOpts{}, nil)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger balances failed for portfolio timeseries")
	}
	seen := make(map[string]bool)
	var investmentAccounts []string
	for _, row := range raw.Rows {
		if !accountMatchesPrefix(row.FullName, prefix) {
			continue
		}
		if isExcludedAccount(row.FullName, tsExcludedAccountPrefixes) {
			continue
		}
		for _, amt := range row.Amounts {
			if !currencySymbols[amt.Commodity] && !tsExcludedSymbols[amt.Commodity] && !seen[row.FullName] {
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

	var cbAssets [][]hledger.Amount
	var cbSrc []int
	if cb != nil {
		for _, sub := range cb.Subreports {
			if sub.Name == "Assets" {
				cbAssets = sub.Totals
				cbSrc = periodSources(cb.Periods, ts.Periods, true)
			}
		}
	}

	snapshots := make([]*floatv1.PortfolioTimeseriesSnapshot, len(ts.Periods))
	for i, date := range ts.Periods {
		snap := &floatv1.PortfolioTimeseriesSnapshot{Date: date}
		for _, sub := range ts.Subreports {
			if sub.Name == "Assets" {
				snap.TotalValue = pickReportedTotal(sub.Totals[i], "USD")
			}
		}
		var cbTotals []hledger.Amount
		if cbSrc != nil {
			cbTotals = pick(cbAssets, cbSrc[i])
		}
		prefer := "USD"
		if snap.TotalValue != nil {
			prefer = snap.TotalValue.Commodity
		}
		snap.CostBasis = pickReportedTotal(cbTotals, prefer)
		if snap.CostBasis == nil && len(cbTotals) > 0 {
			slogctx.FromContext(ctx).DebugContext(ctx, "portfolio cost basis has no currency amount", "date", date, "amounts", len(cbTotals))
		}
		snapshots[i] = snap
	}
	return connect.NewResponse(&floatv1.GetPortfolioTimeseriesResponse{Snapshots: snapshots}), nil
}

func pickReportedTotal(amts []hledger.Amount, prefer string) *floatv1.Amount {
	var picked *hledger.Amount
	for i := range amts {
		a := &amts[i]
		if a.Commodity == prefer {
			picked = a
			break
		}
		if picked == nil && a.Quantity.DecimalMantissa != 0 && currencySymbols[a.Commodity] {
			picked = a
		}
	}
	if picked == nil {
		return nil
	}
	return &floatv1.Amount{
		Commodity: picked.Commodity,
		Quantity:  fmt.Sprintf("%.2f", picked.Quantity.FloatingPoint),
	}
}

// isExcludedAccount returns true if account matches any of the given prefixes exactly or as a path component.
func isExcludedAccount(account string, prefixes []string) bool {
	account = strings.ToLower(account)
	for _, p := range prefixes {
		p = strings.ToLower(p)
		if account == p || strings.HasPrefix(account, p+":") {
			return true
		}
	}
	return false
}

// accountMatchesPrefix reports whether account is prefix or one of its
// subaccounts. Portfolio endpoints use this as a Go-side stand-in for the
// bare account-name query term (e.g. "assets") they used to send to hledger
// directly — it's an account-name filter, not accounting logic, so an exact
// prefix match (rather than hledger's general infix regex matching) is an
// intentional simplification. Matching is case-insensitive to match hledger's
// account-name query semantics.
func accountMatchesPrefix(account, prefix string) bool {
	account, prefix = strings.ToLower(account), strings.ToLower(prefix)
	return account == prefix || strings.HasPrefix(account, prefix+":")
}

// transactionTouchesPrefix reports whether any posting in txn is under prefix.
func transactionTouchesPrefix(txn hledger.Transaction, prefix string) bool {
	for _, p := range txn.Postings {
		if accountMatchesPrefix(p.Account, prefix) {
			return true
		}
	}
	return false
}

func (h *Handler) GetPortfolioSettings(ctx context.Context, _ *connect.Request[floatv1.GetPortfolioSettingsRequest]) (*connect.Response[floatv1.GetPortfolioSettingsResponse], error) {
	cfg := h.loadCfg()
	if cfg == nil {
		return connect.NewResponse(&floatv1.GetPortfolioSettingsResponse{}), nil
	}
	resp := &floatv1.GetPortfolioSettingsResponse{
		ExcludedSymbols:         cfg.Portfolio.ExcludedSymbols,
		ExcludedAccountPrefixes: cfg.Portfolio.ExcludedAccountPrefixes,
	}
	if resp.ExcludedSymbols == nil {
		resp.ExcludedSymbols = []string{}
	}
	if resp.ExcludedAccountPrefixes == nil {
		resp.ExcludedAccountPrefixes = []string{}
	}
	return connect.NewResponse(resp), nil
}

func (h *Handler) UpdatePortfolioSettings(ctx context.Context, req *connect.Request[floatv1.UpdatePortfolioSettingsRequest]) (*connect.Response[floatv1.UpdatePortfolioSettingsResponse], error) {
	if h.loadCfg() == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	if h.configPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server config path not set"))
	}

	err := h.lock.DoConfig(ctx, "update portfolio settings", []string{h.configPath}, func() error {
		cur := h.loadCfg()
		newCfg := *cur
		newCfg.Portfolio.ExcludedSymbols = req.Msg.ExcludedSymbols
		newCfg.Portfolio.ExcludedAccountPrefixes = req.Msg.ExcludedAccountPrefixes
		if err := config.Save(h.configPath, &newCfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		h.cfg.Store(&newCfg)
		return nil
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "update portfolio settings failed")
	}

	slogctx.FromContext(ctx).InfoContext(ctx, "updated portfolio settings")
	return connect.NewResponse(&floatv1.UpdatePortfolioSettingsResponse{}), nil
}
