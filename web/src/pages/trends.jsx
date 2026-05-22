import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { formatCurrency } from "../format.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
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
  // dateStr is "YYYY-MM-DD"; format as "Jan '26"
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

  const snapshots = timeseriesData?.snapshots || [];
  const prev = snapshots[snapshots.length - 2];

  // Use live current-price balances for current net worth (matches home page)
  const balanceRows = balancesData?.report?.rows || [];
  const assetsRow = balanceRows.find((r) => r.fullName === "assets");
  const liabilitiesRow = balanceRows.find((r) => r.fullName === "liabilities");
  const currentNetWorth = assetsRow
    ? parseFloat(assetsRow?.amounts?.[0]?.quantity || 0) + parseFloat(liabilitiesRow?.amounts?.[0]?.quantity || 0)
    : null;

  const prevNetWorth = prev ? parseAmount(prev.netWorth) : null;
  const monthChange = currentNetWorth !== null && prevNetWorth !== null ? currentNetWorth - prevNetWorth : null;

  // YTD: compare to last snapshot from previous year
  const currentYear = new Date().getFullYear().toString();
  const firstThisYear = snapshots.find((s) => s.date && s.date.startsWith(currentYear));
  const ytdChange = currentNetWorth !== null && firstThisYear
    ? currentNetWorth - parseAmount(firstThisYear.netWorth)
    : null;

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-bold">Trends</h2>
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
      </div>

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

          <Card>
            <CardContent>
              {snapshots.length === 0 ? (
                <p className="text-sm text-muted-foreground">No data available for this period.</p>
              ) : (
                <NetWorthChart snapshots={snapshots} />
              )}
            </CardContent>
          </Card>
        </>
      )}
    </div>
  );
}
