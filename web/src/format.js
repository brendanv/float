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

export function formatBalanceAssertion(ba) {
  if (!ba?.amount?.quantity) return "";
  return " = " + formatCurrency(ba.amount.quantity, ba.amount.commodity);
}

export function formatAmounts(amounts) {
  if (!amounts || amounts.length === 0) return "";
  return amounts.map((a) => formatCurrency(a.quantity, a.commodity) + formatCost(a.cost)).join(", ");
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
