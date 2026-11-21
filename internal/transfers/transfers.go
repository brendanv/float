// Package transfers finds transfers through a clearing account (by default
// equity:transfers) that have only one side recorded.
//
// Each side of a transfer between two accounts is its own transaction that
// posts between the source account and the clearing account, so a complete
// pair nets the clearing account to zero. A side whose counterpart never
// arrived usually means a transaction is missing from the journal entirely.
//
// The clearing account's direct children are routes, one per pair of
// counterparties (e.g. equity:transfers:Schwab-Amex). Sides only match within
// a route, and each route learns from history which accounts use it, so an
// unmatched side can say where its counterpart was expected.
//
// The package only pairs transactions hledger has already parsed; it does no
// accounting of its own. The sum of the unmatched sides in a route equals
// hledger's balance of that route.
package transfers

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/brendanv/float/internal/hledger"
)

// DefaultAccount is the clearing account used when config sets none.
const DefaultAccount = "equity:transfers"

// DefaultWindowDays is how far apart (in days) two sides may be dated and
// still match, and how long an unmatched side counts as recent.
const DefaultWindowDays = 5

// Status classifies an unmatched side.
type Status string

const (
	// StatusMissing: older than the window, so the other side is probably
	// missing from the journal.
	StatusMissing Status = "missing"
	// StatusRecent: within the window of today; the other side may simply
	// not have been imported yet.
	StatusRecent Status = "recent"
)

// Options configures FindUnmatched.
type Options struct {
	// Account is the clearing account. Postings to it or any of its
	// sub-accounts count. Empty means DefaultAccount.
	Account string
	// WindowDays bounds the date difference between matching sides. Zero
	// or negative means DefaultWindowDays.
	WindowDays int
	// Today is the current date in the ledger's timezone; only its
	// year/month/day are used.
	Today time.Time
}

// Unmatched is one side of a transfer with no matching other side.
type Unmatched struct {
	Transaction hledger.Transaction
	// Amount is the amount posted to the clearing account.
	Amount hledger.Amount
	Status Status
	// Route is the clearing account's direct child the side posts to (or
	// the clearing account itself for a direct posting).
	Route string
	// ExpectedAccounts is where the other side was expected: the route's
	// accounts that have moved money the opposite way, or, if none has yet,
	// every other account in the route. Sorted; empty when the route has
	// only ever seen this side's account.
	ExpectedAccounts []string
}

// Route summarizes a route with at least one unmatched side.
type Route struct {
	Account string
	// Accounts are the source accounts that have posted to the route, sorted.
	Accounts []string
}

// Report is the result of FindUnmatched.
type Report struct {
	// Unmatched lists the sides with no match, in input order.
	Unmatched []Unmatched
	// Routes lists the routes of the unmatched sides, sorted by account.
	Routes []Route
}

// RouteOf returns the route for a posting account under the clearing
// account: its direct child, or the clearing account itself. ok is false
// when name is not the clearing account or under it.
func RouteOf(name, account string) (route string, ok bool) {
	if name == account {
		return account, true
	}
	rest, found := strings.CutPrefix(name, account+":")
	if !found {
		return "", false
	}
	child, _, _ := strings.Cut(rest, ":")
	return account + ":" + child, true
}

// leg is a candidate transaction: exactly one posting to the clearing
// account, carrying exactly one amount.
type leg struct {
	idx    int // index into the input slice
	date   time.Time
	amount hledger.Amount
	route  string
	source string // first posting outside the clearing account
	key    bucketKey
	neg    bool // clearing amount is negative (money arrived at source)
}

// bucketKey groups legs that could match: same route, same commodity, same
// magnitude. The mantissa is normalized (trailing zeros stripped) so 500.0
// and 500.00 compare equal.
type bucketKey struct {
	route     string
	commodity string
	mantissa  int64 // absolute value
	places    int
}

func normalize(q hledger.AmountQuantity) (mantissa int64, places int, neg bool) {
	m, p := q.DecimalMantissa, q.DecimalPlaces
	if m < 0 {
		m, neg = -m, true
	}
	for p > 0 && m%10 == 0 {
		m /= 10
		p--
	}
	return m, p, neg
}

func toLeg(i int, t hledger.Transaction, account string) (leg, bool) {
	var clearing *hledger.Posting
	route := ""
	source := ""
	for j := range t.Postings {
		p := &t.Postings[j]
		if r, ok := RouteOf(p.Account, account); ok {
			if clearing != nil {
				return leg{}, false // touches the clearing account twice
			}
			clearing, route = p, r
		} else if source == "" {
			source = p.Account
		}
	}
	if clearing == nil || len(clearing.Amounts) != 1 {
		return leg{}, false
	}
	date, err := time.Parse(time.DateOnly, t.Date)
	if err != nil {
		return leg{}, false
	}
	a := clearing.Amounts[0]
	m, p, neg := normalize(a.Quantity)
	if m == 0 {
		return leg{}, false
	}
	return leg{
		idx:    i,
		date:   date,
		amount: a,
		route:  route,
		source: source,
		key:    bucketKey{route: route, commodity: a.Commodity, mantissa: m, places: p},
		neg:    neg,
	}, true
}

func daysApart(a, b time.Time) int {
	d := int(a.Sub(b).Hours() / 24)
	if d < 0 {
		return -d
	}
	return d
}

// routeAccounts records, per route, which source accounts have posted to it
// and in which direction.
type routeAccounts map[string]map[string][2]bool // route -> source -> [pos, neg]

func (ra routeAccounts) add(l leg) {
	m := ra[l.route]
	if m == nil {
		m = make(map[string][2]bool)
		ra[l.route] = m
	}
	dirs := m[l.source]
	if l.neg {
		dirs[1] = true
	} else {
		dirs[0] = true
	}
	m[l.source] = dirs
}

func (ra routeAccounts) all(route string) []string {
	out := make([]string, 0, len(ra[route]))
	for a := range ra[route] {
		out = append(out, a)
	}
	slices.Sort(out)
	return out
}

// expected lists where l's counterpart should be: accounts in l's route that
// have moved money the opposite way, else every other account in the route.
func (ra routeAccounts) expected(l leg) []string {
	opposite := 0 // a positive leg pairs with negative legs, and vice versa
	if !l.neg {
		opposite = 1
	}
	var dir, others []string
	for a, dirs := range ra[l.route] {
		if a == l.source {
			continue
		}
		others = append(others, a)
		if dirs[opposite] {
			dir = append(dir, a)
		}
	}
	if len(dir) == 0 {
		dir = others
	}
	slices.Sort(dir)
	return dir
}

// FindUnmatched returns the transfer sides in txns that have no matching
// other side.
//
// A transaction is a transfer side when exactly one of its postings is to
// the clearing account (or a sub-account) and that posting has a single
// amount. Two sides match when they post to the same route, their clearing
// amounts are equal and opposite in the same commodity (costs are ignored),
// their source accounts (first posting outside the clearing account) differ,
// and their dates are at most WindowDays apart. Pairing is one-to-one,
// closest dates first, so repeated identical transfers each need their own
// counterpart.
func FindUnmatched(txns []hledger.Transaction, opts Options) Report {
	account := cmp.Or(opts.Account, DefaultAccount)
	window := opts.WindowDays
	if window <= 0 {
		window = DefaultWindowDays
	}

	buckets := make(map[bucketKey][]leg)
	var keys []bucketKey
	accounts := make(routeAccounts)
	for i, t := range txns {
		l, ok := toLeg(i, t, account)
		if !ok {
			continue
		}
		if _, seen := buckets[l.key]; !seen {
			keys = append(keys, l.key)
		}
		buckets[l.key] = append(buckets[l.key], l)
		accounts.add(l)
	}

	type pair struct {
		a, b leg // a.idx < b.idx
		days int
	}
	matched := make(map[int]bool)
	for _, k := range keys {
		legs := buckets[k]
		var pairs []pair
		for x := range legs {
			for y := x + 1; y < len(legs); y++ {
				a, b := legs[x], legs[y]
				if a.neg == b.neg || a.source == b.source {
					continue
				}
				if d := daysApart(a.date, b.date); d <= window {
					pairs = append(pairs, pair{a: a, b: b, days: d})
				}
			}
		}
		// Closest dates first; ties go to the earliest transactions so the
		// result is deterministic.
		slices.SortStableFunc(pairs, func(p, q pair) int {
			return cmp.Or(
				cmp.Compare(p.days, q.days),
				p.a.date.Compare(q.a.date),
				cmp.Compare(p.a.idx, q.a.idx),
				cmp.Compare(p.b.idx, q.b.idx),
			)
		})
		for _, p := range pairs {
			if matched[p.a.idx] || matched[p.b.idx] {
				continue
			}
			matched[p.a.idx] = true
			matched[p.b.idx] = true
		}
	}

	var unmatched []leg
	for _, k := range keys {
		for _, l := range buckets[k] {
			if !matched[l.idx] {
				unmatched = append(unmatched, l)
			}
		}
	}
	slices.SortFunc(unmatched, func(a, b leg) int { return cmp.Compare(a.idx, b.idx) })

	today := time.Date(opts.Today.Year(), opts.Today.Month(), opts.Today.Day(), 0, 0, 0, 0, time.UTC)
	report := Report{Unmatched: make([]Unmatched, 0, len(unmatched))}
	routes := make(map[string]bool)
	for _, l := range unmatched {
		status := StatusRecent
		if l.date.AddDate(0, 0, window).Before(today) {
			status = StatusMissing
		}
		report.Unmatched = append(report.Unmatched, Unmatched{
			Transaction:      txns[l.idx],
			Amount:           l.amount,
			Status:           status,
			Route:            l.route,
			ExpectedAccounts: accounts.expected(l),
		})
		if !routes[l.route] {
			routes[l.route] = true
			report.Routes = append(report.Routes, Route{Account: l.route, Accounts: accounts.all(l.route)})
		}
	}
	slices.SortFunc(report.Routes, func(a, b Route) int { return cmp.Compare(a.Account, b.Account) })
	return report
}
