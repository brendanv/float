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
import { ArrowUp, ArrowDown, ArrowUpDown, ChevronLeft, ChevronRight, Loader2 } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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
import {
  Pagination,
  PaginationContent,
  PaginationItem,
} from "@/components/ui/pagination";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";

function today() {
  return new Date().toISOString().slice(0, 10);
}

function oneYearAgo() {
  const d = new Date();
  d.setFullYear(d.getFullYear() - 1);
  return d.toISOString().slice(0, 10);
}

function SortableHeader({ column, children, align = "left" }) {
  const sorted = column.getIsSorted();
  return (
    <button
      className={cn(
        "inline-flex cursor-pointer select-none items-center gap-1 rounded px-1 -mx-1 py-0.5 transition-colors hover:text-foreground",
        align === "right" && "ml-auto flex-row-reverse",
        sorted ? "text-foreground" : "text-muted-foreground",
      )}
      onClick={() => column.toggleSorting(sorted === "asc")}
    >
      {children}
      {sorted === "asc" ? (
        <ArrowUp className="size-3 shrink-0" />
      ) : sorted === "desc" ? (
        <ArrowDown className="size-3 shrink-0" />
      ) : (
        <ArrowUpDown className="size-3 shrink-0 opacity-40" />
      )}
    </button>
  );
}

const colHelper = createColumnHelper();

const priceColumns = [
  colHelper.accessor("date", {
    id: "date",
    header: ({ column }) => <SortableHeader column={column}>Date</SortableHeader>,
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
      header: ({ column }) => <SortableHeader column={column} align="right">Price</SortableHeader>,
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
          variant="ghost"
          size="xs"
          className="text-destructive"
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

  const { pageIndex } = table.getState().pagination;
  const total = table.getFilteredRowModel().rows.length;
  const rangeStart = total === 0 ? 0 : pageIndex * pagination.pageSize + 1;
  const rangeEnd = Math.min((pageIndex + 1) * pagination.pageSize, total);

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

      {total > 0 && (
        <div className="flex w-full flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <Label className="whitespace-nowrap text-sm text-muted-foreground">Rows per page:</Label>
            <Select
              value={String(pagination.pageSize)}
              onValueChange={(val) => {
                table.setPageSize(Number(val));
                setPagination((p) => ({ ...p, pageIndex: 0 }));
              }}
            >
              <SelectTrigger className="h-8 w-16">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="10">10</SelectItem>
                <SelectItem value="25">25</SelectItem>
                <SelectItem value="50">50</SelectItem>
                <SelectItem value="100">100</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="flex items-center gap-2">
            <span className="whitespace-nowrap text-sm text-muted-foreground">
              {rangeStart}–{rangeEnd} of {total}
            </span>
            <Pagination>
              <PaginationContent>
                <PaginationItem>
                  <Button
                    aria-label="Go to previous page"
                    size="icon"
                    variant="ghost"
                    className="size-8"
                    onClick={() => table.previousPage()}
                    disabled={!table.getCanPreviousPage()}
                  >
                    <ChevronLeft className="size-4" />
                  </Button>
                </PaginationItem>
                <PaginationItem>
                  <Button
                    aria-label="Go to next page"
                    size="icon"
                    variant="ghost"
                    className="size-8"
                    onClick={() => table.nextPage()}
                    disabled={!table.getCanNextPage()}
                  >
                    <ChevronRight className="size-4" />
                  </Button>
                </PaginationItem>
              </PaginationContent>
            </Pagination>
          </div>
        </div>
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

  const [date, setDate] = useState(today);
  const [commodity, setCommodity] = useState("");
  const [quantity, setQuantity] = useState("");
  const [currency, setCurrency] = useState("USD");
  const [formError, setFormError] = useState(null);

  const [backfillCommodity, setBackfillCommodity] = useState("");
  const [backfillStartDate, setBackfillStartDate] = useState(oneYearAgo);
  const [backfillEndDate, setBackfillEndDate] = useState(today);
  const [backfillCurrency, setBackfillCurrency] = useState("USD");
  const [backfillResult, setBackfillResult] = useState(null);
  const [backfillError, setBackfillError] = useState(null);

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

  const backfillMutation = useMutation({
    mutationFn: (vars) => ledgerClient.backfillPrices(vars),
    onSuccess: (data) => {
      setBackfillResult({ added: data.prices?.length ?? 0, skipped: data.skippedCount ?? 0 });
      setBackfillError(null);
      queryClient.invalidateQueries({ queryKey: queryKeys.prices() });
    },
    onError: (err) => {
      setBackfillResult(null);
      setBackfillError(err);
    },
  });

  function handleSubmit(e) {
    e.preventDefault();
    setFormError(null);
    addMutation.mutate({ date, commodity: commodity.trim(), quantity: quantity.trim(), currency: currency.trim() });
  }

  function handleDelete(pid) {
    deleteMutation.mutate({ pid });
  }

  function handleBackfill(e) {
    e.preventDefault();
    setBackfillResult(null);
    setBackfillError(null);
    backfillMutation.mutate({
      commodity: backfillCommodity.trim(),
      startDate: backfillStartDate,
      endDate: backfillEndDate,
      currency: backfillCurrency.trim(),
    });
  }

  return (
    <div className="flex flex-col gap-6">
      <h2 className="text-2xl font-bold">Commodity Prices</h2>

      <Card>
        <CardHeader>
          <CardTitle>Add Price</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="flex flex-wrap items-end gap-3">
            <div className="w-full flex flex-col gap-1.5 sm:w-36">
              <Label htmlFor="price-date">Date</Label>
              <Input
                id="price-date"
                type="date"
                value={date}
                onChange={(e) => setDate(e.target.value)}
                required
              />
            </div>
            <div className="w-full flex flex-col gap-1.5 sm:w-32">
              <Label htmlFor="price-commodity">Commodity</Label>
              <Input
                id="price-commodity"
                type="text"
                placeholder="AAPL"
                value={commodity}
                onChange={(e) => setCommodity(e.target.value)}
                required
              />
            </div>
            <div className="w-full flex flex-col gap-1.5 sm:w-32">
              <Label htmlFor="price-quantity">Price</Label>
              <Input
                id="price-quantity"
                type="text"
                placeholder="178.50"
                value={quantity}
                onChange={(e) => setQuantity(e.target.value)}
                required
              />
            </div>
            <div className="w-full flex flex-col gap-1.5 sm:w-24">
              <Label htmlFor="price-currency">Currency</Label>
              <Input
                id="price-currency"
                type="text"
                value={currency}
                onChange={(e) => setCurrency(e.target.value)}
                required
              />
            </div>
            <Button type="submit" disabled={addMutation.isPending}>
              {addMutation.isPending && <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />}
              {addMutation.isPending ? "Adding…" : "Add"}
            </Button>
          </form>
          {formError && <div className="mt-3"><ErrorBanner error={formError} /></div>}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Backfill Price History</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleBackfill} className="flex flex-wrap items-end gap-3">
            <div className="w-full flex flex-col gap-1.5 sm:w-32">
              <Label htmlFor="backfill-commodity">Commodity</Label>
              <Input
                id="backfill-commodity"
                type="text"
                placeholder="AAPL"
                value={backfillCommodity}
                onChange={(e) => setBackfillCommodity(e.target.value)}
                required
              />
            </div>
            <div className="w-full flex flex-col gap-1.5 sm:w-36">
              <Label htmlFor="backfill-start">Start Date</Label>
              <Input
                id="backfill-start"
                type="date"
                value={backfillStartDate}
                onChange={(e) => setBackfillStartDate(e.target.value)}
                required
              />
            </div>
            <div className="w-full flex flex-col gap-1.5 sm:w-36">
              <Label htmlFor="backfill-end">End Date</Label>
              <Input
                id="backfill-end"
                type="date"
                value={backfillEndDate}
                onChange={(e) => setBackfillEndDate(e.target.value)}
                required
              />
            </div>
            <div className="w-full flex flex-col gap-1.5 sm:w-24">
              <Label htmlFor="backfill-currency">Currency</Label>
              <Input
                id="backfill-currency"
                type="text"
                value={backfillCurrency}
                onChange={(e) => setBackfillCurrency(e.target.value)}
                required
              />
            </div>
            <Button type="submit" disabled={backfillMutation.isPending}>
              {backfillMutation.isPending && <Loader2 data-icon="inline-start" className="size-3.5 animate-spin" />}
              {backfillMutation.isPending ? "Fetching…" : "Backfill"}
            </Button>
          </form>
          {backfillResult && (
            <p className="mt-3 text-sm text-green-600 dark:text-green-400">
              Added {backfillResult.added} {backfillResult.added === 1 ? "price" : "prices"}
              {backfillResult.skipped > 0 && ` (${backfillResult.skipped} already existed)`}.
            </p>
          )}
          {backfillError && <div className="mt-3"><ErrorBanner error={backfillError} /></div>}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Price History</CardTitle>
        </CardHeader>
        <CardContent>
          {isLoading && <Loading />}
          {fetchError && <ErrorBanner error={fetchError} />}
          {pricesData && (
            pricesData.prices?.length > 0 ? (
              <PriceHistoryTable prices={pricesData.prices} onDelete={handleDelete} />
            ) : (
              <p className="text-sm text-muted-foreground">No prices recorded yet.</p>
            )
          )}
        </CardContent>
      </Card>
    </div>
  );
}
