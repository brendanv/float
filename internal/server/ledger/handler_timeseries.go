package ledger

import (
	"context"
	"strings"
	"time"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/hledger"
)

// canSliceTimeseries reports whether a GetNetWorthTimeseries /
// GetIncomeStatementTimeseries range can be served by slicing the
// full-history `bs`/`is --monthly` fetch in Go: each bound must be "" or a
// valid first-of-month "YYYY-MM-DD" date, and begin < end when both are
// given. `bs --historical`/`is --monthly` periods are calendar-month
// boundaries; a begin/end that isn't month-aligned makes hledger generate a
// partial first/last period anchored to that exact date instead, and an
// empty/inverted range yields a dateless period — neither of which a slice
// of the full-history period list can reproduce, so such requests keep
// going straight to hledger.
func canSliceTimeseries(begin, end string) bool {
	return isMonthStart(begin) && isMonthStart(end) && (begin == "" || end == "" || begin < end)
}

// isMonthStart reports whether date is "" or a valid first-of-month
// "YYYY-MM-DD" date.
func isMonthStart(date string) bool {
	if date == "" {
		return true
	}
	t, err := time.Parse("2006-01-02", date)
	return err == nil && t.Day() == 1
}

// sumAmounts sums Amounts across all periods, grouped by commodity, using
// exact integer arithmetic to avoid float64 rounding residue. Used to
// recompute a row's cross-period total after slicing.
func sumAmounts(perPeriod [][]hledger.Amount) []hledger.Amount {
	sums := make(map[string]hledger.AmountQuantity)
	var order []string
	for _, amts := range perPeriod {
		for _, amt := range amts {
			if _, ok := sums[amt.Commodity]; !ok {
				order = append(order, amt.Commodity)
			}
			sums[amt.Commodity] = sums[amt.Commodity].Add(amt.Quantity)
		}
	}
	out := make([]hledger.Amount, len(order))
	for i, c := range order {
		out[i] = hledger.Amount{Commodity: c, Quantity: sums[c]}
	}
	return out
}

// monthSequence returns the first-of-month dates in [begin, end), in
// hledger's "YYYY-MM-DD" period-label format. Returns nil unless begin and
// end are both valid month-aligned dates.
func monthSequence(begin, end string) []string {
	b, errB := time.Parse("2006-01-02", begin)
	e, errE := time.Parse("2006-01-02", end)
	if errB != nil || errE != nil {
		return nil
	}
	var out []string
	for d := b; d.Before(e); d = d.AddDate(0, 1, 0) {
		out = append(out, d.Format("2006-01-02"))
	}
	return out
}

// slicedMonths returns the periods hledger itself emits for
// `--monthly date:begin..end`, derived from the full-history fetch's
// periods (canSliceTimeseries(begin, end) must hold, and at least one bound
// must be set). Verified against real hledger (see
// TestTimeseriesSliceMatchesHledger):
//
//   - A missing begin defaults to the first full-history period, a missing
//     end to the month after the last one. (The journal span covers
//     transactions and, for `bs --value`, price directives; the full-history
//     fetch uses identical flags, so its first/last period are exactly
//     hledger's defaults.)
//   - When that span is empty — begin-only at/after the last period, or
//     end-only at/before the first, or no data at all — hledger emits a
//     single period starting at the given bound.
func slicedMonths(periods []string, begin, end string) []string {
	var first, last string
	for _, p := range periods {
		if p == "" { // empty journal: one dateless period
			continue
		}
		if first == "" {
			first = p
		}
		last = p
	}
	start, stop := begin, end
	if start == "" {
		start = first
	}
	if stop == "" && last != "" {
		if t, err := time.Parse("2006-01-02", last); err == nil {
			stop = t.AddDate(0, 1, 0).Format("2006-01-02")
		}
	}
	if months := monthSequence(start, stop); len(months) > 0 {
		return months
	}
	if begin != "" {
		return []string{begin}
	}
	return []string{end}
}

// periodSources maps each output month to the index of the full-history
// period supplying its values: the period starting that month if present,
// otherwise (carry) the latest earlier one, or -1 for none. periods must be
// ascending, as hledger emits them.
func periodSources(periods, months []string, carry bool) []int {
	src := make([]int, len(months))
	k, latest := 0, -1
	for j, m := range months {
		for k < len(periods) && periods[k] <= m {
			if periods[k] != "" {
				latest = k
			}
			k++
		}
		src[j] = -1
		if latest >= 0 && (carry || periods[latest] == m) {
			src[j] = latest
		}
	}
	return src
}

// pick returns amts[i], or nil (zero / no flow) when i < 0.
func pick(amts [][]hledger.Amount, i int) []hledger.Amount {
	if i < 0 {
		return nil
	}
	return amts[i]
}

// sliceBalanceSheetTimeseries reshapes the full-history `bs --monthly
// --historical` timeseries ts into what hledger returns for
// `date:begin..end` (periods per slicedMonths). bs --historical values are
// cumulative from account opening regardless of begin, so each month takes
// the full-history value for that month; months before the ledger's first
// period are zero, and months after its last period carry the last value
// forward (no later transactions or prices exist to change it).
func sliceBalanceSheetTimeseries(ts *hledger.BalanceSheetTimeseries, begin, end string) *hledger.BalanceSheetTimeseries {
	if begin == "" && end == "" {
		return ts
	}
	months := slicedMonths(ts.Periods, begin, end)
	src := periodSources(ts.Periods, months, true)
	out := &hledger.BalanceSheetTimeseries{
		Periods:    months,
		NetWorth:   make([][]hledger.Amount, len(months)),
		Subreports: make([]hledger.BSSubreport, len(ts.Subreports)),
	}
	for j, i := range src {
		out.NetWorth[j] = pick(ts.NetWorth, i)
	}
	for s, sub := range ts.Subreports {
		totals := make([][]hledger.Amount, len(months))
		for j, i := range src {
			totals[j] = pick(sub.Totals, i)
		}
		out.Subreports[s] = hledger.BSSubreport{Name: sub.Name, Totals: totals}
	}
	return out
}

// sliceIncomeStatementTimeseries reshapes the full-history `is --monthly
// --tree` timeseries ts into what hledger returns for `date:begin..end`
// (periods per slicedMonths). Months absent from the full-history fetch get
// an empty (no flow) entry. Each row's cross-period TotalAmounts is
// recomputed over the sliced periods (the full-history total covers every
// period), and rows with no activity in any sliced period are dropped,
// mirroring a ranged `is` only emitting accounts with postings in range.
//
// ok is false when the sliced rows can't be trusted to match hledger's
// --tree row elision (see ambiguousTreeElision); the caller must then ask
// hledger for the range directly.
func sliceIncomeStatementTimeseries(ts *hledger.IncomeStatementTimeseries, begin, end string) (_ *hledger.IncomeStatementTimeseries, ok bool) {
	if begin == "" && end == "" {
		return ts, true
	}
	months := slicedMonths(ts.Periods, begin, end)
	src := periodSources(ts.Periods, months, false)
	out := &hledger.IncomeStatementTimeseries{
		Periods:    months,
		NetAmounts: make([][]hledger.Amount, len(months)),
		Subreports: make([]hledger.ISSubreport, len(ts.Subreports)),
	}
	for j, i := range src {
		out.NetAmounts[j] = pick(ts.NetAmounts, i)
	}
	for s, sub := range ts.Subreports {
		var rows []hledger.ISRow
		for _, row := range sub.Rows {
			perPeriod := make([][]hledger.Amount, len(months))
			anyActivity := false
			for j, i := range src {
				perPeriod[j] = pick(row.PerPeriodAmounts, i)
				anyActivity = anyActivity || len(perPeriod[j]) > 0
			}
			if !anyActivity {
				continue
			}
			rows = append(rows, hledger.ISRow{
				DisplayName:      row.DisplayName,
				FullName:         row.FullName,
				Indent:           row.Indent,
				Section:          row.Section,
				PerPeriodAmounts: perPeriod,
				TotalAmounts:     sumAmounts(perPeriod),
			})
		}
		if ambiguousTreeElision(rows) {
			return nil, false
		}
		totals := make([][]hledger.Amount, len(months))
		for j, i := range src {
			totals[j] = pick(sub.Totals, i)
		}
		out.Subreports[s] = hledger.ISSubreport{Name: sub.Name, Rows: rows, Totals: totals}
	}
	return out, true
}

// ambiguousTreeElision reports whether hledger's `--tree` display could
// differ from rows (the sliced, active rows of one subreport). hledger elides
// a parent account that has no postings of its own and only one active child
// branch, so a parent row the full-history fetch kept (it had several active
// branches over all time) may vanish from a narrower range. With two or more
// active child branches in range hledger always keeps the parent, and with
// none its activity is its own postings; with exactly one, whether it is
// kept depends on its own postings, which the rows alone can't tell.
func ambiguousTreeElision(rows []hledger.ISRow) bool {
	names := make(map[string]bool, len(rows))
	for _, r := range rows {
		names[r.FullName] = true
	}
	branches := make(map[string]map[string]bool)
	for _, r := range rows {
		child := r.FullName
		for i := strings.LastIndex(child, ":"); i >= 0; i = strings.LastIndex(child, ":") {
			parent := child[:i]
			if names[parent] {
				if branches[parent] == nil {
					branches[parent] = make(map[string]bool)
				}
				branches[parent][child] = true
			}
			child = parent
		}
	}
	for _, b := range branches {
		if len(b) == 1 {
			return true
		}
	}
	return false
}

func (h *Handler) GetNetWorthTimeseries(ctx context.Context, req *connect.Request[floatv1.GetNetWorthTimeseriesRequest]) (*connect.Response[floatv1.GetNetWorthTimeseriesResponse], error) {
	var ts *hledger.BalanceSheetTimeseries
	var err error
	if canSliceTimeseries(req.Msg.Begin, req.Msg.End) {
		ts, err = cachedNetWorth(ctx, h.cache, h.hl, "", "")
		if err == nil {
			ts = sliceBalanceSheetTimeseries(ts, req.Msg.Begin, req.Msg.End)
		}
	} else {
		ts, err = cachedNetWorth(ctx, h.cache, h.hl, req.Msg.Begin, req.Msg.End)
	}
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger balance sheet timeseries failed")
	}
	snapshots := make([]*floatv1.NetWorthSnapshot, len(ts.Periods))
	for i, date := range ts.Periods {
		snap := &floatv1.NetWorthSnapshot{Date: date}
		for _, sub := range ts.Subreports {
			switch sub.Name {
			case "Assets":
				snap.Assets = toProtoAmounts(sub.Totals[i])
			case "Liabilities":
				snap.Liabilities = toProtoAmounts(sub.Totals[i])
			}
		}
		snap.NetWorth = toProtoAmounts(ts.NetWorth[i])
		snapshots[i] = snap
	}
	return connect.NewResponse(&floatv1.GetNetWorthTimeseriesResponse{Snapshots: snapshots}), nil
}

func (h *Handler) GetIncomeStatementTimeseries(ctx context.Context, req *connect.Request[floatv1.GetIncomeStatementTimeseriesRequest]) (*connect.Response[floatv1.GetIncomeStatementTimeseriesResponse], error) {
	var ts *hledger.IncomeStatementTimeseries
	var err error
	sliced := false
	if canSliceTimeseries(req.Msg.Begin, req.Msg.End) {
		ts, err = cachedIncomeStatement(ctx, h.cache, h.hl, "", "")
		if err == nil {
			ts, sliced = sliceIncomeStatementTimeseries(ts, req.Msg.Begin, req.Msg.End)
		}
	}
	if err == nil && !sliced {
		ts, err = cachedIncomeStatement(ctx, h.cache, h.hl, req.Msg.Begin, req.Msg.End)
	}
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger income statement timeseries failed")
	}

	var rows []*floatv1.IncomeStatementRow
	for _, sub := range ts.Subreports {
		for _, r := range sub.Rows {
			rows = append(rows, toProtoISRow(r, false))
		}
		// Append a synthetic section-total row using the section totals.
		totalRow := hledger.ISRow{
			DisplayName:      "Total " + sub.Name,
			FullName:         "",
			Indent:           0,
			Section:          sub.Name,
			PerPeriodAmounts: sub.Totals,
		}
		rows = append(rows, toProtoISRow(totalRow, true))
	}

	netAmounts := make([]*floatv1.AmountList, len(ts.NetAmounts))
	for i, amounts := range ts.NetAmounts {
		netAmounts[i] = &floatv1.AmountList{Amounts: toProtoAmounts(amounts)}
	}

	return connect.NewResponse(&floatv1.GetIncomeStatementTimeseriesResponse{
		Periods:    ts.Periods,
		Rows:       rows,
		NetAmounts: netAmounts,
	}), nil
}

func toProtoISRow(r hledger.ISRow, isTotal bool) *floatv1.IncomeStatementRow {
	perPeriod := make([]*floatv1.AmountList, len(r.PerPeriodAmounts))
	for i, amounts := range r.PerPeriodAmounts {
		perPeriod[i] = &floatv1.AmountList{Amounts: toProtoAmounts(amounts)}
	}
	return &floatv1.IncomeStatementRow{
		DisplayName:      r.DisplayName,
		FullName:         r.FullName,
		Indent:           int32(r.Indent),
		Section:          r.Section,
		PerPeriodAmounts: perPeriod,
		TotalAmounts:     toProtoAmounts(r.TotalAmounts),
		IsTotal:          isTotal,
	}
}
