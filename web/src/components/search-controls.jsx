import { useEffect } from "preact/hooks";

function pad2(n) {
  return n < 10 ? "0" + n : "" + n;
}

function fmtDate(y, m, d) {
  return `${y}-${pad2(m)}-${pad2(d)}`;
}

const MONTH_NAMES = ["January", "February", "March", "April", "May", "June",
  "July", "August", "September", "October", "November", "December"];

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
  { label: "This year", fn: thisYear },
  { label: "Last 12 months", fn: last12Months },
  { label: "Last year", fn: lastYear },
  { label: "All", fn: () => ({ from: "", to: "" }) },
];

// Quick filter presets: each returns the complete filter state to apply.
export const QUICK_FILTERS = [
  {
    label: "All unreviewed",
    description: "All unreviewed transactions across all time",
    getFilters: () => ({ dateFrom: "", dateTo: "", status: "unreviewed", account: "", tag: "", payee: "" }),
  },
  {
    label: "Unreviewed this month",
    description: "Unreviewed transactions from this month",
    getFilters: () => ({ ...thisMonth(), status: "unreviewed", account: "", tag: "", payee: "" }),
  },
  {
    label: "Unreviewed last month",
    description: "Unreviewed transactions from last month",
    getFilters: () => ({ ...lastMonth(), status: "unreviewed", account: "", tag: "", payee: "" }),
  },
  {
    label: "Last 30 days",
    description: "All transactions from the last 30 days",
    getFilters: () => ({ ...last30Days(), status: "", account: "", tag: "", payee: "" }),
  },
  {
    label: "No payee set",
    description: "Transactions without a payee assigned",
    getFilters: () => ({ dateFrom: "", dateTo: "", status: "", account: "", tag: "", payee: PAYEE_NONE }),
  },
];

function quickFilterActive(qf, { dateFrom, dateTo, account, tag, status, payee }) {
  const t = qf.getFilters();
  return t.dateFrom === dateFrom && t.dateTo === dateTo &&
    t.status === status && t.account === account &&
    t.tag === tag && t.payee === payee;
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
  onStatusChange,
  onPayeeChange,
  onQuickFilter,
  accounts,
  tags,
}) {
  function applyPreset(preset) {
    const { from, to } = preset.fn();
    onDateRangeChange(from, to);
  }

  function activePresetLabel() {
    for (const p of DATE_PRESETS) {
      const { from, to } = p.fn();
      if (from === dateFrom && to === dateTo) return p.label;
    }
    return null;
  }

  const activePreset = activePresetLabel();

  return (
    <div class="mb-4 space-y-2">
      {/* Quick filters row */}
      {onQuickFilter && (
        <div class="flex flex-wrap items-center gap-1">
          <span class="text-xs text-base-content/50 mr-1 shrink-0">Quick:</span>
          {QUICK_FILTERS.map((qf) => {
            const isActive = quickFilterActive(qf, { dateFrom, dateTo, account, tag, status, payee });
            return (
              <button
                key={qf.label}
                class={`btn btn-xs ${isActive ? "btn-primary" : "btn-ghost"}`}
                onClick={() => onQuickFilter(qf.getFilters())}
                title={qf.description}
              >
                {qf.label}
              </button>
            );
          })}
        </div>
      )}

      {/* Date range row */}
      <div class="flex flex-wrap items-center gap-2">
        <div class="join">
          <input
            type="date"
            class="input input-bordered input-sm join-item"
            value={dateFrom}
            placeholder="From"
            onInput={(e) => onDateRangeChange(e.target.value, dateTo)}
          />
          <input
            type="date"
            class="input input-bordered input-sm join-item"
            value={dateTo ? isoExclusiveToInclusive(dateTo) : ""}
            placeholder="To"
            onInput={(e) => {
              const v = e.target.value;
              onDateRangeChange(dateFrom, v ? inclusiveToExclusiveTo(v) : "");
            }}
          />
        </div>
        <div class="join flex-wrap">
          {DATE_PRESETS.map((p) => (
            <button
              key={p.label}
              class={`btn btn-sm join-item ${activePreset === p.label ? "btn-primary" : ""}`}
              onClick={() => applyPreset(p)}
            >
              {p.label}
            </button>
          ))}
        </div>
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
              <option key={a.fullName} value={a.fullName}>
                {a.fullName}
              </option>
            ))}
          </select>

          <select
            class="select select-bordered select-sm join-item"
            value={tag}
            onChange={(e) => onTagChange(e.target.value)}
          >
            <option value="">Any tag</option>
            {(tags || []).map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
        </div>

        <div class="join">
          <button
            class={`btn btn-sm join-item ${!status ? "btn-active" : ""}`}
            onClick={() => onStatusChange("")}
          >
            All
          </button>
          <button
            class={`btn btn-sm join-item ${status === "reviewed" ? "btn-active" : ""}`}
            onClick={() => onStatusChange("reviewed")}
          >
            Reviewed
          </button>
          <button
            class={`btn btn-sm join-item ${status === "unreviewed" ? "btn-active" : ""}`}
            onClick={() => onStatusChange("unreviewed")}
          >
            Unreviewed
          </button>
        </div>
      </div>

      {/* Active filter chips */}
      {(account || tag || status || payee || !activePreset) && (
        <div class="flex flex-wrap gap-1">
          {payee && (
            <div class="badge badge-neutral gap-1">
              {payee === PAYEE_NONE ? "no payee set" : `payee: ${payee}`}
              <button
                class="cursor-pointer opacity-60 hover:opacity-100"
                onClick={() => onPayeeChange("")}
                aria-label="Clear payee filter"
              >
                ✕
              </button>
            </div>
          )}
          {account && (
            <div class="badge badge-neutral gap-1">
              acct: {account}
              <button
                class="cursor-pointer opacity-60 hover:opacity-100"
                onClick={() => onAccountChange("")}
                aria-label="Clear account filter"
              >
                ✕
              </button>
            </div>
          )}
          {tag && (
            <div class="badge badge-neutral gap-1">
              tag: {tag}
              <button
                class="cursor-pointer opacity-60 hover:opacity-100"
                onClick={() => onTagChange("")}
                aria-label="Clear tag filter"
              >
                ✕
              </button>
            </div>
          )}
          {status && (
            <div class="badge badge-neutral gap-1">
              {status === "reviewed" ? "Reviewed" : "Unreviewed"}
              <button
                class="cursor-pointer opacity-60 hover:opacity-100"
                onClick={() => onStatusChange("")}
                aria-label="Clear status filter"
              >
                ✕
              </button>
            </div>
          )}
          {!activePreset && dateFrom && dateTo && (
            <div class="badge badge-neutral gap-1">
              {dateFrom} – {isoExclusiveToInclusive(dateTo)}
              <button
                class="cursor-pointer opacity-60 hover:opacity-100"
                onClick={() => {
                  const { from, to } = thisMonth();
                  onDateRangeChange(from, to);
                }}
                aria-label="Reset to this month"
              >
                ✕
              </button>
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
