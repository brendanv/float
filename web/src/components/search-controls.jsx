import { useEffect } from "preact/hooks";

function pad2(n) {
  return n < 10 ? "0" + n : "" + n;
}

function fmtDate(y, m, d) {
  return `${y}-${pad2(m)}-${pad2(d)}`;
}

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

export const DATE_PRESETS = [
  { label: "This month", fn: thisMonth },
  { label: "Last month", fn: lastMonth },
  { label: "This year", fn: thisYear },
  { label: "Last 12 months", fn: last12Months },
  { label: "Last year", fn: lastYear },
  { label: "All", fn: () => ({ from: "", to: "" }) },
];

export function SearchControls({
  dateFrom,
  dateTo,
  account,
  tag,
  onDateRangeChange,
  onAccountChange,
  onTagChange,
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
      {/* Date range row */}
      <div class="flex flex-wrap items-center gap-2">
        <div class="flex items-center gap-1">
          <label class="text-sm opacity-60 shrink-0">From</label>
          <input
            type="date"
            class="input input-bordered input-sm"
            value={dateFrom}
            onInput={(e) => onDateRangeChange(e.target.value, dateTo)}
          />
        </div>
        <div class="flex items-center gap-1">
          <label class="text-sm opacity-60 shrink-0">To</label>
          <input
            type="date"
            class="input input-bordered input-sm"
            value={dateTo ? isoExclusiveToInclusive(dateTo) : ""}
            onInput={(e) => {
              const v = e.target.value;
              onDateRangeChange(dateFrom, v ? inclusiveToExclusiveTo(v) : "");
            }}
          />
        </div>
        <div class="flex flex-wrap gap-1">
          {DATE_PRESETS.map((p) => (
            <button
              key={p.label}
              class={`btn btn-xs ${activePreset === p.label ? "btn-primary" : "btn-ghost"}`}
              onClick={() => applyPreset(p)}
            >
              {p.label}
            </button>
          ))}
        </div>
      </div>

      {/* Filter dropdowns row */}
      <div class="flex flex-wrap gap-2">
        <select
          class="select select-bordered select-sm"
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
          class="select select-bordered select-sm"
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

      {/* Active filter chips */}
      {(account || tag || !activePreset) && (
        <div class="flex flex-wrap gap-1">
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
