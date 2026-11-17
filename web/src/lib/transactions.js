// Display helpers for transaction lists: which accounts a row shows as its
// flow, which posting's amount it shows, and per-day / per-list totals.
// They only pick and add up amounts hledger already returned; they never
// re-derive balances.
import { accountType } from "./accounts.js";
import { editablePostings } from "../components/posting-fields.jsx";

export function firstQuantity(posting) {
  if (!posting.amounts || posting.amounts.length === 0) return 0;
  return parseFloat(posting.amounts[0].quantity) || 0;
}

// sideAccount returns the account of the single negative ("from") or
// positive ("to") posting, or "" when that side has zero or several postings.
export function sideAccount(tx, side) {
  const matching = editablePostings(tx.postings)
    .filter((p) => side === "from" ? firstQuantity(p) < 0 : firstQuantity(p) > 0);
  return matching.length === 1 ? matching[0].account : "";
}

// accountSplit returns the singular from/to accounts for a transaction, or
// flags isMultiple when either side has more than one posting and the cells
// should be merged into a single flow (with "N accounts" for that side).
export function accountSplit(tx) {
  const postings = tx.postings || [];
  if (postings.length === 0) return { from: "", to: "", isMultiple: false };
  if (postings.length === 1) {
    return { from: postings[0].account, to: postings[0].account, isMultiple: false };
  }
  const positives = postings.filter((p) => firstQuantity(p) > 0);
  const negatives = postings.filter((p) => firstQuantity(p) < 0);
  if (postings.length === 2 && (negatives.length !== 1 || positives.length !== 1)) {
    return { from: postings[0].account, to: postings[1].account, isMultiple: false };
  }
  if (negatives.length === 1 && positives.length === 1) {
    return { from: negatives[0].account, to: positives[0].account, isMultiple: false };
  }
  return {
    from: negatives.length === 1 ? negatives[0].account : `${negatives.length} accounts`,
    to: positives.length === 1 ? positives[0].account : `${positives.length} accounts`,
    isMultiple: true,
  };
}

// displayAmounts returns the amounts a general-list row shows: the positive
// ("to") posting's amounts, as the list has always shown.
export function displayAmounts(tx) {
  const postings = tx.postings || [];
  if (postings.length === 0) return [];
  if (postings.length === 1) return postings[0].amounts || [];
  const pos = postings.find((p) => firstQuantity(p) > 0);
  return (pos ?? postings[0]).amounts || [];
}

// amountSortValue is the numeric sort key for the Amount column.
export function amountSortValue(tx) {
  const postings = tx.postings || [];
  if (postings.length === 0) return 0;
  const pos = postings.find((p) => firstQuantity(p) > 0);
  return pos ? firstQuantity(pos) : firstQuantity(postings[0]);
}

// focusedAmounts sums the postings to `focusedAccount` (or its subaccounts)
// per commodity, for a general list filtered to one account.
export function focusedAmounts(tx, focusedAccount) {
  const postings = tx.postings || [];
  const focused = postings.filter((p) => p.account === focusedAccount || p.account.startsWith(focusedAccount + ":"));
  if (focused.length === 0) return postings[0]?.amounts || [];
  if (focused.length === 1) return focused[0].amounts || [];
  return addAmounts(focused.flatMap((p) => p.amounts || []));
}

// isIncome reports whether a transaction is income arriving: every posting
// money leaves ("from") is an income account. A sale that books a gain
// alongside the asset it sells is not.
export function isIncome(tx) {
  const from = (tx.postings || []).filter((p) => firstQuantity(p) < 0);
  return from.length > 0 && from.every((p) => accountType(p.account) === "income");
}

// addAmounts sums amounts per commodity, keeping first-seen order.
export function addAmounts(amounts, sign = 1) {
  const sums = new Map();
  for (const a of amounts) {
    const q = (parseFloat(a.quantity) || 0) * sign;
    sums.set(a.commodity, (sums.get(a.commodity) ?? 0) + q);
  }
  return [...sums].map(([commodity, quantity]) => ({ commodity, quantity: String(round(quantity)) }));
}

function round(q) {
  return Math.round(q * 1e8) / 1e8;
}

function nonZero(amounts) {
  return amounts.filter((a) => parseFloat(a.quantity) !== 0);
}

function flowPostings(txs, type) {
  return txs.flatMap((tx) => (tx.postings || []).filter((p) => accountType(p.account) === type));
}

// flowTotals returns what came in (income postings, sign flipped so income
// is positive) and went out (expense postings) across transactions, per
// commodity. Transfers between your own accounts touch neither.
export function flowTotals(txs) {
  const inflow = addAmounts(flowPostings(txs, "income").flatMap((p) => p.amounts || []), -1);
  const outflow = addAmounts(flowPostings(txs, "expense").flatMap((p) => p.amounts || []));
  return { inflow: nonZero(inflow), outflow: nonZero(outflow) };
}

// dayNet is a day's income minus expenses per commodity (non-zero only).
// An empty result means the day only moved money between your accounts.
export function dayNet(txs) {
  const amounts = [...flowPostings(txs, "income"), ...flowPostings(txs, "expense")]
    .flatMap((p) => p.amounts || []);
  return nonZero(addAmounts(amounts, -1));
}

// registerTotals sums an account register's signed changes: increases are
// "in", decreases "out" (as positive amounts).
export function registerTotals(rows) {
  const changes = rows.flatMap((r) => r.change || []);
  const inflow = addAmounts(changes.filter((a) => parseFloat(a.quantity) > 0));
  const outflow = addAmounts(changes.filter((a) => parseFloat(a.quantity) < 0), -1);
  return { inflow: nonZero(inflow), outflow: nonZero(outflow) };
}

// registerDayNet is the focused account's net change over a day's rows.
export function registerDayNet(rows) {
  return nonZero(addAmounts(rows.flatMap((r) => r.change || [])));
}

const SHORT_MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
const WEEKDAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

// dayLabel splits "2026-03-25" into { day: "25 Mar", weekday: "Wed" }, with
// the year added to `day` when it isn't `currentYear`.
export function dayLabel(date, currentYear = new Date().getFullYear()) {
  const [y, m, d] = String(date).split("-").map(Number);
  if (!y || !m || !d) return { day: date, weekday: "" };
  const weekday = WEEKDAYS[new Date(y, m - 1, d).getDay()];
  const day = `${d} ${SHORT_MONTHS[m - 1]}${y === currentYear ? "" : ` ${y}`}`;
  return { day, weekday };
}
