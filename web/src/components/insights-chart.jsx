import { ChartKind } from "@/gen/float/v1/ledger_pb.js";
import { HledgerChartSvg, useHledgerChart } from "./hledger-chart.jsx";
import { Loading } from "./loading.jsx";
import { ErrorBanner } from "./error-banner.jsx";

function insightsChartParams(periodQuery, typeQuery, invert) {
  return {
    kind: ChartKind.BALANCE,
    depth: 2,
    sortByAmount: true,
    invert,
    query: [...periodQuery, typeQuery],
  };
}

function ChartSection({ title, chart }) {
  if (!chart || chart.empty) return null;
  return (
    <div className="mb-6">
      <h5 className="mb-2 text-sm font-semibold uppercase tracking-wide text-muted-foreground">
        {title}
      </h5>
      <HledgerChartSvg chart={chart} />
    </div>
  );
}

// Keep these params in sync with homeInsightsChartSpecs in
// internal/server/ledger/handler_chart.go, which warms them for this month.
export function InsightsChart({ periodQuery }) {
  const expenses = useHledgerChart(insightsChartParams(periodQuery, "type:X", false));
  // Revenues are negative in hledger; invert so their bars grow upward.
  const revenue = useHledgerChart(insightsChartParams(periodQuery, "type:R", true));

  if (expenses.isLoading || revenue.isLoading) return <Loading />;
  if (expenses.error || revenue.error) {
    return <ErrorBanner error={expenses.error || revenue.error} />;
  }

  const bothEmpty = expenses.data?.empty && revenue.data?.empty;
  return (
    <div>
      <ChartSection title="Expenses" chart={expenses.data} />
      <ChartSection title="Revenue" chart={revenue.data} />
      {bothEmpty && (
        <p className="text-sm text-muted-foreground">No expense or revenue data for this period.</p>
      )}
    </div>
  );
}
