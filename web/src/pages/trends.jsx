import { useState, useMemo } from "react";
import { CartesianGrid, Line, LineChart, XAxis, YAxis, PieChart, Pie, Cell } from "recharts";
import { formatCurrency } from "../format.js";
import { useCube } from "../hooks/use-cube.js";
import {
  balanceAt,
  balanceSeries,
  flowByAccount,
  latestPeriod,
  periodsInRange,
} from "../lib/cube-query.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { DashboardGrid, MetricCard, Page } from "../components/page.jsx";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
} from "@/components/ui/chart";

const chartConfig = {
  netWorth: { label: "Net Worth", color: "var(--chart-1)" },
  assets: { label: "Assets", color: "var(--chart-2)" },
  liabilities: { label: "Liabilities", color: "var(--chart-3)" },
};

const RANGES = [
  { label: "1Y", months: 12 },
  { label: "2Y", months: 24 },
  { label: "5Y", months: 60 },
  { label: "All", months: null },
];

const DONUT_COLORS = [
  "#6366f1",
  "#f59e0b",
  "#22c55e",
  "#3b82f6",
  "#a855f7",
  "#14b8a6",
  "#ef4444",
  "#f97316",
];

const MONTH_NAMES = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

/** Start of the range, as a "YYYY-MM-DD" string. Null means unbounded. */
function toBeginDate(months) {
  if (!months) return null;
  const d = new Date();
  d.setMonth(d.getMonth() - months);
  return d.toISOString().slice(0, 10);
}

function formatLabel(period) {
  if (!period) return "";
  const [year, month] = period.split("-");
  return `${MONTH_NAMES[parseInt(month, 10) - 1]} '${year.slice(2)}`;
}

/** The last period of the year before the given one, for a year-to-date base. */
function previousYearEnd(cube, period) {
  if (!period) return null;
  const previous = `${Number(period.slice(0, 4)) - 1}-12`;
  const candidates = cube.periods.filter((p) => p <= previous);
  return candidates.length ? candidates[candidates.length - 1] : null;
}

function NetWorthChart({ data }) {
  return (
    <ChartContainer config={chartConfig} className="h-80 w-full">
      <LineChart accessibilityLayer data={data} margin={{ left: 12, right: 12 }}>
        <CartesianGrid vertical={false} />
        <XAxis dataKey="date" tickLine={false} axisLine={false} tickMargin={8} />
        <YAxis
          tickLine={false}
          axisLine={false}
          tickMargin={8}
          width={80}
          tickFormatter={(v) => formatCurrency(v, "USD")}
        />
        <ChartTooltip
          cursor={false}
          content={
            <ChartTooltipContent
              formatter={(value, name, item) => (
                <>
                  <div className="shrink-0 rounded-[2px] h-2.5 w-2.5" style={{ backgroundColor: item.color }} />
                  <div className="flex flex-1 justify-between items-center gap-2 leading-none">
                    <span className="text-muted-foreground">{chartConfig[name]?.label ?? name}</span>
                    <span className="font-mono font-medium tabular-nums">{formatCurrency(value, "USD")}</span>
                  </div>
                </>
              )}
            />
          }
        />
        <ChartLegend content={<ChartLegendContent />} />
        <Line dataKey="netWorth" type="monotone" stroke="var(--color-netWorth)" strokeWidth={2} dot={false} />
        <Line dataKey="assets" type="monotone" stroke="var(--color-assets)" strokeWidth={2} dot={false} />
        <Line dataKey="liabilities" type="monotone" stroke="var(--color-liabilities)" strokeWidth={2} dot={false} />
      </LineChart>
    </ChartContainer>
  );
}

function ExpenseDonutChart({ categories }) {
  const total = useMemo(() => categories.reduce((sum, c) => sum + c.amount, 0), [categories]);

  if (categories.length === 0) {
    return <p className="text-sm text-muted-foreground">No expense data for this period.</p>;
  }

  const donutConfig = Object.fromEntries(
    categories.map((c, i) => [c.name, { label: c.name, color: DONUT_COLORS[i % DONUT_COLORS.length] }])
  );

  return (
    // Stacked, not side-by-side: this card is a quarter-width column on wide
    // screens, and a horizontal split leaves the legend ~100px, which collapses
    // the truncating category name to zero width and shows amounts with no
    // labels at all.
    <div className="flex flex-col items-center gap-6">
      <ChartContainer config={donutConfig} className="mx-auto h-52 w-52 flex-shrink-0">
        <PieChart>
          <ChartTooltip
            content={
              <ChartTooltipContent
                hideLabel
                formatter={(value, name) => {
                  const pct = total > 0 ? ((value / total) * 100).toFixed(1) : "0";
                  return (
                    <>
                      <div className="shrink-0 rounded-[2px] h-2.5 w-2.5" style={{ backgroundColor: donutConfig[name]?.color }} />
                      <div className="flex flex-1 justify-between items-center gap-2 leading-none">
                        <span className="text-muted-foreground capitalize">{name}</span>
                        <span className="font-mono font-medium tabular-nums">{formatCurrency(value, "USD")} ({pct}%)</span>
                      </div>
                    </>
                  );
                }}
              />
            }
          />
          <Pie
            data={categories}
            dataKey="amount"
            nameKey="name"
            innerRadius="60%"
            outerRadius="80%"
            strokeWidth={2}
            stroke="transparent"
          >
            {categories.map((_, i) => (
              <Cell key={i} fill={DONUT_COLORS[i % DONUT_COLORS.length]} />
            ))}
          </Pie>
        </PieChart>
      </ChartContainer>
      <div className="flex min-w-0 flex-1 flex-col gap-1.5">
        {categories.map((c, i) => (
          <div key={c.name} className="flex items-center gap-2 text-sm">
            <div
              className="h-2.5 w-2.5 flex-shrink-0 rounded-sm"
              style={{ backgroundColor: DONUT_COLORS[i % DONUT_COLORS.length] }}
            />
            <span className="min-w-0 flex-1 truncate capitalize">{c.name}</span>
            <span className="font-mono tabular-nums">{formatCurrency(c.amount, "USD")}</span>
            <span className="w-10 text-right text-xs text-muted-foreground">
              {total > 0 ? `${((c.amount / total) * 100).toFixed(0)}%` : ""}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

export function TrendsPage() {
  const [rangeIdx, setRangeIdx] = useState(0);
  const range = RANGES[rangeIdx];
  const begin = toBeginDate(range.months);

  // One payload backs the whole page. Switching range re-slices it in the
  // browser, so the buttons do not refetch anything.
  const { data: cube, isLoading, error } = useCube();

  const view = useMemo(() => {
    if (!cube) return null;

    const periods = periodsInRange(cube, begin, null);
    // The valued table only holds asset and liability accounts, so an empty
    // account prefix is exactly net worth.
    const netWorthByPeriod = new Map(
      balanceSeries(cube, "valued", "", { from: begin }).map((p) => [p.period, p.total])
    );
    const assetsByPeriod = new Map(
      balanceSeries(cube, "valued", "assets", { from: begin }).map((p) => [p.period, p.total])
    );
    const liabilitiesByPeriod = new Map(
      balanceSeries(cube, "valued", "liabilities", { from: begin }).map((p) => [p.period, p.total])
    );

    const chartData = periods.map((period) => ({
      date: formatLabel(period),
      netWorth: netWorthByPeriod.get(period) ?? 0,
      assets: assetsByPeriod.get(period) ?? 0,
      // Liabilities are negative in the ledger; the chart plots magnitude.
      liabilities: Math.abs(liabilitiesByPeriod.get(period) ?? 0),
    }));

    const current = latestPeriod(cube);
    const previous = current
      ? cube.periods[cube.periods.indexOf(current) - 1] ?? null
      : null;
    const yearBase = previousYearEnd(cube, current);

    // Each of these is an account-tree rollup at one period end — never a sum
    // across periods, which market value does not admit.
    const currentNetWorth = current ? balanceAt(cube, "valued", current, "") : null;
    const previousNetWorth = previous ? balanceAt(cube, "valued", previous, "") : null;
    const yearBaseNetWorth = yearBase ? balanceAt(cube, "valued", yearBase, "") : null;

    // Label with the segment below the top level ("food", not
    // "expenses:food"): the legend column is narrow, and a full path truncates
    // to nothing.
    const categories = flowByAccount(cube, { type: "X", from: begin }, 2)
      .filter((row) => row.total > 0)
      .map((row) => ({
        name: row.account.split(":").slice(1).join(":") || row.account,
        amount: row.total,
      }));
    const MAX = DONUT_COLORS.length - 1;
    const topCategories =
      categories.length <= DONUT_COLORS.length
        ? categories
        : [
            ...categories.slice(0, MAX),
            { name: "other", amount: categories.slice(MAX).reduce((s, c) => s + c.amount, 0) },
          ];

    return {
      chartData,
      currentNetWorth,
      monthChange:
        currentNetWorth !== null && previousNetWorth !== null
          ? currentNetWorth - previousNetWorth
          : null,
      ytdChange:
        currentNetWorth !== null && yearBaseNetWorth !== null
          ? currentNetWorth - yearBaseNetWorth
          : null,
      categories: topCategories,
    };
  }, [cube, begin]);

  return (
    <Page>
      <PageHeader title="Trends" description="Net worth movement and spending mix over time.">
        <div className="flex gap-1">
          {RANGES.map((r, i) => (
            <Button
              key={r.label}
              size="sm"
              variant={rangeIdx === i ? "default" : "ghost"}
              onClick={() => setRangeIdx(i)}
            >
              {r.label}
            </Button>
          ))}
        </div>
      </PageHeader>

      {isLoading && <Loading />}
      {error && <ErrorBanner error={error} />}

      {!isLoading && !error && view && (
        <DashboardGrid>
          <div className="col-span-12 md:col-span-4">
            <MetricCard
              title="Current Net Worth"
              value={view.currentNetWorth !== null ? formatCurrency(view.currentNetWorth, "USD") : "—"}
              valueClassName={view.currentNetWorth !== null && view.currentNetWorth >= 0 ? "text-success" : "text-destructive"}
            />
          </div>
          <div className="col-span-12 md:col-span-4">
            <MetricCard
              title="Change This Month"
              value={view.monthChange !== null ? formatCurrency(view.monthChange, "USD") : "—"}
              valueClassName={view.monthChange !== null && view.monthChange >= 0 ? "text-success" : "text-destructive"}
              description={view.monthChange !== null && view.monthChange >= 0 ? "Up from last month" : "Down from last month"}
            />
          </div>
          <div className="col-span-12 md:col-span-4">
            <MetricCard
              title="YTD Change"
              value={view.ytdChange !== null ? formatCurrency(view.ytdChange, "USD") : "—"}
              valueClassName={view.ytdChange !== null && view.ytdChange >= 0 ? "text-success" : "text-destructive"}
              description={view.ytdChange !== null && view.ytdChange >= 0 ? "Up since Jan 1" : "Down since Jan 1"}
            />
          </div>

          <div className="col-span-12 xl:col-span-8">
            <Card className="h-full">
              <CardHeader>
                <CardTitle>Net Worth Over Time</CardTitle>
              </CardHeader>
              <CardContent>
                {view.chartData.length === 0 ? (
                  <p className="text-sm text-muted-foreground">No data available for this period.</p>
                ) : (
                  <NetWorthChart data={view.chartData} />
                )}
              </CardContent>
            </Card>
          </div>

          <div className="col-span-12 xl:col-span-4">
            <Card className="h-full">
              <CardHeader>
                <CardTitle>Expenses by Category</CardTitle>
              </CardHeader>
              <CardContent>
                <ExpenseDonutChart categories={view.categories} />
              </CardContent>
            </Card>
          </div>
        </DashboardGrid>
      )}
    </Page>
  );
}
