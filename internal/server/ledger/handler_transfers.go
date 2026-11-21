package ledger

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/hledger"
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

// hledgerRegexQuote escapes regex metacharacters so an account name can be
// used literally inside an hledger acct: query.
func hledgerRegexQuote(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(`\.+*?()|[]{}^$`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// routeBalances returns hledger's balance of each route under the clearing
// account. A flat report at one level below the clearing account clips deeper
// sub-accounts into their route and leaves direct postings on the clearing
// account itself, which is exactly the route grouping internal/transfers uses.
func (h *Handler) routeBalances(ctx context.Context, account string) (map[string][]hledger.Amount, error) {
	opts := hledger.BalanceOpts{Depth: strings.Count(account, ":") + 2}
	query := []string{"acct:^" + hledgerRegexQuote(account) + "(:|$)"}
	report, err := cachedBalances(ctx, h.cache, h.hl, opts, query)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]hledger.Amount, len(report.Rows))
	for _, row := range report.Rows {
		out[row.FullName] = row.Amounts
	}
	return out, nil
}

// FindUnmatchedTransfers reports transfer sides through the clearing account
// that have no matching other side. Read-only: it pairs the cached full
// transaction list in Go (internal/transfers), takes route balances from
// hledger, and writes nothing.
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
	report := transfers.FindUnmatched(txns, transfers.Options{
		Account:    account,
		WindowDays: transfers.DefaultWindowDays,
		Today:      time.Now().In(cfg.Location()),
	})

	out := make([]*floatv1.UnmatchedTransfer, 0, len(report.Unmatched))
	shownRoutes := make(map[string]bool)
	for _, u := range report.Unmatched {
		if fromDate != "" && u.Transaction.Date < fromDate {
			continue
		}
		out = append(out, &floatv1.UnmatchedTransfer{
			Transaction:      toProtoTransaction(u.Transaction),
			Amount:           toProtoAmount(u.Amount),
			Status:           string(u.Status),
			Route:            u.Route,
			ExpectedAccounts: u.ExpectedAccounts,
		})
		shownRoutes[u.Route] = true
	}
	// Newest first. Input order is hledger's date/file order, so reversing a
	// stable sort keeps same-day sides in reverse journal order.
	slices.Reverse(out)
	slices.SortStableFunc(out, func(a, b *floatv1.UnmatchedTransfer) int {
		return cmp.Compare(b.Transaction.Date, a.Transaction.Date)
	})

	var routes []*floatv1.TransferRoute
	if len(shownRoutes) > 0 {
		balances, err := h.routeBalances(ctx, account)
		if err != nil {
			return nil, rpcErr(ctx, err, "hledger balances failed")
		}
		for _, r := range report.Routes {
			if !shownRoutes[r.Account] {
				continue
			}
			routes = append(routes, &floatv1.TransferRoute{
				Account:  r.Account,
				Accounts: r.Accounts,
				Balance:  toProtoAmounts(balances[r.Account]),
			})
		}
	}

	return connect.NewResponse(&floatv1.FindUnmatchedTransfersResponse{
		Transfers:  out,
		Account:    account,
		WindowDays: transfers.DefaultWindowDays,
		Routes:     routes,
	}), nil
}
