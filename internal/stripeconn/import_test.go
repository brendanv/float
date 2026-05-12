package stripeconn

import (
	"testing"
	"time"

	"github.com/brendanv/float/internal/rules"
	"github.com/stripe/stripe-go/v82"
)

func TestRenderMinor(t *testing.T) {
	cases := []struct {
		minor    int64
		decimals int
		want     string
	}{
		{0, 2, "0.00"},
		{5000, 2, "50.00"},
		{-5000, 2, "-50.00"},
		{8050, 2, "80.50"},
		{-8050, 2, "-80.50"},
		{1, 2, "0.01"},
		{-1, 2, "-0.01"},
		{1234567, 2, "12345.67"},
		{1000, 0, "1000"},
		{-1000, 0, "-1000"},
		{1234, 3, "1.234"},
	}
	for _, tc := range cases {
		got := renderMinor(tc.minor, tc.decimals)
		if got != tc.want {
			t.Errorf("renderMinor(%d, %d) = %q, want %q", tc.minor, tc.decimals, got, tc.want)
		}
	}
}

func TestCurrencyDecimals(t *testing.T) {
	cases := []struct {
		currency string
		want     int
		ok       bool
	}{
		{"USD", 2, true},
		{"usd", 2, true},
		{"EUR", 2, true},
		{"JPY", 0, true},
		{"krw", 0, true},
		{"", 0, false},
	}
	for _, tc := range cases {
		got, ok := currencyDecimals(tc.currency)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("currencyDecimals(%q) = (%d, %v), want (%d, %v)", tc.currency, got, ok, tc.want, tc.ok)
		}
	}
}

// TestBuildTransactionInputSignConvention is the canonical proof of the
// sign rule. If this test changes, update the docstring on formatAmount.
func TestBuildTransactionInputSignConvention(t *testing.T) {
	type want struct {
		linkedAcct string
		linkedQty  string
		otherAcct  string
		otherQty   string
	}
	cases := []struct {
		name string
		conn Connection
		tx   stripe.FinancialConnectionsTransaction
		want want
	}{
		{
			name: "checking outflow ($50 ATM)",
			conn: Connection{
				HledgerAccount:        "assets:chase:checking",
				DefaultInflowAccount:  "income:unknown",
				DefaultOutflowAccount: "expenses:cash",
				Currency:              "USD",
				AccountCategory:       CategoryCash,
			},
			tx: stripe.FinancialConnectionsTransaction{
				ID:       "tx_atm",
				Amount:   -5000,
				Currency: "usd",
			},
			want: want{
				linkedAcct: "assets:chase:checking",
				linkedQty:  "-50.00",
				otherAcct:  "expenses:cash",
				otherQty:   "50.00",
			},
		},
		{
			name: "checking inflow ($1200 paycheck)",
			conn: Connection{
				HledgerAccount:        "assets:chase:checking",
				DefaultInflowAccount:  "income:salary",
				DefaultOutflowAccount: "expenses:unknown",
				Currency:              "USD",
				AccountCategory:       CategoryCash,
			},
			tx: stripe.FinancialConnectionsTransaction{
				ID:       "tx_pay",
				Amount:   120000,
				Currency: "usd",
			},
			want: want{
				linkedAcct: "assets:chase:checking",
				linkedQty:  "1200.00",
				otherAcct:  "income:salary",
				otherQty:   "-1200.00",
			},
		},
		{
			name: "credit card $80 purchase",
			conn: Connection{
				HledgerAccount:        "liabilities:amex",
				DefaultInflowAccount:  "income:unknown",
				DefaultOutflowAccount: "expenses:foo",
				Currency:              "USD",
				AccountCategory:       CategoryCredit,
			},
			tx: stripe.FinancialConnectionsTransaction{
				ID:       "tx_cc_buy",
				Amount:   -8000,
				Currency: "usd",
			},
			want: want{
				linkedAcct: "liabilities:amex",
				linkedQty:  "-80.00",
				otherAcct:  "expenses:foo",
				otherQty:   "80.00",
			},
		},
		{
			name: "credit card $80 refund",
			conn: Connection{
				HledgerAccount:        "liabilities:amex",
				DefaultInflowAccount:  "income:refunds",
				DefaultOutflowAccount: "expenses:unknown",
				Currency:              "USD",
				AccountCategory:       CategoryCredit,
			},
			tx: stripe.FinancialConnectionsTransaction{
				ID:       "tx_cc_refund",
				Amount:   8000,
				Currency: "usd",
			},
			want: want{
				linkedAcct: "liabilities:amex",
				linkedQty:  "80.00",
				otherAcct:  "income:refunds",
				otherQty:   "-80.00",
			},
		},
		{
			name: "JPY zero-decimal currency",
			conn: Connection{
				HledgerAccount:        "assets:japan:checking",
				DefaultInflowAccount:  "income:unknown",
				DefaultOutflowAccount: "expenses:unknown",
				Currency:              "JPY",
				AccountCategory:       CategoryCash,
			},
			tx: stripe.FinancialConnectionsTransaction{
				ID:       "tx_jpy",
				Amount:   -1500,
				Currency: "jpy",
			},
			want: want{
				linkedAcct: "assets:japan:checking",
				linkedQty:  "-1500",
				otherAcct:  "expenses:unknown",
				otherQty:   "1500",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.conn.ID = "conn1"
			tc.tx.TransactedAt = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC).Unix()
			got, err := buildTransactionInput(tc.conn, &tc.tx, nil, "batch1")
			if err != nil {
				t.Fatalf("buildTransactionInput: %v", err)
			}
			if len(got.Postings) != 2 {
				t.Fatalf("expected 2 postings, got %d", len(got.Postings))
			}
			if got.Postings[0].Account != tc.want.linkedAcct {
				t.Errorf("linked account: %q, want %q", got.Postings[0].Account, tc.want.linkedAcct)
			}
			if got.Postings[0].Quantity != tc.want.linkedQty {
				t.Errorf("linked qty: %q, want %q", got.Postings[0].Quantity, tc.want.linkedQty)
			}
			if got.Postings[1].Account != tc.want.otherAcct {
				t.Errorf("other account: %q, want %q", got.Postings[1].Account, tc.want.otherAcct)
			}
			if got.Postings[1].Quantity != tc.want.otherQty {
				t.Errorf("other qty: %q, want %q", got.Postings[1].Quantity, tc.want.otherQty)
			}
			// Required tags
			if got.Tags[TagStripeTxnID] != tc.tx.ID {
				t.Errorf("stripe-txn-id tag: got %q, want %q", got.Tags[TagStripeTxnID], tc.tx.ID)
			}
			if got.Tags[TagSource] != TagSourceValue {
				t.Errorf("source tag: got %q, want %q", got.Tags[TagSource], TagSourceValue)
			}
			if got.Tags[TagStripeConnID] != "conn1" {
				t.Errorf("stripe-connection tag: got %q, want %q", got.Tags[TagStripeConnID], "conn1")
			}
		})
	}
}

func TestBuildTransactionInputAppliesRules(t *testing.T) {
	conn := Connection{
		ID:                    "conn1",
		HledgerAccount:        "assets:checking",
		DefaultInflowAccount:  "income:unknown",
		DefaultOutflowAccount: "expenses:unknown",
		Currency:              "USD",
	}
	tx := stripe.FinancialConnectionsTransaction{
		ID:           "tx_starbucks",
		Amount:       -550,
		Currency:     "usd",
		Description:  "STARBUCKS #1234",
		TransactedAt: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC).Unix(),
	}
	rs := []rules.Rule{
		{
			Pattern:      "STARBUCKS",
			Payee:        "Starbucks",
			Account:      "expenses:coffee",
			Tags:         map[string]string{"category": "coffee"},
			AutoReviewed: true,
		},
	}
	got, err := buildTransactionInput(conn, &tx, rs, "batch1")
	if err != nil {
		t.Fatalf("buildTransactionInput: %v", err)
	}
	if got.Postings[1].Account != "expenses:coffee" {
		t.Errorf("other account: got %q, want expenses:coffee", got.Postings[1].Account)
	}
	if got.Description != "Starbucks | STARBUCKS #1234" {
		t.Errorf("description: %q", got.Description)
	}
	if got.Tags["category"] != "coffee" {
		t.Errorf("category tag missing: %+v", got.Tags)
	}
	if got.Status != "Cleared" {
		t.Errorf("status: got %q, want Cleared", got.Status)
	}
}

func TestBuildTransactionInputUnknownCurrencyErrors(t *testing.T) {
	conn := Connection{
		ID:                    "conn1",
		HledgerAccount:        "assets:foo",
		DefaultInflowAccount:  "income:unknown",
		DefaultOutflowAccount: "expenses:unknown",
		// Currency intentionally empty so we fall through to the tx's
		// (also-empty) currency.
		Currency: "",
	}
	tx := stripe.FinancialConnectionsTransaction{
		ID:           "tx_x",
		Amount:       -100,
		Currency:     "",
		TransactedAt: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC).Unix(),
	}
	if _, err := buildTransactionInput(conn, &tx, nil, "batch1"); err == nil {
		t.Fatal("expected error for unknown currency, got nil")
	}
}
