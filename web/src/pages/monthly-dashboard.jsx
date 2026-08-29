import { useState, useMemo } from "react";
import { DateRangePicker } from "../components/search-controls.jsx";
import { DATE_PRESETS } from "../components/search-presets.js";
import { formatCurrency } from "../format.js";
import { useCube } from "../hooks/use-cube.js";
import { flowMatrix, flowSeries } from "../lib/cube-query.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

function formatPeriodHeader(period) {
  if (!period) return "";
  const [year, month] = period.split("-");
  return `${MONTHS[parseInt(month, 10) - 1]} '${year.slice(2)}`;
}

function AmountCell({ value, className, invertColor }) {
  if (value === null || value === undefined) {
    return <td className={cn("px-3 py-1.5 text-right font-mono text-sm text-muted-foreground", className)}>—</td>;
  }
  const isRed = invertColor ? value > 0 : value < 0;
  return (
    <td className={cn("px-3 py-1.5 text-right font-mono text-sm", isRed ? "text-red-600 dark:text-red-400" : "text-green-700 dark:text-green-400", className)}>
      {formatCurrency(value, "USD")}
    </td>
  );
}

function SectionHeaderRow({ label, colCount }) {
  return (
    <tr className="bg-muted/50">
      <td
        colSpan={colCount + 2}
        className="sticky left-0 z-10 bg-muted/50 px-3 py-1.5 text-xs font-semibold uppercase tracking-wide text-muted-foreground"
      >
        {label}
      </td>
    </tr>
  );
}

function AccountRow({ row, isRevenue, isTotal }) {
  const invertColor = !isRevenue;
  const indent = (row.depth ?? 0) * 1.25;

  return (
    <tr className={cn("border-b border-border/40 hover:bg-muted/30", isTotal && "font-semibold bg-muted/20")}>
      <td
        className="sticky left-0 z-10 bg-background px-3 py-1.5 text-sm"
        style={{ paddingLeft: isTotal ? "0.75rem" : `${indent + 0.75}rem`, minWidth: "200px" }}
      >
        <span className={cn(isTotal ? "text-foreground" : "text-foreground/90")}>{row.label}</span>
      </td>
      {row.totals.map((v, i) => (
        <AmountCell key={i} value={v} invertColor={invertColor} />
      ))}
      <AmountCell value={row.total} className="border-l border-border/40" invertColor={invertColor} />
    </tr>
  );
}

function NetIncomeRow({ values }) {
  const total = values.reduce((sum, v) => sum + v, 0);
  return (
    <tr className="border-t-2 border-border font-bold bg-muted/30">
      <td className="sticky left-0 z-10 bg-muted/30 px-3 py-2 text-sm font-bold" style={{ minWidth: "200px" }}>
        Net Income
      </td>
      {values.map((v, i) => (
        <AmountCell key={i} value={v} />
      ))}
      <AmountCell value={total} className="border-l border-border/40" />
    </tr>
  );
}

/** Flips the sign of every figure in a matrix row. */
function negateRow(row) {
  return { ...row, totals: row.totals.map((v) => -v), total: -row.total };
}

export function MonthlyDashboardPage() {
  const initial = DATE_PRESETS[4].fn(); // "Last 12 months"
  const [dateFrom, setDateFrom] = useState(initial.from);
  const [dateTo, setDateTo] = useState(initial.to);

  // One payload backs every date range; changing the range re-slices locally.
  const { data: cube, isLoading, error } = useCube();

  const view = useMemo(() => {
    if (!cube) return null;
    const range = { from: dateFrom, to: dateTo };

    // hledger's income statement presents both sections as positive figures
    // with net income as revenues minus expenses. Revenue postings are credits,
    // so their raw sign is flipped for display.
    const revenues = flowMatrix(cube, { ...range, type: "R" });
    const expenses = flowMatrix(cube, { ...range, type: "X" });
    const periods = revenues.periods.length ? revenues.periods : expenses.periods;

    const revenueTotals = flowSeries(cube, { ...range, type: "R" }).map((p) => -p.total);
    const expenseTotals = flowSeries(cube, { ...range, type: "X" }).map((p) => p.total);

    return {
      periods,
      revenueRows: revenues.rows.map(negateRow),
      expenseRows: expenses.rows,
      revenueTotal: { label: "Total Revenues", depth: 0, totals: revenueTotals, total: revenueTotals.reduce((s, v) => s + v, 0) },
      expenseTotal: { label: "Total Expenses", depth: 0, totals: expenseTotals, total: expenseTotals.reduce((s, v) => s + v, 0) },
      netIncome: periods.map((_, i) => (revenueTotals[i] ?? 0) - (expenseTotals[i] ?? 0)),
    };
  }, [cube, dateFrom, dateTo]);

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title="Monthly Dashboard">
        <DateRangePicker
          dateFrom={dateFrom}
          dateTo={dateTo}
          onChange={(from, to) => {
            setDateFrom(from);
            setDateTo(to);
          }}
          align="end"
        />
      </PageHeader>

      {isLoading && <Loading />}
      {error && <ErrorBanner error={error} />}

      {view && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base font-medium text-muted-foreground">
              Income &amp; Expenses by Account
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            <div className="overflow-x-auto">
              <table className="border-collapse text-sm">
                <thead>
                  <tr className="border-b border-border">
                    <th
                      className="sticky left-0 z-20 bg-background px-3 py-2 text-left text-xs font-medium uppercase tracking-wide text-muted-foreground"
                      style={{ minWidth: "200px" }}
                    >
                      Account
                    </th>
                    {view.periods.map((p, i) => (
                      <th key={i} className="px-3 py-2 text-right text-xs font-medium uppercase tracking-wide text-muted-foreground whitespace-nowrap">
                        {formatPeriodHeader(p)}
                      </th>
                    ))}
                    <th className="border-l border-border/40 px-3 py-2 text-right text-xs font-medium uppercase tracking-wide text-muted-foreground whitespace-nowrap">
                      Total
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {view.revenueRows.length > 0 && (
                    <>
                      <SectionHeaderRow label="Revenues" colCount={view.periods.length} />
                      {view.revenueRows.map((row) => (
                        <AccountRow key={row.account} row={row} isRevenue />
                      ))}
                      <AccountRow row={view.revenueTotal} isRevenue isTotal />
                    </>
                  )}
                  {view.expenseRows.length > 0 && (
                    <>
                      <SectionHeaderRow label="Expenses" colCount={view.periods.length} />
                      {view.expenseRows.map((row) => (
                        <AccountRow key={row.account} row={row} isRevenue={false} />
                      ))}
                      <AccountRow row={view.expenseTotal} isRevenue={false} isTotal />
                    </>
                  )}
                  {view.periods.length > 0 && <NetIncomeRow values={view.netIncome} />}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
