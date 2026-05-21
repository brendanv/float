import { useState, useEffect, useRef, useMemo } from "react";
import { Link } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import {
  Chart,
  ArcElement,
  DoughnutController,
  LineController,
  LineElement,
  PointElement,
  LinearScale,
  CategoryScale,
  Filler,
  Tooltip,
  Legend,
} from "chart.js";
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
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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

Chart.register(
  ArcElement, DoughnutController,
  LineController, LineElement, PointElement,
  LinearScale, CategoryScale, Filler, Tooltip, Legend
);

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

function StatCard({ title, value, sub, valueClass }) {
  return (
    <Card className="flex-1 min-w-[160px]">
      <CardHeader className="pb-1">
        <CardTitle className="text-xs font-normal uppercase tracking-wide text-muted-foreground">
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className={cn("font-mono text-2xl font-semibold", valueClass)}>{value}</div>
        {sub && <div className="mt-1 text-xs text-muted-foreground">{sub}</div>}
      </CardContent>
    </Card>
  );
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
  const canvasRef = useRef(null);
  const chartRef = useRef(null);

  useEffect(() => {
    const valued = holdings.filter((h) => h.currentValue);
    if (!canvasRef.current || valued.length === 0) return;

    if (chartRef.current) chartRef.current.destroy();

    chartRef.current = new Chart(canvasRef.current, {
      type: "doughnut",
      data: {
        labels: valued.map((h) => h.symbol),
        datasets: [
          {
            data: valued.map((h) => parseFloat(h.currentValue.quantity)),
            backgroundColor: valued.map((_, i) => CHART_COLORS[i % CHART_COLORS.length]),
            borderWidth: 2,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: true,
        plugins: {
          legend: { position: "right" },
          tooltip: {
            callbacks: {
              label: (ctx) => {
                const h = valued[ctx.dataIndex];
                const pct = h.portfolioPct?.toFixed(1) ?? "0.0";
                return ` ${formatCurrency(h.currentValue.quantity, h.currentValue.commodity)} (${pct}%)`;
              },
            },
          },
        },
      },
    });

    return () => {
      if (chartRef.current) {
        chartRef.current.destroy();
        chartRef.current = null;
      }
    };
  }, [holdings]);

  const valued = holdings.filter((h) => h.currentValue);
  if (valued.length === 0) return null;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Allocation</CardTitle>
      </CardHeader>
      <CardContent className="flex justify-center">
        <div className="portfolio-chart w-full max-w-sm">
          <canvas ref={canvasRef} />
        </div>
      </CardContent>
    </Card>
  );
}

function PortfolioChart({ snapshots }) {
  const canvasRef = useRef(null);
  const chartRef = useRef(null);

  const hasCostBasis = useMemo(
    () => snapshots?.some((s) => s.costBasis),
    [snapshots]
  );

  useEffect(() => {
    if (!canvasRef.current || !snapshots || snapshots.length < 2) return;

    const labels = snapshots.map((s) => formatLabel(s.date));
    const valueData = snapshots.map((s) =>
      s.totalValue ? parseFloat(s.totalValue.quantity) : null
    );
    const costData = snapshots.map((s) =>
      s.costBasis ? parseFloat(s.costBasis.quantity) : null
    );

    if (chartRef.current) chartRef.current.destroy();

    const datasets = [
      {
        label: "Market Value",
        data: valueData,
        borderColor: "rgba(99,102,241,1)",
        backgroundColor: "rgba(99,102,241,0.1)",
        fill: true,
        tension: 0.3,
        pointRadius: 3,
        spanGaps: true,
      },
    ];

    if (hasCostBasis) {
      datasets.push({
        label: "Cost Basis",
        data: costData,
        borderColor: "rgba(251,146,60,1)",
        backgroundColor: "rgba(251,146,60,0.05)",
        fill: false,
        tension: 0.3,
        pointRadius: 3,
        borderDash: [5, 3],
        spanGaps: true,
      });
    }

    chartRef.current = new Chart(canvasRef.current, {
      type: "line",
      data: { labels, datasets },
      options: {
        responsive: true,
        maintainAspectRatio: true,
        interaction: { mode: "index", intersect: false },
        plugins: {
          legend: { display: hasCostBasis },
          tooltip: {
            callbacks: {
              label: (ctx) =>
                ` ${ctx.dataset.label}: ${formatCurrency(ctx.parsed.y?.toString(), "USD")}`,
            },
          },
        },
        scales: {
          x: { grid: { display: false } },
          y: {
            ticks: {
              callback: (v) => formatCurrency(v?.toString(), "USD"),
            },
          },
        },
      },
    });

    return () => {
      if (chartRef.current) {
        chartRef.current.destroy();
        chartRef.current = null;
      }
    };
  }, [snapshots, hasCostBasis]);

  if (!snapshots || snapshots.length < 2) return null;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Portfolio Value Over Time</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="relative h-64">
          <canvas ref={canvasRef} />
        </div>
      </CardContent>
    </Card>
  );
}

const columnHelper = createColumnHelper();

function gainClass(val) {
  if (val === null || val === undefined || isNaN(val)) return "";
  return val >= 0 ? "text-green-600 dark:text-green-400" : "text-red-600 dark:text-red-400";
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
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
        <h2 className="text-2xl font-bold">Investment Portfolio</h2>
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
      </div>

      {isLoading && <Loading />}
      {error && <ErrorBanner error={error} />}

      {data && holdings.length === 0 && (
        <Card>
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
        <>
          <div className="flex flex-wrap gap-4">
            <StatCard
              title="Total Value"
              value={
                totalValue
                  ? formatCurrency(totalValue.quantity, totalValue.commodity)
                  : "—"
              }
              sub={asOfDate ? `as of ${asOfDate}` : undefined}
            />
            <StatCard
              title="Holdings"
              value={holdings.length}
              sub={unvaluedCount > 0 ? `${unvaluedCount} without price data` : "all valued"}
            />
            {totalGain !== null && (
              <StatCard
                title="Unrealized Gain"
                value={formatCurrency(totalGain.value.toString(), totalGain.currency)}
                valueClass={gainClass(totalGain.value)}
                sub={totalGain.value >= 0 ? "total gain" : "total loss"}
              />
            )}
            {unvaluedCount > 0 && (
              <Card className="flex-1 min-w-[160px] border-amber-300 dark:border-amber-700">
                <CardContent className="pt-6 text-sm text-amber-700 dark:text-amber-400">
                  {unvaluedCount} holding{unvaluedCount > 1 ? "s" : ""} missing price data.{" "}
                  <Link to="/prices" className="underline">Add prices →</Link>
                </CardContent>
              </Card>
            )}
          </div>

          <AllocationChart holdings={holdings} />

          <PortfolioChart snapshots={snapshots} />

          <Card>
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
        </>
      )}
    </div>
  );
}
