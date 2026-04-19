function pad2(n) {
  return n < 10 ? "0" + n : "" + n;
}

function fmtDate(y, m, d) {
  return `${y}-${pad2(m)}-${pad2(d)}`;
}

function thisMonth() {
  const now = new Date();
  const y = now.getFullYear(), m = now.getMonth() + 1;
  const nextY = m === 12 ? y + 1 : y;
  const nextM = m === 12 ? 1 : m + 1;
  return { from: fmtDate(y, m, 1), to: fmtDate(nextY, nextM, 1) };
}

function lastMonth() {
  const now = new Date();
  const y = now.getFullYear(), m = now.getMonth() + 1;
  const prevY = m === 1 ? y - 1 : y;
  const prevM = m === 1 ? 12 : m - 1;
  return { from: fmtDate(prevY, prevM, 1), to: fmtDate(y, m, 1) };
}

function thisYear() {
  const y = new Date().getFullYear();
  return { from: fmtDate(y, 1, 1), to: fmtDate(y + 1, 1, 1) };
}

function last12Months() {
  const now = new Date();
  const past = new Date(now);
  past.setMonth(past.getMonth() - 12);
  const tomorrow = new Date(now);
  tomorrow.setDate(tomorrow.getDate() + 1);
  function fmt(d) {
    return fmtDate(d.getFullYear(), d.getMonth() + 1, d.getDate());
  }
  return { from: fmt(past), to: fmt(tomorrow) };
}

function lastYear() {
  const y = new Date().getFullYear();
  return { from: fmtDate(y - 1, 1, 1), to: fmtDate(y, 1, 1) };
}

function last30Days() {
  const now = new Date();
  const past = new Date(now);
  past.setDate(past.getDate() - 30);
  const tomorrow = new Date(now);
  tomorrow.setDate(tomorrow.getDate() + 1);
  function fmt(d) { return fmtDate(d.getFullYear(), d.getMonth() + 1, d.getDate()); }
  return { from: fmt(past), to: fmt(tomorrow) };
}

export const PAYEE_NONE = "\x00none";

export const DATE_PRESETS = [
  { label: "This month", fn: thisMonth },
  { label: "Last month", fn: lastMonth },
  { label: "Last 30 days", fn: last30Days },
  { label: "This year", fn: thisYear },
  { label: "Last 12 months", fn: last12Months },
  { label: "Last year", fn: lastYear },
  { label: "All", fn: () => ({ from: "", to: "" }) },
];

export const QUICK_FILTERS = [
  {
    label: "All",
    apply: (f) => ({ ...f, status: "", payee: f.payee === PAYEE_NONE ? "" : f.payee }),
    isActive: (f) => !f.status && f.payee !== PAYEE_NONE,
  },
  {
    label: "Reviewed",
    apply: (f) => ({ ...f, status: "reviewed", payee: f.payee === PAYEE_NONE ? "" : f.payee }),
    isActive: (f) => f.status === "reviewed",
  },
  {
    label: "Unreviewed",
    apply: (f) => ({ ...f, status: "unreviewed", payee: f.payee === PAYEE_NONE ? "" : f.payee }),
    isActive: (f) => f.status === "unreviewed",
  },
  {
    label: "No payee set",
    description: "Transactions without a payee assigned",
    apply: (f) => ({ ...f, status: "", payee: PAYEE_NONE }),
    isActive: (f) => f.payee === PAYEE_NONE,
  },
];
