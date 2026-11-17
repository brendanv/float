import { Fragment, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { DateRangePicker } from "../components/search-controls.jsx";
import { DATE_PRESETS } from "../components/search-presets.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { Amount } from "../components/amount.jsx";
import { Account } from "../components/account.jsx";
import { Page } from "../components/page.jsx";
import { Segmented } from "../components/segmented.jsx";
import { accountType } from "@/lib/accounts.js";
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

const SECTION_LABELS = { Revenues: "Income", Expenses: "Expenses" };

// Heat hue per section: income arriving is inflow teal, spending is accent.
const SECTION_HUE = { Revenues: "var(--inflow)", Expenses: "var(--primary)" };

const PRECISION_OPTIONS = [
  { value: "whole", label: "$1,234" },
  { value: "cents", label: "$1,234.56" },
];

const CELL = "whitespace-nowrap px-2.5 py-[7px] text-right";
// The account column keeps a hairline on its right edge, like the pinned
// totals, so scrolled months slide under a visible edge.
const STICKY = "sticky left-0 z-10 bg-background shadow-[inset_-1px_0_0_var(--border)]";
// Avg and Total stay pinned on the right so a year of months scrolls
// between the account column and the totals. Sticky cells lose
// border-collapse borders, so their left rule is an inset shadow; Total has
// a fixed width so Avg can sit right next to it.
// On phones there's no room to pin them; they scroll with the months.
const PIN_TOTAL = "md:sticky md:right-0 md:z-10 md:w-28 md:min-w-28 bg-background shadow-[inset_1px_0_0_var(--border)]";
const PIN_AVG = "md:sticky md:right-28 md:z-10 bg-background shadow-[inset_1px_0_0_var(--border)]";
const AVG = cn(PIN_AVG, "text-faint");

function average(values, count) {
  if (!count || values.every((v) => v === null)) return null;
  return values.reduce((sum, v) => sum + (v ?? 0), 0) / count;
}

function total(values) {
  return values.some((v) => v !== null) ? values.reduce((sum, v) => sum + (v ?? 0), 0) : null;
}

// heatStyle shades a cell by its size within its row (empty periods count
// as zero): 4% of the section hue for the row's smallest value up to 24%
// for its largest. Flat rows and zero/negative cells stay unshaded.
function heatStyle(value, row, hue) {
  if (!hue || value === null || value <= 0) return undefined;
  const values = row.map((v) => v ?? 0);
  const max = Math.max(...values);
  const min = Math.min(...values);
  if (max === min) return undefined;
  const pct = Math.round(4 + ((value - min) / (max - min)) * 20);
  return { backgroundColor: `color-mix(in oklch, ${hue} ${pct}%, transparent)` };
}

function MoneyCell({ value, whole, className, style, tone, signed }) {
  return (
    <td className={cn(CELL, className)} style={style}>
      {value === null || value === undefined ? (
        <span className="text-faint">–</span>
      ) : (
        <Amount value={value} commodity="USD" whole={whole} tone={tone} signed={signed} />
      )}
    </td>
  );
}

function SectionHeaderRow({ section, colCount }) {
  return (
    <tr data-slot="matrix-section">
      <td className={cn(STICKY, "px-2.5 pt-[18px] pb-1.5 text-label text-faint uppercase")}>
        {SECTION_LABELS[section] ?? section}
      </td>
      <td colSpan={colCount} />
      <td className={PIN_AVG} />
      <td className={PIN_TOTAL} />
    </tr>
  );
}

function AccountRow({ row, periods, whole }) {
  const values = periods.map((_, i) => parseFirstAmount(row.perPeriodAmounts?.[i]));
  const rowTotal = row.isTotal
    ? sumPeriods(row.perPeriodAmounts, false)
    : parseFirstAmount({ amounts: row.totalAmounts });
  const hue = row.isTotal ? null : SECTION_HUE[row.section];
  const label = SECTION_LABELS[row.section] ?? row.section;

  return (
    <tr
      data-slot={row.isTotal ? "matrix-total" : "matrix-row"}
      className={cn(
        "group",
        row.isTotal ? "border-t border-input font-semibold" : "border-t border-border",
      )}
    >
      <td
        className={cn(STICKY, "max-w-[14rem] px-2.5 py-[7px] text-left group-hover:bg-muted")}
        style={row.isTotal ? undefined : { paddingLeft: `${0.625 + (row.indent ?? 0) * 1}rem` }}
      >
        {row.isTotal ? (
          `Total ${label.toLowerCase()}`
        ) : (
          <Account
            name={row.displayName || row.fullName}
            variant="path"
            type={accountType(row.fullName)}
            title={row.fullName}
            className="max-w-full"
          />
        )}
      </td>
      {values.map((v, i) => (
        <MoneyCell
          key={i}
          value={v}
          whole={whole}
          style={heatStyle(v, values, hue)}
          className="group-hover:bg-muted"
        />
      ))}
      <MoneyCell value={average(values, periods.length)} whole={whole} className={cn(AVG, "group-hover:bg-muted")} />
      <MoneyCell value={rowTotal} whole={whole} className={cn(PIN_TOTAL, "group-hover:bg-muted", !row.isTotal && "font-medium")} />
    </tr>
  );
}

// KeptRow is net income (income − expenses) per period: inflow when kept,
// below when spending exceeded income.
function KeptRow({ periods, netAmounts, whole }) {
  const values = periods.map((_, i) => parseFirstAmount(netAmounts?.[i]));
  const toneOf = (v) => (v === null ? undefined : v < 0 ? "below" : "inflow");
  const avg = average(values, periods.length);
  const sum = total(values);

  return (
    <tr data-slot="matrix-kept" className="border-t-2 border-foreground font-semibold">
      <td className={cn(STICKY, "px-2.5 py-2 text-left")}>Kept</td>
      {values.map((v, i) => (
        <MoneyCell key={i} value={v} whole={whole} tone={toneOf(v)} signed className="py-2" />
      ))}
      <MoneyCell value={avg} whole={whole} tone={toneOf(avg)} signed className={cn(AVG, "py-2")} />
      <MoneyCell value={sum} whole={whole} tone={toneOf(sum)} signed className={cn(PIN_TOTAL, "py-2")} />
    </tr>
  );
}

const HEAD = "px-2.5 pt-1 pb-1.5 text-right text-label font-medium whitespace-nowrap text-faint uppercase";

export function MonthlyDashboardPage() {
  const initial = DATE_PRESETS[4].fn(); // "Last 12 months"
  const [dateFrom, setDateFrom] = useState(initial.from);
  const [dateTo, setDateTo] = useState(initial.to);
  const [precision, setPrecision] = useState("whole");
  const whole = precision === "whole";

  const { data, isLoading, error } = useQuery({
    queryKey: queryKeys.incomeStatementTimeseries(dateFrom, dateTo),
    queryFn: () => ledgerClient.getIncomeStatementTimeseries({ begin: dateFrom, end: dateTo }),
  });

  const periods = data?.periods ?? [];
  const rows = data?.rows ?? [];
  const netAmounts = data?.netAmounts ?? [];

  const sections = ["Revenues", "Expenses"]
    .map((section) => ({ section, rows: rows.filter((r) => r.section === section) }))
    .filter((s) => s.rows.length > 0);

  return (
    <Page>
      <PageHeader title="Monthly Dashboard">
        <DateRangePicker
          dateFrom={dateFrom}
          dateTo={dateTo}
          onChange={(from, to) => { setDateFrom(from); setDateTo(to); }}
          align="end"
        />
        <Segmented
          label="Precision"
          options={PRECISION_OPTIONS}
          value={precision}
          onChange={setPrecision}
        />
      </PageHeader>

      {isLoading && <Loading />}
      {error && <ErrorBanner error={error} />}

      {data && (
        <div className="overflow-x-auto">
          <table data-slot="report-matrix" className="w-full border-collapse text-body tabular-nums">
            <thead>
              <tr>
                <th className={cn(STICKY, HEAD, "z-20 min-w-[9rem] text-left")}>Account</th>
                {periods.map((p, i) => (
                  <th key={i} className={HEAD}>{formatPeriodHeader(p)}</th>
                ))}
                <th className={cn(HEAD, PIN_AVG)}>Avg</th>
                <th className={cn(HEAD, PIN_TOTAL)}>Total</th>
              </tr>
            </thead>
            <tbody>
              {sections.map(({ section, rows: sectionRows }) => (
                <Fragment key={section}>
                  <SectionHeaderRow section={section} colCount={periods.length} />
                  {sectionRows.map((row, i) => (
                    <AccountRow key={i} row={row} periods={periods} whole={whole} />
                  ))}
                </Fragment>
              ))}
              {periods.length > 0 && (
                <KeptRow periods={periods} netAmounts={netAmounts} whole={whole} />
              )}
            </tbody>
          </table>
        </div>
      )}
    </Page>
  );
}
