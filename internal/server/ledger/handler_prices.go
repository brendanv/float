package ledger

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/alphavantage"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/slogctx"
)

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

	cfg := h.loadCfg()
	if cfg == nil || cfg.AlphaVantage.APIKey == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("alpha_vantage.api_key is not configured"))
	}

	av := alphavantage.NewClient(cfg.AlphaVantage.APIKey)
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

func (h *Handler) GetAlphaVantageConfig(ctx context.Context, req *connect.Request[floatv1.GetAlphaVantageConfigRequest]) (*connect.Response[floatv1.GetAlphaVantageConfigResponse], error) {
	cfg := h.loadCfg()
	if cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	key := cfg.AlphaVantage.APIKey
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
	if h.loadCfg() == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	if h.configPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server config path not set"))
	}

	err := h.lock.DoConfig(ctx, "set alphavantage api key", []string{h.configPath}, func() error {
		cur := h.loadCfg()
		newCfg := *cur
		newCfg.AlphaVantage.APIKey = req.Msg.ApiKey
		if err := config.Save(h.configPath, &newCfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		h.cfg.Store(&newCfg)
		return nil
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "set alphavantage api key failed")
	}

	slogctx.FromContext(ctx).InfoContext(ctx, "updated alphavantage api key")
	return connect.NewResponse(&floatv1.SetAlphaVantageApiKeyResponse{}), nil
}
