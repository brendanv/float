// Client-side query engine over the decoded cube.
//
// Every function here is pure and synchronous. A full scan of ~48,000 postings
// grouped by month measures ~0.14ms, so these run inside useMemo on every
// filter change rather than behind a network round trip.
//
// # The rule these functions enforce
//
// Flows sum over time; stocks do not. Posting amounts are distributive and may
// be reduced over any date range and account subtree. Market value and cost
// basis are materialized per period end: they may be rolled up the account tree
// at a fixed period, but summing them across periods is meaningless. The wire
// format tags each measure column, and assertSummableOverTime throws rather
// than letting a caller quietly produce a wrong number.

import { toMajor } from "./cube.js";

const MS_PER_DAY = 24 * 60 * 60 * 1000;

/**
 * Throws if a measure column may not be summed across periods.
 *
 * This is the runtime half of the flows/stocks rule. The other half is that no
 * function in this module offers a range form for a stock measure.
 */
export function assertSummableOverTime(cube, tableName, columnName) {
  const summable = cube.tables[tableName]?.summable?.[columnName];
  if (summable !== "both") {
    throw new Error(
      `${tableName}.${columnName} is "${summable ?? "unknown"}" and cannot be summed across periods — ` +
        `market value and cost basis depend on the price series, not on a sum of deltas`,
    );
  }
}

/** Parses "YYYY-MM-DD" (or a Date) to a UTC-midnight timestamp. */
function utcDay(value) {
  if (value == null || value === "") return null;
  if (value instanceof Date) {
    return Date.UTC(value.getUTCFullYear(), value.getUTCMonth(), value.getUTCDate());
  }
  const [y, m, d] = String(value).split("-").map(Number);
  if (!y || !m || !d) return null;
  return Date.UTC(y, m - 1, d);
}

/** Converts a date to its offset in days from the cube's epoch. */
function dayIndex(cube, value) {
  const day = utcDay(value);
  if (day == null) return null;
  return Math.round((day - utcDay(cube.epochDate)) / MS_PER_DAY);
}

/** Lower bound: first index whose value is >= target. */
function lowerBound(column, length, target) {
  let lo = 0;
  let hi = length;
  while (lo < hi) {
    const mid = (lo + hi) >>> 1;
    if (column[mid] < target) lo = mid + 1;
    else hi = mid;
  }
  return lo;
}

/**
 * Resolves a half-open date interval to the row range [start, end) of the
 * date-sorted posting table.
 *
 * The interval is half-open — `to` is exclusive — matching hledger's
 * `date:FROM..TO`. Treating it as inclusive shifts every month boundary by a
 * day, which is invisible until a transaction lands exactly on one.
 */
export function dateRange(cube, from, to) {
  const table = cube.tables.postings;
  const n = table.rows;
  if (n === 0) return [0, 0];
  const date = table.columns.date;

  let start = 0;
  let end = n;
  if (from) {
    const d = dayIndex(cube, from);
    if (d > 65535) return [0, 0];
    start = lowerBound(date, n, Math.max(d, 0));
  }
  if (to) {
    const d = dayIndex(cube, to);
    if (d < 0) return [0, 0];
    if (d <= 65535) end = lowerBound(date, n, d);
  }
  return start > end ? [0, 0] : [start, end];
}

/**
 * Builds a mask of which account ids fall under a prefix — the account itself
 * and its descendants.
 *
 * This is a tree match, not a substring match: "expenses:hom" must not select
 * "expenses:home". Masks are memoized on the cube, since a dashboard reuses the
 * same handful of prefixes across every interaction.
 */
export function accountMask(cube, prefix) {
  if (!prefix) return null; // null means "every account"
  const key = `acct:${prefix}`;
  let mask = cube._cache.get(key);
  if (!mask) {
    mask = new Uint8Array(cube.accountPaths.length);
    for (let i = 0; i < cube.accountPaths.length; i++) {
      const p = cube.accountPaths[i];
      mask[i] = p === prefix || p.startsWith(`${prefix}:`) ? 1 : 0;
    }
    cube._cache.set(key, mask);
  }
  return mask;
}

/**
 * Whether an account's hledger type letter satisfies a `type:` query letter.
 *
 * hledger has two subtypes: Cash (C) is a kind of Asset and Conversion (V) is a
 * kind of Equity, so `type:A` matches cash accounts while `type:C` does not
 * match plain assets. This is not a detail to skip: hledger's own default
 * inference types a plainly-named "assets:checking" as C, so exact matching
 * would report zero assets on an ordinary ledger.
 */
export function typeMatches(accountType, want) {
  if (accountType === want) return true;
  if (want === "A") return accountType === "C";
  if (want === "E") return accountType === "V";
  return false;
}

/**
 * Builds a mask of account ids matching a prefix and/or an hledger type letter.
 * Returns null when neither is set, meaning "every account".
 */
export function accountFilterMask(cube, prefix, type) {
  if (!prefix && !type) return null;
  const key = `mask:${prefix ?? ""}:${type ?? ""}`;
  let mask = cube._cache.get(key);
  if (!mask) {
    const byPrefix = accountMask(cube, prefix);
    mask = new Uint8Array(cube.accounts.length);
    for (let i = 0; i < cube.accounts.length; i++) {
      const prefixOK = !byPrefix || byPrefix[i] === 1;
      const typeOK = !type || typeMatches(cube.accounts[i].type ?? "", type);
      mask[i] = prefixOK && typeOK ? 1 : 0;
    }
    cube._cache.set(key, mask);
  }
  return mask;
}

function payeeId(cube, payee) {
  if (!payee) return -1;
  const key = "payeeIndex";
  let index = cube._cache.get(key);
  if (!index) {
    index = new Map(cube.payees.map((p, i) => [p, i]));
    cube._cache.set(key, index);
  }
  return index.has(payee) ? index.get(payee) : -2; // -2 matches nothing
}

function commodityId(cube, code) {
  if (!code) return -1;
  const i = cube.commodities.findIndex((c) => c.code === code);
  return i < 0 ? -2 : i;
}

/**
 * Totals matching postings per commodity, in major units.
 *
 * filter: { from, to, account, type, payee, commodity }. `from`/`to` are
 * "YYYY-MM-DD" with an exclusive `to`; `account` matches the account and its
 * descendants; `type` is an hledger account type letter such as "X".
 */
export function flowSums(cube, filter = {}) {
  assertSummableOverTime(cube, "postings", "amount");

  const { columns } = cube.tables.postings;
  const [start, end] = dateRange(cube, filter.from, filter.to);
  const totals = new Map();
  if (start >= end) return totals;

  const mask = accountFilterMask(cube, filter.account, filter.type);
  const wantPayee = payeeId(cube, filter.payee);
  const wantCommodity = commodityId(cube, filter.commodity);

  // Minor units accumulate exactly in a float64 below 2^53; the scale is
  // applied once at the end.
  const minor = new Float64Array(cube.commodities.length);
  for (let i = start; i < end; i++) {
    if (mask && !mask[columns.account[i]]) continue;
    if (wantPayee !== -1 && columns.payee[i] !== wantPayee) continue;
    const c = columns.commodity[i];
    if (wantCommodity !== -1 && c !== wantCommodity) continue;
    minor[c] += columns.amount[i];
  }

  for (let i = 0; i < minor.length; i++) {
    if (minor[i] !== 0) totals.set(cube.commodities[i].code, toMajor(cube, i, minor[i]));
  }
  return totals;
}

/** Convenience: the total for one commodity (default: the reporting currency). */
export function flowTotal(cube, filter = {}, code = null) {
  return flowSums(cube, filter).get(code ?? cube.reportingCurrency) ?? 0;
}

/**
 * Groups matching postings into a per-month series.
 *
 * Returns [{ period: "2024-03", total }] covering every month in the cube's
 * period list that falls inside the filter, including months with no activity
 * so charts do not silently close gaps.
 */
export function flowSeries(cube, filter = {}) {
  assertSummableOverTime(cube, "postings", "amount");

  const { columns } = cube.tables.postings;
  const [start, end] = dateRange(cube, filter.from, filter.to);
  const mask = accountFilterMask(cube, filter.account, filter.type);
  const wantPayee = payeeId(cube, filter.payee);
  const code = filter.commodity ?? cube.reportingCurrency;
  const wantCommodity = commodityId(cube, code);

  const byPeriod = new Map();
  const epoch = utcDay(cube.epochDate);
  for (let i = start; i < end; i++) {
    if (mask && !mask[columns.account[i]]) continue;
    if (wantPayee !== -1 && columns.payee[i] !== wantPayee) continue;
    if (wantCommodity !== -1 && columns.commodity[i] !== wantCommodity) continue;
    const d = new Date(epoch + columns.date[i] * MS_PER_DAY);
    const period = `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
    byPeriod.set(period, (byPeriod.get(period) ?? 0) + columns.amount[i]);
  }

  const commodityIndex = wantCommodity === -1 ? 0 : wantCommodity;
  return periodsInRange(cube, filter.from, filter.to).map((period) => ({
    period,
    total: toMajor(cube, commodityIndex, byPeriod.get(period) ?? 0),
  }));
}

/**
 * Groups matching postings by account subtree at a given depth, for a
 * breakdown chart. Returns [{ account, total }] sorted descending by magnitude.
 */
export function flowByAccount(cube, filter = {}, depth = 2) {
  assertSummableOverTime(cube, "postings", "amount");

  const { columns } = cube.tables.postings;
  const [start, end] = dateRange(cube, filter.from, filter.to);
  const mask = accountFilterMask(cube, filter.account, filter.type);
  const code = filter.commodity ?? cube.reportingCurrency;
  const wantCommodity = commodityId(cube, code);

  // Precompute each account id's truncated path once per depth.
  const key = `depth:${depth}`;
  let truncated = cube._cache.get(key);
  if (!truncated) {
    truncated = cube.accountPaths.map((p) => p.split(":").slice(0, depth).join(":"));
    cube._cache.set(key, truncated);
  }

  const byAccount = new Map();
  for (let i = start; i < end; i++) {
    const acct = columns.account[i];
    if (mask && !mask[acct]) continue;
    if (wantCommodity !== -1 && columns.commodity[i] !== wantCommodity) continue;
    const name = truncated[acct];
    byAccount.set(name, (byAccount.get(name) ?? 0) + columns.amount[i]);
  }

  const commodityIndex = wantCommodity === -1 ? 0 : wantCommodity;
  return [...byAccount]
    .map(([account, minor]) => ({ account, total: toMajor(cube, commodityIndex, minor) }))
    .sort((a, b) => Math.abs(b.total) - Math.abs(a.total));
}

/**
 * Builds an account-by-period matrix with parent rows aggregated, the shape an
 * income statement needs.
 *
 * Every posting contributes to its own account row and to each of its ancestor
 * rows, which is what `hledger is --tree` does. Rows come back sorted by
 * account path with a `depth` for indentation.
 *
 * Unlike hledger's tree rendering this does not collapse single-child chains
 * ("home" and "home:rent" both get a row rather than one combined "home:rent"),
 * so the same totals are shown across slightly more rows.
 */
export function flowMatrix(cube, filter = {}) {
  assertSummableOverTime(cube, "postings", "amount");

  const periods = periodsInRange(cube, filter.from, filter.to);
  const periodIndex = new Map(periods.map((p, i) => [p, i]));
  const { columns } = cube.tables.postings;
  const [start, end] = dateRange(cube, filter.from, filter.to);
  const mask = accountFilterMask(cube, filter.account, filter.type);
  const code = filter.commodity ?? cube.reportingCurrency;
  const wantCommodity = commodityId(cube, code);

  // Each account's own path plus every ancestor path, computed once per cube.
  let ancestors = cube._cache.get("ancestors");
  if (!ancestors) {
    ancestors = cube.accountPaths.map((path) => {
      const parts = path.split(":");
      return parts.map((_, i) => parts.slice(0, i + 1).join(":"));
    });
    cube._cache.set("ancestors", ancestors);
  }

  const epoch = utcDay(cube.epochDate);
  const byAccount = new Map();
  for (let i = start; i < end; i++) {
    const acct = columns.account[i];
    if (mask && !mask[acct]) continue;
    if (wantCommodity !== -1 && columns.commodity[i] !== wantCommodity) continue;

    const d = new Date(epoch + columns.date[i] * MS_PER_DAY);
    const period = `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
    const col = periodIndex.get(period);
    if (col === undefined) continue;

    const amount = columns.amount[i];
    for (const path of ancestors[acct]) {
      let totals = byAccount.get(path);
      if (!totals) {
        totals = new Float64Array(periods.length);
        byAccount.set(path, totals);
      }
      totals[col] += amount;
    }
  }

  const commodityIndex = wantCommodity === -1 ? 0 : wantCommodity;
  const rows = [...byAccount.entries()]
    .sort((a, b) => (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0))
    .map(([account, minor]) => {
      const totals = Array.from(minor, (v) => toMajor(cube, commodityIndex, v));
      return {
        account,
        label: account.split(":").pop(),
        depth: account.split(":").length - 1,
        totals,
        total: totals.reduce((sum, v) => sum + v, 0),
      };
    });

  return { periods, rows };
}

/**
 * Totals a stock measure for a single period, rolled up over an account
 * subtree, in major units.
 *
 * `table` is "valued" (market value) or "cost" (cost basis). There is
 * deliberately no range form: rolling up the account tree at a fixed period is
 * the only legal reduction of a stock measure.
 */
export function balanceAt(cube, tableName, period, account = "") {
  const table = cube.tables[tableName];
  if (!table) throw new Error(`unknown table ${tableName}`);
  const periodIdx = cube.periods.indexOf(period);
  if (periodIdx < 0) return 0;

  const mask = accountMask(cube, account);
  const { columns } = table;
  const reporting = commodityId(cube, cube.reportingCurrency);
  let minor = 0;
  for (let i = 0; i < table.rows; i++) {
    if (columns.period[i] !== periodIdx) continue;
    if (mask && !mask[columns.account[i]]) continue;
    if (columns.commodity[i] !== reporting) continue;
    minor += columns.amount[i];
  }
  return toMajor(cube, reporting < 0 ? 0 : reporting, minor);
}

/**
 * A stock measure across periods, as a series of independent instants.
 *
 * This is not a sum over time: each point is its own account-tree rollup at one
 * period end, which is exactly what a net-worth or portfolio-value chart plots.
 */
export function balanceSeries(cube, tableName, account = "", { from, to } = {}) {
  const table = cube.tables[tableName];
  if (!table) throw new Error(`unknown table ${tableName}`);

  const mask = accountMask(cube, account);
  const { columns } = table;
  const reporting = commodityId(cube, cube.reportingCurrency);
  const minorByPeriod = new Float64Array(cube.periods.length);
  for (let i = 0; i < table.rows; i++) {
    if (mask && !mask[columns.account[i]]) continue;
    if (columns.commodity[i] !== reporting) continue;
    minorByPeriod[columns.period[i]] += columns.amount[i];
  }

  const commodityIndex = reporting < 0 ? 0 : reporting;
  const indexOfPeriod = new Map(cube.periods.map((p, i) => [p, i]));
  return periodsInRange(cube, from, to).map((period) => ({
    period,
    total: toMajor(cube, commodityIndex, minorByPeriod[indexOfPeriod.get(period)] ?? 0),
  }));
}

/**
 * The cube's month labels that fall inside a half-open date interval.
 *
 * A month is included when it starts before `to` and ends on or after `from`,
 * so a range that starts mid-month still includes that month.
 */
export function periodsInRange(cube, from, to) {
  const fromPeriod = from ? String(from).slice(0, 7) : null;
  // `to` is exclusive, so a range ending on the 1st of a month excludes it
  // entirely; any later day in that month includes it.
  const toDay = to ? utcDay(to) : null;
  let toPeriod = null;
  if (toDay != null) {
    const d = new Date(toDay - MS_PER_DAY);
    toPeriod = `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
  }
  return cube.periods.filter(
    (p) => (!fromPeriod || p >= fromPeriod) && (!toPeriod || p <= toPeriod),
  );
}

/** The most recent period the cube covers, or null when it is empty. */
export function latestPeriod(cube) {
  return cube.periods.length ? cube.periods[cube.periods.length - 1] : null;
}
