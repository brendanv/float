package ledger_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/cache"
	"github.com/brendanv/float/internal/hledger"
	serverledger "github.com/brendanv/float/internal/server/ledger"
)

func TestGetBalances(t *testing.T) {
	h := mustHandler(t, map[string][]byte{
		"bal": []byte(balJSON),
	})

	resp, err := h.GetBalances(t.Context(), connect.NewRequest(&floatv1.GetBalancesRequest{}))
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}

	report := resp.Msg.Report
	if report == nil {
		t.Fatal("expected non-nil report")
		return
	}
	if len(report.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(report.Rows))
	}
	row := report.Rows[0]
	if row.FullName != "assets:checking" {
		t.Errorf("FullName = %q, want %q", row.FullName, "assets:checking")
	}
	if len(row.Amounts) != 1 {
		t.Fatalf("expected 1 amount, got %d", len(row.Amounts))
	}
	if row.Amounts[0].Quantity != "7000.00" {
		t.Errorf("Quantity = %q, want %q", row.Amounts[0].Quantity, "7000.00")
	}
	if len(report.Total) != 1 {
		t.Fatalf("expected 1 total, got %d", len(report.Total))
	}
	if report.Total[0].Quantity != "0.00" {
		t.Errorf("Total Quantity = %q, want %q", report.Total[0].Quantity, "0.00")
	}
}

func TestGetBalances_HledgerError(t *testing.T) {
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", errorRunner(t))
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil, nil)
	_, err = h.GetBalances(t.Context(), connect.NewRequest(&floatv1.GetBalancesRequest{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("code = %v, want Internal", connect.CodeOf(err))
	}
}

func TestGetBalances_CacheHit(t *testing.T) {
	var calls atomic.Int64
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.99.6, linux-x86_64\n"), nil, nil
		}
		calls.Add(1)
		return []byte(balJSON), nil, nil
	}
	h, _ := mustHandlerWithCache(t, runner)

	req := connect.NewRequest(&floatv1.GetBalancesRequest{})
	if _, err := h.GetBalances(t.Context(), req); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := h.GetBalances(t.Context(), req); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if calls.Load() != 1 {
		t.Errorf("hledger called %d times, want 1", calls.Load())
	}
}

func TestGetBalances_CacheError(t *testing.T) {
	h, _ := mustHandlerWithCache(t, errorRunner(t))
	_, err := h.GetBalances(t.Context(), connect.NewRequest(&floatv1.GetBalancesRequest{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("code = %v, want Internal", connect.CodeOf(err))
	}
}

func TestGetBalances_WithDepthAndQuery(t *testing.T) {
	var capturedArgs []string
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.99.6, linux-x86_64\n"), nil, nil
		}
		capturedArgs = args
		return []byte(balJSON), nil, nil
	}
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil, nil)

	_, err = h.GetBalances(t.Context(), connect.NewRequest(&floatv1.GetBalancesRequest{
		Depth: 2,
		Query: []string{"expenses"},
	}))
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}

	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "--depth 2") {
		t.Errorf("args %v missing --depth 2", capturedArgs)
	}
	if !strings.Contains(joined, "expenses") {
		t.Errorf("args %v missing query 'expenses'", capturedArgs)
	}
}

func TestListPayees(t *testing.T) {
	h := mustHandler(t, map[string][]byte{
		"payees": []byte(payeesText),
	})

	resp, err := h.ListPayees(t.Context(), connect.NewRequest(&floatv1.ListPayeesRequest{}))
	if err != nil {
		t.Fatalf("ListPayees: %v", err)
	}

	payees := resp.Msg.Payees
	if len(payees) != 2 {
		t.Fatalf("expected 2 payees, got %d", len(payees))
	}
	if payees[0] != "Acme Corp" {
		t.Errorf("payees[0] = %q, want %q", payees[0], "Acme Corp")
	}
	if payees[1] != "Grocery Store" {
		t.Errorf("payees[1] = %q, want %q", payees[1], "Grocery Store")
	}
}

func TestListPayees_HledgerError(t *testing.T) {
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", errorRunner(t))
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil, nil)
	_, err = h.ListPayees(t.Context(), connect.NewRequest(&floatv1.ListPayeesRequest{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("code = %v, want Internal", connect.CodeOf(err))
	}
}

func TestListPayees_CacheHit(t *testing.T) {
	var calls atomic.Int64
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.99.6, linux-x86_64\n"), nil, nil
		}
		calls.Add(1)
		return []byte(payeesText), nil, nil
	}
	h, _ := mustHandlerWithCache(t, runner)

	req := connect.NewRequest(&floatv1.ListPayeesRequest{})
	if _, err := h.ListPayees(t.Context(), req); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := h.ListPayees(t.Context(), req); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if calls.Load() != 1 {
		t.Errorf("hledger called %d times, want 1", calls.Load())
	}
}

func TestListPayees_CacheError(t *testing.T) {
	h, _ := mustHandlerWithCache(t, errorRunner(t))
	_, err := h.ListPayees(t.Context(), connect.NewRequest(&floatv1.ListPayeesRequest{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("code = %v, want Internal", connect.CodeOf(err))
	}
}

func TestGetAccountRegister(t *testing.T) {
	h := mustHandler(t, map[string][]byte{"areg": []byte(aregJSON)})

	resp, err := h.GetAccountRegister(t.Context(), connect.NewRequest(&floatv1.GetAccountRegisterRequest{
		Account: "assets:checking",
	}))
	if err != nil {
		t.Fatalf("GetAccountRegister: %v", err)
	}
	rows := resp.Msg.Rows
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if resp.Msg.Total != 2 {
		t.Errorf("Total = %d, want 2", resp.Msg.Total)
	}
	if resp.Msg.HasNext {
		t.Error("HasNext should be false when all rows returned")
	}

	// Row 0: payroll deposit, status normalized from "Cleared" stays "Cleared",
	// payee/note parsed from "PAYROLL | monthly".
	r0 := rows[0]
	if r0.Fid != "aa001100" {
		t.Errorf("row 0 Fid = %q, want aa001100", r0.Fid)
	}
	if r0.Date != "2026-01-05" {
		t.Errorf("row 0 Date = %q, want 2026-01-05", r0.Date)
	}
	if r0.Description != "PAYROLL | monthly" {
		t.Errorf("row 0 Description = %q", r0.Description)
	}
	if r0.Payee == nil || *r0.Payee != "PAYROLL" {
		t.Errorf("row 0 Payee = %v, want PAYROLL", r0.Payee)
	}
	if r0.Note == nil || *r0.Note != "monthly" {
		t.Errorf("row 0 Note = %v, want monthly", r0.Note)
	}
	if r0.Status != "Cleared" {
		t.Errorf("row 0 Status = %q, want Cleared", r0.Status)
	}
	if len(r0.OtherAccounts) != 1 || r0.OtherAccounts[0] != "income:salary" {
		t.Errorf("row 0 OtherAccounts = %v", r0.OtherAccounts)
	}
	if len(r0.Change) != 1 || r0.Change[0].Quantity != "3500.00" || r0.Change[0].Commodity != "$" {
		t.Errorf("row 0 Change = %+v", r0.Change)
	}
	if len(r0.RunningTotal) != 1 || r0.RunningTotal[0].Quantity != "3500.00" {
		t.Errorf("row 0 RunningTotal = %+v", r0.RunningTotal)
	}

	// Row 1: $45 expense — signed negative, running balance 3455, status
	// normalized from "Unmarked" to "", description without "|" yields nil
	// payee/note.
	r1 := rows[1]
	if r1.Status != "" {
		t.Errorf("row 1 Status = %q, want empty (Unmarked normalized)", r1.Status)
	}
	if r1.Payee != nil || r1.Note != nil {
		t.Errorf("row 1 Payee/Note should be nil, got %v/%v", r1.Payee, r1.Note)
	}
	if r1.Change[0].Quantity != "-45.00" {
		t.Errorf("row 1 Change = %q, want -45.00", r1.Change[0].Quantity)
	}
	if r1.RunningTotal[0].Quantity != "3455.00" {
		t.Errorf("row 1 RunningTotal = %q, want 3455.00", r1.RunningTotal[0].Quantity)
	}
}

func TestGetAccountRegister_EmptyAccount(t *testing.T) {
	h := mustHandler(t, map[string][]byte{"areg": []byte(aregJSON)})
	_, err := h.GetAccountRegister(t.Context(), connect.NewRequest(&floatv1.GetAccountRegisterRequest{
		Account: "   ",
	}))
	if err == nil {
		t.Fatal("expected error for empty account, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestGetAccountRegister_PassesArgsToHledger(t *testing.T) {
	var capturedArgs []string
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.99.6, linux-x86_64\n"), nil, nil
		}
		capturedArgs = args
		return []byte("[]"), nil, nil
	}
	c, err := hledger.NewWithRunner("hledger", "journal.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil, nil)

	_, err = h.GetAccountRegister(t.Context(), connect.NewRequest(&floatv1.GetAccountRegisterRequest{
		Account: "assets:checking",
		Query:   []string{"date:2026-01", "status:cleared"},
	}))
	if err != nil {
		t.Fatalf("GetAccountRegister: %v", err)
	}

	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "areg") {
		t.Errorf("args %v missing areg subcommand", capturedArgs)
	}
	// Focused account must appear as a positional arg, not a query token.
	if !strings.Contains(joined, "assets:checking") {
		t.Errorf("args %v missing focused account 'assets:checking'", capturedArgs)
	}
	if !strings.Contains(joined, "date:2026-01") {
		t.Errorf("args %v missing query token 'date:2026-01'", capturedArgs)
	}
	if !strings.Contains(joined, "status:cleared") {
		t.Errorf("args %v missing query token 'status:cleared'", capturedArgs)
	}
}

// TestGetAccountRegister_SupportedVocabularyFiltersInGo verifies that a query
// entirely within txfilter's supported vocabulary (Phase 2.3) is served by
// fetching the account's unfiltered register once and filtering rows in Go,
// rather than passing the query tokens to hledger.
func TestGetAccountRegister_SupportedVocabularyFiltersInGo(t *testing.T) {
	var capturedArgs []string
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.99.6, linux-x86_64\n"), nil, nil
		}
		capturedArgs = args
		return []byte(aregJSON), nil, nil
	}
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil, nil)

	resp, err := h.GetAccountRegister(t.Context(), connect.NewRequest(&floatv1.GetAccountRegisterRequest{
		Account: "assets:checking",
		Query:   []string{"status:*"},
	}))
	if err != nil {
		t.Fatalf("GetAccountRegister: %v", err)
	}

	joined := strings.Join(capturedArgs, " ")
	if strings.Contains(joined, "status:") {
		t.Errorf("args %v should not include the query token (expected an unfiltered fetch filtered in Go)", capturedArgs)
	}
	if len(resp.Msg.Rows) != 1 {
		t.Fatalf("expected status:* to leave only the Cleared row, got %d rows", len(resp.Msg.Rows))
	}
	if resp.Msg.Rows[0].Fid != "aa001100" {
		t.Errorf("Fid = %q, want %q", resp.Msg.Rows[0].Fid, "aa001100")
	}
}

func TestGetAccountRegister_Pagination(t *testing.T) {
	h := mustHandler(t, map[string][]byte{"areg": []byte(aregJSON)})

	// limit=1 → first row only, HasNext=true, Total=2.
	resp, err := h.GetAccountRegister(t.Context(), connect.NewRequest(&floatv1.GetAccountRegisterRequest{
		Account: "assets:checking",
		Limit:   1,
	}))
	if err != nil {
		t.Fatalf("GetAccountRegister: %v", err)
	}
	if len(resp.Msg.Rows) != 1 || resp.Msg.Rows[0].Fid != "aa001100" {
		t.Errorf("limit=1: unexpected rows %+v", resp.Msg.Rows)
	}
	if resp.Msg.Total != 2 {
		t.Errorf("Total = %d, want 2", resp.Msg.Total)
	}
	if !resp.Msg.HasNext {
		t.Error("HasNext should be true when limit truncates results")
	}

	// offset=1, limit=10 → second row, HasNext=false.
	resp, err = h.GetAccountRegister(t.Context(), connect.NewRequest(&floatv1.GetAccountRegisterRequest{
		Account: "assets:checking",
		Offset:  1,
		Limit:   10,
	}))
	if err != nil {
		t.Fatalf("GetAccountRegister: %v", err)
	}
	if len(resp.Msg.Rows) != 1 || resp.Msg.Rows[0].Fid != "bb002200" {
		t.Errorf("offset=1: unexpected rows %+v", resp.Msg.Rows)
	}
	if resp.Msg.HasNext {
		t.Error("HasNext should be false when fewer than limit rows remain")
	}

	// offset >= total → empty result.
	resp, err = h.GetAccountRegister(t.Context(), connect.NewRequest(&floatv1.GetAccountRegisterRequest{
		Account: "assets:checking",
		Offset:  5,
	}))
	if err != nil {
		t.Fatalf("GetAccountRegister: %v", err)
	}
	if len(resp.Msg.Rows) != 0 {
		t.Errorf("offset>total should yield empty rows, got %d", len(resp.Msg.Rows))
	}
	if resp.Msg.Total != 2 {
		t.Errorf("Total = %d, want 2 (pre-pagination total)", resp.Msg.Total)
	}
}

func TestGetAccountRegister_HledgerError(t *testing.T) {
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.99.6, linux-x86_64\n"), nil, nil
		}
		return nil, []byte("boom"), errors.New("exec failed")
	}
	c, err := hledger.NewWithRunner("hledger", "journal.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil, nil)

	_, err = h.GetAccountRegister(t.Context(), connect.NewRequest(&floatv1.GetAccountRegisterRequest{
		Account: "assets:checking",
	}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("code = %v, want Internal", connect.CodeOf(err))
	}
}

func TestGetAccountRegister_CacheHit(t *testing.T) {
	var callCount atomic.Int32
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.99.6, linux-x86_64\n"), nil, nil
		}
		callCount.Add(1)
		return []byte(aregJSON), nil, nil
	}
	c, err := hledger.NewWithRunner("hledger", "journal.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	cch := cache.New[any](func() uint64 { return 0 })
	h := serverledger.NewHandler(c, nil, "", "", cch, nil, nil, nil)

	req := connect.NewRequest(&floatv1.GetAccountRegisterRequest{Account: "assets:checking"})
	if _, err := h.GetAccountRegister(t.Context(), req); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := h.GetAccountRegister(t.Context(), req); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if got := callCount.Load(); got != 1 {
		t.Errorf("expected 1 hledger call (second served from cache), got %d", got)
	}
}

func TestGetBalances_AmountsHaveNoCost(t *testing.T) {
	h := mustHandler(t, map[string][]byte{
		"bal": []byte(balJSON),
	})

	resp, err := h.GetBalances(t.Context(), connect.NewRequest(&floatv1.GetBalancesRequest{}))
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}
	for _, row := range resp.Msg.Report.Rows {
		for _, amt := range row.Amounts {
			if amt.Cost != nil {
				t.Errorf("balance row %q: amount %+v unexpectedly has Cost = %+v", row.FullName, amt, amt.Cost)
			}
		}
	}
}

func TestListLots(t *testing.T) {
	h := mustLotsHandler(t)
	resp, err := h.ListLots(t.Context(), connect.NewRequest(&floatv1.ListLotsRequest{}))
	if err != nil {
		t.Fatalf("ListLots: %v", err)
	}
	type want struct{ account, date, label, commodity, units, unitCost, totalCost string }
	wantByName := map[string]want{
		"assets:brokerage:{2026-01-15, $50}":         {"assets:brokerage", "2026-01-15", "", "AAPL", "3", "50", "150"},
		`assets:brokerage:{2026-01-16, "mine", $40}`: {"assets:brokerage", "2026-01-16", "mine", "AAPL", "4", "40", "160"},
		"assets:brokerage:{2026-01-20, $60}":         {"assets:brokerage", "2026-01-20", "", "AAPL", "3", "60", "180"},
		"assets:brokerage:{2026-01-25, $1,234.567}":  {"assets:brokerage", "2026-01-25", "", "AAPL", "1", "1234.567", "1234.567"},
		"assets:ira:{2026-01-15, $50}":               {"assets:ira", "2026-01-15", "", "AAPL", "2", "50", "100"},
		"assets:ira:{2026-01-11, $110}":              {"assets:ira", "2026-01-11", "", "VTI", "2", "110", "220"},
		"assets:crypto:{2026-02-15, $40,000}":        {"assets:crypto", "2026-02-15", "", "BTC", "0.375", "40000", "15000"},
	}
	lots := resp.Msg.Lots
	if len(lots) != len(wantByName) {
		t.Fatalf("len(lots) = %d, want %d: %v", len(lots), len(wantByName), lots)
	}
	for _, l := range lots {
		full := l.Account + ":" + l.LotName
		w, ok := wantByName[full]
		if !ok {
			t.Errorf("unexpected lot %q", full)
			continue
		}
		if l.Date != w.date || l.Label != w.label {
			t.Errorf("%s: date/label = %q/%q, want %q/%q", full, l.Date, l.Label, w.date, w.label)
		}
		if l.Units == nil || l.Units.Commodity != w.commodity || l.Units.Quantity != w.units {
			t.Errorf("%s: units = %v, want %s %s", full, l.Units, w.units, w.commodity)
		}
		if l.UnitCost == nil || l.UnitCost.Commodity != "$" || l.UnitCost.Quantity != w.unitCost {
			t.Errorf("%s: unit cost = %v, want $ %s", full, l.UnitCost, w.unitCost)
		}
		if len(l.TotalCost) != 1 || l.TotalCost[0].Commodity != "$" || l.TotalCost[0].Quantity != w.totalCost {
			t.Errorf("%s: total cost = %v, want $ %s", full, l.TotalCost, w.totalCost)
		}
	}
}

func TestListLots_QueryFilters(t *testing.T) {
	h := mustLotsHandler(t)
	resp, err := h.ListLots(t.Context(), connect.NewRequest(&floatv1.ListLotsRequest{Query: []string{"acct:nonexistent"}}))
	if err != nil {
		t.Fatalf("ListLots: %v", err)
	}
	if len(resp.Msg.Lots) != 0 {
		t.Errorf("len(lots) = %d, want 0", len(resp.Msg.Lots))
	}
}
