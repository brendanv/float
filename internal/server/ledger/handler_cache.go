package ledger

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/brendanv/float/internal/cache"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/txfilter"
)

// cacheKey helpers produce deterministic, namespaced keys from RPC parameters.
// Query args are sorted so that ["b","a"] and ["a","b"] produce the same key.

func transactionsKey(query []string) string {
	return "transactions:" + normalizedQueryKeyPart(query)
}

func balancesKey(opts hledger.BalanceOpts, query []string) string {
	return fmt.Sprintf("%s:%d:%s", balancesFamily("balances", opts), opts.Depth, normalizedQueryKeyPart(query))
}

// balancesFamily namespaces --tree reports separately from flat ones.
func balancesFamily(base string, opts hledger.BalanceOpts) string {
	if opts.Tree {
		return base + "tree"
	}
	return base
}

func lotsKey(query []string) string {
	return "lots:" + normalizedQueryKeyPart(query)
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

func balancesValuedKey(valueSpec string, opts hledger.BalanceOpts, query []string) string {
	return fmt.Sprintf("%s:%s:%d:%s", balancesFamily("balancesvalued", opts), valueSpec, opts.Depth, normalizedQueryKeyPart(query))
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

// filteredTransactions serves a transaction query by filtering the single
// canonical full-journal fetch (cachedTransactions(nil)) in Go when query is
// entirely within txfilter's supported vocabulary. Any token outside that
// vocabulary (e.g. an AI-generated or RunHledgerQuery-style query) falls back
// to a dedicated cached hledger query, exactly as before this existed.
func filteredTransactions(ctx context.Context, c *cache.Cache[any], hl *hledger.Client, query []string) ([]hledger.Transaction, error) {
	if f, ok := txfilter.Parse(query); ok {
		all, err := cachedTransactions(ctx, c, hl, nil)
		if err != nil {
			return nil, err
		}
		return f.Filter(all), nil
	}
	return cachedTransactions(ctx, c, hl, query)
}

// cachedBalances fetches balances from cache or hledger.
func cachedBalances(ctx context.Context, c *cache.Cache[any], hl *hledger.Client, opts hledger.BalanceOpts, query []string) (*hledger.BalanceReport, error) {
	return cachedGet(ctx, c, balancesKey(opts, query), func(ctx context.Context) (*hledger.BalanceReport, error) {
		return hl.Balances(ctx, opts, query...)
	})
}

func cachedLots(ctx context.Context, c *cache.Cache[any], hl *hledger.Client, query []string) ([]hledger.LotRow, error) {
	return cachedGet(ctx, c, lotsKey(query), func(ctx context.Context) ([]hledger.LotRow, error) {
		return hl.Lots(ctx, query...)
	})
}

// cachedBalancesValued fetches market-valued balances from cache or hledger.
func cachedBalancesValued(ctx context.Context, c *cache.Cache[any], hl *hledger.Client, valueSpec string, opts hledger.BalanceOpts, query []string) (*hledger.BalanceReport, error) {
	return cachedGet(ctx, c, balancesValuedKey(valueSpec, opts, query), func(ctx context.Context) (*hledger.BalanceReport, error) {
		return hl.BalancesValued(ctx, valueSpec, opts, query...)
	})
}

// cachedAregister fetches account register rows from cache or hledger.
func cachedAregister(ctx context.Context, c *cache.Cache[any], hl *hledger.Client, account string, query []string) ([]hledger.AregisterRow, error) {
	return cachedGet(ctx, c, accountRegisterKey(account, query), func(ctx context.Context) ([]hledger.AregisterRow, error) {
		return hl.Aregister(ctx, account, query...)
	})
}

// filteredAregister serves an account register query by filtering the
// account's single canonical unfiltered fetch (cachedAregister(account, nil))
// in Go when query is entirely within txfilter's supported vocabulary,
// falling back to a dedicated cached hledger query otherwise — mirroring
// filteredTransactions.
//
// Accepting this means the running-balance column always shows the
// account's true historical running balance rather than a subset-relative
// sum, since rows are dropped after hledger has already computed them
// against the full account history. For a date-filtered register this is
// arguably a correction, not a regression, but it is user-visible.
func filteredAregister(ctx context.Context, c *cache.Cache[any], hl *hledger.Client, account string, query []string) ([]hledger.AregisterRow, error) {
	f, ok := txfilter.Parse(query)
	if !ok || queryHasDateToken(query) {
		// hledger areg's date: filtering matches on ANY posting's effective
		// date (including posting-date overrides on legs other than the
		// focused account), unlike print's transaction-date-only semantics
		// that txfilter's date predicate assumes. Rather than guess at areg's
		// exact rule, fall back to hledger for any date-filtered query and
		// keep the Go path for the tag/status/payee/desc filters it was
		// built for.
		return cachedAregister(ctx, c, hl, account, query)
	}
	all, err := cachedAregister(ctx, c, hl, account, nil)
	if err != nil {
		return nil, err
	}
	out := make([]hledger.AregisterRow, 0, len(all))
	for _, row := range all {
		if f.Match(&row.Transaction) {
			out = append(out, row)
		}
	}
	return out, nil
}

// queryHasDateToken reports whether query contains a date: or not:date:
// token.
func queryHasDateToken(query []string) bool {
	for _, tok := range query {
		if strings.HasPrefix(tok, "date:") || strings.HasPrefix(tok, "not:date:") {
			return true
		}
	}
	return false
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

const closedAccountsKey = "closedaccounts"

// cachedClosedAccounts fetches the set of closed account names (including
// sub-accounts inheriting the tag) from cache or hledger.
func cachedClosedAccounts(ctx context.Context, c *cache.Cache[any], hl *hledger.Client) (map[string]bool, error) {
	return cachedGet(ctx, c, closedAccountsKey, func(ctx context.Context) (map[string]bool, error) {
		names, err := hl.ClosedAccounts(ctx)
		if err != nil {
			return nil, err
		}
		set := make(map[string]bool, len(names))
		for _, n := range names {
			set[n] = true
		}
		return set, nil
	})
}

const unusedAccountsKey = "unusedaccounts"

// cachedUnusedAccounts fetches unused account names from cache or hledger.
func cachedUnusedAccounts(ctx context.Context, c *cache.Cache[any], hl *hledger.Client) ([]string, error) {
	return cachedGet(ctx, c, unusedAccountsKey, func(ctx context.Context) ([]string, error) {
		return hl.UnusedAccounts(ctx)
	})
}
