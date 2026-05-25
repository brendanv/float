import { useState, useMemo } from "react";
import { Link } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { CartesianGrid, Line, LineChart, XAxis, YAxis, PieChart, Pie, Cell } from "recharts";
import {
  useReactTable,
  getCoreRowModel,
  getSortedRowModel,
  flexRender,
  createColumnHelper,
} from "@tanstack/react-table";
import { ChevronUp, ChevronDown, ChevronsUpDown } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { formatCurrency } from "../format.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { Stats15 } from "../components/stats-15.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { DashboardGrid, MetricCard, Page } from "../components/page.jsx";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
} from "@/components/ui/chart";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

const CHART_COLORS = [
  "#6366f1", "#22d3ee", "#f59e0b", "#34d399", "#f43f5e",
  "#a78bfa", "#38bdf8", "#fb923c", "#4ade80", "#e879f9",
];

function formatLabel(dateStr) {
  if (!dateStr) return "";
  const parts = dateStr.split("-");
  const year = parts[0];
  const month = parseInt(parts[1], 10);
  if (!month) return dateStr;
  const months = ["Jan","Feb","Mar","Apr","May","Jun","Jul","Aug","Sep","Oct","Nov","Dec"];
  return `${months[month - 1]} '${year.slice(2)}`;
}

function SortIcon({ column }) {
  const sorted = column.getIsSorted();
  if (sorted === "asc") return <ChevronUp className="inline ml-1 h-3 w-3" />;
  if (sorted === "desc") return <ChevronDown className="inline ml-1 h-3 w-3" />;
  return <ChevronsUpDown className="inline ml-1 h-3 w-3 opacity-40" />;
}

function SortableHeader({ column, children, className }) {
  return (
    <button
      className={cn("flex items-center gap-0.5 hover:text-foreground transition-colors", className)}
      onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
    >
      {children}
      <SortIcon column={column} />
    </button>
  );
}

function AllocationChart({ holdings }) {
  const valued = holdings.filter((h) => h.currentValue);
  if (valued.length === 0) {
    return (
      <Card className="h-full">
        <CardHeader>
          <CardTitle>Allocation</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">No valued holdings available.</p>
        </CardContent>
      </Card>
    );
  }

  const chartData = valued.map((h) => ({
    symbol: h.symbol,
    value: parseFloat(h.currentValue.quantity),
    commodity: h.currentValue.commodity,
    portfolioPct: h.portfolioPct,
  }));

  const chartConfig = Object.fromEntries(
    valued.map((h, i) => [h.symbol, { label: h.symbol, color: CHART_COLORS[i % CHART_COLORS.length] }])
  );

  return (
    <Card>
      <CardHeader>
        <CardTitle>Allocation</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex flex-col gap-6 sm:flex-row sm:items-center">
          <ChartContainer config={chartConfig} className="mx-auto h-52 w-52 flex-shrink-0">
            <PieChart>
              <ChartTooltip
                content={
                  <ChartTooltipContent
                    hideLabel
                    formatter={(value, name) => {
                      const h = valued.find((h) => h.symbol === name);
                      const pct = h?.portfolioPct?.toFixed(1) ?? "0.0";
                      return (
                        <>
                          <div className="shrink-0 rounded-[2px] h-2.5 w-2.5" style={{ backgroundColor: chartConfig[name]?.color }} />
                          <div className="flex flex-1 justify-between items-center gap-2 leading-none">
                            <span className="text-muted-foreground">{name}</span>
                            <span className="font-mono font-medium tabular-nums">
                              {formatCurrency(h.currentValue.quantity, h.currentValue.commodity)} ({pct}%)
                            </span>
                          </div>
                        </>
                      );
                    }}
                  />
                }
              />
              <Pie
                data={chartData}
                dataKey="value"
                nameKey="symbol"
                innerRadius="60%"
                outerRadius="80%"
                strokeWidth={2}
                stroke="transparent"
              >
                {chartData.map((_, i) => (
                  <Cell key={i} fill={CHART_COLORS[i % CHART_COLORS.length]} />
                ))}
              </Pie>
            </PieChart>
          </ChartContainer>
          <div className="flex min-w-0 flex-1 flex-col gap-1.5">
            {chartData.map((item, i) => (
              <div key={item.symbol} className="flex items-center gap-2 text-sm">
                <div
                  className="h-2.5 w-2.5 flex-shrink-0 rounded-sm"
                  style={{ backgroundColor: CHART_COLORS[i % CHART_COLORS.length] }}
                />
                <span className="min-w-0 flex-1 font-mono font-semibold">{item.symbol}</span>
                <span className="font-mono tabular-nums">{formatCurrency(item.value.toString(), item.commodity)}</span>
                <span className="w-12 text-right text-xs text-muted-foreground">
                  {item.portfolioPct > 0 ? `${item.portfolioPct.toFixed(1)}%` : ""}
                </span>
              </div>
            ))}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function findSnapshotAtOrBefore(sorted, targetDate) {
  let result = null;
  for (const s of sorted) {
    if (s.date <= targetDate) result = s;
    else break;
  }
  return result;
}

function MultiPeriodReturns({ snapshots }) {
  const items = useMemo(() => {
    if (!snapshots?.length) return [];
    const sorted = [...snapshots].sort((a, b) => a.date.localeCompare(b.date));
    const latest = sorted[sorted.length - 1];
    if (!latest?.totalValue) return [];

    const currentVal = parseFloat(latest.totalValue.quantity);
    const currency = latest.totalValue.commodity;
    const latestDate = new Date(latest.date + "T00:00:00");

    function subMonths(d, n) {
      const r = new Date(d);
      r.setMonth(r.getMonth() - n);
      return r.toISOString().slice(0, 10);
    }

    const periodDefs = [
      { label: "1 Month", date: subMonths(latestDate, 1) },
      { label: "3 Months", date: subMonths(latestDate, 3) },
      { label: "YTD", date: `${latestDate.getFullYear()}-01-01` },
      { label: "All Time", date: sorted[0].date },
    ];

    return periodDefs.map(({ label, date }) => {
      const snap = findSnapshotAtOrBefore(sorted, date);
      if (!snap?.totalValue || snap.date === latest.date) return null;
      const startVal = parseFloat(snap.totalValue.quantity);
      if (startVal === 0) return null;
      const gain = currentVal - startVal;
      const pct = (gain / startVal) * 100;
      const positive = gain >= 0;
      const sign = positive ? "+" : "";
      return {
        label,
        value: `${sign}${formatCurrency(gain.toFixed(2), currency)}`,
        percentage: `${sign}${pct.toFixed(1)}%`,
        positive,
      };
    }).filter(Boolean);
  }, [snapshots]);

  if (!items.length) return null;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Returns</CardTitle>
      </CardHeader>
      <CardContent>
        <Stats15 items={items} />
      </CardContent>
    </Card>
  );
}

function PortfolioChart({ snapshots }) {
  const hasCostBasis = useMemo(
    () => snapshots?.some((s) => s.costBasis),
    [snapshots]
  );

  if (!snapshots || snapshots.length < 2) {
    return (
      <Card className="h-full">
        <CardHeader>
          <CardTitle>Portfolio Value Over Time</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">Not enough snapshots to chart portfolio value.</p>
        </CardContent>
      </Card>
    );
  }

  const chartData = snapshots.map((s) => ({
    date: formatLabel(s.date),
    marketValue: s.totalValue ? parseFloat(s.totalValue.quantity) : null,
    ...(hasCostBasis && { costBasis: s.costBasis ? parseFloat(s.costBasis.quantity) : null }),
  }));

  const chartConfig = {
    marketValue: { label: "Market Value", color: "var(--chart-1)" },
    ...(hasCostBasis && { costBasis: { label: "Cost Basis", color: "var(--chart-2)" } }),
  };

  return (
    <Card className="h-full">
      <CardHeader>
        <CardTitle>Portfolio Value Over Time</CardTitle>
      </CardHeader>
      <CardContent>
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
              tickFormatter={(v) => formatCurrency(v?.toString(), "USD")}
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
                        <span className="font-mono font-medium tabular-nums">{formatCurrency(value?.toString(), "USD")}</span>
                      </div>
                    </>
                  )}
                />
              }
            />
            <ChartLegend content={<ChartLegendContent />} />
            <Line
              dataKey="marketValue"
              type="monotone"
              stroke="var(--color-marketValue)"
              strokeWidth={2}
              dot={false}
            />
            {hasCostBasis && (
              <Line
                dataKey="costBasis"
                type="monotone"
                stroke="var(--color-costBasis)"
                strokeWidth={2}
                dot={false}
                strokeDasharray="5 3"
              />
            )}
          </LineChart>
        </ChartContainer>
      </CardContent>
    </Card>
  );
}

const columnHelper = createColumnHelper();

function gainClass(val) {
  if (val === null || val === undefined || isNaN(val)) return "";
  return val >= 0 ? "text-success" : "text-destructive";
}

const columns = [
  columnHelper.accessor("symbol", {
    header: ({ column }) => (
      <SortableHeader column={column}>Symbol</SortableHeader>
    ),
    cell: (info) => (
      <span className="font-mono font-semibold">{info.getValue()}</span>
    ),
    sortingFn: "alphanumeric",
  }),
  columnHelper.accessor("account", {
    header: ({ column }) => (
      <SortableHeader column={column}>Account</SortableHeader>
    ),
    cell: (info) => (
      <span className="font-mono text-xs text-muted-foreground">{info.getValue()}</span>
    ),
    sortingFn: "alphanumeric",
  }),
  columnHelper.accessor("quantity", {
    header: ({ column }) => (
      <SortableHeader column={column} className="ml-auto">Quantity</SortableHeader>
    ),
    cell: (info) => (
      <span className="block text-right font-mono">{info.getValue()}</span>
    ),
    sortingFn: (a, b) => parseFloat(a.original.quantity) - parseFloat(b.original.quantity),
  }),
  columnHelper.accessor("latestPrice", {
    header: ({ column }) => (
      <SortableHeader column={column} className="ml-auto">Latest Price</SortableHeader>
    ),
    cell: (info) => {
      const v = info.getValue();
      return (
        <span className="block text-right font-mono">
          {v ? formatCurrency(v.quantity, v.commodity) : <span className="text-muted-foreground">—</span>}
        </span>
      );
    },
    sortingFn: (a, b) => {
      const av = a.original.latestPrice ? parseFloat(a.original.latestPrice.quantity) : -Infinity;
      const bv = b.original.latestPrice ? parseFloat(b.original.latestPrice.quantity) : -Infinity;
      return av - bv;
    },
  }),
  columnHelper.accessor("priceDate", {
    header: ({ column }) => (
      <SortableHeader column={column} className="ml-auto">Price Date</SortableHeader>
    ),
    cell: (info) => (
      <span className="block text-right font-mono text-xs text-muted-foreground">
        {info.getValue() || "—"}
      </span>
    ),
    sortingFn: "alphanumeric",
  }),
  columnHelper.accessor("currentValue", {
    header: ({ column }) => (
      <SortableHeader column={column} className="ml-auto">Current Value</SortableHeader>
    ),
    cell: (info) => {
      const v = info.getValue();
      return (
        <span className={cn("block text-right font-mono font-semibold", !v && "text-muted-foreground")}>
          {v ? formatCurrency(v.quantity, v.commodity) : "—"}
        </span>
      );
    },
    sortingFn: (a, b) => {
      const av = a.original.currentValue ? parseFloat(a.original.currentValue.quantity) : -Infinity;
      const bv = b.original.currentValue ? parseFloat(b.original.currentValue.quantity) : -Infinity;
      return av - bv;
    },
  }),
  columnHelper.accessor("bookValue", {
    header: ({ column }) => (
      <SortableHeader column={column} className="ml-auto">Book Value</SortableHeader>
    ),
    cell: (info) => {
      const v = info.getValue();
      return (
        <span className="block text-right font-mono text-muted-foreground">
          {v ? formatCurrency(v.quantity, v.commodity) : "—"}
        </span>
      );
    },
    sortingFn: (a, b) => {
      const av = a.original.bookValue ? parseFloat(a.original.bookValue.quantity) : -Infinity;
      const bv = b.original.bookValue ? parseFloat(b.original.bookValue.quantity) : -Infinity;
      return av - bv;
    },
  }),
  columnHelper.accessor("unrealizedGain", {
    header: ({ column }) => (
      <SortableHeader column={column} className="ml-auto">Unrealized Gain</SortableHeader>
    ),
    cell: (info) => {
      const v = info.getValue();
      const num = v ? parseFloat(v.quantity) : null;
      return (
        <span className={cn("block text-right font-mono", gainClass(num))}>
          {v ? formatCurrency(v.quantity, v.commodity) : "—"}
        </span>
      );
    },
    sortingFn: (a, b) => {
      const av = a.original.unrealizedGain ? parseFloat(a.original.unrealizedGain.quantity) : -Infinity;
      const bv = b.original.unrealizedGain ? parseFloat(b.original.unrealizedGain.quantity) : -Infinity;
      return av - bv;
    },
  }),
  columnHelper.accessor("unrealizedGainPct", {
    header: ({ column }) => (
      <SortableHeader column={column} className="ml-auto">Gain %</SortableHeader>
    ),
    cell: (info) => {
      const v = info.getValue();
      const hasGain = info.row.original.unrealizedGain !== null && info.row.original.unrealizedGain !== undefined;
      return (
        <span className={cn("block text-right font-mono", gainClass(hasGain ? v : null))}>
          {hasGain ? `${v >= 0 ? "+" : ""}${v.toFixed(2)}%` : "—"}
        </span>
      );
    },
    sortingFn: (a, b) => {
      const hasA = a.original.unrealizedGain !== null && a.original.unrealizedGain !== undefined;
      const hasB = b.original.unrealizedGain !== null && b.original.unrealizedGain !== undefined;
      const av = hasA ? (a.original.unrealizedGainPct ?? 0) : -Infinity;
      const bv = hasB ? (b.original.unrealizedGainPct ?? 0) : -Infinity;
      return av - bv;
    },
  }),
  columnHelper.accessor("portfolioPct", {
    header: ({ column }) => (
      <SortableHeader column={column} className="ml-auto">% of Portfolio</SortableHeader>
    ),
    cell: (info) => {
      const v = info.getValue();
      return (
        <span className="block text-right font-mono text-muted-foreground">
          {v > 0 ? `${v.toFixed(1)}%` : "—"}
        </span>
      );
    },
    sortingFn: "basic",
  }),
];

export function PortfolioPage() {
  const [accountPrefix, setAccountPrefix] = useState("");
  const [sorting, setSorting] = useState([{ id: "currentValue", desc: true }]);

  const { data, isLoading, error } = useQuery({
    queryKey: queryKeys.portfolioHoldings(accountPrefix),
    queryFn: () => ledgerClient.getPortfolioHoldings({ accountPrefix }),
  });

  const { data: tsData } = useQuery({
    queryKey: queryKeys.portfolioTimeseries(accountPrefix),
    queryFn: () => ledgerClient.getPortfolioTimeseries({ accountPrefix }),
  });

  const holdings = data?.holdings ?? [];
  const totalValue = data?.totalValue;
  const asOfDate = data?.asOfDate;
  const snapshots = tsData?.snapshots ?? [];
  const valuedCount = holdings.filter((h) => h.currentValue).length;
  const unvaluedCount = holdings.length - valuedCount;

  const totalGain = useMemo(() => {
    let sum = null;
    let currency = "USD";
    for (const h of holdings) {
      if (h.unrealizedGain) {
        sum = (sum ?? 0) + parseFloat(h.unrealizedGain.quantity);
        currency = h.unrealizedGain.commodity;
      }
    }
    return sum !== null ? { value: sum, currency } : null;
  }, [holdings]);

  const table = useReactTable({
    data: holdings,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  });

  return (
    <Page>
      <PageHeader title="Investment Portfolio" description="Holdings, allocation, performance, and valuation coverage.">
        <div className="flex items-end gap-2">
          <Label htmlFor="account-prefix" className="whitespace-nowrap text-xs text-muted-foreground">
            Account prefix
          </Label>
          <Input
            id="account-prefix"
            className="h-8 w-48 text-sm font-mono"
            placeholder="assets"
            value={accountPrefix}
            onChange={(e) => setAccountPrefix(e.target.value)}
          />
        </div>
      </PageHeader>

      {isLoading && <Loading />}
      {error && <ErrorBanner error={error} />}

      {data && holdings.length === 0 && (
        <Card className="h-full">
          <CardContent className="pt-6">
            <p className="text-sm text-muted-foreground">
              No non-currency commodity holdings found under{" "}
              <code>{accountPrefix || "assets"}</code>.
              Record investment transactions with commodity amounts (e.g.{" "}
              <code>10 AAPL @ 175 USD</code>) to see your portfolio here.
            </p>
          </CardContent>
        </Card>
      )}

      {data && holdings.length > 0 && (
        <DashboardGrid>
          <div className="col-span-12 md:col-span-4">
            <MetricCard
              title="Total Value"
              value={
                totalValue
                  ? formatCurrency(totalValue.quantity, totalValue.commodity)
                  : "—"
              }
              description={asOfDate ? `As of ${asOfDate}` : undefined}
            />
          </div>
          <div className="col-span-12 md:col-span-4">
            <MetricCard
              title="Holdings"
              value={holdings.length}
              description={unvaluedCount > 0 ? `${unvaluedCount} without price data` : "All holdings valued"}
            />
          </div>
          <div className="col-span-12 md:col-span-4">
            <MetricCard
              title="Unrealized Gain"
              value={totalGain !== null ? formatCurrency(totalGain.value.toString(), totalGain.currency) : "—"}
              valueClassName={totalGain !== null ? gainClass(totalGain.value) : undefined}
              description={totalGain !== null ? (totalGain.value >= 0 ? "Total gain" : "Total loss") : "No cost basis data"}
            />
          </div>

          {unvaluedCount > 0 && (
            <Card className="col-span-12 border-destructive/30 bg-destructive/5">
              <CardContent className="py-4 text-sm text-destructive">
                {unvaluedCount} holding{unvaluedCount > 1 ? "s" : ""} missing price data.{" "}
                <Link to="/prices" className="underline">Add prices →</Link>
              </CardContent>
            </Card>
          )}

          <div className="col-span-12 xl:col-span-8">
            <PortfolioChart snapshots={snapshots} />
          </div>

          <div className="col-span-12 xl:col-span-4 flex flex-col gap-6">
            <MultiPeriodReturns snapshots={snapshots} />
            <AllocationChart holdings={holdings} />
          </div>

          <Card className="col-span-12 pb-0">
            <CardHeader>
              <CardTitle>Holdings</CardTitle>
            </CardHeader>
            <CardContent className="overflow-x-auto p-0">
              <Table>
                <TableHeader>
                  {table.getHeaderGroups().map((headerGroup) => (
                    <TableRow key={headerGroup.id}>
                      {headerGroup.headers.map((header) => (
                        <TableHead key={header.id}>
                          {header.isPlaceholder
                            ? null
                            : flexRender(header.column.columnDef.header, header.getContext())}
                        </TableHead>
                      ))}
                    </TableRow>
                  ))}
                </TableHeader>
                <TableBody>
                  {table.getRowModel().rows.map((row) => (
                    <TableRow key={row.id}>
                      {row.getVisibleCells().map((cell) => (
                        <TableCell key={cell.id}>
                          {flexRender(cell.column.columnDef.cell, cell.getContext())}
                        </TableCell>
                      ))}
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </DashboardGrid>
      )}
    </Page>
  );
}
