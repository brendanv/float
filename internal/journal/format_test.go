package journal

import (
	"strings"
	"testing"
	"time"
)

func TestDraftFormatBalanceAssertion(t *testing.T) {
	date := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	base := func(p PostingInput) TransactionInput {
		return TransactionInput{
			Date:        date,
			Description: "test",
			Postings: []PostingInput{
				p,
				{Account: "income:salary"},
			},
		}
	}

	tests := []struct {
		name    string
		posting PostingInput
		want    string // the line we expect for the first posting
	}{
		{
			name:    "no assertion, no comment",
			posting: PostingInput{Account: "assets:checking", Commodity: "$", Quantity: "100.00"},
			want:    "    assets:checking  100.00 $\n",
		},
		{
			name: "= assertion with amount",
			posting: PostingInput{
				Account: "assets:checking", Commodity: "$", Quantity: "100.00",
				BalanceAssertion: &BalanceAssertionInput{Commodity: "$", Quantity: "500.00"},
			},
			want: "    assets:checking  100.00 $ = 500.00 $\n",
		},
		{
			name: "= assertion on auto-balance posting",
			posting: PostingInput{
				Account:          "assets:checking",
				BalanceAssertion: &BalanceAssertionInput{Commodity: "$", Quantity: "500.00"},
			},
			want: "    assets:checking = 500.00 $\n",
		},
		{
			name: "=* inclusive assertion",
			posting: PostingInput{
				Account: "assets:savings", Commodity: "$", Quantity: "200.00",
				BalanceAssertion: &BalanceAssertionInput{Commodity: "$", Quantity: "200.00", Inclusive: true},
			},
			want: "    assets:savings  200.00 $ =* 200.00 $\n",
		},
		{
			name: "== total assertion",
			posting: PostingInput{
				Account: "assets:checking", Commodity: "$", Quantity: "-50.00",
				BalanceAssertion: &BalanceAssertionInput{Commodity: "$", Quantity: "1250.00", Total: true},
			},
			want: "    assets:checking  -50.00 $ == 1250.00 $\n",
		},
		{
			name: "==* inclusive total assertion",
			posting: PostingInput{
				Account: "assets:savings", Commodity: "$", Quantity: "100.00",
				BalanceAssertion: &BalanceAssertionInput{Commodity: "$", Quantity: "100.00", Inclusive: true, Total: true},
			},
			want: "    assets:savings  100.00 $ ==* 100.00 $\n",
		},
		{
			name: "assertion before inline comment",
			posting: PostingInput{
				Account: "assets:checking", Commodity: "$", Quantity: "100.00",
				Comment:          "verified",
				BalanceAssertion: &BalanceAssertionInput{Commodity: "$", Quantity: "500.00"},
			},
			want: "    assets:checking  100.00 $ = 500.00 $  ; verified\n",
		},
		{
			name: "auto-balance + assertion + comment",
			posting: PostingInput{
				Account:          "assets:checking",
				Comment:          "balance check",
				BalanceAssertion: &BalanceAssertionInput{Commodity: "$", Quantity: "500.00"},
			},
			want: "    assets:checking = 500.00 $  ; balance check\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := draftFormat(base(tt.posting), "aa000001")
			if !strings.Contains(got, tt.want) {
				t.Errorf("draftFormat output missing expected line:\nwant: %q\ngot:\n%s", tt.want, got)
			}
		})
	}
}

func TestPostingAssertionString(t *testing.T) {
	tests := []struct {
		name string
		ba   *BalanceAssertionInput
		want string
	}{
		{name: "nil", ba: nil, want: ""},
		{name: "simple", ba: &BalanceAssertionInput{Commodity: "$", Quantity: "100.00"}, want: " = 100.00 $"},
		{name: "inclusive", ba: &BalanceAssertionInput{Commodity: "$", Quantity: "100.00", Inclusive: true}, want: " =* 100.00 $"},
		{name: "total", ba: &BalanceAssertionInput{Commodity: "$", Quantity: "100.00", Total: true}, want: " == 100.00 $"},
		{name: "inclusive total", ba: &BalanceAssertionInput{Commodity: "$", Quantity: "100.00", Inclusive: true, Total: true}, want: " ==* 100.00 $"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := postingAssertionString(PostingInput{BalanceAssertion: tt.ba})
			if got != tt.want {
				t.Errorf("postingAssertionString = %q, want %q", got, tt.want)
			}
		})
	}
}
