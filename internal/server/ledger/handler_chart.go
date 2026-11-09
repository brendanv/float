package ledger

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/cache"
	"github.com/brendanv/float/internal/hledger"
)

// chartDarkSelector is the web UI's dark-theme class (Tailwind `.dark` on an
// ancestor). hledger's prefers-color-scheme styles are rescoped to it so
// charts follow the app's theme toggle rather than the OS setting.
const chartDarkSelector = ".dark"

func chartKey(spec hledger.ChartSpec) string {
	return fmt.Sprintf("chart:%d:%d:%t:%t:%s", spec.Kind, spec.Depth, spec.Invert, spec.SortByAmount, normalizedQueryKeyPart(spec.Query))
}

// cachedChart fetches a sanitized hledger SVG chart from cache or hledger.
func cachedChart(ctx context.Context, c *cache.Cache[any], hl *hledger.Client, spec hledger.ChartSpec) (*hledger.Chart, error) {
	return cachedGet(ctx, c, chartKey(spec), func(ctx context.Context) (*hledger.Chart, error) {
		return hl.Chart(ctx, spec)
	})
}

func chartKindFromProto(k floatv1.ChartKind) (hledger.ChartKind, error) {
	switch k {
	case floatv1.ChartKind_CHART_KIND_BALANCE:
		return hledger.ChartBalance, nil
	default:
		return 0, fmt.Errorf("unsupported chart kind %v", k)
	}
}

// homeInsightsChartSpecs mirrors the two charts the web UI home page requests
// for a period query (expenses and inverted revenues at depth 2, sorted by
// amount), so they can be warmed for the current month.
func homeInsightsChartSpecs(periodQuery []string) []hledger.ChartSpec {
	base := func(typeQuery string, invert bool) hledger.ChartSpec {
		return hledger.ChartSpec{
			Kind:         hledger.ChartBalance,
			Depth:        2,
			Invert:       invert,
			SortByAmount: true,
			DarkSelector: chartDarkSelector,
			Query:        append(append([]string(nil), periodQuery...), typeQuery),
		}
	}
	return []hledger.ChartSpec{base("type:X", false), base("type:R", true)}
}

func (h *Handler) GetChart(ctx context.Context, req *connect.Request[floatv1.GetChartRequest]) (*connect.Response[floatv1.GetChartResponse], error) {
	kind, err := chartKindFromProto(req.Msg.Kind)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if req.Msg.Depth < 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("depth must be >= 0"))
	}
	spec := hledger.ChartSpec{
		Kind:         kind,
		Depth:        int(req.Msg.Depth),
		Invert:       req.Msg.Invert,
		SortByAmount: req.Msg.SortByAmount,
		DarkSelector: chartDarkSelector,
		Query:        req.Msg.Query,
	}
	chart, err := cachedChart(ctx, h.cache, h.hl, spec)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger chart failed")
	}
	return connect.NewResponse(&floatv1.GetChartResponse{
		Svg:   chart.SVG,
		Title: chart.Title,
		Empty: chart.Empty,
	}), nil
}
