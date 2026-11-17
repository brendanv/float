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
  netWorthSummary,
  netWorthSummaryQuery,
  netWorthTimeseriesQuery,
  shortMonthLabel,
  timeseriesBegin,
} from "@/lib/net-worth.js";
import {
  CHART_STROKE,
  CHART_X_AXIS,
  CHART_Y_AXIS,
  ChartAreaFill,
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  ChartTooltipRow,
  chartEndLabel,
  compactCurrency,
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
        <ChartAreaFill id={fillId} color="var(--color-netWorth)" opacity={0.1} />
        <CartesianGrid vertical={false} />
        <XAxis dataKey="date" {...CHART_X_AXIS} />
        <YAxis {...CHART_Y_AXIS} tickFormatter={(v) => compactCurrency(v)} />
        <ChartTooltip
          cursor={false}
          content={
            <ChartTooltipContent
              formatter={(value, name, item) => (
                <ChartTooltipRow color={item.color} label={chartConfig[name]?.label ?? name}>
                  <Amount value={value} commodity="USD" />
                </ChartTooltipRow>
              )}
            />
          }
        />
        <Line
          dataKey="liabilities"
          type="monotone"
          stroke="var(--color-liabilities)"
          strokeWidth={CHART_STROKE}
          dot={false}
          isAnimationActive={false}
        >
          <LabelList dataKey="liabilities" content={chartEndLabel("Liabilities", "var(--color-liabilities)", last)} />
        </Line>
        <Line
          dataKey="assets"
          type="monotone"
          stroke="var(--color-assets)"
          strokeWidth={CHART_STROKE}
          dot={false}
          isAnimationActive={false}
        >
          <LabelList dataKey="assets" content={chartEndLabel("Assets", "var(--color-assets)", last)} />
        </Line>
        <Area
          dataKey="netWorth"
          type="monotone"
          stroke="var(--color-netWorth)"
          strokeWidth={CHART_STROKE}
          fill={`url(#${fillId})`}
          dot={false}
          isAnimationActive={false}
        >
          <LabelList dataKey="netWorth" content={chartEndLabel("Net worth", "var(--color-netWorth)", last)} />
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
                    <ChartTooltipRow color={donutConfig[name]?.color} label={name}>
                      <Amount value={value} commodity="USD" />
                      <span className="ml-1.5 font-normal text-faint">{pct}%</span>
                    </ChartTooltipRow>
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
            stroke="var(--background)"
            paddingAngle={0}
            isAnimationActive={false}
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

  const { data: balancesData } = useQuery(netWorthSummaryQuery());

  const expenseQueryParams = useMemo(() => ({
    query: begin ? ["type:X", `date:${begin}..`] : ["type:X"],
    depth: 2,
  }), [begin]);

  const { data: expensesData, isLoading: expensesLoading } = useQuery({
    queryKey: queryKeys.balances(expenseQueryParams),
    queryFn: () => ledgerClient.getBalances(expenseQueryParams),
  });

  const snapshots = timeseriesData?.snapshots || [];

  const { netWorth: currentNetWorth, commodity } = netWorthSummary(balancesData?.report);

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
            amount={currentNetWorth !== null ? { commodity, quantity: String(currentNetWorth) } : undefined}
            tone={currentNetWorth !== null && currentNetWorth < 0 ? "below" : "ink"}
            delta={monthChange !== null && (
              <>
                <Amount value={monthChange} commodity={commodity} signed />
                {prevMonth && ` vs ${prevMonth}`}
              </>
            )}
            deltaTone={deltaTone(monthChange)}
            sub={ytdChange !== null && (
              <><Amount value={ytdChange} commodity={commodity} signed /> this year</>
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
