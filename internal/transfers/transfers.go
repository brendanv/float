// Package transfers finds transfers through a clearing account (by default
// equity:transfers) that have only one side recorded.
//
// Each side of a transfer between two accounts is its own transaction that
// posts between the source account and the clearing account, so a complete
// pair nets the clearing account to zero. A side whose counterpart never
// arrived usually means a transaction is missing from the journal entirely.
//
// The package only pairs transactions hledger has already parsed; it does no
// accounting of its own. The sum of the unmatched sides equals hledger's
// balance of the clearing account.
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
}

// leg is a candidate transaction: exactly one posting to the clearing
// account, carrying exactly one amount.
type leg struct {
	idx    int // index into the input slice
	date   time.Time
	amount hledger.Amount
	source string // first posting outside the clearing account
	key    amountKey
	neg    bool
}

// amountKey groups legs whose clearing amounts have the same magnitude and
// commodity. The mantissa is normalized (trailing zeros stripped) so 500.0
// and 500.00 compare equal.
type amountKey struct {
	commodity string
	mantissa  int64 // absolute value
	places    int
}

func inAccount(name, account string) bool {
	return name == account || strings.HasPrefix(name, account+":")
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
	source := ""
	for j := range t.Postings {
		p := &t.Postings[j]
		if inAccount(p.Account, account) {
			if clearing != nil {
				return leg{}, false // touches the clearing account twice
			}
			clearing = p
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
		source: source,
		key:    amountKey{commodity: a.Commodity, mantissa: m, places: p},
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

// FindUnmatched returns the transfer sides in txns that have no matching
// other side, in input order.
//
// A transaction is a transfer side when exactly one of its postings is to
// the clearing account (or a sub-account) and that posting has a single
// amount. Two sides match when their clearing amounts are equal and
// opposite in the same commodity (costs are ignored), their source accounts
// (first posting outside the clearing account) differ, and their dates are
// at most WindowDays apart. Pairing is one-to-one, closest dates first, so
// repeated identical transfers each need their own counterpart.
func FindUnmatched(txns []hledger.Transaction, opts Options) []Unmatched {
	account := cmp.Or(opts.Account, DefaultAccount)
	window := opts.WindowDays
	if window <= 0 {
		window = DefaultWindowDays
	}

	buckets := make(map[amountKey][]leg)
	var keys []amountKey
	for i, t := range txns {
		l, ok := toLeg(i, t, account)
		if !ok {
			continue
		}
		if _, seen := buckets[l.key]; !seen {
			keys = append(keys, l.key)
		}
		buckets[l.key] = append(buckets[l.key], l)
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
	out := make([]Unmatched, 0, len(unmatched))
	for _, l := range unmatched {
		status := StatusRecent
		if l.date.AddDate(0, 0, window).Before(today) {
			status = StatusMissing
		}
		out = append(out, Unmatched{Transaction: txns[l.idx], Amount: l.amount, Status: status})
	}
	return out
}
