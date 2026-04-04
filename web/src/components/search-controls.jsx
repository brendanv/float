import { useEffect, useRef, useState } from "preact/hooks";

function pad2(n) {
  return n < 10 ? "0" + n : "" + n;
}

function fmtDate(y, m, d) {
  return `${y}-${pad2(m)}-${pad2(d)}`;
}

const MONTH_NAMES = ["January", "February", "March", "April", "May", "June",
  "July", "August", "September", "October", "November", "December"];

const SHORT_MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun",
  "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

function shiftMonth(dateFrom, delta) {
  const d = new Date(dateFrom + "T00:00:00");
  d.setMonth(d.getMonth() + delta);
  d.setDate(1);
  const y = d.getFullYear(), m = d.getMonth() + 1;
  const ny = m === 12 ? y + 1 : y, nm = m === 12 ? 1 : m + 1;
  return { from: fmtDate(y, m, 1), to: fmtDate(ny, nm, 1) };
}

const PERIOD_BAR_PRESETS = ["This month", "Last month", "This year", "Last year"];

// Returns { from, to } where to is exclusive (first day after range).
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

// Sentinel value for the payee filter meaning "transactions with no payee set".
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

// Quick filter presets for the status/workflow axis.
// apply(currentFilters) returns the new full filter state.
// isActive(currentFilters) returns true when this preset matches the current state.
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

// Format a date range for display in the date picker button.
function formatDateRange(dateFrom, dateTo) {
  if (!dateFrom) return "All time";
  const from = new Date(dateFrom + "T00:00:00");
  if (dateTo) {
    const to = new Date(dateTo + "T00:00:00"); // exclusive end
    // Single whole month?
    if (from.getDate() === 1 && to.getDate() === 1) {
      const next = new Date(from);
      next.setMonth(next.getMonth() + 1);
      if (next.getTime() === to.getTime()) {
        return `${MONTH_NAMES[from.getMonth()]} ${from.getFullYear()}`;
      }
    }
    // Whole year?
    if (from.getMonth() === 0 && from.getDate() === 1 &&
        to.getMonth() === 0 && to.getDate() === 1 &&
        to.getFullYear() === from.getFullYear() + 1) {
      return String(from.getFullYear());
    }
    // Generic: show inclusive end date
    const toInc = new Date(to);
    toInc.setDate(toInc.getDate() - 1);
    const fs = `${SHORT_MONTHS[from.getMonth()]} ${from.getDate()}`;
    const ts = `${SHORT_MONTHS[toInc.getMonth()]} ${toInc.getDate()}`;
    if (from.getFullYear() === toInc.getFullYear()) {
      return `${fs} – ${ts}, ${from.getFullYear()}`;
    }
    return `${fs}, ${from.getFullYear()} – ${ts}, ${toInc.getFullYear()}`;
  }
  return dateFrom;
}

export function PeriodBar({ dateFrom, dateTo, onChange }) {
  const presets = DATE_PRESETS.filter(p => PERIOD_BAR_PRESETS.includes(p.label));

  function activePresetLabel() {
    for (const p of presets) {
      const { from, to } = p.fn();
      if (from === dateFrom && to === dateTo) return p.label;
    }
    return null;
  }

  const activeLabel = activePresetLabel();

  function displayLabel() {
    if (activeLabel) return activeLabel;
    if (!dateFrom) return "";
    const d = new Date(dateFrom + "T00:00:00");
    return `${MONTH_NAMES[d.getMonth()]} ${d.getFullYear()}`;
  }

  return (
    <div class="flex items-center gap-2 flex-wrap mb-4">
      <button class="btn btn-sm btn-ghost" onClick={() => { const r = shiftMonth(dateFrom, -1); onChange(r.from, r.to); }}>‹</button>
      <span class="text-sm font-medium min-w-32 text-center">{displayLabel()}</span>
      <button class="btn btn-sm btn-ghost" onClick={() => { const r = shiftMonth(dateFrom, 1); onChange(r.from, r.to); }}>›</button>
      <div class="flex gap-1 ml-2">
        {presets.map((p) => {
          const { from, to } = p.fn();
          const isActive = from === dateFrom && to === dateTo;
          return (
            <button
              key={p.label}
              class={`btn btn-xs ${isActive ? "btn-primary" : "btn-ghost"}`}
              onClick={() => onChange(from, to)}
            >
              {p.label}
            </button>
          );
        })}
      </div>
    </div>
  );
}

export function SearchControls({
  dateFrom,
  dateTo,
  account,
  tag,
  status,
  payee,
  onDateRangeChange,
  onAccountChange,
  onTagChange,
  onPayeeChange,
  onQuickFilter,
  accounts,
  tags,
}) {
  const [dateOpen, setDateOpen] = useState(false);
  const [quickOpen, setQuickOpen] = useState(false);
  const dateRef = useRef(null);
  const quickRef = useRef(null);

  useEffect(() => {
    function handleClick(e) {
      if (dateRef.current && !dateRef.current.contains(e.target)) setDateOpen(false);
      if (quickRef.current && !quickRef.current.contains(e.target)) setQuickOpen(false);
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  const currentFilters = { dateFrom, dateTo, account, tag, status, payee };
  const activeQuickFilter = QUICK_FILTERS.find(qf => qf.isActive(currentFilters));

  function shiftDate(delta) {
    const now = new Date();
    const base = dateFrom || fmtDate(now.getFullYear(), now.getMonth() + 1, 1);
    const r = shiftMonth(base, delta);
    onDateRangeChange(r.from, r.to);
  }

  return (
    <div class="mb-4 space-y-2">
      {/* Date range + quick filters row */}
      <div class="flex flex-wrap items-center gap-1">
        <button class="btn btn-sm btn-ghost px-2" onClick={() => shiftDate(-1)}>‹</button>

        {/* Date range dropdown */}
        <div class="relative" ref={dateRef}>
          <button
            class="btn btn-sm font-normal gap-1"
            onClick={() => { setDateOpen(o => !o); setQuickOpen(false); }}
          >
            {formatDateRange(dateFrom, dateTo)}
            <span class="opacity-40 text-xs">▾</span>
          </button>
          {dateOpen && (
            <div class="absolute top-full left-0 z-50 mt-1 w-56 bg-base-100 border border-base-300 rounded-box shadow-lg py-1">
              {DATE_PRESETS.map(p => {
                const { from, to } = p.fn();
                const isActive = from === dateFrom && to === dateTo;
                return (
                  <button
                    key={p.label}
                    class={`w-full text-left px-4 py-1.5 text-sm hover:bg-base-200 flex justify-between items-center${isActive ? " font-semibold" : ""}`}
                    onClick={() => { onDateRangeChange(from, to); setDateOpen(false); }}
                  >
                    {p.label}
                    {isActive && <span class="text-primary text-xs">✓</span>}
                  </button>
                );
              })}
              <div class="border-t border-base-300 mt-1 pt-2 px-3 pb-2 space-y-1">
                <div class="text-xs text-base-content/50 mb-1">Custom range</div>
                <input
                  type="date"
                  class="input input-xs input-bordered w-full"
                  value={dateFrom}
                  onInput={(e) => onDateRangeChange(e.target.value, dateTo)}
                />
                <input
                  type="date"
                  class="input input-xs input-bordered w-full"
                  value={dateTo ? isoExclusiveToInclusive(dateTo) : ""}
                  onInput={(e) => {
                    const v = e.target.value;
                    onDateRangeChange(dateFrom, v ? inclusiveToExclusiveTo(v) : "");
                  }}
                />
              </div>
            </div>
          )}
        </div>

        <button class="btn btn-sm btn-ghost px-2" onClick={() => shiftDate(1)}>›</button>

        {/* Quick filters dropdown */}
        {onQuickFilter && (
          <div class="relative ml-1" ref={quickRef}>
            <button
              class={`btn btn-sm gap-1 ${activeQuickFilter?.label !== "All" ? "btn-primary" : "btn-ghost"}`}
              onClick={() => { setQuickOpen(o => !o); setDateOpen(false); }}
            >
              {activeQuickFilter?.label ?? "Filter"}
              <span class="opacity-40 text-xs">▾</span>
            </button>
            {quickOpen && (
              <div class="absolute top-full left-0 z-50 mt-1 w-56 bg-base-100 border border-base-300 rounded-box shadow-lg py-1">
                {QUICK_FILTERS.map(qf => {
                  const isActive = qf.isActive(currentFilters);
                  return (
                    <button
                      key={qf.label}
                      class={`w-full text-left px-4 py-1.5 text-sm hover:bg-base-200 flex justify-between items-center${isActive ? " font-semibold" : ""}`}
                      onClick={() => { onQuickFilter(qf.apply(currentFilters)); setQuickOpen(false); }}
                      title={qf.description ?? qf.label}
                    >
                      {qf.label}
                      {isActive && <span class="text-primary text-xs">✓</span>}
                    </button>
                  );
                })}
              </div>
            )}
          </div>
        )}
      </div>

      {/* Filter dropdowns row */}
      <div class="flex flex-wrap gap-2">
        <div class="join">
          <select
            class="select select-bordered select-sm join-item"
            value={account}
            onChange={(e) => onAccountChange(e.target.value)}
          >
            <option value="">All accounts</option>
            {(accounts || []).map((a) => (
              <option key={a.fullName} value={a.fullName}>{a.fullName}</option>
            ))}
          </select>
          <select
            class="select select-bordered select-sm join-item"
            value={tag}
            onChange={(e) => onTagChange(e.target.value)}
          >
            <option value="">Any tag</option>
            {(tags || []).map((t) => (
              <option key={t} value={t}>{t}</option>
            ))}
          </select>
        </div>
      </div>

      {/* Active filter chips */}
      {(account || tag || payee) && (
        <div class="flex flex-wrap gap-1">
          {payee && (
            <div class="badge badge-neutral gap-1">
              {payee === PAYEE_NONE ? "no payee set" : `payee: ${payee}`}
              <button class="cursor-pointer opacity-60 hover:opacity-100" onClick={() => onPayeeChange("")} aria-label="Clear payee filter">✕</button>
            </div>
          )}
          {account && (
            <div class="badge badge-neutral gap-1">
              acct: {account}
              <button class="cursor-pointer opacity-60 hover:opacity-100" onClick={() => onAccountChange("")} aria-label="Clear account filter">✕</button>
            </div>
          )}
          {tag && (
            <div class="badge badge-neutral gap-1">
              tag: {tag}
              <button class="cursor-pointer opacity-60 hover:opacity-100" onClick={() => onTagChange("")} aria-label="Clear tag filter">✕</button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// The date inputs show inclusive end dates to users, but we store exclusive end dates
// (first day after range) for hledger semantics.

// Convert exclusive-end "YYYY-MM-DD" to the inclusive end date shown in the "To" input.
function isoExclusiveToInclusive(exclusive) {
  if (!exclusive) return "";
  const d = new Date(exclusive + "T00:00:00");
  d.setDate(d.getDate() - 1);
  return fmtDate(d.getFullYear(), d.getMonth() + 1, d.getDate());
}

// Convert inclusive "To" input value back to an exclusive end date for hledger.
function inclusiveToExclusiveTo(inclusive) {
  if (!inclusive) return "";
  const d = new Date(inclusive + "T00:00:00");
  d.setDate(d.getDate() + 1);
  return fmtDate(d.getFullYear(), d.getMonth() + 1, d.getDate());
}
