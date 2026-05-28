package ledger_test

import (
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
)

// writeAssertionJournal writes a small journal with an asserted asset account,
// an unasserted liability account, and non-asset/liability accounts that must be
// excluded from the result. Returns the data dir.
func writeAssertionJournal(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	journal := `2026-01-01 (aa000001) Opening balance
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

	// Only asset and liability accounts with postings: assets:checking and
	// liabilities:card. equity:opening and expenses:food must be excluded.
	if len(got) != 2 {
		var names []string
		for _, a := range got {
			names = append(names, a.Account)
		}
		t.Fatalf("expected 2 accounts, got %d: %v", len(got), names)
	}

	// Never-asserted account sorts first.
	card := got[0]
	if card.Account != "liabilities:card" {
		t.Errorf("first account = %q, want liabilities:card", card.Account)
	}
	if card.Type != "L" {
		t.Errorf("liabilities:card type = %q, want L", card.Type)
	}
	if card.LastAssertionDate != nil {
		t.Errorf("liabilities:card last_assertion_date = %q, want nil", card.GetLastAssertionDate())
	}
	if card.LastTransaction == nil || card.LastTransaction.Date != "2026-01-15" {
		t.Errorf("liabilities:card last_transaction date = %v, want 2026-01-15", card.LastTransaction)
	}

	checking := got[1]
	if checking.Account != "assets:checking" {
		t.Errorf("second account = %q, want assets:checking", checking.Account)
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
}
