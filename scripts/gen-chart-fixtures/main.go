// Command gen-chart-fixtures renders the sanitized hledger SVG charts that the
// Playwright mock API (web/tests/mock-api.js) returns for GetChart, using the
// same hledger.Client.Chart path as floatd. Re-run it after upgrading hledger:
//
//	go run ./scripts/gen-chart-fixtures
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/brendanv/float/internal/hledger"
)

func main() {
	dir := flag.String("dir", "web/tests/fixtures", "fixture directory containing charts.journal")
	flag.Parse()
	if err := run(context.Background(), *dir); err != nil {
		fmt.Fprintln(os.Stderr, "gen-chart-fixtures:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, dir string) error {
	c, err := hledger.New("hledger", filepath.Join(dir, "charts.journal"))
	if err != nil {
		return err
	}
	// Mirrors the home page Insights requests (web/src/components/insights-chart.jsx).
	charts := map[string]hledger.ChartSpec{
		"chart-expenses.svg": {Kind: hledger.ChartBalance, Depth: 2, SortByAmount: true, Query: []string{"type:X"}},
		"chart-revenue.svg":  {Kind: hledger.ChartBalance, Depth: 2, SortByAmount: true, Invert: true, Query: []string{"type:R"}},
		"chart-empty.svg":    {Kind: hledger.ChartBalance, Depth: 2, Query: []string{"date:1999"}},
	}
	for name, spec := range charts {
		spec.DarkSelector = ".dark"
		chart, err := c.Chart(ctx, spec)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(chart.SVG+"\n"), 0o644); err != nil {
			return err
		}
		fmt.Println("wrote", filepath.Join(dir, name))
	}
	return nil
}
