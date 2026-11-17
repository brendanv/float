import { useState, useEffect, useRef } from "react";
import { Check, ChevronDown, X, Search, CalendarDays } from "lucide-react";
import { useIsMobile } from "@/hooks/use-mobile.js";
import { Calendar } from "@/components/ui/calendar";
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
import { Separator } from "@/components/ui/separator";
import { cn } from "@/lib/utils";
import { filterAndSortAccountNames, splitAccount } from "@/lib/accounts";
import { isHledgerQuery } from "@/lib/hledger-query";
import { DATE_PRESETS, PAYEE_NONE } from "./search-presets.js";

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
function shortDate(d) {
  return `${SHORT_MONTHS[d.getMonth()]} ${d.getDate()}, ${d.getFullYear()}`;
}

function formatDateRange(dateFrom, dateTo) {
  if (!dateFrom) {
    if (!dateTo) return "All time";
    return `Until ${shortDate(new Date(isoExclusiveToInclusive(dateTo) + "T00:00:00"))}`;
  }
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
  return `Since ${shortDate(from)}`;
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

export function DateRangePicker({ dateFrom, dateTo, onChange, align = "start", embedded = false }) {
  const [open, setOpen] = useState(false);
  const isMobile = useIsMobile();
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
            className={cn(
              "flex items-center gap-2 outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring aria-expanded:bg-muted",
              embedded
                ? cn(CHIP, CHIP_OFF, "touch-target gap-1.5 px-2.5")
                : "h-8 pointer-coarse:min-h-10 rounded-md border border-input px-3 text-sm font-medium",
            )}
          >
            <CalendarDays
              className={cn("shrink-0 text-muted-foreground", embedded ? "size-3.5" : "size-4")}
            />
            <span className="whitespace-nowrap">{formatDateRange(dateFrom, dateTo)}</span>
            {embedded
              ? <ChevronDown className="size-3.5 shrink-0 opacity-60" />
              : <span className="shrink-0 text-xs opacity-40">▾</span>}
          </button>
        }
      />
      <PopoverContent
        align={align}
        // On phones the presets stack above a single-month calendar (40px
        // cells don't fit beside the sidebar) and the popover scrolls.
        className="w-auto flex-row gap-0 p-0 max-md:max-h-(--available-height) max-md:flex-col max-md:overflow-y-auto"
      >
        {/* Presets sidebar */}
        <div className="flex min-w-[140px] flex-col border-r p-2 max-md:border-r-0 max-md:border-b">
          <p className="mb-1.5 px-2 text-xs font-medium text-muted-foreground">Presets</p>
          <div className="grid max-md:grid-cols-2">
            {DATE_PRESETS.map((p) => {
              const { from, to } = p.fn();
              const isActive = from === dateFrom && to === dateTo;
              return (
                <button
                  key={p.label}
                  type="button"
                  className={cn(
                    "flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm hover:bg-muted pointer-coarse:py-2.5",
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
          </div>
          <Separator className="my-2" />
          <p className="mb-1.5 px-2 text-xs font-medium text-muted-foreground">Custom range</p>
          <div className="flex flex-col gap-1 px-2 max-md:flex-row max-md:gap-2">
            <div className="flex min-w-0 flex-col gap-0.5 max-md:flex-1">
              <span className="text-xs text-muted-foreground">From</span>
              <Input
                type="date"
                className="h-7 text-xs"
                value={dateFrom || ""}
                onChange={(e) => onChange(e.target.value, dateTo)}
              />
            </div>
            <div className="flex min-w-0 flex-col gap-0.5 max-md:flex-1">
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
            numberOfMonths={isMobile ? 1 : 2}
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

export function PeriodBar({ dateFrom, dateTo, onChange, className }) {
  return (
    <div className={cn("flex items-center gap-2", className)}>
      <DateRangePicker dateFrom={dateFrom} dateTo={dateTo} onChange={onChange} />
    </div>
  );
}


// ── transaction toolbar ────────────────────────────────────────────────────

// Filter chips: pills on paper; an active filter is accent-soft.
const CHIP =
  "inline-flex h-7 shrink-0 items-center whitespace-nowrap rounded-full border text-[12.5px] transition-colors";
const CHIP_OFF =
  "border-input bg-background text-muted-foreground hover:bg-muted hover:text-foreground";
const CHIP_ON = "border-transparent bg-accent font-medium text-primary";
// Chips keep their 28px pill on touch devices; `touch-target` extends the hit
// area to 40px (the chip row pads itself on coarse pointers so its sideways
// scroll doesn't clip that area).
const CHIP_BUTTON =
  "touch-target inline-flex h-full min-w-0 items-center gap-1.5 rounded-full px-2.5 outline-none focus-visible:ring-2 focus-visible:ring-ring";

// FilterChip is the pill frame around a chip's button (`children`). When
// the filter is active and clearable it adds a trailing clear button.
function FilterChip({ active, onClear, clearLabel, className, children }) {
  return (
    <span data-slot="filter-chip" data-active={active || undefined} className={cn(CHIP, active ? CHIP_ON : CHIP_OFF, className)}>
      {children}
      {active && onClear && (
        <button
          type="button"
          className="touch-target -ml-1.5 mr-1 inline-flex size-5 items-center justify-center rounded-full outline-none hover:bg-primary/10 focus-visible:ring-2 focus-visible:ring-ring"
          onClick={onClear}
          aria-label={clearLabel}
          title={clearLabel}
        >
          <X className="size-3" />
        </button>
      )}
    </span>
  );
}

// ChipSelect is a filter chip that opens a list of options, with a search
// box when `searchable`. `value` "" means no filter (shows `placeholder`).
function ChipSelect({ value, onChange, options, placeholder, allLabel, searchPlaceholder, searchable = false, display, clearLabel, emptyText }) {
  const [open, setOpen] = useState(false);
  const selected = options.find((o) => o.value === value);
  const active = !!value;

  function pick(next) {
    onChange(next);
    setOpen(false);
  }

  return (
    <FilterChip active={active} onClear={() => onChange("")} clearLabel={clearLabel}>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger
          render={
            <button type="button" role="combobox" aria-expanded={open} className={cn(CHIP_BUTTON, active && "pr-2")}>
              <span className="max-w-48 truncate" title={selected?.title}>
                {active ? (display ? display(value) : selected?.label ?? value) : placeholder}
              </span>
              {!active && <ChevronDown className="size-3.5 shrink-0 opacity-60" />}
            </button>
          }
        />
        <PopoverContent align="start" className="w-60 p-0">
          <Command>
            {searchable && <CommandInput placeholder={searchPlaceholder} />}
            <CommandList>
              <CommandEmpty>{emptyText}</CommandEmpty>
              <CommandGroup>
                <CommandItem value="__all__" onSelect={() => pick("")} data-checked={!value ? "true" : undefined}>
                  {allLabel}
                </CommandItem>
                {options.map((o) => (
                  <CommandItem
                    key={o.value}
                    value={o.value}
                    onSelect={() => pick(o.value)}
                    data-checked={value === o.value ? "true" : undefined}
                  >
                    {o.label}
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </FilterChip>
  );
}

// SearchField is the one wide search box. Plain words search descriptions;
// text with an hledger query keyword is sent as a query (and set in mono).
function SearchField({ value, onChange }) {
  const [local, setLocal] = useState(value || "");
  const timerRef = useRef(null);
  const isMobile = useIsMobile();

  useEffect(() => {
    setLocal(value || "");
  }, [value]);

  useEffect(() => () => clearTimeout(timerRef.current), []);

  function handleInput(e) {
    const v = e.target.value;
    setLocal(v);
    clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => onChange(v), 300);
  }

  return (
    <div className="relative flex min-w-0 flex-1 items-center">
      <Search className="pointer-events-none absolute left-2.5 size-3.5 text-faint" />
      <Input
        type="search"
        aria-label="Search transactions"
        placeholder={isMobile ? "Search or query" : "Search descriptions, or type an hledger query like acct:dining amt:>50"}
        value={local}
        onInput={handleInput}
        className={cn("h-8 bg-popover pl-8 dark:bg-popover", isHledgerQuery(local) && "font-mono text-code")}
      />
    </div>
  );
}

const STATUS_OPTIONS = [
  { value: "reviewed", label: "Reviewed" },
  { value: "nopayee", label: "No payee set" },
];

// SearchControls is the transactions toolbar: one search field, then the
// filter chips (period, account, "Needs review", status, tags, payee) and
// an optional summary on the right.
export function SearchControls({
  dateFrom,
  dateTo,
  account,
  tag,
  status,
  payee,
  search,
  searchKey,
  onDateRangeChange,
  onAccountChange,
  onTagChange,
  onPayeeChange,
  onSearchChange,
  onStatusChange,
  accounts,
  tags,
  reviewCount,
  summary,
}) {
  const needsReview = status === "unreviewed";
  // "Needs review" has its own chip; the Status chip covers the rest.
  const statusValue = payee === PAYEE_NONE ? "nopayee" : status === "reviewed" ? "reviewed" : "";

  function setStatusValue(next) {
    if (next === "nopayee") {
      onStatusChange("");
      onPayeeChange(PAYEE_NONE);
      return;
    }
    onStatusChange(next);
    if (payee === PAYEE_NONE) onPayeeChange("");
  }

  function toggleNeedsReview() {
    if (needsReview) {
      onStatusChange("");
      return;
    }
    // The chip's count is over all dates, so turning it on shows all time
    // too and the list matches the count.
    onStatusChange("unreviewed");
    onDateRangeChange("", "");
    if (payee === PAYEE_NONE) onPayeeChange("");
  }

  const accountOptions = filterAndSortAccountNames(accounts).map((name) => ({ value: name, label: name, title: name }));
  const tagOptions = (tags || []).map((t) => ({ value: t, label: `#${t}` }));

  return (
    <div data-slot="transaction-toolbar" className="flex flex-col gap-2.5">
      {onSearchChange && <SearchField key={searchKey} value={search} onChange={onSearchChange} />}
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        {/* Phones scroll the chip row sideways rather than wrapping it. */}
        <div className="-mx-4 flex w-[calc(100%+2rem)] min-w-0 items-center gap-1.5 overflow-x-auto px-4 [scrollbar-width:none] pointer-coarse:-my-1.5 pointer-coarse:py-1.5 sm:mx-0 sm:w-auto sm:flex-1 sm:flex-wrap sm:overflow-visible sm:px-0">
          <DateRangePicker dateFrom={dateFrom} dateTo={dateTo} onChange={onDateRangeChange} embedded />
          <ChipSelect
            value={account}
            onChange={onAccountChange}
            options={accountOptions}
            placeholder="Any account"
            allLabel="All accounts"
            searchable
            searchPlaceholder="Search accounts..."
            emptyText="No account found."
            display={(v) => splitAccount(v).leaf}
            clearLabel="Clear account filter"
          />
          <FilterChip active={needsReview}>
            <button type="button" aria-pressed={needsReview} onClick={toggleNeedsReview} className={CHIP_BUTTON}>
              Needs review{reviewCount > 0 ? ` · ${reviewCount}` : ""}
            </button>
          </FilterChip>
          <ChipSelect
            value={statusValue}
            onChange={setStatusValue}
            options={STATUS_OPTIONS}
            placeholder="Status"
            allLabel="Any status"
            emptyText="No status."
            clearLabel="Clear status filter"
          />
          <ChipSelect
            value={tag}
            onChange={onTagChange}
            options={tagOptions}
            placeholder="Tags"
            allLabel="Any tag"
            searchable
            searchPlaceholder="Search tags..."
            emptyText="No tag found."
            clearLabel="Clear tag filter"
          />
          {payee && payee !== PAYEE_NONE && (
            <FilterChip active onClear={() => onPayeeChange("")} clearLabel="Clear payee filter">
              <span className={cn(CHIP_BUTTON, "pr-2")}>payee: {payee}</span>
            </FilterChip>
          )}
        </div>
        {summary}
      </div>
    </div>
  );
}
