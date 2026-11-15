export function todayStr() {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

const CURRENCY_SYMBOLS = {
  "$": "$", // legacy compatibility until $ → USD migration
  USD: "$", EUR: "€", GBP: "£", JPY: "¥", CAD: "CA$", AUD: "A$",
  CHF: "CHF ", CNY: "¥", INR: "₹", MXN: "MX$",
};

export function formatCurrency(quantity, commodity) {
  const val = parseFloat(quantity);
  if (isNaN(val)) return "—";
  const sym = CURRENCY_SYMBOLS[commodity];
  if (sym) {
    const abs = Math.abs(val).toLocaleString("en-US", {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    });
    return (val < 0 ? "-" : "") + sym + abs;
  }
  // Non-fiat commodity (e.g. "AAPL"): preserve original precision
  const raw = String(quantity).trim();
  if (!commodity) return raw;
  return (val < 0 ? "-" : "") + Math.abs(val) + " " + commodity;
}

export function formatCost(cost) {
  if (!cost || !cost.quantity) return "";
  return (cost.isTotal ? " @@ " : " @ ") + formatCurrency(cost.quantity, cost.commodity);
}

export function formatCostBasis(cb) {
  if (!cb) return "";
  const parts = [];
  if (cb.date) parts.push(cb.date);
  if (cb.label) parts.push(JSON.stringify(cb.label));
  if (cb.cost?.quantity) parts.push(formatCurrency(cb.cost.quantity, cb.cost.commodity));
  if (parts.length === 0) return "";
  return " {" + parts.join(", ") + "}";
}

export function formatBalanceAssertion(ba) {
  if (!ba?.amount?.quantity) return "";
  return " = " + formatCurrency(ba.amount.quantity, ba.amount.commodity);
}

export function formatAmounts(amounts) {
  if (!amounts || amounts.length === 0) return "";
  const nonZero = amounts.filter((a) => parseFloat(a.quantity) !== 0);
  const display = nonZero.length > 0 ? nonZero : amounts;
  return display.map((a) => formatCurrency(a.quantity, a.commodity) + formatCostBasis(a.costBasis) + formatCost(a.cost)).join(", ");
}

export function formatDate(dateStr) {
  if (!dateStr) return "";
  return dateStr;
}

const MONTH_NAMES = [
  "January", "February", "March", "April", "May", "June",
  "July", "August", "September", "October", "November", "December",
];

export function monthName(month) {
  return MONTH_NAMES[month - 1] || "";
}

// formatTimestamp renders an RFC 3339 timestamp in the viewer's locale and
// timezone, e.g. "May 15, 2026, 4:30 AM". Unparseable input is returned as-is.
export function formatTimestamp(ts) {
  if (!ts) return "";
  const d = new Date(ts);
  if (isNaN(d)) return String(ts);
  return d.toLocaleString(undefined, {
    year: "numeric", month: "short", day: "numeric", hour: "numeric", minute: "2-digit",
  });
}

const RELATIVE_UNITS = [
  ["year", 365 * 24 * 3600],
  ["month", 30 * 24 * 3600],
  ["week", 7 * 24 * 3600],
  ["day", 24 * 3600],
  ["hour", 3600],
  ["minute", 60],
];

// formatRelativeTime renders an RFC 3339 timestamp relative to `now`, e.g.
// "3 hours ago" or "in 2 days". Under a minute reads "just now".
export function formatRelativeTime(ts, now = new Date()) {
  if (!ts) return "";
  const d = new Date(ts);
  if (isNaN(d)) return String(ts);
  const seconds = (d - now) / 1000;
  const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });
  for (const [unit, size] of RELATIVE_UNITS) {
    if (Math.abs(seconds) >= size) return rtf.format(Math.round(seconds / size), unit);
  }
  return "just now";
}
