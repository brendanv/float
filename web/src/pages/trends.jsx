import { useState, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { CartesianGrid, Line, LineChart, XAxis, YAxis, PieChart, Pie, Cell } from "recharts";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { formatCurrency } from "../format.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
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

function toBeginDate(months) {
  if (!months) return "";
  const d = new Date();
  d.setMonth(d.getMonth() - months);
  return d.toISOString().slice(0, 10);
}

function parseAmount(amounts) {
  if (!amounts || amounts.length === 0) return 0;
  return parseFloat(amounts[0].quantity) || 0;
}

function formatLabel(dateStr) {
  if (!dateStr) return "";
  const [year, month] = dateStr.split("-");
  const months = ["Jan","Feb","Mar","Apr","May","Jun","Jul","Aug","Sep","Oct","Nov","Dec"];
  return `${months[parseInt(month, 10) - 1]} '${year.slice(2)}`;
}

function StatCard({ title, value, desc, valueClass }) {
  return (
    <Card className="flex-1">
      <CardHeader>
        <CardTitle className="text-xs font-normal uppercase tracking-wide text-muted-foreground">{title}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className={cn("font-mono text-2xl font-semibold", valueClass)}>{value}</div>
        {desc && <div className="mt-1 text-xs text-muted-foreground">{desc}</div>}
      </CardContent>
    </Card>
  );
}

function NetWorthChart({ snapshots }) {
  const chartData = snapshots.map((s) => ({
    date: formatLabel(s.date),
    netWorth: parseAmount(s.netWorth),
    assets: parseAmount(s.assets),
    liabilities: Math.abs(parseAmount(s.liabilities)),
  }));

  return (
    <ChartContainer config={chartConfig} className="h-80 w-full">
      <LineChart
        accessibilityLayer
        data={chartData}
        margin={{ left: 12, right: 12 }}
      >
        <CartesianGrid vertical={false} />
        <XAxis
          dataKey="date"
          tickLine={false}
          axisLine={false}
          tickMargin={8}
        />
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
        <Line
          dataKey="netWorth"
          type="monotone"
          stroke="var(--color-netWorth)"
          strokeWidth={2}
          dot={false}
        />
        <Line
          dataKey="assets"
          type="monotone"
          stroke="var(--color-assets)"
          strokeWidth={2}
          dot={false}
        />
        <Line
          dataKey="liabilities"
          type="monotone"
          stroke="var(--color-liabilities)"
          strokeWidth={2}
          dot={false}
        />
      </LineChart>
    </ChartContainer>
  );
}

function ExpenseDonutChart({ expenseRows }) {
  const categories = useMemo(() => {
    const all = (expenseRows || [])
      .filter((r) => r.fullName && r.fullName.includes(":"))
      .map((r) => ({ name: r.displayName, amount: parseFloat(r.amounts?.[0]?.quantity || 0) }))
      .filter((c) => c.amount > 0)
      .sort((a, b) => b.amount - a.amount);
    const MAX = DONUT_COLORS.length - 1;
    if (all.length <= DONUT_COLORS.length) return all;
    const top = all.slice(0, MAX);
    const otherAmount = all.slice(MAX).reduce((s, c) => s + c.amount, 0);
    return [...top, { name: "other", amount: otherAmount }];
  }, [expenseRows]);

  const total = useMemo(() => categories.reduce((sum, c) => sum + c.amount, 0), [categories]);

  if (categories.length === 0) {
    return <p className="text-sm text-muted-foreground">No expense data for this period.</p>;
  }

  const donutConfig = Object.fromEntries(
    categories.map((c, i) => [c.name, { label: c.name, color: DONUT_COLORS[i % DONUT_COLORS.length] }])
  );

  return (
    <div className="flex flex-col gap-6 sm:flex-row sm:items-center">
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
  const end = "";

  const { data: timeseriesData, isLoading, error } = useQuery({
    queryKey: queryKeys.netWorthTimeseries(begin),
    queryFn: () => ledgerClient.getNetWorthTimeseries({ begin, end }),
  });

  const { data: balancesData } = useQuery({
    queryKey: queryKeys.balances({ depth: 1, value: "now,USD" }),
    queryFn: () => ledgerClient.getBalances({ depth: 1, value: "now,USD" }),
  });

  const expenseQueryParams = useMemo(() => ({
    query: begin ? ["type:X", `date:${begin}..`] : ["type:X"],
    depth: 2,
  }), [begin]);

  const { data: expensesData, isLoading: expensesLoading } = useQuery({
    queryKey: queryKeys.balances(expenseQueryParams),
    queryFn: () => ledgerClient.getBalances(expenseQueryParams),
  });

  const snapshots = timeseriesData?.snapshots || [];
  const prev = snapshots[snapshots.length - 2];

  const balanceRows = balancesData?.report?.rows || [];
  const assetsRow = balanceRows.find((r) => r.fullName === "assets");
  const liabilitiesRow = balanceRows.find((r) => r.fullName === "liabilities");
  const currentNetWorth = assetsRow
    ? parseFloat(assetsRow?.amounts?.[0]?.quantity || 0) + parseFloat(liabilitiesRow?.amounts?.[0]?.quantity || 0)
    : null;

  const prevNetWorth = prev ? parseAmount(prev.netWorth) : null;
  const monthChange = currentNetWorth !== null && prevNetWorth !== null ? currentNetWorth - prevNetWorth : null;

  const currentYear = new Date().getFullYear().toString();
  const firstThisYear = snapshots.find((s) => s.date && s.date.startsWith(currentYear));
  const ytdChange = currentNetWorth !== null && firstThisYear
    ? currentNetWorth - parseAmount(firstThisYear.netWorth)
    : null;

  const expenseRows = expensesData?.report?.rows || [];

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title="Trends">
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

      {!isLoading && !error && (
        <>
          <div className="flex flex-col gap-3 sm:flex-row">
            <StatCard
              title="Current Net Worth"
              value={currentNetWorth !== null ? formatCurrency(currentNetWorth, "USD") : "—"}
              valueClass={currentNetWorth !== null && currentNetWorth >= 0 ? "text-success" : "text-destructive"}
            />
            <StatCard
              title="Change This Month"
              value={monthChange !== null ? formatCurrency(monthChange, "USD") : "—"}
              valueClass={monthChange !== null && monthChange >= 0 ? "text-success" : "text-destructive"}
              desc={monthChange !== null && monthChange >= 0 ? "▲ vs last month" : "▼ vs last month"}
            />
            <StatCard
              title="YTD Change"
              value={ytdChange !== null ? formatCurrency(ytdChange, "USD") : "—"}
              valueClass={ytdChange !== null && ytdChange >= 0 ? "text-success" : "text-destructive"}
              desc={ytdChange !== null && ytdChange >= 0 ? "▲ since Jan 1" : "▼ since Jan 1"}
            />
          </div>

          <div className="grid gap-6 lg:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle className="text-xs font-normal uppercase tracking-wide text-muted-foreground">Net Worth Over Time</CardTitle>
              </CardHeader>
              <CardContent>
                {snapshots.length === 0 ? (
                  <p className="text-sm text-muted-foreground">No data available for this period.</p>
                ) : (
                  <NetWorthChart snapshots={snapshots} />
                )}
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-xs font-normal uppercase tracking-wide text-muted-foreground">Expenses by Category</CardTitle>
              </CardHeader>
              <CardContent>
                {expensesLoading ? (
                  <div className="flex h-52 items-center justify-center">
                    <Loading />
                  </div>
                ) : (
                  <ExpenseDonutChart expenseRows={expenseRows} />
                )}
              </CardContent>
            </Card>
          </div>
        </>
      )}
    </div>
  );
}
