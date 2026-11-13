package journal_test

import (
	"errors"
	"testing"
	"time"

	"github.com/brendanv/float/internal/journal"
)

func TestParseInputs(t *testing.T) {
	c := mustHledger(t)
	date := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	good := journal.TransactionInput{
		Date:        date,
		Description: "Cafe | latte",
		Tags:        map[string]string{"trip": "nyc"},
		Postings: []journal.PostingInput{
			{Account: "expenses:food", Commodity: "USD", Quantity: "4.50"},
			{Account: "assets:checking"},
		},
	}
	bad := journal.TransactionInput{
		Date:        date,
		Description: "unbalanced",
		Postings: []journal.PostingInput{
			{Account: "expenses:food", Commodity: "USD", Quantity: "4.50"},
			{Account: "assets:checking", Commodity: "USD", Quantity: "-4.00"},
		},
	}

	tests := []struct {
		name      string
		inputs    []journal.TransactionInput
		wantCount int
		wantIndex int // -1 = no error expected
	}{
		{name: "empty", inputs: nil, wantCount: 0, wantIndex: -1},
		{name: "valid batch", inputs: []journal.TransactionInput{good, good}, wantCount: 2, wantIndex: -1},
		{name: "invalid in middle", inputs: []journal.TransactionInput{good, bad, good}, wantIndex: 1},
		{name: "single invalid", inputs: []journal.TransactionInput{bad}, wantIndex: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			txns, err := journal.ParseInputs(t.Context(), c, tt.inputs)
			if tt.wantIndex >= 0 {
				var ie *journal.InputError
				if !errors.As(err, &ie) {
					t.Fatalf("expected *InputError, got %v", err)
				}
				if ie.Index != tt.wantIndex {
					t.Errorf("Index = %d, want %d", ie.Index, tt.wantIndex)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(txns) != tt.wantCount {
				t.Fatalf("got %d transactions, want %d", len(txns), tt.wantCount)
			}
			for _, tx := range txns {
				if tx.Date != "2026-03-04" || tx.Description != "Cafe | latte" {
					t.Errorf("unexpected transaction %s %q", tx.Date, tx.Description)
				}
				if got := tx.Postings[1].Amounts[0].Quantity.FloatingPoint; got != -4.5 {
					t.Errorf("auto-balanced amount = %v, want -4.5", got)
				}
			}
		})
	}
}
