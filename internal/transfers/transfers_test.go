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

// side is a transfer side: source posts -clearing, the clearing account posts clearing.
func side(fid, date, source, clearing string) hledger.Transaction {
	neg := "-" + clearing
	if strings.HasPrefix(clearing, "-") {
		neg = clearing[1:]
	}
	return hledger.Transaction{
		FID:  fid,
		Date: date,
		Postings: []hledger.Posting{
			posting(source, "USD", neg),
			posting("equity:transfers", "USD", clearing),
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
			name: "sub-accounts of the clearing account count",
			txns: []hledger.Transaction{
				side("a", "2026-01-05", "assets:checking", "500.00"),
				{
					FID:  "b",
					Date: "2026-01-05",
					Postings: []hledger.Posting{
						posting("liabilities:card", "USD", "500.00"),
						posting("equity:transfers:card", "USD", "-500.00"),
					},
				},
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
			got := fids(transfers.FindUnmatched(tt.txns, opts))
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindUnmatchedAmount(t *testing.T) {
	got := transfers.FindUnmatched([]hledger.Transaction{side("a", "2026-01-05", "assets:checking", "12.50")}, transfers.Options{Today: today})
	if len(got) != 1 {
		t.Fatalf("got %d unmatched, want 1", len(got))
	}
	if a := got[0].Amount; a.Commodity != "USD" || a.Quantity.Format() != "12.50" {
		t.Errorf("amount = %s %s, want 12.50 USD", a.Quantity.Format(), a.Commodity)
	}
}

// TestFindUnmatchedMatchesHledgerBalance runs real hledger over a fixture and
// checks the unmatched sides add up to hledger's balance of the clearing
// account: every matched pair nets to zero, so whatever is left must be the
// unmatched sides.
func TestFindUnmatchedMatchesHledgerBalance(t *testing.T) {
	c, err := hledger.New("hledger", filepath.Join("testdata", "transfers.journal"))
	if err != nil {
		t.Skipf("hledger not available: %v", err)
	}
	txns, err := c.Transactions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	got := transfers.FindUnmatched(txns, transfers.Options{Today: today})

	if want := []string{"aa000005:missing", "aa000007:missing", "aa000008:missing"}; !slices.Equal(fids(got), want) {
		t.Fatalf("unmatched = %v, want %v", fids(got), want)
	}

	var sum hledger.AmountQuantity
	for _, u := range got {
		sum = sum.Add(u.Amount.Quantity)
	}
	report, err := c.Balances(t.Context(), hledger.BalanceOpts{}, "acct:^equity:transfers$")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Total) != 1 {
		t.Fatalf("hledger balance total = %+v, want one amount", report.Total)
	}
	want := report.Total[0].Quantity
	negWant := hledger.AmountQuantity{DecimalMantissa: -want.DecimalMantissa, DecimalPlaces: want.DecimalPlaces}
	if diff := sum.Add(negWant); diff.DecimalMantissa != 0 {
		t.Errorf("sum of unmatched = %s, hledger balance = %s", sum.Format(), want.Format())
	}
}
