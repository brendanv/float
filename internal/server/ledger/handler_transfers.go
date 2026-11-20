package ledger

import (
	"cmp"
	"context"
	"slices"
	"time"

	"connectrpc.com/connect"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/transfers"
)

// transfersAccountOf returns the configured transfers clearing account, or
// the default when config leaves it unset.
func transfersAccountOf(cfg *config.Config) string {
	if cfg == nil {
		return transfers.DefaultAccount
	}
	return cmp.Or(cfg.Transfers.Account, transfers.DefaultAccount)
}

// FindUnmatchedTransfers reports transfer sides through the clearing account
// that have no matching other side. Read-only: it pairs the cached full
// transaction list in Go (internal/transfers) and writes nothing.
func (h *Handler) FindUnmatchedTransfers(ctx context.Context, req *connect.Request[floatv1.FindUnmatchedTransfersRequest]) (*connect.Response[floatv1.FindUnmatchedTransfersResponse], error) {
	fromDate := req.Msg.FromDate
	if fromDate != "" {
		if _, err := time.Parse(time.DateOnly, fromDate); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}

	cfg := h.loadCfg()
	account := transfersAccountOf(cfg)

	// Match over all history so a side just after from_date can still pair
	// with one just before it.
	txns, err := cachedTransactions(ctx, h.cache, h.hl, nil)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger transactions failed")
	}
	unmatched := transfers.FindUnmatched(txns, transfers.Options{
		Account:    account,
		WindowDays: transfers.DefaultWindowDays,
		Today:      time.Now().In(cfg.Location()),
	})

	out := make([]*floatv1.UnmatchedTransfer, 0, len(unmatched))
	for _, u := range unmatched {
		if fromDate != "" && u.Transaction.Date < fromDate {
			continue
		}
		out = append(out, &floatv1.UnmatchedTransfer{
			Transaction: toProtoTransaction(u.Transaction),
			Amount:      toProtoAmount(u.Amount),
			Status:      string(u.Status),
		})
	}
	// Newest first. Input order is hledger's date/file order, so reversing a
	// stable sort keeps same-day sides in reverse journal order.
	slices.Reverse(out)
	slices.SortStableFunc(out, func(a, b *floatv1.UnmatchedTransfer) int {
		return cmp.Compare(b.Transaction.Date, a.Transaction.Date)
	})

	return connect.NewResponse(&floatv1.FindUnmatchedTransfersResponse{
		Transfers:  out,
		Account:    account,
		WindowDays: transfers.DefaultWindowDays,
	}), nil
}
