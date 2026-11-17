import { ledgerClient } from "@/client.js";
import { queryKeys } from "@/query-keys.js";
import { monthName } from "@/format.js";

// Shared by Overview and Reports › Net worth so both read the same
// GetNetWorthTimeseries query (and cache entry) for the default 12 months.

// timeseriesBegin returns the begin date for the last `months` months, or ""
// for all history.
export function timeseriesBegin(months) {
  if (!months) return "";
  const d = new Date();
  d.setMonth(d.getMonth() - months);
  return d.toISOString().slice(0, 10);
}

// netWorthTimeseriesQuery is the React Query options object for
// GetNetWorthTimeseries from `begin` to now.
export function netWorthTimeseriesQuery(begin) {
  return {
    queryKey: queryKeys.netWorthTimeseries(begin),
    queryFn: () => ledgerClient.getNetWorthTimeseries({ begin, end: "" }),
  };
}

// firstQuantity reads the first amount of a single-commodity (valued) list.
export function firstQuantity(amounts) {
  if (!amounts || amounts.length === 0) return 0;
  return parseFloat(amounts[0].quantity) || 0;
}

// netWorthChanges compares the current net worth with the previous monthly
// snapshot and with the first snapshot of this year. Values are null when
// unknown. `prevMonth` is the month name of the previous snapshot.
export function netWorthChanges(snapshots, current) {
  const list = snapshots || [];
  const prev = list[list.length - 2];
  const year = new Date().getFullYear().toString();
  const firstThisYear = list.find((s) => s.date && s.date.startsWith(year));
  const known = current !== null && current !== undefined;
  return {
    monthChange: known && prev ? current - firstQuantity(prev.netWorth) : null,
    prevMonth: prev?.date ? monthName(parseInt(prev.date.slice(5, 7), 10)) : "",
    ytdChange: known && firstThisYear ? current - firstQuantity(firstThisYear.netWorth) : null,
  };
}

const SHORT_MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

// shortMonthLabel formats "2025-04-01" as "Apr ’25".
export function shortMonthLabel(dateStr) {
  if (!dateStr) return "";
  const [year, month] = dateStr.split("-");
  return `${SHORT_MONTHS[parseInt(month, 10) - 1]} ’${year.slice(2)}`;
}

// deltaTone maps a change to a DeltaChip tone: up is inflow, down is below.
export function deltaTone(change) {
  if (change === null || change === undefined || change === 0) return "ink";
  return change > 0 ? "inflow" : "below";
}
