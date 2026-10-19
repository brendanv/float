import { useState, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import {
  useReactTable,
  getCoreRowModel,
  getSortedRowModel,
  createColumnHelper,
  flexRender,
} from "@tanstack/react-table";
import { Layers } from "lucide-react";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { formatAmounts, formatCurrency, formatDate } from "../format.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { PageHeader } from "../components/page-header.jsx";
import { EmptyState } from "../components/empty-state.jsx";
import { FilterInput } from "../components/filter-input.jsx";
import { TableSortHeader } from "../components/table-sort-header.jsx";
import { Page, PageCard } from "../components/page.jsx";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Card, CardContent } from "@/components/ui/card";

const colHelper = createColumnHelper();

function unitsText(lot) {
  return lot.units ? formatCurrency(lot.units.quantity, lot.units.commodity) : "";
}

function unitCostText(lot) {
  return lot.unitCost ? formatAmounts([{ ...lot.unitCost, costBasis: undefined }]) : "";
}

function totalCostText(lot) {
  return formatAmounts((lot.totalCost || []).map((a) => ({ ...a, costBasis: undefined })));
}

const lotColumns = [
  colHelper.accessor("account", {
    id: "account",
    header: ({ column }) => <TableSortHeader column={column}>Account</TableSortHeader>,
    cell: ({ getValue, table }) => (
      <button
        type="button"
        className="cursor-pointer text-left hover:underline"
        onClick={() => table.options.meta.onSelectAccount(getValue())}
      >
        {getValue()}
      </button>
    ),
    sortingFn: "alphanumeric",
  }),
  colHelper.accessor((lot) => lot.units?.commodity ?? "", {
    id: "commodity",
    header: ({ column }) => <TableSortHeader column={column}>Commodity</TableSortHeader>,
    cell: ({ getValue }) => <span className="font-mono font-semibold">{getValue()}</span>,
    sortingFn: "alphanumeric",
  }),
  colHelper.accessor("date", {
    id: "date",
    header: ({ column }) => <TableSortHeader column={column}>Acquired</TableSortHeader>,
    cell: ({ getValue }) => <span className="font-mono">{formatDate(getValue())}</span>,
    sortingFn: "alphanumeric",
  }),
  colHelper.accessor("label", {
    id: "label",
    header: "Label",
    cell: ({ getValue }) => getValue() || <span className="text-muted-foreground">-</span>,
    enableSorting: false,
  }),
  colHelper.accessor((lot) => parseFloat(lot.units?.quantity ?? "0"), {
    id: "units",
    header: ({ column }) => <TableSortHeader column={column} align="right">Units</TableSortHeader>,
    cell: ({ row }) => <span className="block text-right font-mono">{unitsText(row.original)}</span>,
    meta: { headerClass: "text-right", cellClass: "text-right" },
  }),
  colHelper.accessor((lot) => parseFloat(lot.unitCost?.quantity ?? "0"), {
    id: "unitCost",
    header: ({ column }) => <TableSortHeader column={column} align="right">Unit cost</TableSortHeader>,
    cell: ({ row }) => <span className="block text-right font-mono">{unitCostText(row.original)}</span>,
    meta: { headerClass: "text-right", cellClass: "text-right" },
  }),
  colHelper.accessor((lot) => parseFloat(lot.totalCost?.[0]?.quantity ?? "0"), {
    id: "totalCost",
    header: ({ column }) => <TableSortHeader column={column} align="right">Total cost</TableSortHeader>,
    cell: ({ row }) => <span className="block text-right font-mono">{totalCostText(row.original)}</span>,
    meta: { headerClass: "text-right", cellClass: "text-right" },
  }),
];

function LotsTable({ lots }) {
  const navigate = useNavigate();
  const [sorting, setSorting] = useState([
    { id: "account", desc: false },
    { id: "date", desc: false },
  ]);

  const table = useReactTable({
    data: lots,
    columns: lotColumns,
    state: { sorting },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    meta: {
      onSelectAccount: (account) => navigate({ to: "/transactions", search: { account } }),
    },
  });

  const rows = table.getRowModel().rows;

  return (
    <div>
      <div className="hidden overflow-x-auto sm:block">
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
            {rows.map((row) => (
              <TableRow key={row.id}>
                {row.getVisibleCells().map((cell) => (
                  <TableCell key={cell.id} className={cell.column.columnDef.meta?.cellClass}>
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      <div className="-mx-4 flex flex-col gap-2 sm:hidden">
        {rows.map((row) => {
          const lot = row.original;
          return (
            <Card
              key={row.id}
              size="sm"
              className="cursor-pointer ring-0"
              onClick={() => table.options.meta.onSelectAccount(lot.account)}
            >
              <CardContent className="flex flex-col gap-1.5">
                <div className="flex items-center justify-between gap-2">
                  <span className="font-mono text-xs text-muted-foreground">{formatDate(lot.date)}</span>
                  <span className="font-mono text-sm font-semibold">{unitsText(lot)}</span>
                </div>
                <div className="truncate text-sm">{lot.account}</div>
                {lot.label && <div className="text-xs text-muted-foreground">{lot.label}</div>}
                <div className="flex items-center justify-between gap-2 font-mono text-xs text-muted-foreground">
                  <span>@ {unitCostText(lot)}</span>
                  <span>{totalCostText(lot)}</span>
                </div>
              </CardContent>
            </Card>
          );
        })}
      </div>
    </div>
  );
}

export function LotsPage() {
  const [filter, setFilter] = useState("");
  const query = useMemo(() => (filter.trim() ? [`acct:${filter.trim()}`] : []), [filter]);

  const { data, isLoading, error } = useQuery({
    queryKey: queryKeys.lots(query),
    queryFn: () => ledgerClient.listLots({ query }),
  });

  const lots = data?.lots ?? [];

  return (
    <Page>
      <PageHeader title="Lots" description="Open lots and their cost basis." />

      <PageCard>
        <FilterInput value={filter} onChange={setFilter} />
        {isLoading && <Loading />}
        {error && <ErrorBanner error={error} />}
        {data && (
          lots.length > 0 ? (
            <LotsTable lots={lots} />
          ) : (
            <EmptyState
              icon={Layers}
              title="No open lots"
              description={filter ? "No lots match the current filter." : "Lots appear here once you record purchases with a cost basis."}
            />
          )
        )}
      </PageCard>
    </Page>
  );
}
