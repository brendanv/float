// Pure helpers for the Portfolio page: time ranges, price staleness, and
// contribution-adjusted performance from GetPortfolioTimeseries snapshots.


export const PORTFOLIO_RANGES = [
  { label: "1Y", value: "1y", months: 12 },
  { label: "2Y", value: "2y", months: 24 },
  { label: "5Y", value: "5y", months: 60 },
  { label: "All", value: "all", months: null },
];

export const STALE_PRICE_DAYS = 30;
export const VERY_STALE_PRICE_DAYS = 90;

export function daysSinceDate(dateStr) {
  if (!dateStr) return null;
  const then = new Date(dateStr + "T00:00:00");
  const now = new Date();
  return Math.floor((now - then) / (1000 * 60 * 60 * 24));
}

export function formatLabel(dateStr) {
  if (!dateStr) return "";
  const parts = dateStr.split("-");
  const year = parts[0];
  const month = parseInt(parts[1], 10);
  if (!month) return dateStr;
  const months = [
    "Jan",
    "Feb",
    "Mar",
    "Apr",
    "May",
    "Jun",
    "Jul",
    "Aug",
    "Sep",
    "Oct",
    "Nov",
    "Dec",
  ];
  return `${months[month - 1]} '${year.slice(2)}`;
}

function parseAmountValue(amount) {
  if (!amount) return null;
  const value = parseFloat(amount.quantity);
  return Number.isFinite(value) ? value : null;
}

export function findSnapshotAtOrBefore(sorted, targetDate) {
  let result = null;
  for (const s of sorted) {
    if (s.date <= targetDate) result = s;
    else break;
  }
  return result;
}

export function sortedValuedSnapshots(snapshots) {
  return (snapshots ?? [])
    .filter((s) => s.totalValue && s.costBasis)
    .map((s) => ({
      ...s,
      totalValueNum: parseAmountValue(s.totalValue),
      costBasisNum: parseAmountValue(s.costBasis),
    }))
    .filter((s) => s.totalValueNum !== null && s.costBasisNum !== null)
    .sort((a, b) => a.date.localeCompare(b.date));
}

export function subMonths(date, months) {
  const result = new Date(date);
  result.setMonth(result.getMonth() - months);
  return result.toISOString().slice(0, 10);
}

export function beginDateForRange(rangeValue) {
  const range = PORTFOLIO_RANGES.find((r) => r.value === rangeValue);
  if (!range?.months) return "";
  return subMonths(new Date(), range.months);
}

export function calculateContributionAdjustedPeriod(start, end) {
  if (!start || !end || start.date === end.date) return null;

  const netContributions = end.costBasisNum - start.costBasisNum;
  const marketChange = end.totalValueNum - start.totalValueNum;
  const investmentGain = marketChange - netContributions;

  // Use net invested capital as the return base: beginning value plus positive
  // contributions made during the period. Withdrawals reduce gain but do not
  // shrink the denominator below beginning value. This keeps the metric focused
  // on performance instead of deposit timing without pretending to be an exact
  // daily time-weighted return.
  const returnBase = start.totalValueNum + Math.max(netContributions, 0);
  if (returnBase <= 0) return null;

  return {
    netContributions,
    investmentGain,
    returnPct: (investmentGain / returnBase) * 100,
    currency: end.totalValue.commodity,
  };
}

export function buildPortfolioPerformance(snapshots, holdings) {
  const sorted = sortedValuedSnapshots(snapshots);
  const latest = sorted[sorted.length - 1];

  if (latest) {
    return {
      netContributions: latest.costBasisNum,
      investmentGain: latest.totalValueNum - latest.costBasisNum,
      currency: latest.totalValue.commodity,
      source: "timeseries",
    };
  }

  let currentValue = 0;
  let costBasis = 0;
  let sawValue = false;
  let sawBasis = false;
  let currency = "USD";

  for (const holding of holdings ?? []) {
    const value = parseAmountValue(holding.currentValue);
    if (value !== null) {
      currentValue += value;
      currency = holding.currentValue.commodity;
      sawValue = true;
    }

    const basis = parseAmountValue(holding.bookValue);
    if (basis !== null) {
      costBasis += basis;
      currency = holding.bookValue.commodity;
      sawBasis = true;
    }
  }

  if (!sawValue || !sawBasis) return null;
  return {
    netContributions: costBasis,
    investmentGain: currentValue - costBasis,
    currency,
    source: "holdings",
  };
}

// periodReturns lists the contribution-adjusted gain for 1M, 3M and YTD,
// measured back from the latest valued snapshot. Periods without a start
// snapshot (or a usable return base) are dropped.
export function periodReturns(snapshots) {
  const sorted = sortedValuedSnapshots(snapshots);
  const latest = sorted[sorted.length - 1];
  if (!latest) return [];

  const latestDate = new Date(latest.date + "T00:00:00");
  const periodDefs = [
    { label: "1M", name: "1 month", date: subMonths(latestDate, 1) },
    { label: "3M", name: "3 months", date: subMonths(latestDate, 3) },
    { label: "YTD", name: "Year to date", date: `${latestDate.getFullYear()}-01-01` },
  ];

  return periodDefs
    .map(({ label, name, date }) => {
      const start = findSnapshotAtOrBefore(sorted, date);
      const period = calculateContributionAdjustedPeriod(start, latest);
      return period ? { label, name, ...period } : null;
    })
    .filter(Boolean);
}

// gainTone maps a signed gain to an Amount/DeltaChip tone.
export function gainTone(value) {
  if (value === null || value === undefined || isNaN(value) || value === 0) return "ink";
  return value > 0 ? "inflow" : "below";
}

// formatPct formats a signed percentage ("+12.3%", "-4.0%").
export function formatPct(value, digits = 1) {
  if (value === null || value === undefined || isNaN(value)) return "";
  return `${value > 0 ? "+" : ""}${value.toFixed(digits)}%`;
}
