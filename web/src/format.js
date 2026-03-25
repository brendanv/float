export function formatAmounts(amounts) {
  if (!amounts || amounts.length === 0) return "";
  return amounts
    .map((a) => `${a.commodity}${a.quantity}`)
    .join(", ");
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
