package ledger

import (
	"testing"
	"time"

	stripeClient "github.com/brendanv/float/internal/stripe"
)

func TestStripeTransactionToHledger(t *testing.T) {
	base := time.Date(2026, 5, 10, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		name           string
		txn            stripeClient.Transaction
		hledgerAccount string
		wantDate       string
		wantDesc       string
		wantStatus     string
		wantMainAmt    float64
		wantMainComm   string
		wantCounterAmt float64
		wantCounterAcc string
	}{
		{
			name: "posted transaction",
			txn: stripeClient.Transaction{
				ID:           "fca_txn_posted",
				AccountID:    "fca_acct1",
				AmountCents:  12345,
				Currency:     "usd",
				Description:  "Coffee shop",
				TransactedAt: base,
				Status:       "posted",
			},
			hledgerAccount: "assets:checking:chase",
			wantDate:       "2026-05-10",
			wantDesc:       "Coffee shop",
			wantStatus:     "",
			wantMainAmt:    123.45,
			wantMainComm:   "USD",
			wantCounterAmt: -123.45,
			wantCounterAcc: "expenses:unknown",
		},
		{
			name: "status is not carried into the journal entry",
			txn: stripeClient.Transaction{
				ID:           "fca_txn_pend",
				AccountID:    "fca_acct1",
				AmountCents:  500,
				Currency:     "usd",
				Description:  "Some charge",
				TransactedAt: base,
				Status:       "pending",
			},
			hledgerAccount: "assets:savings",
			wantDate:       "2026-05-10",
			wantDesc:       "Some charge",
			wantStatus:     "",
			wantMainAmt:    5.00,
			wantMainComm:   "USD",
			wantCounterAmt: -5.00,
			wantCounterAcc: "expenses:unknown",
		},
		{
			name: "negative amount (refund)",
			txn: stripeClient.Transaction{
				ID:           "fca_txn_refund",
				AccountID:    "fca_acct1",
				AmountCents:  -2000,
				Currency:     "usd",
				Description:  "Refund",
				TransactedAt: base,
				Status:       "posted",
			},
			hledgerAccount: "assets:checking",
			wantDate:       "2026-05-10",
			wantDesc:       "Refund",
			wantStatus:     "",
			wantMainAmt:    -20.00,
			wantMainComm:   "USD",
			wantCounterAmt: 20.00,
			wantCounterAcc: "expenses:unknown",
		},
		{
			name: "currency uppercased",
			txn: stripeClient.Transaction{
				ID:           "fca_txn_eur",
				AccountID:    "fca_acct2",
				AmountCents:  1000,
				Currency:     "eur",
				Description:  "Euro charge",
				TransactedAt: base,
				Status:       "posted",
			},
			hledgerAccount: "assets:eur",
			wantDate:       "2026-05-10",
			wantDesc:       "Euro charge",
			wantStatus:     "",
			wantMainAmt:    10.00,
			wantMainComm:   "EUR",
			wantCounterAmt: -10.00,
			wantCounterAcc: "expenses:unknown",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ht := stripeTransactionToHledger(tc.txn, tc.hledgerAccount)

			if ht.Date != tc.wantDate {
				t.Errorf("Date: got %q, want %q", ht.Date, tc.wantDate)
			}
			if ht.Description != tc.wantDesc {
				t.Errorf("Description: got %q, want %q", ht.Description, tc.wantDesc)
			}
			if ht.Status != tc.wantStatus {
				t.Errorf("Status: got %q, want %q", ht.Status, tc.wantStatus)
			}
			if len(ht.Postings) != 2 {
				t.Fatalf("Postings: got %d, want 2", len(ht.Postings))
			}
			main := ht.Postings[0]
			if main.Account != tc.hledgerAccount {
				t.Errorf("main posting account: got %q, want %q", main.Account, tc.hledgerAccount)
			}
			if len(main.Amounts) == 0 {
				t.Fatal("main posting has no amounts")
			}
			if main.Amounts[0].Commodity != tc.wantMainComm {
				t.Errorf("main posting commodity: got %q, want %q", main.Amounts[0].Commodity, tc.wantMainComm)
			}
			if main.Amounts[0].Quantity.FloatingPoint != tc.wantMainAmt {
				t.Errorf("main posting amount: got %v, want %v", main.Amounts[0].Quantity.FloatingPoint, tc.wantMainAmt)
			}
			counter := ht.Postings[1]
			if counter.Account != tc.wantCounterAcc {
				t.Errorf("counter posting account: got %q, want %q", counter.Account, tc.wantCounterAcc)
			}
			if counter.Amounts[0].Quantity.FloatingPoint != tc.wantCounterAmt {
				t.Errorf("counter posting amount: got %v, want %v", counter.Amounts[0].Quantity.FloatingPoint, tc.wantCounterAmt)
			}
		})
	}
}

func TestStripeTransactionToInput(t *testing.T) {
	base := time.Date(2026, 5, 10, 22, 45, 0, 0, time.UTC)
	batchID := "stripe-fca-acct1/2026-05-10-a1b2c3d4"

	tests := []struct {
		name           string
		txn            stripeClient.Transaction
		hledgerAccount string
		wantDate       time.Time
		wantStatus     string
		wantStripeTag  string
		wantMainQty    string
		wantCounterQty string
		wantMainComm   string
	}{
		{
			name: "posted transaction",
			txn: stripeClient.Transaction{
				ID:           "fca_txn_123",
				AmountCents:  9999,
				Currency:     "usd",
				Description:  "Amazon",
				TransactedAt: base,
				Status:       "posted",
			},
			hledgerAccount: "assets:checking",
			wantDate:       time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
			wantStatus:     "",
			wantStripeTag:  "fca_txn_123",
			wantMainQty:    "99.99",
			wantCounterQty: "-99.99",
			wantMainComm:   "USD",
		},
		{
			name: "status is not carried into the journal entry",
			txn: stripeClient.Transaction{
				ID:           "fca_txn_456",
				AmountCents:  100,
				Currency:     "usd",
				Description:  "Gas station",
				TransactedAt: base,
				Status:       "pending",
			},
			hledgerAccount: "assets:checking",
			wantDate:       time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
			wantStatus:     "",
			wantStripeTag:  "fca_txn_456",
			wantMainQty:    "1.00",
			wantCounterQty: "-1.00",
			wantMainComm:   "USD",
		},
		{
			name: "timestamp truncated to day boundary",
			txn: stripeClient.Transaction{
				ID:           "fca_txn_789",
				AmountCents:  5000,
				Currency:     "usd",
				Description:  "Dinner",
				TransactedAt: time.Date(2026, 5, 15, 23, 59, 59, 0, time.UTC),
				Status:       "posted",
			},
			hledgerAccount: "assets:checking",
			wantDate:       time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC),
			wantStatus:     "",
			wantStripeTag:  "fca_txn_789",
			wantMainQty:    "50.00",
			wantCounterQty: "-50.00",
			wantMainComm:   "USD",
		},
		{
			name: "negative amount (refund)",
			txn: stripeClient.Transaction{
				ID:           "fca_txn_refund",
				AmountCents:  -3000,
				Currency:     "usd",
				Description:  "Refund",
				TransactedAt: base,
				Status:       "posted",
			},
			hledgerAccount: "assets:checking",
			wantDate:       time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
			wantStatus:     "",
			wantStripeTag:  "fca_txn_refund",
			wantMainQty:    "-30.00",
			wantCounterQty: "30.00",
			wantMainComm:   "USD",
		},
		{
			name: "currency uppercased",
			txn: stripeClient.Transaction{
				ID:           "fca_txn_eur",
				AmountCents:  800,
				Currency:     "eur",
				Description:  "Croissant",
				TransactedAt: base,
				Status:       "posted",
			},
			hledgerAccount: "assets:eur",
			wantDate:       time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
			wantStatus:     "",
			wantStripeTag:  "fca_txn_eur",
			wantMainQty:    "8.00",
			wantCounterQty: "-8.00",
			wantMainComm:   "EUR",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inp := stripeTransactionToInput(tc.txn, tc.hledgerAccount, batchID)

			if !inp.Date.Equal(tc.wantDate) {
				t.Errorf("Date: got %v, want %v", inp.Date, tc.wantDate)
			}
			if inp.Description != tc.txn.Description {
				t.Errorf("Description: got %q, want %q", inp.Description, tc.txn.Description)
			}
			if inp.Status != tc.wantStatus {
				t.Errorf("Status: got %q, want %q", inp.Status, tc.wantStatus)
			}

			if _, hasVisible := inp.Tags["stripe-txn"]; hasVisible {
				t.Error("stripe-txn must not appear in user-visible Tags")
			}

			if got := inp.FloatMeta["float-import"]; got != batchID {
				t.Errorf("FloatMeta[float-import]: got %q, want %q", got, batchID)
			}
			if got := inp.FloatMeta["float-stripe-txn"]; got != tc.wantStripeTag {
				t.Errorf("FloatMeta[float-stripe-txn]: got %q, want %q", got, tc.wantStripeTag)
			}

			if len(inp.Postings) != 2 {
				t.Fatalf("Postings: got %d, want 2", len(inp.Postings))
			}
			main := inp.Postings[0]
			if main.Account != tc.hledgerAccount {
				t.Errorf("main posting account: got %q, want %q", main.Account, tc.hledgerAccount)
			}
			if main.Commodity != tc.wantMainComm {
				t.Errorf("main posting commodity: got %q, want %q", main.Commodity, tc.wantMainComm)
			}
			if main.Quantity != tc.wantMainQty {
				t.Errorf("main posting quantity: got %q, want %q", main.Quantity, tc.wantMainQty)
			}
			counter := inp.Postings[1]
			if counter.Account != "expenses:unknown" {
				t.Errorf("counter posting account: got %q, want %q", counter.Account, "expenses:unknown")
			}
			if counter.Commodity != tc.wantMainComm {
				t.Errorf("counter posting commodity: got %q, want %q", counter.Commodity, tc.wantMainComm)
			}
			if counter.Quantity != tc.wantCounterQty {
				t.Errorf("counter posting quantity: got %q, want %q", counter.Quantity, tc.wantCounterQty)
			}
		})
	}
}

func TestStripeAccountSlug(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"fca_acct_xyz", "fca-acct-xyz"},
		{"fca_ACCT_XYZ", "fca-acct-xyz"},
		{"fca-already-dashed", "fca-already-dashed"},
	}
	for _, tc := range tests {
		if got := stripeAccountSlug(tc.in); got != tc.want {
			t.Errorf("stripeAccountSlug(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
