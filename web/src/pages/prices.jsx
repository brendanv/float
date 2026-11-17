import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  useReactTable,
  getCoreRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  getFilteredRowModel,
  createColumnHelper,
  flexRender,
} from "@tanstack/react-table";
import { Tag } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys, invalidateLedgerQueries } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { EmptyState } from "../components/empty-state.jsx";
import { TableSortHeader } from "../components/table-sort-header.jsx";
import { TablePagination } from "../components/table-pagination.jsx";
import { Page, Section } from "../components/page.jsx";
import { Amount } from "../components/amount.jsx";
import { Notice } from "@/components/ui/alert";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Form, FormField, FormRow, FormActions } from "@/components/ui/form";
import { cn, ROW_ACTION } from "@/lib/utils";

function today() {
  return new Date().toISOString().slice(0, 10);
}

function oneYearAgo() {
  const d = new Date();
  d.setFullYear(d.getFullYear() - 1);
  return d.toISOString().slice(0, 10);
}

// A price older than this many days is stale: worth a backfill.
const STALE_DAYS = 7;

function localToday() {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

// daysBetween counts whole days between two "YYYY-MM-DD" dates.
function daysBetween(from, to) {
  return Math.round((Date.parse(to) - Date.parse(from)) / 86_400_000);
}

function dayAfter(date) {
  const next = new Date(date);
  next.setUTCDate(next.getUTCDate() + 1);
  return next.toISOString().slice(0, 10);
}

function formatAge(days) {
  if (days <= 0) return "today";
  if (days === 1) return "1 day";
  if (days < 60) return `${days} days`;
  if (days < 730) return `${Math.round(days / 30.44)} months`;
  return `${Math.round(days / 365.25)} years`;
}

// Rows shown before "Show all": stale held commodities sort first, so the
// collapsed table is the to-do list even with dozens of commodities.
const COLLAPSED_ROWS = 10;

// latestByCommodity joins each commodity's most recent P directive with the
// set of commodities currently held (null while unknown: then every priced
// commodity counts as held). Order: held commodities with no price, then
// held ones oldest price first, then priced commodities no longer held.
// Only held commodities can be stale; a sold position needs no new prices.
function latestByCommodity(prices, held, today) {
  const latest = new Map();
  for (const p of prices) {
    const cur = latest.get(p.commodity);
    if (!cur || p.date > cur.date) latest.set(p.commodity, p);
  }
  const names = new Set([...latest.keys(), ...(held ?? [])]);
  const rank = (r) => (!r.held ? 2 : r.date ? 1 : 0);
  return [...names]
    .map((commodity) => {
      const p = latest.get(commodity);
      const isHeld = held ? held.has(commodity) : true;
      const age = p ? daysBetween(p.date, today) : null;
      return {
        commodity,
        date: p?.date ?? "",
        price: p?.price,
        age,
        held: isHeld,
        stale: isHeld && (age === null || age > STALE_DAYS),
      };
    })
    .sort((a, b) => rank(a) - rank(b)
      || (a.held ? a.date.localeCompare(b.date) : b.date.localeCompare(a.date))
      || a.commodity.localeCompare(b.commodity));
}

// backfillStart is where a backfill for these rows should begin: the day
// after the oldest price, or a year back for a commodity with none.
function backfillStart(rows) {
  const dates = rows.map((r) => (r.date ? dayAfter(r.date) : oneYearAgo()));
  return dates.reduce((min, d) => (d < min ? d : min));
}

function latestSummary(rows, holdingsKnown) {
  const stale = rows.filter((r) => r.stale).length;
  if (!holdingsKnown) {
    return stale > 0 ? `${stale} of ${rows.length} older than ${STALE_DAYS} days` : `All ${rows.length} up to date`;
  }
  const held = rows.filter((r) => r.held).length;
  if (held === 0) return "Nothing held";
  return stale > 0
    ? `${stale} of ${held} held ${stale === 1 ? "needs" : "need"} a price`
    : `All ${held} held up to date`;
}

function LatestPricesTable({ rows, onBackfill }) {
  const [showAll, setShowAll] = useState(false);
  const shown = showAll ? rows : rows.slice(0, COLLAPSED_ROWS);
  return (
    <div className="flex flex-col gap-2">
      <Table>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead>Commodity</TableHead>
            <TableHead className="text-right">Latest price</TableHead>
            <TableHead className="max-sm:hidden">As of</TableHead>
            <TableHead>Age</TableHead>
            <TableHead className="w-px" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {shown.map((r) => (
            <TableRow
              key={r.commodity}
              className="group/row"
              data-slot="latest-price"
              data-stale={r.stale || undefined}
              data-held={r.held || undefined}
            >
              <TableCell>
                <span className={cn("font-mono text-code font-medium", !r.held && "text-muted-foreground")}>
                  {r.commodity}
                </span>
                {!r.held && <span className="ml-2 text-[12px] text-faint">not held</span>}
              </TableCell>
              <TableCell className="text-right">
                {r.price
                  ? <Amount value={{ quantity: r.price.quantity, commodity: r.price.commodity }} />
                  : <span className="text-faint">—</span>}
              </TableCell>
              <TableCell className="text-muted-foreground max-sm:hidden">{r.date || "—"}</TableCell>
              <TableCell className={r.stale ? "text-warning" : "text-muted-foreground"}>
                {r.age === null ? "no price" : formatAge(r.age)}
                {r.stale && <span className="sr-only"> (stale)</span>}
              </TableCell>
              <TableCell className="w-px text-right">
                <Button
                  variant="ghost"
                  size="xs"
                  className={cn("text-muted-foreground", !r.stale && ROW_ACTION)}
                  aria-label={`Backfill ${r.commodity}`}
                  onClick={() => onBackfill([r.commodity], backfillStart([r]))}
                >
                  Backfill
                </Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {rows.length > COLLAPSED_ROWS && (
        <Button variant="ghost" size="sm" className="self-start text-muted-foreground" onClick={() => setShowAll((v) => !v)}>
          {showAll ? "Show fewer" : `Show all ${rows.length} commodities`}
        </Button>
      )}
    </div>
  );
}

const colHelper = createColumnHelper();

const priceColumns = [
  colHelper.accessor("date", {
    id: "date",
    header: ({ column }) => <TableSortHeader column={column}>Date</TableSortHeader>,
    cell: ({ getValue }) => <span className="text-muted-foreground">{getValue()}</span>,
    sortingFn: "alphanumeric",
    filterFn: "includesString",
  }),
  colHelper.accessor("commodity", {
    id: "commodity",
    header: "Commodity",
    cell: ({ getValue }) => <span className="font-mono text-code font-medium">{getValue()}</span>,
    filterFn: "includesString",
  }),
  colHelper.accessor(
    (row) => parseFloat(row.price?.quantity ?? "0"),
    {
      id: "price",
      header: ({ column }) => <TableSortHeader column={column} align="right">Price</TableSortHeader>,
      cell: ({ row }) => {
        const p = row.original;
        if (!p.price) return null;
        // The raw P directive is in the title for copying into a journal.
        return (
          <span
            className="block text-right"
            title={`P ${p.date} ${p.commodity} ${p.price.quantity} ${p.price.commodity}`}
          >
            <Amount value={{ quantity: p.price.quantity, commodity: p.price.commodity }} />
          </span>
        );
      },
      meta: { headerClass: "text-right", cellClass: "text-right" },
    },
  ),
  colHelper.display({
    id: "actions",
    header: "",
    meta: { headerClass: "w-px", cellClass: "w-px text-right" },
    cell: ({ row, table }) => {
      const { onDelete } = table.options.meta;
      const p = row.original;
      if (!p.pid) return null;
      return (
        <Button
          variant="destructive-ghost"
          size="xs"
          className={ROW_ACTION}
          aria-label={`Delete ${p.commodity} price on ${p.date}`}
          onClick={() => onDelete(p.pid)}
        >
          Delete
        </Button>
      );
    },
  }),
];

function PriceHistoryTable({ prices, onDelete }) {
  const [commodityFilter, setCommodityFilter] = useState("");
  const [sorting, setSorting] = useState([{ id: "date", desc: true }]);
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 25 });

  const columnFilters = useMemo(
    () => (commodityFilter ? [{ id: "commodity", value: commodityFilter }] : []),
    [commodityFilter],
  );

  const table = useReactTable({
    data: prices,
    columns: priceColumns,
    state: { sorting, pagination, columnFilters },
    onSortingChange: setSorting,
    onPaginationChange: setPagination,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    meta: { onDelete },
  });

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <Label htmlFor="filter-commodity" className="whitespace-nowrap text-control text-muted-foreground">
          Commodity
        </Label>
        <Input
          id="filter-commodity"
          type="text"
          placeholder="AAPL"
          value={commodityFilter}
          onChange={(e) => {
            setCommodityFilter(e.target.value);
            setPagination((p) => ({ ...p, pageIndex: 0 }));
          }}
          className="h-8 w-40 font-mono text-code"
        />
      </div>

      <Table>
        <TableHeader>
          {table.getHeaderGroups().map((headerGroup) => (
            <TableRow key={headerGroup.id} className="hover:bg-transparent">
              {headerGroup.headers.map((header) => (
                <TableHead
                  key={header.id}
                  className={header.column.columnDef.meta?.headerClass}
                >
                  {header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}
                </TableHead>
              ))}
            </TableRow>
          ))}
        </TableHeader>
        <TableBody>
          {table.getRowModel().rows.length === 0 ? (
            <TableRow>
              <TableCell colSpan={4} className="text-center text-muted-foreground">
                No prices match the current filter.
              </TableCell>
            </TableRow>
          ) : (
            table.getRowModel().rows.map((row) => (
              <TableRow key={row.id} className="group/row">
                {row.getVisibleCells().map((cell) => (
                  <TableCell key={cell.id} className={cell.column.columnDef.meta?.cellClass}>
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </TableCell>
                ))}
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>

      <TablePagination table={table} />
    </div>
  );
}

function priceSummary(prices) {
  const commodities = new Set(prices.map((p) => p.commodity)).size;
  return `${prices.length} ${prices.length === 1 ? "price" : "prices"} · ${commodities} ${commodities === 1 ? "commodity" : "commodities"}`;
}

// BackfillResults reports a backfill run: one critical Notice per failed
// commodity and one info Notice listing what was added.
function BackfillResults({ results }) {
  const added = results.filter((r) => !r.error);
  return (
    <div data-slot="backfill-results" className="flex flex-col gap-2">
      {results.filter((r) => r.error).map((r) => (
        <Notice key={r.commodity} variant="critical">
          <span className="font-mono text-code font-medium">{r.commodity}</span>:{" "}
          {r.error.message ?? String(r.error)}
        </Notice>
      ))}
      {added.length > 0 && (
        <Notice variant="info">
          {added.map((r, i) => (
            <span key={r.commodity}>
              {i > 0 && " · "}
              <span className="font-mono text-code font-medium">{r.commodity}</span>: added {r.added}{" "}
              {r.added === 1 ? "price" : "prices"}
              {r.skipped > 0 && ` (${r.skipped} already existed)`}
            </span>
          ))}
        </Notice>
      )}
    </div>
  );
}

export function PricesPage() {
  const queryClient = useQueryClient();

  const { data: pricesData, isLoading, error: fetchError } = useQuery({
    queryKey: queryKeys.prices(),
    queryFn: () => ledgerClient.listPrices({}),
  });

  // Held commodities (lots aggregated, currencies and zero positions
  // dropped by floatd), so sold positions don't read as stale. Same query as
  // Investments › Holdings with no account filter.
  const { data: holdingsData } = useQuery({
    queryKey: queryKeys.portfolioHoldings(""),
    queryFn: () => ledgerClient.getPortfolioHoldings({ accountPrefix: "" }),
  });

  const [date, setDate] = useState(today);
  const [commodity, setCommodity] = useState("");
  const [quantity, setQuantity] = useState("");
  const [currency, setCurrency] = useState("USD");
  const [formError, setFormError] = useState(null);

  const [backfillCommodities, setBackfillCommodities] = useState("");
  const [backfillStartDate, setBackfillStartDate] = useState(oneYearAgo);
  const [backfillEndDate, setBackfillEndDate] = useState(today);
  const [backfillCurrency, setBackfillCurrency] = useState("USD");
  const [backfillResults, setBackfillResults] = useState(null);
  const [backfillError, setBackfillError] = useState(null);
  const [backfillPending, setBackfillPending] = useState(false);
  const [backfillProgress, setBackfillProgress] = useState(null);

  const addMutation = useMutation({
    mutationFn: (vars) => ledgerClient.addPrice(vars),
    onSuccess: () => {
      setCommodity("");
      setQuantity("");
      setFormError(null);
      invalidateLedgerQueries(queryClient);
    },
    onError: (err) => setFormError(err),
  });

  const deleteMutation = useMutation({
    mutationFn: ({ pid }) => ledgerClient.deletePrice({ pid }),
    onSuccess: () => invalidateLedgerQueries(queryClient),
    onError: (err) => setFormError(err),
  });

  function handleSubmit(e) {
    e.preventDefault();
    setFormError(null);
    addMutation.mutate({ date, commodity: commodity.trim(), quantity: quantity.trim(), currency: currency.trim() });
  }

  function handleDelete(pid) {
    deleteMutation.mutate({ pid });
  }

  async function handleBackfill(e) {
    e.preventDefault();
    const commodities = backfillCommodities
      .split(/[\s,]+/)
      .map((s) => s.trim())
      .filter(Boolean);
    if (commodities.length === 0) return;

    setBackfillPending(true);
    setBackfillResults(null);
    setBackfillError(null);
    setBackfillProgress({ current: 0, total: commodities.length });

    const results = [];
    let firstError = null;
    for (let i = 0; i < commodities.length; i++) {
      setBackfillProgress({ current: i + 1, total: commodities.length });
      try {
        const data = await ledgerClient.backfillPrices({
          commodity: commodities[i],
          startDate: backfillStartDate,
          endDate: backfillEndDate,
          currency: backfillCurrency.trim(),
        });
        results.push({ commodity: commodities[i], added: data.prices?.length ?? 0, skipped: data.skippedCount ?? 0 });
      } catch (err) {
        if (!firstError) firstError = err;
        results.push({ commodity: commodities[i], error: err });
      }
    }

    setBackfillPending(false);
    setBackfillProgress(null);
    setBackfillResults(results);
    setBackfillError(firstError);
    invalidateLedgerQueries(queryClient);
  }

  // prefillBackfill fills the backfill form and brings it into view.
  function prefillBackfill(commodities, startDate) {
    setBackfillCommodities(commodities.join(", "));
    setBackfillStartDate(startDate);
    setBackfillEndDate(today());
    const input = document.getElementById("backfill-commodities");
    input?.scrollIntoView({ block: "center", behavior: "smooth" });
    input?.focus({ preventScroll: true });
  }

  const heldSymbols = holdingsData ? new Set(holdingsData.holdings.map((h) => h.symbol)) : null;
  const latest = latestByCommodity(pricesData?.prices ?? [], heldSymbols, localToday());
  const stale = latest.filter((r) => r.stale);

  function handlePrefill() {
    const prices = pricesData?.prices;
    if (!prices?.length) return;
    const commodities = [...new Set(prices.map((p) => p.commodity))].sort();
    setBackfillCommodities(commodities.join(", "));
    const maxDate = prices.reduce((max, p) => (p.date > max ? p.date : max), "");
    if (maxDate) {
      const next = new Date(maxDate);
      next.setDate(next.getDate() + 1);
      setBackfillStartDate(next.toISOString().slice(0, 10));
    }
    setBackfillEndDate(today());
  }

  return (
    <Page>
      {latest.length > 0 && (
        <Section
          title="Latest by commodity"
          description={latestSummary(latest, !!heldSymbols)}
          action={stale.length > 0 && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => prefillBackfill(stale.map((r) => r.commodity), backfillStart(stale))}
            >
              Backfill stale
            </Button>
          )}
        >
          <LatestPricesTable rows={latest} onBackfill={prefillBackfill} />
        </Section>
      )}

      <Section title="Add price">
        <Form onSubmit={handleSubmit}>
          {formError && <ErrorBanner error={formError} />}
          <FormRow cols={4} className="sm:grid-cols-2 lg:grid-cols-4">
            <FormField label="Date" htmlFor="price-date">
              <Input
                id="price-date"
                type="date"
                value={date}
                onChange={(e) => setDate(e.target.value)}
                required
              />
            </FormField>
            <FormField label="Commodity" htmlFor="price-commodity">
              <Input
                id="price-commodity"
                type="text"
                placeholder="AAPL"
                value={commodity}
                onChange={(e) => setCommodity(e.target.value)}
                required
              />
            </FormField>
            <FormField label="Price" htmlFor="price-quantity">
              <Input
                id="price-quantity"
                type="number"
                step="any"
                placeholder="178.50"
                value={quantity}
                onChange={(e) => setQuantity(e.target.value)}
                required
              />
            </FormField>
            <FormField label="Currency" htmlFor="price-currency">
              <Input
                id="price-currency"
                type="text"
                value={currency}
                onChange={(e) => setCurrency(e.target.value)}
                required
              />
            </FormField>
          </FormRow>
          <FormActions>
            <Button
              type="submit"
              isLoading={addMutation.isPending}
              loadingText="Adding..."
            >
              Add Price
            </Button>
          </FormActions>
        </Form>
      </Section>

      <Section title="Backfill price history" description="Daily closes from Alpha Vantage">
        <Form onSubmit={handleBackfill}>
          {backfillError && <ErrorBanner error={backfillError} />}
          {backfillResults && <BackfillResults results={backfillResults} />}
          <FormRow cols={4} className="sm:grid-cols-2 lg:grid-cols-4">
            <FormField label="Commodities" htmlFor="backfill-commodities">
              <Input
                id="backfill-commodities"
                type="text"
                placeholder="AAPL or AAPL, MSFT, GOOG"
                value={backfillCommodities}
                onChange={(e) => setBackfillCommodities(e.target.value)}
                required
              />
            </FormField>
            <FormField label="Start date" htmlFor="backfill-start">
              <Input
                id="backfill-start"
                type="date"
                value={backfillStartDate}
                onChange={(e) => setBackfillStartDate(e.target.value)}
                required
              />
            </FormField>
            <FormField label="End date" htmlFor="backfill-end">
              <Input
                id="backfill-end"
                type="date"
                value={backfillEndDate}
                onChange={(e) => setBackfillEndDate(e.target.value)}
                required
              />
            </FormField>
            <FormField label="Currency" htmlFor="backfill-currency">
              <Input
                id="backfill-currency"
                type="text"
                value={backfillCurrency}
                onChange={(e) => setBackfillCurrency(e.target.value)}
                required
              />
            </FormField>
          </FormRow>
          <FormActions>
            <Button
              type="button"
              variant="outline"
              onClick={handlePrefill}
              disabled={!pricesData?.prices?.length}
            >
              Prefill from existing
            </Button>
            <Button
              type="submit"
              isLoading={backfillPending}
              loadingText={
                backfillProgress
                  ? `Fetching ${backfillProgress.current}/${backfillProgress.total}...`
                  : "Backfilling..."
              }
            >
              Backfill
            </Button>
          </FormActions>
        </Form>
      </Section>

      <Section
        title="Price history"
        description={pricesData?.prices?.length ? priceSummary(pricesData.prices) : undefined}
      >
        {isLoading && <Loading />}
        {fetchError && <ErrorBanner error={fetchError} />}
        {pricesData && (
          pricesData.prices?.length > 0 ? (
            <PriceHistoryTable prices={pricesData.prices} onDelete={handleDelete} />
          ) : (
            <EmptyState
              icon={Tag}
              title="No prices recorded yet"
              description="Add a price above or backfill from AlphaVantage."
            />
          )
        )}
      </Section>
    </Page>
  );
}
