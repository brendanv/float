import { useState, useMemo, useId } from "react";
import { useQuery } from "@tanstack/react-query";
import { Area, CartesianGrid, ComposedChart, LabelList, Line, XAxis, YAxis, PieChart, Pie, Cell } from "recharts";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { Amount } from "../components/amount.jsx";
import { MetricHero, Page, Section } from "../components/page.jsx";
import { Segmented } from "../components/segmented.jsx";
import { SERIES_COLORS, foldToOther } from "@/lib/chart-colors";
import {
  deltaTone,
  firstQuantity as parseAmount,
  netWorthChanges,
  netWorthTimeseriesQuery,
  shortMonthLabel,
  timeseriesBegin,
} from "@/lib/net-worth.js";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
} from "@/components/ui/chart";

// Net worth is ink (the primary series, with an area fill); assets and
// liabilities take their account-type colors. Three series, so they are
// labelled at their line ends instead of in a legend.
const chartConfig = {
  netWorth: { label: "Net worth", color: "var(--foreground)" },
  assets: { label: "Assets", color: "var(--acct-asset)" },
  liabilities: { label: "Liabilities", color: "var(--acct-liability)" },
};

const RANGES = [
  { label: "1Y", months: 12 },
  { label: "2Y", months: 24 },
  { label: "5Y", months: 60 },
  { label: "All", months: null },
];

const compactUSD = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
  notation: "compact",
  maximumFractionDigits: 1,
});

// endLabel renders a series name and an endpoint dot at the last point.
function endLabel(name, color, lastIndex) {
  function EndLabel({ x, y, index }) {
    if (index !== lastIndex || x == null || y == null) return null;
    return (
      <g>
        <circle cx={x} cy={y} r={3} fill={color} stroke="var(--background)" strokeWidth={1.5} />
        <text x={x + 8} y={y} dy="0.35em" fill={color} fontSize={11.5} fontWeight={500}>
          {name}
        </text>
      </g>
    );
  }
  return EndLabel;
}

function NetWorthChart({ snapshots }) {
  const fillId = useId().replace(/:/g, "");
  const chartData = snapshots.map((s) => ({
    date: shortMonthLabel(s.date),
    netWorth: parseAmount(s.netWorth),
    assets: parseAmount(s.assets),
    liabilities: Math.abs(parseAmount(s.liabilities)),
  }));
  const last = chartData.length - 1;

  return (
    <ChartContainer config={chartConfig} className="trends-chart aspect-auto h-80 w-full">
      <ComposedChart
        accessibilityLayer
        data={chartData}
        margin={{ top: 8, left: 4, right: 76, bottom: 0 }}
      >
        <defs>
          <linearGradient id={fillId} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" stopColor="var(--color-netWorth)" stopOpacity={0.1} />
            <stop offset="1" stopColor="var(--color-netWorth)" stopOpacity={0} />
          </linearGradient>
        </defs>
        <CartesianGrid vertical={false} />
        <XAxis
          dataKey="date"
          tickLine={false}
          axisLine={false}
          tickMargin={8}
          minTickGap={24}
        />
        <YAxis
          tickLine={false}
          axisLine={false}
          tickMargin={4}
          width={52}
          tickFormatter={(v) => compactUSD.format(v)}
        />
        <ChartTooltip
          cursor={false}
          content={
            <ChartTooltipContent
              formatter={(value, name, item) => (
                <>
                  <div className="h-2.5 w-2.5 shrink-0 rounded-[2px]" style={{ backgroundColor: item.color }} />
                  <div className="flex flex-1 items-center justify-between gap-2 leading-none">
                    <span className="text-muted-foreground">{chartConfig[name]?.label ?? name}</span>
                    <Amount value={value} commodity="USD" className="font-medium text-foreground" />
                  </div>
                </>
              )}
            />
          }
        />
        <Line
          dataKey="liabilities"
          type="monotone"
          stroke="var(--color-liabilities)"
          strokeWidth={1.5}
          dot={false}
          isAnimationActive={false}
        >
          <LabelList dataKey="liabilities" content={endLabel("Liabilities", "var(--color-liabilities)", last)} />
        </Line>
        <Line
          dataKey="assets"
          type="monotone"
          stroke="var(--color-assets)"
          strokeWidth={1.5}
          dot={false}
          isAnimationActive={false}
        >
          <LabelList dataKey="assets" content={endLabel("Assets", "var(--color-assets)", last)} />
        </Line>
        <Area
          dataKey="netWorth"
          type="monotone"
          stroke="var(--color-netWorth)"
          strokeWidth={1.8}
          fill={`url(#${fillId})`}
          dot={false}
          isAnimationActive={false}
        >
          <LabelList dataKey="netWorth" content={endLabel("Net worth", "var(--color-netWorth)", last)} />
        </Area>
      </ComposedChart>
    </ChartContainer>
  );
}

function ExpenseDonutChart({ expenseRows }) {
  const categories = useMemo(() => {
    const all = (expenseRows || [])
      .filter((r) => r.fullName && r.fullName.includes(":"))
      .map((r) => ({ name: r.fullName.split(":").slice(1).join(":"), amount: parseFloat(r.amounts?.[0]?.quantity || 0) }))
      .filter((c) => c.amount > 0)
      .sort((a, b) => b.amount - a.amount);
    return foldToOther(all).map((c, i) => ({ ...c, color: SERIES_COLORS[i] }));
  }, [expenseRows]);

  const total = useMemo(() => categories.reduce((sum, c) => sum + c.amount, 0), [categories]);

  if (categories.length === 0) {
    return <p className="text-body text-muted-foreground">No expense data for this period.</p>;
  }

  const donutConfig = Object.fromEntries(
    categories.map((c) => [c.name, { label: c.name, color: c.color }])
  );

  return (
    <div className="flex flex-col gap-4">
      <ChartContainer config={donutConfig} className="mx-auto aspect-square h-48 flex-shrink-0">
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
                        <span className="text-muted-foreground">{name}</span>
                        <span className="font-medium text-foreground">
                          <Amount value={value} commodity="USD" /> ({pct}%)
                        </span>
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
            stroke="var(--card)"
            paddingAngle={0}
          >
            {categories.map((c) => (
              <Cell key={c.name} fill={c.color} />
            ))}
          </Pie>
        </PieChart>
      </ChartContainer>
      <div className="flex min-w-0 flex-col gap-1.5">
        {categories.map((c) => (
          <div key={c.name} className="flex items-center gap-2">
            <div
              className="h-2.5 w-2.5 flex-shrink-0 rounded-sm"
              style={{ backgroundColor: c.color }}
            />
            <span className="min-w-0 flex-1 truncate" title={c.name}>{c.name}</span>
            <Amount value={c.amount} commodity="USD" />
            <span className="w-10 text-right text-[12px] text-faint">
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
  const begin = timeseriesBegin(range.months);

  const { data: timeseriesData, isLoading, error } = useQuery(netWorthTimeseriesQuery(begin));

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

  const balanceRows = balancesData?.report?.rows || [];
  const assetsRow = balanceRows.find((r) => r.fullName === "assets");
  const liabilitiesRow = balanceRows.find((r) => r.fullName === "liabilities");
  const currentNetWorth = assetsRow
    ? parseFloat(assetsRow?.amounts?.[0]?.quantity || 0) + parseFloat(liabilitiesRow?.amounts?.[0]?.quantity || 0)
    : null;

  const { monthChange, prevMonth, ytdChange } = netWorthChanges(snapshots, currentNetWorth);

  const expenseRows = expensesData?.report?.rows || [];

  return (
    <Page>
      <PageHeader title="Trends">
        <Segmented
          label="Range"
          options={RANGES.map((r, i) => ({ value: i, label: r.label }))}
          value={rangeIdx}
          onChange={setRangeIdx}
        />
      </PageHeader>

      {isLoading && <Loading />}
      {error && <ErrorBanner error={error} />}

      {!isLoading && !error && (
        <>
          <MetricHero
            label="Net worth"
            value={currentNetWorth === null ? <span className="text-faint">—</span> : undefined}
            amount={currentNetWorth !== null ? { commodity: "USD", quantity: String(currentNetWorth) } : undefined}
            tone={currentNetWorth !== null && currentNetWorth < 0 ? "below" : "ink"}
            delta={monthChange !== null && (
              <>
                <Amount value={monthChange} commodity="USD" signed />
                {prevMonth && ` vs ${prevMonth}`}
              </>
            )}
            deltaTone={deltaTone(monthChange)}
            sub={ytdChange !== null && (
              <><Amount value={ytdChange} commodity="USD" signed /> this year</>
            )}
          />

          <div className="grid gap-x-12 gap-y-8 xl:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
            <Section title="Over time" description={range.months ? `Last ${range.months / 12 === 1 ? "12 months" : `${range.months / 12} years`}` : "All history"} headerClassName="[&>span]:ml-auto">
              {snapshots.length === 0 ? (
                <p className="text-body text-muted-foreground">No data available for this period.</p>
              ) : (
                <NetWorthChart snapshots={snapshots} />
              )}
            </Section>

            <Section title="Spending by category">
              {expensesLoading ? (
                <div className="flex h-52 items-center justify-center">
                  <Loading />
                </div>
              ) : (
                <ExpenseDonutChart expenseRows={expenseRows} />
              )}
            </Section>
          </div>
        </>
      )}
    </Page>
  );
}
