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
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { EmptyState } from "../components/empty-state.jsx";
import { TableSortHeader } from "../components/table-sort-header.jsx";
import { TablePagination } from "../components/table-pagination.jsx";
import { Page, PageCard } from "../components/page.jsx";
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

function today() {
  return new Date().toISOString().slice(0, 10);
}

function oneYearAgo() {
  const d = new Date();
  d.setFullYear(d.getFullYear() - 1);
  return d.toISOString().slice(0, 10);
}

const colHelper = createColumnHelper();

const priceColumns = [
  colHelper.accessor("date", {
    id: "date",
    header: ({ column }) => <TableSortHeader column={column}>Date</TableSortHeader>,
    cell: ({ getValue }) => <span className="font-mono">{getValue()}</span>,
    sortingFn: "alphanumeric",
    filterFn: "includesString",
  }),
  colHelper.accessor("commodity", {
    id: "commodity",
    header: "Commodity",
    cell: ({ getValue }) => <span className="font-mono font-semibold">{getValue()}</span>,
    filterFn: "includesString",
  }),
  colHelper.accessor(
    (row) => parseFloat(row.price?.quantity ?? "0"),
    {
      id: "price",
      header: ({ column }) => <TableSortHeader column={column} align="right">Price</TableSortHeader>,
      cell: ({ row }) => (
        <span className="block text-right font-mono">
          {row.original.price?.quantity} {row.original.price?.commodity}
        </span>
      ),
      meta: { headerClass: "text-right", cellClass: "text-right" },
    },
  ),
  colHelper.display({
    id: "actions",
    header: "",
    cell: ({ row, table }) => {
      const { onDelete } = table.options.meta;
      const p = row.original;
      if (!p.pid) return null;
      return (
        <Button
          variant="destructive-ghost"
          size="xs"
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
        <Label htmlFor="filter-commodity" className="whitespace-nowrap text-sm">
          Filter by commodity:
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
          className="h-8 w-40"
        />
      </div>

      <Table>
        <TableHeader>
          {table.getHeaderGroups().map((headerGroup) => (
            <TableRow key={headerGroup.id}>
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
              <TableRow key={row.id}>
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

export function PricesPage() {
  const queryClient = useQueryClient();

  const { data: pricesData, isLoading, error: fetchError } = useQuery({
    queryKey: queryKeys.prices(),
    queryFn: () => ledgerClient.listPrices({}),
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
      queryClient.invalidateQueries({ queryKey: queryKeys.prices() });
    },
    onError: (err) => setFormError(err),
  });

  const deleteMutation = useMutation({
    mutationFn: ({ pid }) => ledgerClient.deletePrice({ pid }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.prices() }),
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
    queryClient.invalidateQueries({ queryKey: queryKeys.prices() });
  }

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
      <PageHeader title="Commodity Prices" />

      <PageCard title="Add Price">
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
      </PageCard>

      <PageCard title="Backfill Price History">
        <Form onSubmit={handleBackfill}>
          {backfillError && <ErrorBanner error={backfillError} />}
          {backfillResults && (
            <div className="flex flex-col gap-1">
              {backfillResults.map((r) =>
                r.error ? (
                  <p key={r.commodity} className="text-xs text-destructive">
                    {r.commodity}: {r.error.message ?? String(r.error)}
                  </p>
                ) : (
                  <p key={r.commodity} className="text-xs text-success">
                    {r.commodity}: added {r.added} {r.added === 1 ? "price" : "prices"}
                    {r.skipped > 0 && ` (${r.skipped} already existed)`}.
                  </p>
                ),
              )}
            </div>
          )}
          <FormRow cols={4} className="sm:grid-cols-2 lg:grid-cols-4">
            <FormField label="Commodity / Commodities" htmlFor="backfill-commodities">
              <Input
                id="backfill-commodities"
                type="text"
                placeholder="AAPL or AAPL, MSFT, GOOG"
                value={backfillCommodities}
                onChange={(e) => setBackfillCommodities(e.target.value)}
                required
              />
            </FormField>
            <FormField label="Start Date" htmlFor="backfill-start">
              <Input
                id="backfill-start"
                type="date"
                value={backfillStartDate}
                onChange={(e) => setBackfillStartDate(e.target.value)}
                required
              />
            </FormField>
            <FormField label="End Date" htmlFor="backfill-end">
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
      </PageCard>

      <PageCard title="Price History">
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
      </PageCard>
    </Page>
  );
}
