package ledger

import (
	"container/list"
	"context"
	"sync"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/warm"
)

// openAccountsQuery excludes postings to closed accounts.
var openAccountsQuery = []string{"not:tag:" + hledger.ClosedAccountTag}

// openAssetLiabilityQuery limits a report to open asset and liability
// accounts. The web UI reads two valued reports with it: the net-worth
// summary at depth 1, whose total is net worth (NET_WORTH_SUMMARY_PARAMS in
// web/src/lib/net-worth.js, used by Overview and Trends), and the Overview
// account list as a --tree report (ACCOUNT_TREE_PARAMS in
// web/src/pages/home.jsx). Keep them in sync.
var openAssetLiabilityQuery = append(append([]string(nil), openAccountsQuery...), "type:AL")

// recentAccountsCap bounds the in-memory LRU of account names touched this
// process lifetime, warmed alongside the fixed dataset set.
const recentAccountsCap = 20

// recentAccounts is a small thread-safe LRU of account names, used to decide
// which per-account `areg` fetches are worth warming proactively.
type recentAccounts struct {
	mu       sync.Mutex
	capacity int
	list     *list.List
	index    map[string]*list.Element
}

func newRecentAccounts(capacity int) *recentAccounts {
	return &recentAccounts{capacity: capacity, list: list.New(), index: make(map[string]*list.Element)}
}

// touch records account as most-recently-used.
func (r *recentAccounts) touch(account string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if el, ok := r.index[account]; ok {
		r.list.MoveToFront(el)
		return
	}
	r.index[account] = r.list.PushFront(account)
	if r.list.Len() > r.capacity {
		oldest := r.list.Back()
		if oldest != nil {
			r.list.Remove(oldest)
			delete(r.index, oldest.Value.(string))
		}
	}
}

// snapshot returns the current account names, most-recently-used first.
func (r *recentAccounts) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, r.list.Len())
	for el := r.list.Front(); el != nil; el = el.Next() {
		out = append(out, el.Value.(string))
	}
	return out
}

// WarmEntries returns the fixed, priority-ordered set of warm.Entry values
// that populate the query cache for the datasets this handler's RPCs read
// from: the canonical full-history transaction/balance/timeseries fetches,
// the cheap filter-widget lookups, and unfiltered aregister fetches for
// recently-touched accounts. Returns nil when caching is disabled (h.cache
// == nil), since warming would just repeat uncached hledger work for
// nothing.
func (h *Handler) WarmEntries() []warm.Entry {
	if h.cache == nil {
		return nil
	}
	entries := []warm.Entry{
		{Name: "transactions", Load: func(ctx context.Context) error {
			_, err := cachedTransactions(ctx, h.cache, h.hl, nil)
			return err
		}},
		{Name: "accounts", Load: func(ctx context.Context) error {
			_, err := cachedAccounts(ctx, h.cache, h.hl)
			return err
		}},
		{Name: "closedaccounts", Load: func(ctx context.Context) error {
			_, err := cachedClosedAccounts(ctx, h.cache, h.hl)
			return err
		}},
		{Name: "tags", Load: func(ctx context.Context) error {
			_, err := cachedTags(ctx, h.cache, h.hl)
			return err
		}},
		{Name: "payees", Load: func(ctx context.Context) error {
			_, err := cachedPayees(ctx, h.cache, h.hl)
			return err
		}},
		{Name: "balances", Load: func(ctx context.Context) error {
			_, err := cachedBalances(ctx, h.cache, h.hl, hledger.BalanceOpts{}, nil)
			return err
		}},
		{Name: "balancesvalued depth 0", Load: func(ctx context.Context) error {
			_, err := cachedBalancesValued(ctx, h.cache, h.hl, "now,USD", hledger.BalanceOpts{}, nil)
			return err
		}},
		{Name: "balancesvalued depth 1", Load: func(ctx context.Context) error {
			_, err := cachedBalancesValued(ctx, h.cache, h.hl, "now,USD", hledger.BalanceOpts{Depth: 1}, nil)
			return err
		}},
		{Name: "balancesvalued depth 1 open asset/liability", Load: func(ctx context.Context) error {
			_, err := cachedBalancesValued(ctx, h.cache, h.hl, "now,USD", hledger.BalanceOpts{Depth: 1}, openAssetLiabilityQuery)
			return err
		}},
		{Name: "balancesvalued tree open asset/liability", Load: func(ctx context.Context) error {
			_, err := cachedBalancesValued(ctx, h.cache, h.hl, "now,USD", hledger.BalanceOpts{Tree: true}, openAssetLiabilityQuery)
			return err
		}},
		{Name: "networth", Load: func(ctx context.Context) error {
			_, err := cachedNetWorth(ctx, h.cache, h.hl, "", "")
			return err
		}},
		{Name: "incomestatement", Load: func(ctx context.Context) error {
			_, err := cachedIncomeStatement(ctx, h.cache, h.hl, "", "")
			return err
		}},
	}
	for _, account := range h.recentAccounts.snapshot() {
		account := account
		entries = append(entries, warm.Entry{
			Name: "aregister:" + account,
			Load: func(ctx context.Context) error {
				_, err := cachedAregister(ctx, h.cache, h.hl, account, nil)
				return err
			},
		})
	}
	return entries
}
