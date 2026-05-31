package ledger_test

import (
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
)

// writeInvestmentAssertionJournal writes a journal with an investment account
// holding two lots of the same commodity at different cost bases, plus a full
// liquidation, so the net position is zero. Returns the data dir.
func writeInvestmentAssertionJournal(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	journal := `2026-01-01 (bb000001) Buy 10 VTSAX
    assets:investments:brokerage    10 VTSAX @ $100
    assets:checking                $-1000

2026-01-15 (bb000002) Buy 5 more VTSAX
    assets:investments:brokerage    5 VTSAX @ $110
    assets:checking                $-550

2026-02-01 (bb000003) Sell 10 VTSAX first lot
    assets:investments:brokerage   -10 VTSAX @ $100
    assets:checking                $1000

2026-02-15 (bb000004) Sell 5 VTSAX second lot
    assets:investments:brokerage    -5 VTSAX @ $110
    assets:checking                $550
`
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte(journal), 0o644); err != nil {
		t.Fatalf("write journal: %v", err)
	}
	return dir
}

func TestGetBalanceAssertionStatus_ZeroInvestmentPosition(t *testing.T) {
	dir := writeInvestmentAssertionJournal(t)
	h := mustRealHandler(t, dir)

	resp, err := h.GetBalanceAssertionStatus(t.Context(), connect.NewRequest(&floatv1.GetBalanceAssertionStatusRequest{}))
	if err != nil {
		t.Fatalf("GetBalanceAssertionStatus: %v", err)
	}

	// Find the brokerage account.
	var brokerage *floatv1.AccountAssertionStatus
	for _, a := range resp.Msg.Accounts {
		if a.Account == "assets:investments:brokerage" {
			brokerage = a
			break
		}
	}
	if brokerage == nil {
		t.Fatal("assets:investments:brokerage not found in response")
	}

	// The net VTSAX position is zero; it must not appear in the balance.
	for _, amt := range brokerage.Balance {
		if amt.Commodity == "VTSAX" {
			t.Errorf("VTSAX balance should be omitted (net zero), got quantity %q", amt.Quantity)
		}
	}
}

// writeAssertionJournal writes a small journal with an asserted asset account,
// an unasserted liability account, and non-asset/liability accounts that must be
// excluded from the result. Returns the data dir.
func writeAssertionJournal(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	journal := `2025-12-01 (aa000000) Savings opening balance
    assets:savings             $5000 = $5000
    equity:opening

2026-01-01 (aa000001) Opening balance
    assets:checking            $1000 = $1000
    equity:opening

2026-01-15 (aa000002) Pay credit card
    liabilities:card           $100
    assets:checking           $-100

2026-02-01 (aa000003) Groceries
    expenses:food              $50
    assets:checking           $-50
`
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte(journal), 0o644); err != nil {
		t.Fatalf("write journal: %v", err)
	}
	return dir
}

func TestGetBalanceAssertionStatus(t *testing.T) {
	dir := writeAssertionJournal(t)
	h := mustRealHandler(t, dir)

	resp, err := h.GetBalanceAssertionStatus(t.Context(), connect.NewRequest(&floatv1.GetBalanceAssertionStatusRequest{}))
	if err != nil {
		t.Fatalf("GetBalanceAssertionStatus: %v", err)
	}
	got := resp.Msg.Accounts

	// Only asset and liability accounts with postings: assets:checking,
	// assets:savings, and liabilities:card. equity:opening and expenses:food must
	// be excluded.
	if len(got) != 3 {
		var names []string
		for _, a := range got {
			names = append(names, a.Account)
		}
		t.Fatalf("expected 3 accounts, got %d: %v", len(got), names)
	}

	// Accounts sort by transactions since the last assertion, highest first.
	checking := got[0]
	if checking.Account != "assets:checking" {
		t.Errorf("first account = %q, want assets:checking", checking.Account)
	}
	if checking.GetTransactionsSinceLastAssertion() != 2 {
		t.Errorf("assets:checking transactions_since_last_assertion = %d, want 2", checking.GetTransactionsSinceLastAssertion())
	}

	card := got[1]
	if card.Account != "liabilities:card" {
		t.Errorf("second account = %q, want liabilities:card", card.Account)
	}
	if card.Type != "L" {
		t.Errorf("liabilities:card type = %q, want L", card.Type)
	}
	if card.LastAssertionDate != nil {
		t.Errorf("liabilities:card last_assertion_date = %q, want nil", card.GetLastAssertionDate())
	}
	if card.GetTransactionsSinceLastAssertion() != 1 {
		t.Errorf("liabilities:card transactions_since_last_assertion = %d, want 1", card.GetTransactionsSinceLastAssertion())
	}
	if card.LastTransaction == nil || card.LastTransaction.Date != "2026-01-15" {
		t.Errorf("liabilities:card last_transaction date = %v, want 2026-01-15", card.LastTransaction)
	}
	if checking.Type != "A" {
		t.Errorf("assets:checking type = %q, want A", checking.Type)
	}
	if checking.GetLastAssertionDate() != "2026-01-01" {
		t.Errorf("assets:checking last_assertion_date = %q, want 2026-01-01", checking.GetLastAssertionDate())
	}
	// Most recent transaction touching checking is the 2026-02-01 groceries entry.
	if checking.LastTransaction == nil || checking.LastTransaction.Date != "2026-02-01" {
		t.Errorf("assets:checking last_transaction date = %v, want 2026-02-01", checking.LastTransaction)
	}
	if len(checking.Balance) == 0 {
		t.Errorf("assets:checking balance is empty, want a current balance")
	}

	savings := got[2]
	if savings.Account != "assets:savings" {
		t.Errorf("third account = %q, want assets:savings", savings.Account)
	}
	if savings.GetLastAssertionDate() != "2025-12-01" {
		t.Errorf("assets:savings last_assertion_date = %q, want 2025-12-01", savings.GetLastAssertionDate())
	}
	if savings.GetTransactionsSinceLastAssertion() != 0 {
		t.Errorf("assets:savings transactions_since_last_assertion = %d, want 0", savings.GetTransactionsSinceLastAssertion())
	}
}
