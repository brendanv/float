import { useState, useEffect, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { Chart, LineController, LineElement, PointElement, LinearScale, CategoryScale, Filler, Tooltip, Legend } from "chart.js";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

Chart.register(LineController, LineElement, PointElement, LinearScale, CategoryScale, Filler, Tooltip, Legend);

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

function formatCurrency(value) {
  if (value === null || value === undefined) return "";
  const abs = Math.abs(value);
  const formatted = abs.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  return (value < 0 ? "-$" : "$") + formatted;
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
  const canvasRef = useRef(null);
  const chartRef = useRef(null);

  useEffect(() => {
    if (!canvasRef.current || !snapshots || snapshots.length === 0) return;

    const labels = snapshots.map((s) => formatLabel(s.date));
    const assetsData = snapshots.map((s) => parseAmount(s.assets));
    const liabilitiesData = snapshots.map((s) => Math.abs(parseAmount(s.liabilities)));
    const netWorthData = snapshots.map((s) => parseAmount(s.netWorth));

    if (chartRef.current) {
      chartRef.current.destroy();
    }

    chartRef.current = new Chart(canvasRef.current, {
      type: "line",
      data: {
        labels,
        datasets: [
          {
            label: "Net Worth",
            data: netWorthData,
            borderColor: "rgba(99,102,241,1)",
            backgroundColor: "rgba(99,102,241,0.1)",
            fill: true,
            tension: 0.3,
            pointRadius: 3,
          },
          {
            label: "Assets",
            data: assetsData,
            borderColor: "rgba(34,197,94,1)",
            backgroundColor: "rgba(34,197,94,0.05)",
            fill: false,
            tension: 0.3,
            pointRadius: 3,
          },
          {
            label: "Liabilities",
            data: liabilitiesData,
            borderColor: "rgba(239,68,68,1)",
            backgroundColor: "rgba(239,68,68,0.05)",
            fill: false,
            tension: 0.3,
            pointRadius: 3,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: true,
        interaction: { mode: "index", intersect: false },
        plugins: {
          legend: { position: "top" },
          tooltip: {
            callbacks: {
              label: (ctx) => ` ${ctx.dataset.label}: ${formatCurrency(ctx.parsed.y)}`,
            },
          },
        },
        scales: {
          x: { grid: { display: false } },
          y: {
            ticks: {
              callback: (v) => formatCurrency(v),
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
  }, [snapshots]);

  return (
    <div className="trends-chart relative h-80">
      <canvas ref={canvasRef} />
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

  const snapshots = timeseriesData?.snapshots || [];
  const latest = snapshots[snapshots.length - 1];
  const prev = snapshots[snapshots.length - 2];

  const currentNetWorth = latest ? parseAmount(latest.netWorth) : null;
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
              value={currentNetWorth !== null ? formatCurrency(currentNetWorth) : "—"}
              valueClass={currentNetWorth !== null && currentNetWorth >= 0 ? "text-success" : "text-destructive"}
            />
            <StatCard
              title="Change This Month"
              value={monthChange !== null ? formatCurrency(monthChange) : "—"}
              valueClass={monthChange !== null && monthChange >= 0 ? "text-success" : "text-destructive"}
              desc={monthChange !== null && monthChange >= 0 ? "▲ vs last month" : "▼ vs last month"}
            />
            <StatCard
              title="YTD Change"
              value={ytdChange !== null ? formatCurrency(ytdChange) : "—"}
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
