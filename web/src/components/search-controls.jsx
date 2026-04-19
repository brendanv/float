import { useState, useEffect, useRef } from "react";
import { ChevronLeft, ChevronRight, Check, X, Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import { cn } from "@/lib/utils";

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
    <div className="mb-4 flex flex-wrap items-center gap-2">
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={() => { const r = shiftMonth(dateFrom, -1); onChange(r.from, r.to); }}
        aria-label="Previous month"
      >
        <ChevronLeft />
      </Button>
      <span className="min-w-32 text-center text-sm font-medium">{displayLabel()}</span>
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={() => { const r = shiftMonth(dateFrom, 1); onChange(r.from, r.to); }}
        aria-label="Next month"
      >
        <ChevronRight />
      </Button>
      <div className="ml-2 flex gap-1">
        {presets.map((p) => {
          const { from, to } = p.fn();
          const isActive = from === dateFrom && to === dateTo;
          return (
            <Button
              key={p.label}
              variant={isActive ? "default" : "ghost"}
              size="xs"
              onClick={() => onChange(from, to)}
            >
              {p.label}
            </Button>
          );
        })}
      </div>
    </div>
  );
}

function DebouncedSearch({ value, onChange }) {
  const [local, setLocal] = useState(value || "");
  const timerRef = useRef(null);

  useEffect(() => {
    setLocal(value || "");
  }, [value]);

  function handleInput(e) {
    const v = e.target.value;
    setLocal(v);
    clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => onChange(v), 300);
  }

  return (
    <div className="relative ml-auto">
      <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
      <Input
        type="search"
        placeholder="Search..."
        value={local}
        onInput={handleInput}
        className="h-8 w-48 pl-7 text-sm"
      />
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
  search,
  onDateRangeChange,
  onAccountChange,
  onTagChange,
  onPayeeChange,
  onSearchChange,
  onQuickFilter,
  accounts,
  tags,
}) {
  const currentFilters = { dateFrom, dateTo, account, tag, status, payee };
  const activeQuickFilter = QUICK_FILTERS.find(qf => qf.isActive(currentFilters));

  function shiftDate(delta) {
    const now = new Date();
    const base = dateFrom || fmtDate(now.getFullYear(), now.getMonth() + 1, 1);
    const r = shiftMonth(base, delta);
    onDateRangeChange(r.from, r.to);
  }

  return (
    <div className="mb-4 flex flex-col gap-2">
      {/* Date range + quick filters row */}
      <div className="flex flex-wrap items-center gap-1">
        <Button variant="ghost" size="icon-sm" onClick={() => shiftDate(-1)} aria-label="Previous month">
          <ChevronLeft />
        </Button>

        {/* Date range dropdown */}
        <Popover>
          <PopoverTrigger
            render={
              <Button variant="outline" size="sm" className="font-normal">
                {formatDateRange(dateFrom, dateTo)}
                <span className="text-xs opacity-40">▾</span>
              </Button>
            }
          />
          <PopoverContent align="start" className="w-56 p-1">
            {DATE_PRESETS.map(p => {
              const { from, to } = p.fn();
              const isActive = from === dateFrom && to === dateTo;
              return (
                <button
                  key={p.label}
                  type="button"
                  className={cn(
                    "flex w-full items-center justify-between rounded-md px-3 py-1.5 text-left text-sm hover:bg-muted",
                    isActive && "font-semibold",
                  )}
                  onClick={() => onDateRangeChange(from, to)}
                >
                  {p.label}
                  {isActive && <Check className="size-3.5 text-primary" />}
                </button>
              );
            })}
            <Separator className="my-1" />
            <div className="flex flex-col gap-1 px-2 pt-1 pb-1">
              <div className="mb-1 text-xs text-muted-foreground">Custom range</div>
              <Input
                type="date"
                className="h-7 w-full"
                value={dateFrom}
                onChange={(e) => onDateRangeChange(e.target.value, dateTo)}
              />
              <Input
                type="date"
                className="h-7 w-full"
                value={dateTo ? isoExclusiveToInclusive(dateTo) : ""}
                onChange={(e) => {
                  const v = e.target.value;
                  onDateRangeChange(dateFrom, v ? inclusiveToExclusiveTo(v) : "");
                }}
              />
            </div>
          </PopoverContent>
        </Popover>

        <Button variant="ghost" size="icon-sm" onClick={() => shiftDate(1)} aria-label="Next month">
          <ChevronRight />
        </Button>

        {/* Quick filters dropdown */}
        {onQuickFilter && (
          <Popover>
            <PopoverTrigger
              render={
                <Button
                  variant={activeQuickFilter?.label !== "All" ? "default" : "ghost"}
                  size="sm"
                  className="ml-1"
                >
                  {activeQuickFilter?.label ?? "Filter"}
                  <span className="text-xs opacity-40">▾</span>
                </Button>
              }
            />
            <PopoverContent align="start" className="w-56 p-1">
              {QUICK_FILTERS.map(qf => {
                const isActive = qf.isActive(currentFilters);
                return (
                  <button
                    key={qf.label}
                    type="button"
                    className={cn(
                      "flex w-full items-center justify-between rounded-md px-3 py-1.5 text-left text-sm hover:bg-muted",
                      isActive && "font-semibold",
                    )}
                    onClick={() => onQuickFilter(qf.apply(currentFilters))}
                    title={qf.description ?? qf.label}
                  >
                    {qf.label}
                    {isActive && <Check className="size-3.5 text-primary" />}
                  </button>
                );
              })}
            </PopoverContent>
          </Popover>
        )}

        <div className="ml-1 flex gap-1">
          <Select
            value={account || ""}
            onValueChange={(v) => onAccountChange(v === "__all__" ? "" : v)}
          >
            <SelectTrigger size="sm">
              <SelectValue placeholder="All accounts">
                {account || "All accounts"}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__all__">All accounts</SelectItem>
              {(accounts || []).map((a) => (
                <SelectItem key={a.fullName} value={a.fullName}>{a.fullName}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select
            value={tag || ""}
            onValueChange={(v) => onTagChange(v === "__any__" ? "" : v)}
          >
            <SelectTrigger size="sm">
              <SelectValue placeholder="Any tag">
                {tag || "Any tag"}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__any__">Any tag</SelectItem>
              {(tags || []).map((t) => (
                <SelectItem key={t} value={t}>{t}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {onSearchChange && (
          <DebouncedSearch value={search} onChange={onSearchChange} />
        )}
      </div>

      {/* Active filter chips */}
      {(account || tag || payee || search) && (
        <div className="flex flex-wrap gap-1">
          {search && (
            <Badge variant="secondary" className="gap-1">
              search: {search}
              <button
                type="button"
                className="cursor-pointer opacity-60 hover:opacity-100"
                onClick={() => onSearchChange("")}
                aria-label="Clear search filter"
              >
                <X className="size-3" />
              </button>
            </Badge>
          )}
          {payee && (
            <Badge variant="secondary" className="gap-1">
              {payee === PAYEE_NONE ? "no payee set" : `payee: ${payee}`}
              <button
                type="button"
                className="cursor-pointer opacity-60 hover:opacity-100"
                onClick={() => onPayeeChange("")}
                aria-label="Clear payee filter"
              >
                <X className="size-3" />
              </button>
            </Badge>
          )}
          {account && (
            <Badge variant="secondary" className="gap-1">
              acct: {account}
              <button
                type="button"
                className="cursor-pointer opacity-60 hover:opacity-100"
                onClick={() => onAccountChange("")}
                aria-label="Clear account filter"
              >
                <X className="size-3" />
              </button>
            </Badge>
          )}
          {tag && (
            <Badge variant="secondary" className="gap-1">
              tag: {tag}
              <button
                type="button"
                className="cursor-pointer opacity-60 hover:opacity-100"
                onClick={() => onTagChange("")}
                aria-label="Clear tag filter"
              >
                <X className="size-3" />
              </button>
            </Badge>
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
