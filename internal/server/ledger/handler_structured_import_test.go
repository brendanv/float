package ledger_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	serverledger "github.com/brendanv/float/internal/server/ledger"

	"github.com/brendanv/float/internal/testgen"
)

func structuredTxn(date, desc, account, qty string) *floatv1.StructuredTransaction {
	return &floatv1.StructuredTransaction{
		Date:        date,
		Description: desc,
		Postings: []*floatv1.PostingInput{
			{Account: account, Commodity: "USD", Quantity: qty},
			{Account: "assets:checking"},
		},
	}
}

func countTransactions(t *testing.T, h *serverledger.Handler) int32 {
	t.Helper()
	resp, err := h.ListTransactions(t.Context(), connect.NewRequest(&floatv1.ListTransactionsRequest{}))
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	return resp.Msg.Total
}

func TestImportStructuredTransactions_Validation(t *testing.T) {
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 400, NumTxns: 1, WithFIDs: true})
	h := mustRealHandler(t, dir)
	good := structuredTxn("2026-03-01", "Coffee", "expenses:food", "4.50")

	tests := []struct {
		name    string
		req     *floatv1.ImportStructuredTransactionsRequest
		wantMsg string
	}{
		{name: "empty", req: &floatv1.ImportStructuredTransactionsRequest{}, wantMsg: "must not be empty"},
		{
			name:    "missing date",
			req:     &floatv1.ImportStructuredTransactionsRequest{Transactions: []*floatv1.StructuredTransaction{good, structuredTxn("", "x", "expenses:food", "1")}},
			wantMsg: "transaction 1: date is required",
		},
		{
			name:    "bad date",
			req:     &floatv1.ImportStructuredTransactionsRequest{Transactions: []*floatv1.StructuredTransaction{structuredTxn("03/01/2026", "x", "expenses:food", "1")}},
			wantMsg: "invalid date",
		},
		{
			name: "one posting",
			req: &floatv1.ImportStructuredTransactionsRequest{Transactions: []*floatv1.StructuredTransaction{{
				Date: "2026-03-01", Description: "x",
				Postings: []*floatv1.PostingInput{{Account: "expenses:food", Commodity: "USD", Quantity: "1"}},
			}}},
			wantMsg: "at least 2 postings",
		},
		{
			name: "bad status",
			req: &floatv1.ImportStructuredTransactionsRequest{Transactions: []*floatv1.StructuredTransaction{{
				Date: "2026-03-01", Description: "x", Status: "Reviewed", Postings: good.Postings,
			}}},
			wantMsg: "invalid status",
		},
		{
			name: "reserved tag",
			req: &floatv1.ImportStructuredTransactionsRequest{Transactions: []*floatv1.StructuredTransaction{{
				Date: "2026-03-01", Description: "x", Tags: map[string]string{"float-import": "x"}, Postings: good.Postings,
			}}},
			wantMsg: "reserved",
		},
		{
			name:    "reserved source",
			req:     &floatv1.ImportStructuredTransactionsRequest{Source: "stripe", Transactions: []*floatv1.StructuredTransaction{good}},
			wantMsg: "reserved",
		},
		{
			name:    "invalid source",
			req:     &floatv1.ImportStructuredTransactionsRequest{Source: "My Agent", Transactions: []*floatv1.StructuredTransaction{good}},
			wantMsg: "lowercase",
		},
		{
			name: "unbalanced transaction reported by index",
			req: &floatv1.ImportStructuredTransactionsRequest{Transactions: []*floatv1.StructuredTransaction{good, {
				Date: "2026-03-02", Description: "unbalanced",
				Postings: []*floatv1.PostingInput{
					{Account: "expenses:food", Commodity: "USD", Quantity: "10"},
					{Account: "assets:checking", Commodity: "USD", Quantity: "-9"},
				},
			}}},
			wantMsg: "transaction 1:",
		},
	}
	before := countTransactions(t, h)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := h.ImportStructuredTransactions(t.Context(), connect.NewRequest(tt.req))
			if connect.CodeOf(err) != connect.CodeInvalidArgument {
				t.Fatalf("code = %v (err %v), want InvalidArgument", connect.CodeOf(err), err)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
	if after := countTransactions(t, h); after != before {
		t.Errorf("transaction count changed from %d to %d after rejected requests", before, after)
	}
}

func TestImportStructuredTransactions_DryRunWritesNothing(t *testing.T) {
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 401, NumTxns: 2, WithFIDs: true})
	h := mustRealHandler(t, dir)
	before := countTransactions(t, h)

	resp, err := h.ImportStructuredTransactions(t.Context(), connect.NewRequest(&floatv1.ImportStructuredTransactionsRequest{
		DryRun: true,
		Transactions: []*floatv1.StructuredTransaction{
			structuredTxn("2026-03-01", "Coffee", "expenses:food", "4.50"),
			structuredTxn("2026-03-02", "Book", "expenses:shopping", "12.00"),
		},
	}))
	if err != nil {
		t.Fatalf("ImportStructuredTransactions: %v", err)
	}
	m := resp.Msg
	if m.ImportBatchId != "" || m.ImportedCount != 0 || len(m.Transactions) != 0 {
		t.Errorf("dry run reported writes: batch=%q imported=%d txns=%d", m.ImportBatchId, m.ImportedCount, len(m.Transactions))
	}
	if len(m.Candidates) != 2 {
		t.Fatalf("got %d candidates, want 2", len(m.Candidates))
	}
	c := m.Candidates[0].Transaction
	if c.Fid != "" {
		t.Errorf("candidate Fid = %q, want empty", c.Fid)
	}
	if c.Status != "" {
		t.Errorf("candidate Status = %q, want Unmarked", c.Status)
	}
	// The auto-balanced posting is filled in by hledger.
	if got := c.Postings[1].Amounts[0].Quantity; got != "-4.50" {
		t.Errorf("auto-balanced quantity = %q, want -4.50", got)
	}
	if after := countTransactions(t, h); after != before {
		t.Errorf("dry run changed transaction count from %d to %d", before, after)
	}
}

func TestImportStructuredTransactions_ImportAndDedup(t *testing.T) {
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 402, NumTxns: 2, WithFIDs: true})
	h := mustRealHandler(t, dir)
	before := countTransactions(t, h)

	req := &floatv1.ImportStructuredTransactionsRequest{
		Source: "agent",
		Transactions: []*floatv1.StructuredTransaction{
			structuredTxn("2026-03-01", "Coffee", "expenses:food", "4.50"),
			{
				Date: "2026-03-02", Description: "Lunch", Payee: "Deli", Tags: map[string]string{"trip": "nyc"},
				Postings: []*floatv1.PostingInput{
					{Account: "expenses:food", Commodity: "USD", Quantity: "15.00"},
					{Account: "assets:checking"},
				},
			},
		},
	}
	resp, err := h.ImportStructuredTransactions(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("ImportStructuredTransactions: %v", err)
	}
	m := resp.Msg
	if m.ImportedCount != 2 || m.SkippedDuplicateCount != 0 {
		t.Fatalf("imported=%d skipped=%d, want 2/0", m.ImportedCount, m.SkippedDuplicateCount)
	}
	if !strings.HasPrefix(m.ImportBatchId, "agent/") {
		t.Errorf("ImportBatchId = %q, want agent/ prefix", m.ImportBatchId)
	}
	if len(m.Transactions) != 2 {
		t.Fatalf("got %d transactions, want 2", len(m.Transactions))
	}
	for _, tx := range m.Transactions {
		if tx.Fid == "" {
			t.Error("written transaction has empty Fid")
		}
		if tx.Status != "" {
			t.Errorf("Status = %q, want Unmarked", tx.Status)
		}
		if tx.GetImportBatchId() != m.ImportBatchId {
			t.Errorf("ImportBatchId = %q, want %q", tx.GetImportBatchId(), m.ImportBatchId)
		}
		if _, ok := tx.Tags["float-import-source"]; ok {
			t.Error("hidden float-import-source tag leaked into user-visible tags")
		}
	}
	lunch := m.Transactions[1]
	if lunch.GetPayee() != "Deli" || lunch.GetNote() != "Lunch" || lunch.Tags["trip"] != "nyc" {
		t.Errorf("lunch = payee %q note %q tags %v", lunch.GetPayee(), lunch.GetNote(), lunch.Tags)
	}
	if after := countTransactions(t, h); after != before+2 {
		t.Errorf("transaction count = %d, want %d", after, before+2)
	}

	imports, err := h.ListImports(t.Context(), connect.NewRequest(&floatv1.ListImportsRequest{}))
	if err != nil {
		t.Fatalf("ListImports: %v", err)
	}
	var found bool
	for _, imp := range imports.Msg.Imports {
		if imp.ImportBatchId == m.ImportBatchId {
			found = true
			if imp.Source != "agent" || imp.TransactionCount != 2 {
				t.Errorf("import summary = source %q count %d, want agent/2", imp.Source, imp.TransactionCount)
			}
		}
	}
	if !found {
		t.Errorf("batch %q not in ListImports", m.ImportBatchId)
	}

	// Re-sending the same request is a no-op: everything is a duplicate.
	resp, err = h.ImportStructuredTransactions(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	m = resp.Msg
	if m.ImportedCount != 0 || m.SkippedDuplicateCount != 2 || m.ImportBatchId != "" {
		t.Errorf("re-import: imported=%d skipped=%d batch=%q, want 0/2/empty", m.ImportedCount, m.SkippedDuplicateCount, m.ImportBatchId)
	}
	for i, c := range m.Candidates {
		if !c.IsDuplicate {
			t.Errorf("candidate %d not flagged duplicate", i)
		}
	}
	if after := countTransactions(t, h); after != before+2 {
		t.Errorf("transaction count after re-import = %d, want %d", after, before+2)
	}

	// include_duplicates writes them anyway.
	req.IncludeDuplicates = true
	resp, err = h.ImportStructuredTransactions(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("import with duplicates: %v", err)
	}
	if resp.Msg.ImportedCount != 2 {
		t.Errorf("ImportedCount = %d, want 2", resp.Msg.ImportedCount)
	}
}

func TestImportStructuredTransactions_ApplyRules(t *testing.T) {
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 403, NumTxns: 1, WithFIDs: true})
	rulesJSON := `[{"id":"r1","pattern":"(?i)starbucks","payee":"Starbucks","account":"expenses:coffee","tags":{"habit":"coffee"},"priority":0,"auto_reviewed":true}]`
	if err := os.WriteFile(filepath.Join(dir, "rules.json"), []byte(rulesJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	h := mustRealHandler(t, dir)

	txns := []*floatv1.StructuredTransaction{
		structuredTxn("2026-03-01", "STARBUCKS #123", "expenses:unknown", "5.00"),
		{
			Date: "2026-03-02", Description: "STARBUCKS #456", Payee: "Coffee Shop",
			Postings: []*floatv1.PostingInput{
				{Account: "expenses:unknown", Commodity: "USD", Quantity: "6.00"},
				{Account: "assets:checking"},
			},
		},
		structuredTxn("2026-03-03", "Hardware store", "expenses:home", "20.00"),
	}

	t.Run("rules ignored by default", func(t *testing.T) {
		resp, err := h.ImportStructuredTransactions(t.Context(), connect.NewRequest(&floatv1.ImportStructuredTransactionsRequest{DryRun: true, Transactions: txns}))
		if err != nil {
			t.Fatal(err)
		}
		c := resp.Msg.Candidates[0]
		if c.MatchedRuleId != "" || c.Transaction.Postings[0].Account != "expenses:unknown" {
			t.Errorf("rule applied without apply_rules: rule=%q account=%q", c.MatchedRuleId, c.Transaction.Postings[0].Account)
		}
	})

	t.Run("apply_rules", func(t *testing.T) {
		resp, err := h.ImportStructuredTransactions(t.Context(), connect.NewRequest(&floatv1.ImportStructuredTransactionsRequest{ApplyRules: true, Transactions: txns}))
		if err != nil {
			t.Fatal(err)
		}
		m := resp.Msg
		if m.ImportedCount != 3 {
			t.Fatalf("ImportedCount = %d, want 3", m.ImportedCount)
		}
		first, second, third := m.Transactions[0], m.Transactions[1], m.Transactions[2]
		if m.Candidates[0].MatchedRuleId != "r1" {
			t.Errorf("MatchedRuleId = %q, want r1", m.Candidates[0].MatchedRuleId)
		}
		if first.GetPayee() != "Starbucks" || first.Postings[0].Account != "expenses:coffee" || first.Tags["habit"] != "coffee" || first.Status != "Cleared" {
			t.Errorf("first = payee %q account %q tags %v status %q", first.GetPayee(), first.Postings[0].Account, first.Tags, first.Status)
		}
		// An explicit payee wins over the rule's.
		if second.GetPayee() != "Coffee Shop" || second.Postings[0].Account != "expenses:coffee" {
			t.Errorf("second = payee %q account %q", second.GetPayee(), second.Postings[0].Account)
		}
		if m.Candidates[2].MatchedRuleId != "" || third.Status != "" || third.Postings[0].Account != "expenses:home" {
			t.Errorf("third = rule %q status %q account %q", m.Candidates[2].MatchedRuleId, third.Status, third.Postings[0].Account)
		}
	})
}

func TestAddTransactionStatus(t *testing.T) {
	unmarked, pending := "", "Pending"
	invalid := "Done"
	tests := []struct {
		name       string
		status     *string
		wantStatus string
		wantErr    bool
	}{
		{name: "unset defaults to cleared", status: nil, wantStatus: "Cleared"},
		{name: "explicit unmarked", status: &unmarked, wantStatus: ""},
		{name: "pending", status: &pending, wantStatus: "Pending"},
		{name: "invalid", status: &invalid, wantErr: true},
	}
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 404, NumTxns: 1, WithFIDs: true})
	h := mustRealHandler(t, dir)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := h.AddTransaction(t.Context(), connect.NewRequest(&floatv1.AddTransactionRequest{
				Description: "status test",
				Date:        "2026-03-05",
				Status:      tt.status,
				Postings: []*floatv1.PostingInput{
					{Account: "expenses:food", Commodity: "USD", Quantity: "3.00"},
					{Account: "assets:checking"},
				},
			}))
			if tt.wantErr {
				if connect.CodeOf(err) != connect.CodeInvalidArgument {
					t.Fatalf("code = %v, want InvalidArgument", connect.CodeOf(err))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := resp.Msg.Transaction.Status; got != tt.wantStatus {
				t.Errorf("Status = %q, want %q", got, tt.wantStatus)
			}
		})
	}
}
