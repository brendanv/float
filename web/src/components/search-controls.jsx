import { useState, useEffect, useRef } from "react";
import { Check, X, Search, CalendarDays } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Calendar } from "@/components/ui/calendar";
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
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import { cn } from "@/lib/utils";
import { DATE_PRESETS, QUICK_FILTERS, PAYEE_NONE } from "./search-presets.js";

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

export function DateRangePicker({ dateFrom, dateTo, onChange, align = "start" }) {
  const [open, setOpen] = useState(false);
  const [month, setMonth] = useState(() => {
    if (dateFrom) return new Date(dateFrom + "T00:00:00");
    const now = new Date();
    now.setDate(1);
    return now;
  });

  // Sync calendar month when dateFrom changes (e.g. after preset selection)
  useEffect(() => {
    if (dateFrom) setMonth(new Date(dateFrom + "T00:00:00"));
  }, [dateFrom]);

  const calendarSelected = (() => {
    const from = dateFrom ? new Date(dateFrom + "T00:00:00") : undefined;
    const toInclStr = dateTo ? isoExclusiveToInclusive(dateTo) : undefined;
    const to = toInclStr ? new Date(toInclStr + "T00:00:00") : undefined;
    if (!from) return undefined;
    return { from, to };
  })();

  function handlePreset(from, to) {
    onChange(from, to);
    setOpen(false);
  }

  function handleCalendarSelect(range) {
    if (!range) { onChange("", ""); return; }
    const fromStr = range.from
      ? fmtDate(range.from.getFullYear(), range.from.getMonth() + 1, range.from.getDate())
      : "";
    let toStr = "";
    if (range.to) {
      const toEx = new Date(range.to);
      toEx.setDate(toEx.getDate() + 1);
      toStr = fmtDate(toEx.getFullYear(), toEx.getMonth() + 1, toEx.getDate());
    }
    onChange(fromStr, toStr);
    if (range.from && range.to) setOpen(false);
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <button
            type="button"
            className="flex h-8 items-center gap-2 rounded-none border border-border px-3 text-sm font-medium hover:bg-muted"
          >
            <CalendarDays className="size-4 text-muted-foreground" />
            <span>{formatDateRange(dateFrom, dateTo)}</span>
            <span className="text-xs opacity-40">▾</span>
          </button>
        }
      />
      <PopoverContent
        align={align}
        className="w-auto flex-row gap-0 p-0"
      >
        {/* Presets sidebar */}
        <div className="flex min-w-[140px] flex-col border-r p-2">
          <p className="mb-1.5 px-2 text-xs font-medium text-muted-foreground">Presets</p>
          {DATE_PRESETS.map((p) => {
            const { from, to } = p.fn();
            const isActive = from === dateFrom && to === dateTo;
            return (
              <button
                key={p.label}
                type="button"
                className={cn(
                  "flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm hover:bg-muted",
                  isActive && "font-semibold",
                )}
                onClick={() => handlePreset(from, to)}
              >
                {isActive
                  ? <Check className="size-3.5 text-primary shrink-0" />
                  : <span className="size-3.5 shrink-0" />}
                {p.label}
              </button>
            );
          })}
          <Separator className="my-2" />
          <p className="mb-1.5 px-2 text-xs font-medium text-muted-foreground">Custom range</p>
          <div className="flex flex-col gap-1 px-2">
            <div className="flex flex-col gap-0.5">
              <span className="text-xs text-muted-foreground">From</span>
              <Input
                type="date"
                className="h-7 text-xs"
                value={dateFrom || ""}
                onChange={(e) => onChange(e.target.value, dateTo)}
              />
            </div>
            <div className="flex flex-col gap-0.5">
              <span className="text-xs text-muted-foreground">To</span>
              <Input
                type="date"
                className="h-7 text-xs"
                value={dateTo ? isoExclusiveToInclusive(dateTo) : ""}
                onChange={(e) => {
                  const v = e.target.value;
                  onChange(dateFrom, v ? inclusiveToExclusiveTo(v) : "");
                }}
              />
            </div>
          </div>
        </div>

        {/* Calendar */}
        <div className="p-2">
          <Calendar
            mode="range"
            numberOfMonths={2}
            selected={calendarSelected}
            onSelect={handleCalendarSelect}
            month={month}
            onMonthChange={setMonth}
          />
        </div>
      </PopoverContent>
    </Popover>
  );
}

export function PeriodBar({ dateFrom, dateTo, onChange }) {
  return (
    <div className="mb-4 flex items-center gap-2">
      <DateRangePicker dateFrom={dateFrom} dateTo={dateTo} onChange={onChange} />
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
    <div className="relative flex flex-1 items-center">
      <Search className="pointer-events-none absolute left-2 size-3.5 text-muted-foreground" />
      <Input
        type="search"
        placeholder="Search..."
        value={local}
        onInput={handleInput}
        className="h-7 flex-1 border-0 bg-transparent pl-7 shadow-none focus-visible:ring-0"
      />
    </div>
  );
}

function AccountCombobox({ value, onChange, accounts }) {
  const [open, setOpen] = useState(false);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <button
            type="button"
            role="combobox"
            aria-expanded={open}
            className={cn(
              "flex h-7 items-center gap-1 px-2.5 text-xs hover:bg-muted",
              value ? "font-semibold" : "",
            )}
          >
            <span className="truncate">{value || "All accounts"}</span>
            <span className="text-xs opacity-40">▾</span>
          </button>
        }
      />
      <PopoverContent align="start" className="w-56 p-0">
        <Command>
          <CommandInput placeholder="Search accounts..." />
          <CommandList>
            <CommandEmpty>No account found.</CommandEmpty>
            <CommandGroup>
              <CommandItem
                value="__all__"
                onSelect={() => { onChange(""); setOpen(false); }}
                data-checked={!value ? "true" : undefined}
              >
                All accounts
              </CommandItem>
              {(accounts || []).map((a) => (
                <CommandItem
                  key={a.fullName}
                  value={a.fullName}
                  onSelect={() => { onChange(a.fullName); setOpen(false); }}
                  data-checked={value === a.fullName ? "true" : undefined}
                >
                  {a.fullName}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
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

  const hasActiveFilters = account || tag || payee || search;

  return (
    <div className="mb-4 flex flex-col">
      <div className="flex items-center border border-border">
        <DateRangePicker
          dateFrom={dateFrom}
          dateTo={dateTo}
          onChange={onDateRangeChange}
        />

        <Separator orientation="vertical" className="h-5" />

        {onQuickFilter && (
          <>
            <Popover>
              <PopoverTrigger
                render={
                  <button
                    type="button"
                    className={cn(
                      "flex h-7 items-center gap-1 px-2.5 text-xs hover:bg-muted",
                      activeQuickFilter?.label !== "All" ? "font-semibold" : "",
                    )}
                  >
                    {activeQuickFilter?.label ?? "Filter"}
                    <span className="text-xs opacity-40">▾</span>
                  </button>
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
            <Separator orientation="vertical" className="h-5" />
          </>
        )}

        <AccountCombobox
          value={account}
          onChange={onAccountChange}
          accounts={accounts}
        />

        <Separator orientation="vertical" className="h-5" />

        <Select
          value={tag || ""}
          onValueChange={(v) => onTagChange(v === "__any__" ? "" : v)}
        >
          <SelectTrigger size="sm" className="border-0 bg-transparent shadow-none hover:bg-muted focus-visible:ring-0 dark:bg-transparent dark:hover:bg-muted">
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

        {onSearchChange && (
          <>
            <Separator orientation="vertical" className="h-5" />
            <DebouncedSearch value={search} onChange={onSearchChange} />
          </>
        )}
      </div>

      {hasActiveFilters && (
        <div className="flex flex-wrap items-center gap-1 border border-t-0 border-border px-2 py-1.5">
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
