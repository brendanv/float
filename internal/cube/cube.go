// Package cube builds a precomputed, column-oriented read model of the ledger
// for the web dashboards.
//
// # Why
//
// hledger re-parses the whole journal on every invocation, so every dashboard
// query pays the same multi-second floor regardless of how small the question
// is. internal/cache hides that only within a generation, and txlock bumps the
// generation on every write — so adding one transaction makes every dashboard
// cold again. The cube is built once per generation and shipped whole to the
// browser, which then answers filters and drilldowns locally.
//
// # The one rule that matters
//
// Flows sum over time; stocks do not.
//
// Posting amounts are distributive: the client may slice and roll them up
// freely along both the date and account-hierarchy axes. Market-valued
// balances and cost basis are not derivable from a sum of posting deltas —
// valuation depends on the price series and cost basis on hledger's lot
// matching — so they are materialized per period end and may only be rolled up
// the account tree at a fixed period, never summed across periods. Every
// measure column carries its Summability into the wire format, and the client
// query engine refuses the illegal reduction rather than trusting convention.
//
// # What this package is not
//
// The cube is a cache, never a source of truth. No mutation path reads it, and
// it must always be safe to delete and rebuild. hledger remains the accounting
// engine: this package only aggregates numbers hledger already computed.
package cube

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/brendanv/float/internal/hledger"
)

// DefaultReportingCurrency matches the currency the existing valued reports
// hardcode in internal/hledger.
const DefaultReportingCurrency = "USD"

// dateLayout is hledger's date rendering in CSV output.
const dateLayout = "2006-01-02"

// Summability records which reductions a measure column admits. It is encoded
// per column in the wire format so the client cannot silently misuse a measure.
type Summability string

const (
	// SumBoth marks a distributive measure, summable over both the date and
	// account-hierarchy axes. Posting amounts are the only such measure.
	SumBoth Summability = "both"
	// SumAccountOnly marks a measure that may be rolled up the account tree at
	// a fixed period but never summed across periods.
	SumAccountOnly Summability = "account-only"
)

// Commodity is an interned commodity and the decimal scale its amounts are
// stored at. Scale is the maximum precision observed for that commodity across
// every source report, so no amount is ever rounded on the way in.
type Commodity struct {
	Code  string `json:"code"`
	Scale int32  `json:"scale"`
}

// PostingTable is the flow fact table: one entry per posting, column-oriented
// and sorted ascending by Date so a date-range filter is a binary search over a
// contiguous run.
//
// All slices have the same length.
type PostingTable struct {
	Date      []uint16 // days since Cube.EpochDate
	Account   []uint32 // index into Cube.Accounts
	Payee     []uint32 // index into Cube.Payees
	Commodity []uint16 // index into Cube.Commodities
	Amount    []int64  // minor units at the commodity's scale
}

// Len returns the number of postings.
func (t *PostingTable) Len() int { return len(t.Date) }

// BalanceTable is a stock measure at period ends: sparse, with no entry where
// an account had no balance in that period. Amount is SumAccountOnly.
//
// All slices have the same length.
type BalanceTable struct {
	Period    []uint16 // index into Cube.Periods
	Account   []uint32 // index into Cube.Accounts
	Commodity []uint16 // index into Cube.Commodities
	Amount    []int64  // minor units at the commodity's scale
}

// Len returns the number of balance entries.
func (t *BalanceTable) Len() int { return len(t.Period) }

// Cube is the complete derived read model for one generation.
type Cube struct {
	Generation        uint64
	BuiltAt           time.Time
	ConfigHash        string
	ReportingCurrency string

	// EpochDate is the day PostingTable.Date counts from — the earliest date in
	// the journal.
	EpochDate time.Time

	Accounts    *Dict
	Payees      *Dict
	Commodities []Commodity
	// Periods are month labels in ascending order, e.g. "2016-01".
	Periods []string

	Postings PostingTable
	Valued   BalanceTable // market value at each period end
	Cost     BalanceTable // cost basis at each period end
}

// Options configures a build.
type Options struct {
	// Generation is the txlock generation this cube describes. It is stamped
	// into the payload and forms part of the immutable URL the client fetches.
	Generation uint64
	// ConfigHash covers config that changes bucketing or valuation without
	// bumping the generation. See ConfigHash.
	ConfigHash string
	// ReportingCurrency is the currency valued balances are converted to.
	// Defaults to DefaultReportingCurrency.
	ReportingCurrency string
}

// ConfigHash derives the config fingerprint for Options. Timezone affects month
// bucketing and the reporting currency affects valuation, and neither bumps the
// txlock generation — so both must participate in the cache key or a config
// change would serve a stale cube.
func ConfigHash(timezone, reportingCurrency string) string {
	return fmt.Sprintf("tz=%s;cur=%s", timezone, reportingCurrency)
}

// Build runs hledger and assembles the read model. It performs no writes and
// must never be called from inside txlock.Do: a build takes seconds, and a
// build failure must not be able to fail or revert a journal write.
func Build(ctx context.Context, hl *hledger.Client, opts Options) (*Cube, error) {
	currency := opts.ReportingCurrency
	if currency == "" {
		currency = DefaultReportingCurrency
	}

	// The three reports are independent hledger processes; running them
	// concurrently turns a ~10s serial build into roughly the slowest of them.
	var (
		postings []hledger.PostingRow
		valued   *hledger.PeriodBalances
		cost     *hledger.PeriodBalances
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		postings, err = hl.PostingRows(gctx)
		return err
	})
	g.Go(func() (err error) {
		valued, err = hl.PeriodBalancesValued(gctx, "end,"+currency)
		return err
	})
	g.Go(func() (err error) {
		cost, err = hl.PeriodBalancesCost(gctx)
		return err
	})
	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("cube: build: %w", err)
	}

	b := newBuilder()
	if err := b.addPostings(postings); err != nil {
		return nil, err
	}
	if err := b.addBalances(valued, &b.valued); err != nil {
		return nil, fmt.Errorf("cube: valued balances: %w", err)
	}
	if err := b.addBalances(cost, &b.cost); err != nil {
		return nil, fmt.Errorf("cube: cost balances: %w", err)
	}
	return b.finish(opts, currency)
}

// pendingPosting is a posting parsed but not yet rescaled, held until every
// source has been seen and each commodity's final scale is known.
type pendingPosting struct {
	date      time.Time
	account   uint32
	payee     uint32
	commodity uint16
	amount    rawAmount
}

// pendingBalance is the BalanceTable equivalent of pendingPosting.
type pendingBalance struct {
	period    string
	account   uint32
	commodity uint16
	amount    rawAmount
}

type builder struct {
	accounts    *Dict
	payees      *Dict
	commodities *Dict
	// scales[i] is the maximum decimal places seen for commodity i across all
	// source reports. Amounts are rescaled to it once every source is parsed.
	scales []int

	postings []pendingPosting
	valued   []pendingBalance
	cost     []pendingBalance
	periods  map[string]struct{}
}

func newBuilder() *builder {
	return &builder{
		accounts:    NewDict(),
		payees:      NewDict(),
		commodities: NewDict(),
		periods:     make(map[string]struct{}),
	}
}

// internCommodity interns a commodity code and widens its recorded scale to
// cover the given amount.
func (b *builder) internCommodity(code string, a rawAmount) (uint16, error) {
	id := b.commodities.Intern(code)
	if id > math.MaxUint16 {
		return 0, fmt.Errorf("cube: more than %d commodities", math.MaxUint16)
	}
	for int(id) >= len(b.scales) {
		b.scales = append(b.scales, 0)
	}
	if a.scale > b.scales[id] {
		b.scales[id] = a.scale
	}
	return uint16(id), nil
}

func (b *builder) addPostings(rows []hledger.PostingRow) error {
	b.postings = make([]pendingPosting, 0, len(rows))
	for i, r := range rows {
		// A posting with no amount column is hledger's rendering of an elided
		// posting it could not price; there is nothing to aggregate.
		if r.Amount == "" {
			continue
		}
		date, err := time.Parse(dateLayout, r.Date)
		if err != nil {
			return fmt.Errorf("cube: posting %d: parse date %q: %w", i, r.Date, err)
		}
		amt, err := parseDecimal(r.Amount)
		if err != nil {
			return fmt.Errorf("cube: posting %d (%s %s): %w", i, r.Date, r.Account, err)
		}
		commodity, err := b.internCommodity(r.Commodity, amt)
		if err != nil {
			return err
		}
		b.postings = append(b.postings, pendingPosting{
			date:      date,
			account:   b.accounts.Intern(r.Account),
			payee:     b.payees.Intern(hledger.PayeeOf(r.Description)),
			commodity: commodity,
			amount:    amt,
		})
	}
	return nil
}

func (b *builder) addBalances(src *hledger.PeriodBalances, dst *[]pendingBalance) error {
	if src == nil {
		return nil
	}
	for _, p := range src.Periods {
		b.periods[p] = struct{}{}
	}
	for _, row := range src.Rows {
		for i, raw := range row.Amounts {
			// An empty cell means the account had no balance that period, which
			// a sparse table represents by omitting the entry entirely.
			if raw == "" {
				continue
			}
			amt, err := parseDecimal(raw)
			if err != nil {
				return fmt.Errorf("%s @ %s: %w", row.Account, src.Periods[i], err)
			}
			if amt.mantissa == 0 {
				continue
			}
			commodity, err := b.internCommodity(row.Commodity, amt)
			if err != nil {
				return err
			}
			*dst = append(*dst, pendingBalance{
				period:    src.Periods[i],
				account:   b.accounts.Intern(row.Account),
				commodity: commodity,
				amount:    amt,
			})
		}
	}
	return nil
}

func (b *builder) finish(opts Options, currency string) (*Cube, error) {
	c := &Cube{
		Generation:        opts.Generation,
		BuiltAt:           time.Now().UTC(),
		ConfigHash:        opts.ConfigHash,
		ReportingCurrency: currency,
		Accounts:          b.accounts,
		Payees:            b.payees,
	}

	c.Commodities = make([]Commodity, b.commodities.Len())
	for i, code := range b.commodities.Values() {
		c.Commodities[i] = Commodity{Code: code, Scale: int32(b.scales[i])}
	}

	c.Periods = make([]string, 0, len(b.periods))
	for p := range b.periods {
		c.Periods = append(c.Periods, p)
	}
	sort.Strings(c.Periods)
	periodIndex := make(map[string]uint16, len(c.Periods))
	for i, p := range c.Periods {
		if i > math.MaxUint16 {
			return nil, fmt.Errorf("cube: more than %d periods", math.MaxUint16)
		}
		periodIndex[p] = uint16(i)
	}

	if err := b.finishPostings(c); err != nil {
		return nil, err
	}
	valued, err := b.finishBalances(b.valued, periodIndex)
	if err != nil {
		return nil, fmt.Errorf("cube: valued balances: %w", err)
	}
	cost, err := b.finishBalances(b.cost, periodIndex)
	if err != nil {
		return nil, fmt.Errorf("cube: cost balances: %w", err)
	}
	c.Valued, c.Cost = valued, cost
	return c, nil
}

func (b *builder) finishPostings(c *Cube) error {
	n := len(b.postings)
	if n == 0 {
		c.EpochDate = time.Time{}
		return nil
	}

	// Sort by date so the client can binary-search a date range into one
	// contiguous run instead of scanning the whole table.
	sort.SliceStable(b.postings, func(i, j int) bool {
		return b.postings[i].date.Before(b.postings[j].date)
	})

	epoch := b.postings[0].date
	c.EpochDate = epoch

	t := PostingTable{
		Date:      make([]uint16, n),
		Account:   make([]uint32, n),
		Payee:     make([]uint32, n),
		Commodity: make([]uint16, n),
		Amount:    make([]int64, n),
	}
	for i, p := range b.postings {
		days := int64(p.date.Sub(epoch) / (24 * time.Hour))
		if days < 0 || days > math.MaxUint16 {
			return fmt.Errorf("cube: journal spans %d days, limit is %d", days, math.MaxUint16)
		}
		amount, err := rescale(p.amount, b.scales[p.commodity])
		if err != nil {
			return fmt.Errorf("cube: posting %d: %w", i, err)
		}
		t.Date[i] = uint16(days)
		t.Account[i] = p.account
		t.Payee[i] = p.payee
		t.Commodity[i] = p.commodity
		t.Amount[i] = amount
	}
	c.Postings = t
	return nil
}

func (b *builder) finishBalances(pending []pendingBalance, periodIndex map[string]uint16) (BalanceTable, error) {
	n := len(pending)
	t := BalanceTable{
		Period:    make([]uint16, n),
		Account:   make([]uint32, n),
		Commodity: make([]uint16, n),
		Amount:    make([]int64, n),
	}
	for i, p := range pending {
		idx, ok := periodIndex[p.period]
		if !ok {
			return BalanceTable{}, fmt.Errorf("unknown period %q", p.period)
		}
		amount, err := rescale(p.amount, b.scales[p.commodity])
		if err != nil {
			return BalanceTable{}, err
		}
		t.Period[i] = idx
		t.Account[i] = p.account
		t.Commodity[i] = p.commodity
		t.Amount[i] = amount
	}
	return t, nil
}
