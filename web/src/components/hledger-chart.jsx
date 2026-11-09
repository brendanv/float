import { useQuery } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { cn } from "@/lib/utils";

/**
 * Fetches an hledger-rendered SVG chart via GetChart. `params` is a
 * GetChartRequest: { kind: ChartKind.X, query, depth, invert, sortByAmount }.
 * Resolves to { svg, title, empty }.
 */
export function useHledgerChart(params) {
  return useQuery({
    queryKey: queryKeys.chart(params),
    queryFn: () => ledgerClient.getChart(params),
  });
}

/**
 * Inlines an hledger SVG chart. Inlining (rather than <img>) keeps hledger's
 * <title> tooltips working and lets style.css map the chart's chrome onto the
 * app theme. The markup is safe to inline: floatd re-serializes it through an
 * SVG element/attribute allowlist (internal/hledger sanitizeSVG), and hledger
 * escapes all journal text.
 */
export function HledgerChartSvg({ chart, className }) {
  if (!chart?.svg) return null;
  return (
    <div
      className={cn("hledger-chart-host", className)}
      role="figure"
      aria-label={chart.title || undefined}
      dangerouslySetInnerHTML={{ __html: chart.svg }}
    />
  );
}
