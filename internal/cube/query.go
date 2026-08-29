package cube

import (
	"sort"
	"strings"
	"time"
)

// FlowFilter selects postings for a flow aggregation.
//
// The date interval is half-open: [From, To). This matches hledger's
// `date:FROM..TO` query, whose upper bound is exclusive — a fact worth stating
// loudly, because treating it as inclusive silently shifts every month boundary
// by one day. A zero From or To leaves that end unbounded.
//
// Account matches the named account and its descendants (an exact match, or a
// match on the account path followed by ":"). It is not hledger's substring
// regex: the cube's account axis is a tree, and "expenses:food" must not match
// "expenses:foodstuffs". An empty Account matches every account, and the same
// holds for Payee and Commodity.
type FlowFilter struct {
	From      time.Time
	To        time.Time
	Account   string
	Payee     string
	Commodity string
}

// FlowSums totals matching postings per commodity, keyed by commodity code,
// in minor units at that commodity's scale.
//
// Flow measures are distributive, so this reduction is legal over any
// combination of date range and account subtree. The stock measures in
// Cube.Valued and Cube.Cost have no equivalent: see BalanceAt.
func (c *Cube) FlowSums(f FlowFilter) map[string]int64 {
	out := make(map[string]int64)
	n := c.Postings.Len()
	if n == 0 {
		return out
	}

	lo, hi := c.dayRange(f.From, f.To)
	if lo >= hi {
		return out
	}

	accountOK := c.accountMask(f.Account)
	payeeOK := dictMask(c.Payees, f.Payee)

	for i := lo; i < hi; i++ {
		if !accountOK[c.Postings.Account[i]] {
			continue
		}
		if payeeOK != nil && !payeeOK[c.Postings.Payee[i]] {
			continue
		}
		commodity := c.Commodities[c.Postings.Commodity[i]].Code
		if f.Commodity != "" && commodity != f.Commodity {
			continue
		}
		out[commodity] += c.Postings.Amount[i]
	}
	return out
}

// BalanceAt totals a stock measure for a single period, rolled up over the
// named account and its descendants, keyed by commodity code.
//
// Rolling up the account tree at a fixed period is the only legal reduction of
// a stock measure. There is deliberately no range form of this method: summing
// market value or cost basis across periods is meaningless, and offering the
// signature would invite it.
func (c *Cube) BalanceAt(t *BalanceTable, period, account string) map[string]int64 {
	out := make(map[string]int64)
	periodIdx := -1
	for i, p := range c.Periods {
		if p == period {
			periodIdx = i
			break
		}
	}
	if periodIdx < 0 {
		return out
	}

	accountOK := c.accountMask(account)
	for i := 0; i < t.Len(); i++ {
		if int(t.Period[i]) != periodIdx || !accountOK[t.Account[i]] {
			continue
		}
		out[c.Commodities[t.Commodity[i]].Code] += t.Amount[i]
	}
	return out
}

// dayRange converts a half-open date interval to the half-open row range
// [lo, hi) of the date-sorted posting table. Postings are sorted by date, so
// each bound is one binary search rather than a scan.
func (c *Cube) dayRange(from, to time.Time) (int, int) {
	n := c.Postings.Len()
	lo, hi := 0, n
	if !from.IsZero() {
		d := c.dayIndex(from)
		if d > int(^uint16(0)) {
			return 0, 0
		}
		if d < 0 {
			d = 0
		}
		lo = sort.Search(n, func(i int) bool { return int(c.Postings.Date[i]) >= d })
	}
	if !to.IsZero() {
		d := c.dayIndex(to)
		if d < 0 {
			return 0, 0
		}
		if d <= int(^uint16(0)) {
			hi = sort.Search(n, func(i int) bool { return int(c.Postings.Date[i]) >= d })
		}
	}
	return lo, hi
}

// dayIndex converts a date to its offset from the cube's epoch. The result may
// be negative or beyond the table's range; callers clamp.
func (c *Cube) dayIndex(t time.Time) int {
	y, m, d := t.Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return int(day.Sub(c.EpochDate) / (24 * time.Hour))
}

// accountMask returns a lookup of which account ids match the prefix — the
// account itself and its descendants. A nil-safe all-true mask is returned for
// an empty prefix.
func (c *Cube) accountMask(prefix string) []bool {
	paths := c.Accounts.Values()
	mask := make([]bool, len(paths))
	for i, p := range paths {
		mask[i] = prefix == "" || p == prefix || strings.HasPrefix(p, prefix+":")
	}
	return mask
}

// dictMask returns a lookup of which dict ids equal want, or nil when want is
// empty (meaning "no filter").
func dictMask(d *Dict, want string) []bool {
	if want == "" {
		return nil
	}
	mask := make([]bool, d.Len())
	if id, ok := d.ID(want); ok {
		mask[id] = true
	}
	return mask
}
