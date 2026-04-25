import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { DateRangePicker } from "../components/search-controls.jsx";
import { DATE_PRESETS } from "../components/search-presets.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";

const MONTHS = ["Jan","Feb","Mar","Apr","May","Jun","Jul","Aug","Sep","Oct","Nov","Dec"];

function formatPeriodHeader(dateStr) {
  if (!dateStr) return "";
  const [year, month] = dateStr.split("-");
  return `${MONTHS[parseInt(month, 10) - 1]} '${year.slice(2)}`;
}

function parseFirstAmount(amountList) {
  if (!amountList?.amounts?.length) return null;
  const q = amountList.amounts[0]?.quantity;
  return q ? parseFloat(q) : null;
}

function formatAmount(value, negate = false) {
  if (value === null || value === undefined) return "—";
  const v = negate ? -value : value;
  const abs = Math.abs(v);
  const str = abs.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  return (v < 0 ? "-$" : "$") + str;
}

function sumPeriods(perPeriodAmounts, negate) {
  if (!perPeriodAmounts?.length) return null;
  let total = 0;
  let hasAny = false;
  for (const al of perPeriodAmounts) {
    const v = parseFirstAmount(al);
    if (v !== null) { total += v; hasAny = true; }
  }
  if (!hasAny) return null;
  return negate ? -total : total;
}

function AmountCell({ value, className }) {
  if (value === null || value === undefined) {
    return <td className={cn("px-3 py-1.5 text-right font-mono text-sm text-muted-foreground", className)}>—</td>;
  }
  const isNeg = value < 0;
  return (
    <td className={cn("px-3 py-1.5 text-right font-mono text-sm", isNeg ? "text-red-600 dark:text-red-400" : "text-green-700 dark:text-green-400", className)}>
      {formatAmount(value)}
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

function AccountRow({ row, periods, isRevenue }) {
  const negate = isRevenue;
  const indent = (row.indent ?? 0) * 1.25;

  return (
    <tr className={cn("border-b border-border/40 hover:bg-muted/30", row.isTotal && "font-semibold bg-muted/20")}>
      <td
        className="sticky left-0 z-10 bg-background px-3 py-1.5 text-sm"
        style={{ paddingLeft: row.isTotal ? "0.75rem" : `${indent + 0.75}rem`, minWidth: "200px" }}
      >
        <span className={cn(row.isTotal && "text-foreground", !row.isTotal && "text-foreground/90")}>
          {row.displayName || row.fullName}
        </span>
      </td>
      {periods.map((_, i) => {
        const al = row.perPeriodAmounts?.[i];
        const v = parseFirstAmount(al);
        const display = v !== null ? (negate ? -v : v) : null;
        return <AmountCell key={i} value={display} />;
      })}
      <AmountCell
        value={row.isTotal
          ? sumPeriods(row.perPeriodAmounts, negate)
          : (() => { const v = parseFirstAmount({ amounts: row.totalAmounts }); return v !== null ? (negate ? -v : v) : null; })()
        }
        className="border-l border-border/40"
      />
    </tr>
  );
}

function NetIncomeRow({ periods, netAmounts }) {
  const periodValues = periods.map((_, i) => parseFirstAmount(netAmounts?.[i]) ?? null);
  const total = periodValues.reduce((sum, v) => (v !== null ? sum + v : sum), 0);
  const hasAny = periodValues.some((v) => v !== null);

  return (
    <tr className="border-t-2 border-border font-bold bg-muted/30">
      <td
        className="sticky left-0 z-10 bg-muted/30 px-3 py-2 text-sm font-bold"
        style={{ minWidth: "200px" }}
      >
        Net Income
      </td>
      {periods.map((_, i) => (
        <AmountCell key={i} value={periodValues[i]} />
      ))}
      <AmountCell value={hasAny ? total : null} className="border-l border-border/40" />
    </tr>
  );
}

export function MonthlyDashboardPage() {
  const initial = DATE_PRESETS[4].fn(); // "Last 12 months"
  const [dateFrom, setDateFrom] = useState(initial.from);
  const [dateTo, setDateTo] = useState(initial.to);

  const { data, isLoading, error } = useQuery({
    queryKey: queryKeys.incomeStatementTimeseries(dateFrom, dateTo),
    queryFn: () => ledgerClient.getIncomeStatementTimeseries({ begin: dateFrom, end: dateTo }),
  });

  const periods = data?.periods ?? [];
  const rows = data?.rows ?? [];
  const netAmounts = data?.netAmounts ?? [];

  const revenueRows = rows.filter((r) => r.section === "Revenues");
  const expenseRows = rows.filter((r) => r.section === "Expenses");

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between gap-4">
        <h1 className="text-xl font-semibold">Monthly Dashboard</h1>
        <DateRangePicker
          dateFrom={dateFrom}
          dateTo={dateTo}
          onChange={(from, to) => { setDateFrom(from); setDateTo(to); }}
          align="end"
        />
      </div>

      {isLoading && <Loading />}
      {error && <ErrorBanner error={error} />}

      {data && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base font-medium text-muted-foreground">
              Income &amp; Expenses by Account
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            <div className="overflow-x-auto">
              <table className="w-full border-collapse text-sm">
                <thead>
                  <tr className="border-b border-border">
                    <th
                      className="sticky left-0 z-20 bg-background px-3 py-2 text-left text-xs font-medium uppercase tracking-wide text-muted-foreground"
                      style={{ minWidth: "200px" }}
                    >
                      Account
                    </th>
                    {periods.map((p, i) => (
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
                  {revenueRows.length > 0 && (
                    <>
                      <SectionHeaderRow label="Revenues" colCount={periods.length} />
                      {revenueRows.map((row, i) => (
                        <AccountRow key={i} row={row} periods={periods} isRevenue={true} />
                      ))}
                    </>
                  )}
                  {expenseRows.length > 0 && (
                    <>
                      <SectionHeaderRow label="Expenses" colCount={periods.length} />
                      {expenseRows.map((row, i) => (
                        <AccountRow key={i} row={row} periods={periods} isRevenue={false} />
                      ))}
                    </>
                  )}
                  {periods.length > 0 && (
                    <NetIncomeRow periods={periods} netAmounts={netAmounts} />
                  )}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
