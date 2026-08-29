package cube_test

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/brendanv/float/internal/cube"
	"github.com/brendanv/float/internal/hledger"
)

const (
	simpleJournal = "testdata/simple.journal"
	multiJournal  = "testdata/multi.journal"
)

func newClient(t *testing.T, journal string) *hledger.Client {
	t.Helper()
	hl, err := hledger.New("hledger", journal)
	if err != nil {
		t.Fatalf("hledger.New(%s): %v", journal, err)
	}
	return hl
}

func build(t *testing.T, journal string) *cube.Cube {
	t.Helper()
	c, err := cube.Build(t.Context(), newClient(t, journal), cube.Options{
		Generation: 1,
		ConfigHash: cube.ConfigHash("UTC", "USD"),
	})
	if err != nil {
		t.Fatalf("Build(%s): %v", journal, err)
	}
	return c
}

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	if s == "" {
		return time.Time{}
	}
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse date %q: %v", s, err)
	}
	return d
}

// quantityMinorUnits converts an hledger amount quantity to integer minor
// units at the given scale. hledger already reports an exact decimal mantissa
// and place count, so the comparison never touches floating point on either
// side.
func quantityMinorUnits(t *testing.T, q hledger.AmountQuantity, scale int) int64 {
	t.Helper()
	v := q.DecimalMantissa
	for p := q.DecimalPlaces; p < scale; p++ {
		v *= 10
	}
	if q.DecimalPlaces > scale {
		t.Fatalf("hledger reported %d decimal places, cube scale is %d", q.DecimalPlaces, scale)
	}
	return v
}

// scaleOf returns the scale the cube assigned to a commodity.
func scaleOf(t *testing.T, c *cube.Cube, code string) int {
	t.Helper()
	for _, cm := range c.Commodities {
		if cm.Code == code {
			return int(cm.Scale)
		}
	}
	t.Fatalf("commodity %q not in cube", code)
	return 0
}

// hledgerAccountTotal asks hledger for the total of one account subtree over a
// query, returning minor units per commodity. The account is matched with an
// anchored regex so it covers the account and its descendants and nothing else
// — the same semantics FlowFilter.Account promises.
func hledgerAccountTotal(t *testing.T, hl *hledger.Client, account string, extraQuery []string, scale int) map[string]int64 {
	t.Helper()
	query := extraQuery
	if account != "" {
		query = append([]string{"^" + account + "(:|$)"}, query...)
	}
	report, err := hl.Balances(t.Context(), 0, query...)
	if err != nil {
		t.Fatalf("hledger Balances(%v): %v", query, err)
	}
	out := make(map[string]int64)
	for _, amt := range report.Total {
		v := quantityMinorUnits(t, amt.Quantity, scale)
		if v != 0 {
			out[amt.Commodity] = v
		}
	}
	return out
}

// TestFlowSumsMatchHledger is the golden cross-check: for a table of slices,
// the cube's own aggregation must equal what hledger reports for the equivalent
// query. It is the test that stands between a fast dashboard and a fast, wrong
// one.
func TestFlowSumsMatchHledger(t *testing.T) {
	hl := newClient(t, simpleJournal)
	c := build(t, simpleJournal)
	scale := scaleOf(t, c, "USD")

	cases := []struct {
		name    string
		account string
		from    string
		to      string
	}{
		// A balanced journal sums to zero over any slice when no account filter
		// is applied, so every date-sensitive case below names an account —
		// otherwise both sides are zero and the case discriminates nothing.
		{name: "whole journal balances", account: ""},
		{name: "all expenses", account: "expenses"},
		{name: "expense subtree", account: "expenses:food"},
		{name: "leaf account", account: "expenses:food:groceries"},
		{name: "sibling leaf", account: "expenses:food:restaurants"},
		{name: "assets subtree", account: "assets"},
		{name: "revenues", account: "revenues"},
		{name: "one month", account: "expenses", from: "2023-01-01", to: "2023-02-01"},
		{name: "one quarter", account: "expenses", from: "2023-01-01", to: "2023-04-01"},
		{name: "one year", account: "expenses", from: "2023-01-01", to: "2024-01-01"},
		{name: "subtree in a year", account: "expenses:food", from: "2023-01-01", to: "2024-01-01"},
		{name: "open start", account: "expenses", to: "2023-06-01"},
		{name: "open end", account: "expenses", from: "2023-06-01"},
		{name: "single day with activity", account: "expenses", from: "2023-01-07", to: "2023-01-08"},
		{name: "single day without activity", account: "expenses", from: "2023-01-08", to: "2023-01-09"},
		{name: "empty range", account: "expenses", from: "2023-04-01", to: "2023-05-01"},
		{name: "range before the journal", account: "expenses", from: "2020-01-01", to: "2021-01-01"},
		{name: "range after the journal", account: "expenses", from: "2030-01-01", to: "2031-01-01"},
		{name: "range spanning the whole journal", account: "expenses", from: "2000-01-01", to: "2099-01-01"},
		{name: "account that does not exist", account: "expenses:nope"},
		// "expenses:hom" must not match "expenses:home" — the account axis is a
		// tree, not hledger's substring regex.
		{name: "prefix is not substring", account: "expenses:hom"},
		// Bounds landing exactly on a posting date. hledger's date:A..B upper
		// bound is exclusive, so 2023-01-07 as an end date must exclude that
		// day's transaction while the same date as a start must include it.
		// Without these, an inclusive upper bound passes every other case.
		{name: "upper bound on a posting date", account: "expenses", from: "2023-01-01", to: "2023-01-07"},
		{name: "lower bound on a posting date", account: "expenses", from: "2023-01-07", to: "2023-01-21"},
		{name: "bounds on posting dates both ends", account: "expenses", from: "2023-01-05", to: "2023-01-20"},
		{name: "upper bound on the last posting", account: "expenses", from: "2024-01-01", to: "2024-02-29"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var extra []string
			if tc.from != "" || tc.to != "" {
				extra = append(extra, "date:"+tc.from+".."+tc.to)
			}
			want := hledgerAccountTotal(t, hl, tc.account, extra, scale)

			got := c.FlowSums(cube.FlowFilter{
				From:    mustDate(t, tc.from),
				To:      mustDate(t, tc.to),
				Account: tc.account,
			})
			// Drop zero totals so an explicit zero and an absent key compare equal.
			for k, v := range got {
				if v == 0 {
					delete(got, k)
				}
			}

			if len(got) != len(want) {
				t.Fatalf("commodity count: got %v, want %v", got, want)
			}
			for commodity, wantV := range want {
				if got[commodity] != wantV {
					t.Errorf("%s: got %d, want %d (minor units)", commodity, got[commodity], wantV)
				}
			}
		})
	}
}

// TestFlowSumsByPayee checks the payee dimension against hledger, including the
// "payee | note" split that float writes into descriptions.
func TestFlowSumsByPayee(t *testing.T) {
	hl := newClient(t, simpleJournal)
	c := build(t, simpleJournal)
	scale := scaleOf(t, c, "USD")

	for _, payee := range []string{"Corner Store", "Trattoria", "Acme Payroll", "Landlord"} {
		t.Run(payee, func(t *testing.T) {
			want := hledgerAccountTotal(t, hl, "expenses", []string{"payee:^" + payee + "$"}, scale)
			got := c.FlowSums(cube.FlowFilter{Account: "expenses", Payee: payee})
			for k, v := range got {
				if v == 0 {
					delete(got, k)
				}
			}
			if len(got) != len(want) {
				t.Fatalf("got %v, want %v", got, want)
			}
			for commodity, wantV := range want {
				if got[commodity] != wantV {
					t.Errorf("%s: got %d, want %d", commodity, got[commodity], wantV)
				}
			}
		})
	}
}

// TestValuedBalancesMatchHledger checks the stock measures on the priced,
// multi-commodity fixture. A suite that only ever sees single-currency USD
// flows would pass while the valuation path was broken, so this fixture is the
// one that actually exercises Rule 1.
func TestValuedBalancesMatchHledger(t *testing.T) {
	hl := newClient(t, multiJournal)
	c := build(t, multiJournal)
	usdScale := scaleOf(t, c, "USD")

	// Market value is reported in the reporting currency, so every valued
	// balance must be USD regardless of what commodity the account holds.
	for i := 0; i < c.Valued.Len(); i++ {
		if code := c.Commodities[c.Valued.Commodity[i]].Code; code != "USD" {
			t.Fatalf("valued balance %d is in %q, want USD", i, code)
		}
	}

	for _, period := range []string{"2024-01", "2024-02", "2024-03"} {
		t.Run(period, func(t *testing.T) {
			// hledger's month-end valued balance for the whole asset tree.
			report, err := hl.BalancesValued(t.Context(), "end,USD", 0, "^assets(:|$)", "date:.."+nextMonth(t, period))
			if err != nil {
				t.Fatalf("BalancesValued: %v", err)
			}
			var want int64
			for _, amt := range report.Total {
				want += quantityMinorUnits(t, amt.Quantity, usdScale)
			}

			got := c.BalanceAt(&c.Valued, period, "assets")["USD"]
			if got != want {
				t.Errorf("valued assets at %s: got %d, want %d (minor units)", period, got, want)
			}
		})
	}
}

// TestValuedDiffersFromCost guards the reason both tables exist: if market
// value and cost basis were equal everywhere, the fixture would not be
// exercising the distinction the design rests on.
func TestValuedDiffersFromCost(t *testing.T) {
	c := build(t, multiJournal)
	valued := c.BalanceAt(&c.Valued, "2024-03", "assets")["USD"]
	cost := c.BalanceAt(&c.Cost, "2024-03", "assets")["USD"]
	if valued == cost {
		t.Fatalf("market value and cost basis are both %d; the fixture is not exercising valuation", valued)
	}
}

// TestCommodityScales checks that a commodity's scale is the widest precision
// seen for it, so no amount is rounded on the way into the cube.
func TestCommodityScales(t *testing.T) {
	c := build(t, multiJournal)
	want := map[string]int32{"USD": 2, "BTC": 8, "VTI": 0}
	for _, cm := range c.Commodities {
		w, ok := want[cm.Code]
		if !ok {
			continue
		}
		if cm.Scale < w {
			t.Errorf("commodity %s: scale %d, want at least %d", cm.Code, cm.Scale, w)
		}
	}
	if _, ok := want["BTC"]; ok {
		var found bool
		for _, cm := range c.Commodities {
			if cm.Code == "BTC" {
				found = true
			}
		}
		if !found {
			t.Error("BTC missing from cube commodities")
		}
	}
}

// TestPostingsSortedByDate is what makes the client's binary-search date filter
// correct; an unsorted table would silently return partial ranges.
func TestPostingsSortedByDate(t *testing.T) {
	c := build(t, simpleJournal)
	for i := 1; i < c.Postings.Len(); i++ {
		if c.Postings.Date[i] < c.Postings.Date[i-1] {
			t.Fatalf("postings not sorted at %d: %d < %d", i, c.Postings.Date[i], c.Postings.Date[i-1])
		}
	}
}

// TestPostingsBalance checks the cube captured every posting: a complete double
// -entry journal sums to zero per commodity.
func TestPostingsBalance(t *testing.T) {
	c := build(t, simpleJournal)
	for commodity, total := range c.FlowSums(cube.FlowFilter{}) {
		if total != 0 {
			t.Errorf("%s postings sum to %d, want 0 — a posting was dropped or double-counted", commodity, total)
		}
	}
}

func TestEncodeLayout(t *testing.T) {
	c := build(t, simpleJournal)
	payload, err := cube.Encode(c)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	if got := string(payload[:len(cube.Magic)]); got != cube.Magic {
		t.Fatalf("magic: got %q, want %q", got, cube.Magic)
	}
	headerLen := int(binary.LittleEndian.Uint32(payload[len(cube.Magic):]))
	prefix := len(cube.Magic) + 4

	var hdr struct {
		Generation uint64 `json:"generation"`
		EpochDate  string `json:"epochDate"`
		ConfigHash string `json:"configHash"`
		Accounts   []struct {
			Path   string `json:"path"`
			Parent int32  `json:"parent"`
			Depth  int32  `json:"depth"`
		} `json:"accounts"`
		Payees []string `json:"payees"`
		Tables map[string]struct {
			Rows     int    `json:"rows"`
			SortedBy string `json:"sortedBy"`
			Columns  map[string]struct {
				Type     string `json:"type"`
				Offset   int    `json:"offset"`
				Summable string `json:"summable"`
			} `json:"columns"`
		} `json:"tables"`
	}
	if err := json.Unmarshal(payload[prefix:prefix+headerLen], &hdr); err != nil {
		t.Fatalf("header unmarshal: %v", err)
	}

	if hdr.Generation != 1 {
		t.Errorf("generation: got %d, want 1", hdr.Generation)
	}
	if hdr.EpochDate != "2023-01-05" {
		t.Errorf("epochDate: got %q, want 2023-01-05", hdr.EpochDate)
	}
	if hdr.Tables["postings"].Rows != c.Postings.Len() {
		t.Errorf("postings rows: got %d, want %d", hdr.Tables["postings"].Rows, c.Postings.Len())
	}
	if hdr.Tables["postings"].SortedBy != "date" {
		t.Errorf("postings sortedBy: got %q, want date", hdr.Tables["postings"].SortedBy)
	}

	// Summability must survive to the wire, since it is what stops the client
	// summing market value across periods.
	if got := hdr.Tables["postings"].Columns["amount"].Summable; got != "both" {
		t.Errorf("postings.amount summable: got %q, want both", got)
	}
	for _, table := range []string{"valued", "cost"} {
		if got := hdr.Tables[table].Columns["amount"].Summable; got != "account-only" {
			t.Errorf("%s.amount summable: got %q, want account-only", table, got)
		}
	}

	// Every column must land on an 8-byte boundary: Float64Array construction
	// throws on a misaligned byte offset, which would break the zero-copy decode.
	dataStart := prefix + headerLen
	if r := dataStart % 8; r != 0 {
		dataStart += 8 - r
	}
	for tableName, table := range hdr.Tables {
		for colName, col := range table.Columns {
			if (dataStart+col.Offset)%8 != 0 {
				t.Errorf("%s.%s absolute offset %d is not 8-byte aligned", tableName, colName, dataStart+col.Offset)
			}
		}
	}

	// Spot-check that the amount column decodes back to the cube's values.
	amountCol := hdr.Tables["postings"].Columns["amount"]
	for i := 0; i < c.Postings.Len(); i++ {
		at := dataStart + amountCol.Offset + i*8
		got := int64(math.Float64frombits(binary.LittleEndian.Uint64(payload[at:])))
		if got != c.Postings.Amount[i] {
			t.Fatalf("amount[%d]: decoded %d, want %d", i, got, c.Postings.Amount[i])
		}
	}

	// Account hierarchy metadata.
	byPath := map[string]int32{}
	depth := map[string]int32{}
	for _, a := range hdr.Accounts {
		depth[a.Path] = a.Depth
		byPath[a.Path] = a.Parent
	}
	if depth["expenses:food:groceries"] != 3 {
		t.Errorf("depth of expenses:food:groceries: got %d, want 3", depth["expenses:food:groceries"])
	}
	// No posting hits "expenses:food" directly, so it is not interned and the
	// leaf has no interned ancestor.
	if byPath["expenses:food:groceries"] != -1 {
		t.Errorf("expenses:food:groceries parent: got %d, want -1 (no interned ancestor)", byPath["expenses:food:groceries"])
	}
}

func TestEncodeEmptyCube(t *testing.T) {
	// An empty journal must still produce a well-formed payload rather than
	// panicking on a zero-length table.
	c := &cube.Cube{
		Generation:        3,
		Accounts:          cube.NewDict(),
		Payees:            cube.NewDict(),
		ReportingCurrency: "USD",
	}
	payload, err := cube.Encode(c)
	if err != nil {
		t.Fatalf("Encode empty: %v", err)
	}
	headerLen := int(binary.LittleEndian.Uint32(payload[len(cube.Magic):]))
	var hdr map[string]any
	if err := json.Unmarshal(payload[len(cube.Magic)+4:len(cube.Magic)+4+headerLen], &hdr); err != nil {
		t.Fatalf("header unmarshal: %v", err)
	}
	for _, key := range []string{"payees", "periods", "accounts"} {
		if hdr[key] == nil {
			t.Errorf("header %q is null; the client indexes into it and needs []", key)
		}
	}
}

func nextMonth(t *testing.T, period string) string {
	t.Helper()
	d, err := time.Parse("2006-01", period)
	if err != nil {
		t.Fatalf("parse period %q: %v", period, err)
	}
	return d.AddDate(0, 1, 0).Format("2006-01-02")
}

// TestWriteWebFixture regenerates the encoded payload the web tests decode, so
// the JS decoder is exercised against bytes this package actually produced
// rather than a hand-rolled imitation of the format.
//
// Regenerate after any wire-format change:
//
//	FLOAT_WRITE_CUBE_FIXTURE=1 go test ./internal/cube/ -run TestWriteWebFixture
func TestWriteWebFixture(t *testing.T) {
	if os.Getenv("FLOAT_WRITE_CUBE_FIXTURE") == "" {
		t.Skip("set FLOAT_WRITE_CUBE_FIXTURE=1 to regenerate web/tests/fixtures/cube.bin")
	}
	payload, err := cube.Encode(build(t, simpleJournal))
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	path := filepath.Join("..", "..", "web", "tests", "fixtures", "cube.bin")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	t.Logf("wrote %s (%d bytes)", path, len(payload))
}

// TestFlowSumsByTypeMatchHledger checks the account-type dimension against
// hledger's own `type:` query. Types come from account directives, so deriving
// them from the top-level account name would be a guess that breaks on any
// ledger that renames or re-types its trees.
func TestFlowSumsByTypeMatchHledger(t *testing.T) {
	hl := newClient(t, simpleJournal)
	c := build(t, simpleJournal)
	scale := scaleOf(t, c, "USD")

	for _, tc := range []struct{ letter, query string }{
		{letter: "X", query: "type:X"},
		{letter: "R", query: "type:R"},
		{letter: "A", query: "type:A"},
		{letter: "C", query: "type:C"},
		{letter: "L", query: "type:L"},
	} {
		t.Run(tc.letter, func(t *testing.T) {
			report, err := hl.Balances(t.Context(), 0, tc.query)
			if err != nil {
				t.Fatalf("Balances(%s): %v", tc.query, err)
			}
			var want int64
			for _, amt := range report.Total {
				want += quantityMinorUnits(t, amt.Quantity, scale)
			}
			got := c.FlowSums(cube.FlowFilter{Type: tc.letter})["USD"]
			if got != want {
				t.Errorf("type:%s: got %d, want %d (minor units)", tc.letter, got, want)
			}
		})
	}
}

// TestTypeSubtypesMatchHledger pins the Cash-is-an-Asset relation on the
// fixture that actually mixes the two: multi.journal has an A-typed brokerage
// account and a C-typed checking account, so an exact-match comparison would
// undercount type:A here.
func TestTypeSubtypesMatchHledger(t *testing.T) {
	hl := newClient(t, multiJournal)
	c := build(t, multiJournal)
	scale := scaleOf(t, c, "USD")

	for _, letter := range []string{"A", "C", "X", "R"} {
		t.Run(letter, func(t *testing.T) {
			report, err := hl.Balances(t.Context(), 0, "type:"+letter, "cur:USD")
			if err != nil {
				t.Fatalf("Balances: %v", err)
			}
			var want int64
			for _, amt := range report.Total {
				if amt.Commodity == "USD" {
					want += quantityMinorUnits(t, amt.Quantity, scale)
				}
			}
			got := c.FlowSums(cube.FlowFilter{Type: letter, Commodity: "USD"})["USD"]
			if got != want {
				t.Errorf("type:%s USD: got %d, want %d", letter, got, want)
			}
		})
	}

	// The relation is directional: every cash account is an asset, but not
	// every asset is cash.
	if !cube.TypeMatches("C", "A") {
		t.Error("type:A must match a cash account")
	}
	if cube.TypeMatches("A", "C") {
		t.Error("type:C must not match a plain asset account")
	}
}
