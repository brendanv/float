// Display helpers for transaction lists: which accounts a row shows as its
// flow and which posting's amount it shows. They only pick amounts hledger
// already returned; list and day totals come from hledger reports, not from
// adding up postings here.
import { accountType, NEGATE_TYPES } from "./accounts.js";
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

// registerDisplay says how an account register shows its Change and Balance
// amounts for `account`. Credit-normal accounts (liabilities, income,
// equity) flip the sign, as on the Accounts page, so a card's balance reads
// as the amount owed and income reads as earned. Tones follow the account
// type, from hledger's raw (unflipped) quantity:
// - change is `inflow` when an asset grows or income arrives;
// - balance is `below` when an asset is overdrawn or a liability is owed.
// Everything else stays ink.
export function registerDisplay(account) {
  const type = accountType(account);
  return {
    negate: NEGATE_TYPES.has(type),
    changeTone: (q) => ((type === "asset" && q > 0) || (type === "income" && q < 0) ? "inflow" : "auto"),
    balanceTone: (q) => ((type === "asset" || type === "liability") && q < 0 ? "below" : "auto"),
  };
}

// isIncome reports whether a transaction is income arriving: every posting
// money leaves ("from") is an income account. A sale that books a gain
// alongside the asset it sells is not.
export function isIncome(tx) {
  const from = (tx.postings || []).filter((p) => firstQuantity(p) < 0);
  return from.length > 0 && from.every((p) => accountType(p.account) === "income");
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
