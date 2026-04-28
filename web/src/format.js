const CURRENCY_SYMBOLS = {
  USD: "$", EUR: "€", GBP: "£", JPY: "¥", CAD: "CA$", AUD: "A$",
  CHF: "CHF ", CNY: "¥", INR: "₹", MXN: "MX$",
};

export function formatCurrency(quantity, commodity) {
  const val = parseFloat(quantity);
  if (isNaN(val)) return "—";
  const sym = CURRENCY_SYMBOLS[commodity];
  const abs = Math.abs(val).toLocaleString("en-US", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
  if (sym) return (val < 0 ? "-" : "") + sym + abs;
  // Non-fiat commodity (e.g. "AAPL"): quantity then ticker
  return (val < 0 ? "-" : "") + abs + " " + commodity;
}

export function formatAmounts(amounts) {
  if (!amounts || amounts.length === 0) return "";
  return amounts.map((a) => formatCurrency(a.quantity, a.commodity)).join(", ");
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
