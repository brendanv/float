package ledger_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/hledger"
	serverledger "github.com/brendanv/float/internal/server/ledger"
)

func TestGetNetWorthTimeseries(t *testing.T) {
	h := mustHandler(t, map[string][]byte{
		"bs": []byte(bsTimeseriesJSON),
	})

	resp, err := h.GetNetWorthTimeseries(t.Context(), connect.NewRequest(&floatv1.GetNetWorthTimeseriesRequest{}))
	if err != nil {
		t.Fatalf("GetNetWorthTimeseries: %v", err)
	}

	snapshots := resp.Msg.Snapshots
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snapshots))
	}

	s0 := snapshots[0]
	if s0.Date != "2026-01-01" {
		t.Errorf("snapshot[0].Date = %q, want %q", s0.Date, "2026-01-01")
	}
	if len(s0.Assets) != 1 {
		t.Fatalf("expected 1 asset amount in snapshot[0], got %d", len(s0.Assets))
	}
	if s0.Assets[0].Commodity != "$" {
		t.Errorf("Assets[0].Commodity = %q, want %q", s0.Assets[0].Commodity, "$")
	}
	if s0.Assets[0].Quantity != "3335.00" {
		t.Errorf("Assets[0].Quantity = %q, want %q", s0.Assets[0].Quantity, "3335.00")
	}
	if len(s0.Liabilities) != 0 {
		t.Errorf("expected 0 liability amounts in snapshot[0], got %d", len(s0.Liabilities))
	}
	if len(s0.NetWorth) != 1 {
		t.Fatalf("expected 1 net worth amount in snapshot[0], got %d", len(s0.NetWorth))
	}
	if s0.NetWorth[0].Quantity != "3335.00" {
		t.Errorf("NetWorth[0].Quantity = %q, want %q", s0.NetWorth[0].Quantity, "3335.00")
	}

	s1 := snapshots[1]
	if s1.Date != "2026-02-01" {
		t.Errorf("snapshot[1].Date = %q, want %q", s1.Date, "2026-02-01")
	}
	if len(s1.Assets) != 1 {
		t.Fatalf("expected 1 asset amount in snapshot[1], got %d", len(s1.Assets))
	}
	if s1.Assets[0].Quantity != "6820.00" {
		t.Errorf("Assets[0].Quantity = %q, want %q", s1.Assets[0].Quantity, "6820.00")
	}
}

func TestGetNetWorthTimeseries_HledgerError(t *testing.T) {
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", errorRunner(t))
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil, nil)
	_, err = h.GetNetWorthTimeseries(t.Context(), connect.NewRequest(&floatv1.GetNetWorthTimeseriesRequest{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("code = %v, want Internal", connect.CodeOf(err))
	}
}

func TestGetNetWorthTimeseries_CacheHit(t *testing.T) {
	var calls atomic.Int64
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.99.6, linux-x86_64\n"), nil, nil
		}
		calls.Add(1)
		return []byte(bsTimeseriesJSON), nil, nil
	}
	h, _ := mustHandlerWithCache(t, runner)

	req := connect.NewRequest(&floatv1.GetNetWorthTimeseriesRequest{})
	if _, err := h.GetNetWorthTimeseries(t.Context(), req); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := h.GetNetWorthTimeseries(t.Context(), req); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if calls.Load() != 1 {
		t.Errorf("hledger called %d times, want 1", calls.Load())
	}
}

func TestGetNetWorthTimeseries_CacheError(t *testing.T) {
	h, _ := mustHandlerWithCache(t, errorRunner(t))
	_, err := h.GetNetWorthTimeseries(t.Context(), connect.NewRequest(&floatv1.GetNetWorthTimeseriesRequest{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("code = %v, want Internal", connect.CodeOf(err))
	}
}

// TestGetNetWorthTimeseries_WithDateRange verifies that a month-aligned
// begin/end is served by slicing a single full-history hledger fetch in Go
// (Phase 2.5), rather than asking hledger for that date range directly:
// `bs --historical` values are cumulative from account opening regardless of
// begin, so trimming periods after the fact is equivalent.
func TestGetNetWorthTimeseries_WithDateRange(t *testing.T) {
	var capturedArgs []string
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.99.6, linux-x86_64\n"), nil, nil
		}
		capturedArgs = args
		return []byte(bsTimeseriesJSON), nil, nil
	}
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil, nil)

	resp, err := h.GetNetWorthTimeseries(t.Context(), connect.NewRequest(&floatv1.GetNetWorthTimeseriesRequest{
		Begin: "2026-02-01",
		End:   "",
	}))
	if err != nil {
		t.Fatalf("GetNetWorthTimeseries: %v", err)
	}

	joined := strings.Join(capturedArgs, " ")
	if strings.Contains(joined, "date:") {
		t.Errorf("args %v should not include a date: query for a month-aligned range (expected a full-history fetch sliced in Go)", capturedArgs)
	}

	snapshots := resp.Msg.Snapshots
	if len(snapshots) != 1 || snapshots[0].Date != "2026-02-01" {
		t.Fatalf("expected slicing to leave only the 2026-02-01 snapshot, got %+v", snapshots)
	}
}

// TestGetNetWorthTimeseries_NonAlignedRangePassesThrough verifies that a
// begin/end that isn't a first-of-month date still goes straight to hledger
// (bs would otherwise generate a partial period anchored to that exact date,
// which slicing the full-history period list can't reproduce).
func TestGetNetWorthTimeseries_NonAlignedRangePassesThrough(t *testing.T) {
	var capturedArgs []string
	runner := func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.99.6, linux-x86_64\n"), nil, nil
		}
		capturedArgs = args
		return []byte(bsTimeseriesJSON), nil, nil
	}
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	h := serverledger.NewHandler(c, nil, "", "", nil, nil, nil, nil)

	_, err = h.GetNetWorthTimeseries(t.Context(), connect.NewRequest(&floatv1.GetNetWorthTimeseriesRequest{
		Begin: "2026-01-15",
		End:   "",
	}))
	if err != nil {
		t.Fatalf("GetNetWorthTimeseries: %v", err)
	}

	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "2026-01-15") {
		t.Errorf("args %v missing begin date for a non-month-aligned range", capturedArgs)
	}
}
