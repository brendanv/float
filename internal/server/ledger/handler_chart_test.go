package ledger_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/hledger"
)

const chartSVG = `<svg xmlns="http://www.w3.org/2000/svg" class="hledger-chart"><title>Chart</title>` +
	`<style>@media (prefers-color-scheme:dark){.hledger-chart .bg{fill:#000}}</style>` +
	`<script>alert(1)</script><g><title>expenses:food · Balance · $120.00</title><path d="M0 0"/></g></svg>`

// chartRunner serves chartSVG for `bal -O svg` and records the args of the
// last chart invocation.
func chartRunner(t *testing.T, calls *atomic.Int64, lastArgs *[]string) hledger.CommandRunner {
	t.Helper()
	return func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.99.6, linux-x86_64\n"), nil, nil
		}
		calls.Add(1)
		*lastArgs = append([]string(nil), args...)
		return []byte(chartSVG), nil, nil
	}
}

func TestGetChart(t *testing.T) {
	var calls atomic.Int64
	var args []string
	h, _ := mustHandlerWithCache(t, chartRunner(t, &calls, &args))

	req := &floatv1.GetChartRequest{
		Kind:         floatv1.ChartKind_CHART_KIND_BALANCE,
		Query:        []string{"date:2026-01-01..2026-02-01", "type:R"},
		Depth:        2,
		Invert:       true,
		SortByAmount: true,
	}
	resp, err := h.GetChart(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("GetChart: %v", err)
	}

	joined := strings.Join(args, " ")
	for _, want := range []string{"bal -O svg", "--depth 2", "--invert", "-S", "date:2026-01-01..2026-02-01 type:R"} {
		if !strings.Contains(joined, want) {
			t.Errorf("hledger args %q missing %q", joined, want)
		}
	}
	if resp.Msg.Title != "Chart" {
		t.Errorf("Title = %q, want %q", resp.Msg.Title, "Chart")
	}
	if resp.Msg.Empty {
		t.Error("Empty = true, want false")
	}
	if strings.Contains(resp.Msg.Svg, "script") {
		t.Errorf("Svg contains script: %s", resp.Msg.Svg)
	}
	if !strings.Contains(resp.Msg.Svg, ":where(.dark) .hledger-chart .bg{fill:#000}") {
		t.Errorf("Svg dark styles not rescoped to .dark: %s", resp.Msg.Svg)
	}

	// Same request with query terms reordered is served from cache.
	req.Query = []string{"type:R", "date:2026-01-01..2026-02-01"}
	before := calls.Load()
	if _, err := h.GetChart(t.Context(), connect.NewRequest(req)); err != nil {
		t.Fatalf("GetChart (cached): %v", err)
	}
	if got := calls.Load() - before; got != 0 {
		t.Errorf("hledger invoked %d time(s) for a cached chart, want 0", got)
	}
}

func TestGetChart_InvalidArgument(t *testing.T) {
	tests := []struct {
		name string
		req  *floatv1.GetChartRequest
	}{
		{name: "unspecified kind", req: &floatv1.GetChartRequest{}},
		{name: "negative depth", req: &floatv1.GetChartRequest{Kind: floatv1.ChartKind_CHART_KIND_BALANCE, Depth: -1}},
		{name: "flag-shaped query", req: &floatv1.GetChartRequest{Kind: floatv1.ChartKind_CHART_KIND_BALANCE, Query: []string{"--output-file=/tmp/x"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int64
			var args []string
			h, _ := mustHandlerWithCache(t, chartRunner(t, &calls, &args))
			_, err := h.GetChart(t.Context(), connect.NewRequest(tt.req))
			if connect.CodeOf(err) != connect.CodeInvalidArgument {
				t.Errorf("code = %v (err %v), want InvalidArgument", connect.CodeOf(err), err)
			}
			if calls.Load() != 0 {
				t.Errorf("hledger invoked %d time(s), want 0", calls.Load())
			}
		})
	}
}
