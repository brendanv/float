import { useEffect, useRef } from "react";
import { Link } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import {
  Chart,
  ArcElement,
  DoughnutController,
  Tooltip,
  Legend,
} from "chart.js";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
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
import { cn } from "@/lib/utils";

Chart.register(ArcElement, DoughnutController, Tooltip, Legend);

const CHART_COLORS = [
  "#6366f1", "#22d3ee", "#f59e0b", "#34d399", "#f43f5e",
  "#a78bfa", "#38bdf8", "#fb923c", "#4ade80", "#e879f9",
];

function formatCurrency(quantity, commodity) {
  if (quantity === undefined || quantity === null) return "—";
  const val = parseFloat(quantity);
  if (isNaN(val)) return "—";
  const sym = commodity === "USD" ? "$" : (commodity ?? "");
  const abs = Math.abs(val).toLocaleString("en-US", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
  return (val < 0 ? "-" : "") + sym + abs;
}

function StatCard({ title, value, sub }) {
  return (
    <Card className="flex-1 min-w-[160px]">
      <CardHeader className="pb-1">
        <CardTitle className="text-xs font-normal uppercase tracking-wide text-muted-foreground">
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="font-mono text-2xl font-semibold">{value}</div>
        {sub && <div className="mt-1 text-xs text-muted-foreground">{sub}</div>}
      </CardContent>
    </Card>
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

export function PortfolioPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: queryKeys.portfolioHoldings(),
    queryFn: () => ledgerClient.getPortfolioHoldings({}),
  });

  const holdings = data?.holdings ?? [];
  const totalValue = data?.totalValue;
  const asOfDate = data?.asOfDate;
  const valuedCount = holdings.filter((h) => h.currentValue).length;
  const unvaluedCount = holdings.length - valuedCount;

  return (
    <div className="flex flex-col gap-6">
      <h2 className="text-2xl font-bold">Investment Portfolio</h2>

      {isLoading && <Loading />}
      {error && <ErrorBanner error={error} />}

      {data && holdings.length === 0 && (
        <Card>
          <CardContent className="pt-6">
            <p className="text-sm text-muted-foreground">
              No non-currency commodity holdings found under <code>assets</code>.
              Record investment transactions with commodity amounts (e.g.{" "}
              <code>10 AAPL</code>) to see your portfolio here.
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

          <Card>
            <CardHeader>
              <CardTitle>Holdings</CardTitle>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Symbol</TableHead>
                    <TableHead>Account</TableHead>
                    <TableHead className="text-right">Quantity</TableHead>
                    <TableHead className="text-right">Latest Price</TableHead>
                    <TableHead className="text-right">Price Date</TableHead>
                    <TableHead className="text-right">Current Value</TableHead>
                    <TableHead className="text-right">% of Portfolio</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {holdings.map((h, i) => (
                    <TableRow key={`${h.account}-${h.symbol}-${i}`}>
                      <TableCell className="font-mono font-semibold">{h.symbol}</TableCell>
                      <TableCell className="font-mono text-xs text-muted-foreground">{h.account}</TableCell>
                      <TableCell className="text-right font-mono">{h.quantity}</TableCell>
                      <TableCell className="text-right font-mono">
                        {h.latestPrice
                          ? formatCurrency(h.latestPrice.quantity, h.latestPrice.commodity)
                          : <span className="text-muted-foreground">—</span>}
                      </TableCell>
                      <TableCell className="text-right font-mono text-xs text-muted-foreground">
                        {h.priceDate || <span>—</span>}
                      </TableCell>
                      <TableCell className={cn("text-right font-mono font-semibold", h.currentValue ? "" : "text-muted-foreground")}>
                        {h.currentValue
                          ? formatCurrency(h.currentValue.quantity, h.currentValue.commodity)
                          : "—"}
                      </TableCell>
                      <TableCell className="text-right font-mono text-muted-foreground">
                        {h.portfolioPct > 0
                          ? `${h.portfolioPct.toFixed(1)}%`
                          : "—"}
                      </TableCell>
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
