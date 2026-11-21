package transfers_test

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/transfers"
)

// amt builds an hledger amount from a decimal string such as "-500.00".
func amt(commodity, s string) hledger.Amount {
	places := 0
	if i := strings.IndexByte(s, '.'); i >= 0 {
		places = len(s) - i - 1
	}
	m, err := strconv.ParseInt(strings.Replace(s, ".", "", 1), 10, 64)
	if err != nil {
		panic(err)
	}
	return hledger.Amount{Commodity: commodity, Quantity: hledger.AmountQuantity{DecimalMantissa: m, DecimalPlaces: places}}
}

func posting(account, commodity, qty string) hledger.Posting {
	return hledger.Posting{Account: account, Amounts: []hledger.Amount{amt(commodity, qty)}}
}

// side is a transfer side through equity:transfers: source posts -clearing,
// the clearing account posts clearing.
func side(fid, date, source, clearing string) hledger.Transaction {
	return routeSide(fid, date, "equity:transfers", source, clearing)
}

// routeSide is side posting to the given clearing (route) account.
func routeSide(fid, date, route, source, clearing string) hledger.Transaction {
	neg := "-" + clearing
	if strings.HasPrefix(clearing, "-") {
		neg = clearing[1:]
	}
	return hledger.Transaction{
		FID:  fid,
		Date: date,
		Postings: []hledger.Posting{
			posting(source, "USD", neg),
			posting(route, "USD", clearing),
		},
	}
}

func fids(us []transfers.Unmatched) []string {
	out := make([]string, len(us))
	for i, u := range us {
		out[i] = u.Transaction.FID + ":" + string(u.Status)
	}
	return out
}

var today = time.Date(2026, 3, 1, 15, 0, 0, 0, time.UTC)

func TestFindUnmatched(t *testing.T) {
	tests := []struct {
		name string
		txns []hledger.Transaction
		opts transfers.Options
		want []string
	}{
		{
			name: "matched pair",
			txns: []hledger.Transaction{
				side("a", "2026-01-05", "assets:checking", "500.00"),
				side("b", "2026-01-06", "liabilities:card", "-500.00"),
			},
			want: []string{},
		},
		{
			name: "lone side is missing",
			txns: []hledger.Transaction{side("a", "2026-01-05", "assets:checking", "500.00")},
			want: []string{"a:missing"},
		},
		{
			name: "window edge matches",
			txns: []hledger.Transaction{
				side("a", "2026-01-01", "assets:checking", "500.00"),
				side("b", "2026-01-06", "liabilities:card", "-500.00"),
			},
			want: []string{},
		},
		{
			name: "past window edge does not match",
			txns: []hledger.Transaction{
				side("a", "2026-01-01", "assets:checking", "500.00"),
				side("b", "2026-01-07", "liabilities:card", "-500.00"),
			},
			want: []string{"a:missing", "b:missing"},
		},
		{
			name: "custom window",
			txns: []hledger.Transaction{
				side("a", "2026-01-01", "assets:checking", "500.00"),
				side("b", "2026-01-07", "liabilities:card", "-500.00"),
			},
			opts: transfers.Options{WindowDays: 10},
			want: []string{},
		},
		{
			name: "same sign does not match",
			txns: []hledger.Transaction{
				side("a", "2026-01-05", "assets:checking", "500.00"),
				side("b", "2026-01-05", "liabilities:card", "500.00"),
			},
			want: []string{"a:missing", "b:missing"},
		},
		{
			name: "same source account does not match",
			txns: []hledger.Transaction{
				side("a", "2026-01-05", "assets:checking", "500.00"),
				side("b", "2026-01-05", "assets:checking", "-500.00"),
			},
			want: []string{"a:missing", "b:missing"},
		},
		{
			name: "different amounts do not match",
			txns: []hledger.Transaction{
				side("a", "2026-01-05", "assets:checking", "500.00"),
				side("b", "2026-01-05", "liabilities:card", "-499.99"),
			},
			want: []string{"a:missing", "b:missing"},
		},
		{
			name: "decimal places are normalized",
			txns: []hledger.Transaction{
				side("a", "2026-01-05", "assets:checking", "100"),
				side("b", "2026-01-05", "assets:savings", "-100.00"),
			},
			want: []string{},
		},
		{
			name: "different commodities do not match",
			txns: []hledger.Transaction{
				side("a", "2026-01-05", "assets:checking", "100.00"),
				{
					FID:  "b",
					Date: "2026-01-05",
					Postings: []hledger.Posting{
						posting("assets:savings", "EUR", "100.00"),
						posting("equity:transfers", "EUR", "-100.00"),
					},
				},
			},
			want: []string{"a:missing", "b:missing"},
		},
		{
			name: "duplicates pair one-to-one, closest date first",
			txns: []hledger.Transaction{
				side("a", "2026-01-01", "assets:checking", "100.00"),
				side("b", "2026-01-04", "assets:checking", "100.00"),
				side("c", "2026-01-05", "assets:savings", "-100.00"),
			},
			want: []string{"a:missing"},
		},
		{
			name: "equal distance ties go to the earlier side",
			txns: []hledger.Transaction{
				side("a", "2026-01-03", "assets:checking", "100.00"),
				side("b", "2026-01-07", "assets:checking", "100.00"),
				side("c", "2026-01-05", "assets:savings", "-100.00"),
			},
			want: []string{"b:missing"},
		},
		{
			name: "sides in the same route match",
			txns: []hledger.Transaction{
				routeSide("a", "2026-01-05", "equity:transfers:Schwab-Amex", "assets:schwab", "500.00"),
				routeSide("b", "2026-01-06", "equity:transfers:Schwab-Amex", "liabilities:amex", "-500.00"),
			},
			want: []string{},
		},
		{
			name: "sides in different routes do not match",
			txns: []hledger.Transaction{
				routeSide("a", "2026-01-05", "equity:transfers:Schwab-Amex", "assets:schwab", "500.00"),
				routeSide("b", "2026-01-05", "equity:transfers:Chase-Savings", "assets:savings", "-500.00"),
			},
			want: []string{"a:missing", "b:missing"},
		},
		{
			name: "a route posting does not match the parent account",
			txns: []hledger.Transaction{
				side("a", "2026-01-05", "assets:checking", "500.00"),
				routeSide("b", "2026-01-05", "equity:transfers:card", "liabilities:card", "-500.00"),
			},
			want: []string{"a:missing", "b:missing"},
		},
		{
			name: "deeper sub-accounts belong to their route",
			txns: []hledger.Transaction{
				routeSide("a", "2026-01-05", "equity:transfers:Schwab-Amex", "assets:schwab", "500.00"),
				routeSide("b", "2026-01-05", "equity:transfers:Schwab-Amex:pending", "liabilities:amex", "-500.00"),
			},
			want: []string{},
		},
		{
			name: "similarly named account is not the clearing account",
			txns: []hledger.Transaction{{
				FID:  "a",
				Date: "2026-01-05",
				Postings: []hledger.Posting{
					posting("assets:checking", "USD", "-5.00"),
					posting("equity:transfersx", "USD", "5.00"),
				},
			}},
			want: []string{},
		},
		{
			name: "two clearing postings are ignored",
			txns: []hledger.Transaction{{
				FID:  "a",
				Date: "2026-01-05",
				Postings: []hledger.Posting{
					posting("equity:transfers", "USD", "-5.00"),
					posting("equity:transfers", "USD", "5.00"),
				},
			}},
			want: []string{},
		},
		{
			name: "non-transfer transactions are ignored",
			txns: []hledger.Transaction{{
				FID:  "a",
				Date: "2026-01-05",
				Postings: []hledger.Posting{
					posting("expenses:food", "USD", "5.00"),
					posting("assets:checking", "USD", "-5.00"),
				},
			}},
			want: []string{},
		},
		{
			name: "within the window of today is recent",
			txns: []hledger.Transaction{
				side("a", "2026-02-24", "assets:checking", "1.00"),
				side("b", "2026-02-23", "assets:checking", "2.00"),
				side("c", "2026-03-04", "assets:checking", "3.00"),
			},
			want: []string{"a:recent", "b:missing", "c:recent"},
		},
		{
			name: "custom clearing account",
			txns: []hledger.Transaction{
				side("a", "2026-01-05", "assets:checking", "500.00"),
				{
					FID:  "b",
					Date: "2026-01-05",
					Postings: []hledger.Posting{
						posting("liabilities:card", "USD", "5.00"),
						posting("assets:in-transit", "USD", "-5.00"),
					},
				},
			},
			opts: transfers.Options{Account: "assets:in-transit"},
			want: []string{"b:missing"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.opts
			opts.Today = today
			got := fids(transfers.FindUnmatched(tt.txns, opts).Unmatched)
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRouteOf(t *testing.T) {
	tests := []struct {
		name, want string
		ok         bool
	}{
		{"equity:transfers", "equity:transfers", true},
		{"equity:transfers:Schwab-Amex", "equity:transfers:Schwab-Amex", true},
		{"equity:transfers:Schwab-Amex:pending", "equity:transfers:Schwab-Amex", true},
		{"equity:transfersx", "", false},
		{"equity", "", false},
		{"assets:checking", "", false},
	}
	for _, tt := range tests {
		got, ok := transfers.RouteOf(tt.name, "equity:transfers")
		if got != tt.want || ok != tt.ok {
			t.Errorf("RouteOf(%q) = %q, %v; want %q, %v", tt.name, got, ok, tt.want, tt.ok)
		}
	}
}

func TestFindUnmatchedExpectedAccounts(t *testing.T) {
	const cards = "equity:transfers:Chase-Cards"
	tests := []struct {
		name string
		txns []hledger.Transaction
		want map[string][]string // fid -> expected accounts
	}{
		{
			// Checking pays two cards through one route. A lone card payment
			// points at checking, not at the other card; a lone checking
			// payment could be for either card.
			name: "direction narrows a shared route",
			txns: []hledger.Transaction{
				routeSide("p1", "2026-01-05", cards, "assets:checking", "500.00"),
				routeSide("p2", "2026-01-05", cards, "liabilities:sapphire", "-500.00"),
				routeSide("p3", "2026-01-10", cards, "assets:checking", "200.00"),
				routeSide("p4", "2026-01-10", cards, "liabilities:freedom", "-200.00"),
				routeSide("lone-card", "2026-02-01", cards, "liabilities:sapphire", "-300.00"),
				routeSide("lone-checking", "2026-02-10", cards, "assets:checking", "150.00"),
			},
			want: map[string][]string{
				"lone-card":     {"assets:checking"},
				"lone-checking": {"liabilities:freedom", "liabilities:sapphire"},
			},
		},
		{
			name: "falls back to every other account without opposite-direction history",
			txns: []hledger.Transaction{
				routeSide("a", "2026-01-05", cards, "assets:checking", "10.00"),
				routeSide("b", "2026-01-20", cards, "assets:savings", "20.00"),
			},
			want: map[string][]string{
				"a": {"assets:savings"},
				"b": {"assets:checking"},
			},
		},
		{
			name: "a route seen from one account expects nothing",
			txns: []hledger.Transaction{routeSide("a", "2026-01-05", cards, "assets:checking", "10.00")},
			want: map[string][]string{"a": {}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := make(map[string][]string)
			for _, u := range transfers.FindUnmatched(tt.txns, transfers.Options{Today: today}).Unmatched {
				if u.Route != cards {
					t.Errorf("%s route = %q, want %q", u.Transaction.FID, u.Route, cards)
				}
				got[u.Transaction.FID] = append([]string{}, u.ExpectedAccounts...)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("unmatched = %v, want %v", got, tt.want)
			}
			for fid, want := range tt.want {
				if !slices.Equal(got[fid], want) {
					t.Errorf("%s expected accounts = %v, want %v", fid, got[fid], want)
				}
			}
		})
	}
}

func TestFindUnmatchedRoutes(t *testing.T) {
	report := transfers.FindUnmatched([]hledger.Transaction{
		routeSide("a", "2026-01-05", "equity:transfers:Schwab-Amex", "assets:schwab", "500.00"),
		routeSide("b", "2026-01-06", "equity:transfers:Schwab-Amex", "liabilities:amex", "-500.00"),
		routeSide("c", "2026-01-20", "equity:transfers:Schwab-Amex", "assets:schwab", "75.00"),
		routeSide("d", "2026-01-05", "equity:transfers:Chase-Savings", "assets:chase", "40.00"),
		routeSide("e", "2026-01-06", "equity:transfers:Chase-Savings", "assets:savings", "-40.00"),
		side("f", "2026-01-07", "assets:checking", "9.00"),
	}, transfers.Options{Today: today})

	var got []string
	for _, r := range report.Routes {
		got = append(got, r.Account+"="+strings.Join(r.Accounts, ","))
	}
	// Only routes with unmatched sides; Chase-Savings is fully matched.
	want := []string{
		"equity:transfers=assets:checking",
		"equity:transfers:Schwab-Amex=assets:schwab,liabilities:amex",
	}
	if !slices.Equal(got, want) {
		t.Errorf("routes = %v, want %v", got, want)
	}
}

func TestFindUnmatchedAmount(t *testing.T) {
	got := transfers.FindUnmatched([]hledger.Transaction{side("a", "2026-01-05", "assets:checking", "12.50")}, transfers.Options{Today: today}).Unmatched
	if len(got) != 1 {
		t.Fatalf("got %d unmatched, want 1", len(got))
	}
	if a := got[0].Amount; a.Commodity != "USD" || a.Quantity.Format() != "12.50" {
		t.Errorf("amount = %s %s, want 12.50 USD", a.Quantity.Format(), a.Commodity)
	}
}

// TestFindUnmatchedMatchesHledgerBalance runs real hledger over a fixture and
// checks each route's unmatched sides add up to hledger's balance of that
// route: every matched pair nets to zero, so whatever is left must be the
// unmatched sides. Routes are read from `bal --depth 3`, which clips deeper
// accounts into their route and leaves direct equity:transfers postings on
// the parent.
func TestFindUnmatchedMatchesHledgerBalance(t *testing.T) {
	c, err := hledger.New("hledger", filepath.Join("testdata", "transfers.journal"))
	if err != nil {
		t.Skipf("hledger not available: %v", err)
	}
	txns, err := c.Transactions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	got := transfers.FindUnmatched(txns, transfers.Options{Today: today}).Unmatched

	want := []string{"aa000011:missing", "aa000005:missing", "aa000007:missing", "aa000008:missing"}
	if !slices.Equal(fids(got), want) {
		t.Fatalf("unmatched = %v, want %v", fids(got), want)
	}

	sums := make(map[string]hledger.AmountQuantity)
	for _, u := range got {
		sums[u.Route] = sums[u.Route].Add(u.Amount.Quantity)
	}
	report, err := c.Balances(t.Context(), hledger.BalanceOpts{Depth: 3}, "acct:^equity:transfers(:|$)")
	if err != nil {
		t.Fatal(err)
	}
	balances := make(map[string]hledger.AmountQuantity)
	for _, row := range report.Rows {
		if len(row.Amounts) != 1 {
			t.Fatalf("%s balance = %+v, want one amount", row.FullName, row.Amounts)
		}
		balances[row.FullName] = row.Amounts[0].Quantity
	}
	for route := range balances {
		if _, ok := sums[route]; !ok {
			sums[route] = hledger.AmountQuantity{}
		}
	}
	for route, sum := range sums {
		bal := balances[route]
		negBal := hledger.AmountQuantity{DecimalMantissa: -bal.DecimalMantissa, DecimalPlaces: bal.DecimalPlaces}
		if diff := sum.Add(negBal); diff.DecimalMantissa != 0 {
			t.Errorf("%s: sum of unmatched = %s, hledger balance = %s", route, sum.Format(), bal.Format())
		}
	}
}
