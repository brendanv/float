package ui

import (
	"strings"
	"testing"

	floatv1 "github.com/brendanv/float/gen/float/v1"
)

func TestPrimaryPosting_ExpensesFirst(t *testing.T) {
	tx := &floatv1.Transaction{
		Postings: []*floatv1.Posting{
			{Account: "assets:checking"},
			{Account: "expenses:food"},
		},
	}
	post := primaryPosting(tx)
	if post == nil || post.Account != "expenses:food" {
		t.Errorf("expected expenses:food, got %v", post)
	}
}

func TestPrimaryPosting_IncomeFirst(t *testing.T) {
	tx := &floatv1.Transaction{
		Postings: []*floatv1.Posting{
			{Account: "assets:checking"},
			{Account: "income:salary"},
		},
	}
	post := primaryPosting(tx)
	if post == nil || post.Account != "income:salary" {
		t.Errorf("expected income:salary, got %v", post)
	}
}

func TestPrimaryPosting_FallsBackToFirst(t *testing.T) {
	tx := &floatv1.Transaction{
		Postings: []*floatv1.Posting{
			{Account: "assets:checking"},
			{Account: "liabilities:visa"},
		},
	}
	post := primaryPosting(tx)
	if post == nil || post.Account != "assets:checking" {
		t.Errorf("expected first posting, got %v", post)
	}
}

func TestPrimaryPosting_NoPostings(t *testing.T) {
	tx := &floatv1.Transaction{}
	post := primaryPosting(tx)
	if post != nil {
		t.Errorf("expected nil, got %v", post)
	}
}

func TestTransactionsPanel_ColumnWidths(t *testing.T) {
	tests := []struct {
		name      string
		width     int
		wantDesc  int
		wantAcct  int
	}{
		{
			name:  "80 columns",
			width: 80,
			// remaining = 80 - 2 (St) - 10 (Date) - 13 (Amount) - 4 (separators) = 51
			// descWidth = 51 * 40 / 100 = 20
			// acctWidth = 51 - 20 = 31
			wantDesc: 20,
			wantAcct: 31,
		},
		{
			name:  "120 columns",
			width: 120,
			// remaining = 120 - 2 - 10 - 13 - 4 = 91
			// descWidth = 91 * 40 / 100 = 36
			// acctWidth = 91 - 36 = 55
			wantDesc: 36,
			wantAcct: 55,
		},
		{
			name:  "60 columns",
			width: 60,
			// remaining = 60 - 2 - 10 - 13 - 4 = 31
			// descWidth = 31 * 40 / 100 = 12
			// acctWidth = 31 - 12 = 19
			wantDesc: 12,
			wantAcct: 19,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newTransactionsPanel()
			p.SetSize(tc.width, 20)
			cols := p.table.Columns()
			if len(cols) != 5 {
				t.Fatalf("expected 5 columns, got %d", len(cols))
			}
			// Columns: [0]=St, [1]=Date, [2]=Description, [3]=Amount, [4]=Account
			if cols[2].Width != tc.wantDesc {
				t.Errorf("desc width: want %d, got %d", tc.wantDesc, cols[2].Width)
			}
			if cols[4].Width != tc.wantAcct {
				t.Errorf("acct width: want %d, got %d", tc.wantAcct, cols[4].Width)
			}
		})
	}
}

func makeSampleTransactions() []*floatv1.Transaction {
	return []*floatv1.Transaction{
		{
			Date:        "2026-01-01",
			Description: "Groceries",
			Postings: []*floatv1.Posting{
				{Account: "expenses:food", Amounts: []*floatv1.Amount{{Quantity: "50.00", Commodity: "USD"}}},
				{Account: "assets:checking", Amounts: []*floatv1.Amount{{Quantity: "-50.00", Commodity: "USD"}}},
			},
		},
		{
			Date:        "2026-01-02",
			Description: "Salary",
			Postings: []*floatv1.Posting{
				{Account: "income:salary", Amounts: []*floatv1.Amount{{Quantity: "-2000.00", Commodity: "USD"}}},
				{Account: "assets:checking", Amounts: []*floatv1.Amount{{Quantity: "2000.00", Commodity: "USD"}}},
			},
		},
	}
}

func TestTransactionsPanel_RebuildRows_Normal(t *testing.T) {
	p := newTransactionsPanel()
	p.SetSize(80, 20)
	p.SetTransactions(makeSampleTransactions())

	rows := p.table.Rows()
	if len(rows) != 2 {
		t.Fatalf("normal mode: expected 2 rows (one per tx), got %d", len(rows))
	}
	if len(p.rowToTx) != 2 {
		t.Fatalf("expected 2 rowToTx entries, got %d", len(p.rowToTx))
	}
	// Columns: [0]=St, [1]=Date, [2]=Description, [3]=Amount, [4]=Account
	if rows[0][1] != "2026-01-01" {
		t.Errorf("expected date 2026-01-01, got %q", rows[0][1])
	}
	if !strings.Contains(rows[0][4], "expenses:food") {
		t.Errorf("expected expenses:food account, got %q", rows[0][4])
	}
}

func TestTransactionsPanel_RebuildRows_Split(t *testing.T) {
	p := newTransactionsPanel()
	p.SetSize(80, 20)
	p.SetTransactions(makeSampleTransactions())
	p.splitView = true
	p.rebuildRows()

	rows := p.table.Rows()
	if len(rows) != 4 {
		t.Fatalf("split mode: expected 4 rows (2 postings per tx), got %d", len(rows))
	}
	if len(p.rowToTx) != 4 {
		t.Fatalf("expected 4 rowToTx entries, got %d", len(p.rowToTx))
	}
	if p.rowToTx[0] != 0 || p.rowToTx[1] != 0 {
		t.Errorf("first two rows should map to tx 0, got %v", p.rowToTx[:2])
	}
	if p.rowToTx[2] != 1 || p.rowToTx[3] != 1 {
		t.Errorf("last two rows should map to tx 1, got %v", p.rowToTx[2:])
	}
}

func TestTransactionsPanel_View_Loading(t *testing.T) {
	p := newTransactionsPanel()
	p.SetSize(80, 20)
	view := p.View()
	if view == "" {
		t.Fatal("expected non-empty view for loading state")
	}
}

func TestTransactionsPanel_View_Error(t *testing.T) {
	p := newTransactionsPanel()
	p.SetSize(80, 20)
	p.SetError("timeout")
	view := p.View()
	if !strings.Contains(view, "!") {
		t.Errorf("expected ! in error view, got: %q", view)
	}
	if !strings.Contains(view, "timeout") {
		t.Errorf("expected error text, got: %q", view)
	}
}

func TestTransactionsPanel_View_Loaded(t *testing.T) {
	p := newTransactionsPanel()
	p.SetSize(80, 20)
	p.SetTransactions(makeSampleTransactions())
	view := p.View()
	if !strings.Contains(view, "Groceries") {
		t.Errorf("expected description in view, got: %q", view)
	}
	if !strings.Contains(view, "2026-01-01") {
		t.Errorf("expected date in view, got: %q", view)
	}
}

func TestTransactionsPanel_View_TooSmall(t *testing.T) {
	p := newTransactionsPanel()
	p.SetSize(80, 2)
	view := p.View()
	if view != "" {
		t.Errorf("expected empty view for height < 3, got: %q", view)
	}
}
