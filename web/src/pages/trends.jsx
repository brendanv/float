import { useState, useEffect, useRef } from "preact/hooks";
import { Chart, LineController, LineElement, PointElement, LinearScale, CategoryScale, Filler, Tooltip, Legend } from "chart.js";
import { ledgerClient } from "../client.js";
import { useRpc } from "../hooks/use-rpc.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";

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
    <div class="stat">
      <div class="stat-title">{title}</div>
      <div class={`stat-value text-2xl ${valueClass || ""}`}>{value}</div>
      {desc && <div class="stat-desc">{desc}</div>}
    </div>
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
    <div class="trends-chart" style={{ position: "relative", height: "320px" }}>
      <canvas ref={canvasRef} />
    </div>
  );
}

export function TrendsPage() {
  const [rangeIdx, setRangeIdx] = useState(0);
  const range = RANGES[rangeIdx];
  const begin = toBeginDate(range.months);
  const end = "";

  const timeseries = useRpc(
    () => ledgerClient.getNetWorthTimeseries({ begin, end }),
    [begin]
  );

  const snapshots = timeseries.data?.snapshots || [];
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
    <div class="space-y-6">
      <div class="flex items-center justify-between">
        <h2 class="text-xl font-bold">Trends</h2>
        <div class="join">
          {RANGES.map((r, i) => (
            <button
              key={r.label}
              class={`join-item btn btn-sm ${rangeIdx === i ? "btn-primary" : "btn-ghost"}`}
              onClick={() => setRangeIdx(i)}
            >
              {r.label}
            </button>
          ))}
        </div>
      </div>

      {timeseries.loading && <Loading />}
      {timeseries.error && <ErrorBanner error={timeseries.error} />}

      {!timeseries.loading && !timeseries.error && (
        <>
          <div class="stats stats-horizontal shadow w-full">
            <StatCard
              title="Current Net Worth"
              value={currentNetWorth !== null ? formatCurrency(currentNetWorth) : "—"}
              valueClass={currentNetWorth !== null && currentNetWorth >= 0 ? "text-success" : "text-error"}
            />
            <StatCard
              title="Change This Month"
              value={monthChange !== null ? formatCurrency(monthChange) : "—"}
              valueClass={monthChange !== null && monthChange >= 0 ? "text-success" : "text-error"}
              desc={monthChange !== null && monthChange >= 0 ? "▲ vs last month" : "▼ vs last month"}
            />
            <StatCard
              title="YTD Change"
              value={ytdChange !== null ? formatCurrency(ytdChange) : "—"}
              valueClass={ytdChange !== null && ytdChange >= 0 ? "text-success" : "text-error"}
              desc={ytdChange !== null && ytdChange >= 0 ? "▲ since Jan 1" : "▼ since Jan 1"}
            />
          </div>

          <div class="card bg-base-100 shadow-sm">
            <div class="card-body p-4">
              {snapshots.length === 0 ? (
                <p class="text-base-content/60 text-sm">No data available for this period.</p>
              ) : (
                <NetWorthChart snapshots={snapshots} />
              )}
            </div>
          </div>
        </>
      )}
    </div>
  );
}
