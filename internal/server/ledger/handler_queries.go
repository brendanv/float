package ledger

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/hledger"
)

func (h *Handler) GetAccountRegister(ctx context.Context, req *connect.Request[floatv1.GetAccountRegisterRequest]) (*connect.Response[floatv1.GetAccountRegisterResponse], error) {
	account := strings.TrimSpace(req.Msg.Account)
	if account == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("account is required"))
	}
	rows, err := filteredAregister(ctx, h.cache, h.hl, account, req.Msg.Query)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger aregister failed")
	}
	h.recentAccounts.touch(account)
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
	opts := hledger.BalanceOpts{Depth: int(req.Msg.Depth), Tree: req.Msg.Tree}
	if req.Msg.Value != "" {
		report, err = cachedBalancesValued(ctx, h.cache, h.hl, req.Msg.Value, opts, req.Msg.Query)
	} else {
		report, err = cachedBalances(ctx, h.cache, h.hl, opts, req.Msg.Query)
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

func (h *Handler) ListLots(ctx context.Context, req *connect.Request[floatv1.ListLotsRequest]) (*connect.Response[floatv1.ListLotsResponse], error) {
	rows, err := cachedLots(ctx, h.cache, h.hl, req.Msg.Query)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger lots failed")
	}
	lots := make([]*floatv1.Lot, len(rows))
	for i, r := range rows {
		lots[i] = toProtoLot(r)
	}
	return connect.NewResponse(&floatv1.ListLotsResponse{Lots: lots}), nil
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
