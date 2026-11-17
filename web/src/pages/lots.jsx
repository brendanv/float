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
import { formatDate } from "../format.js";
import { Loading } from "../components/loading.jsx";
import { ErrorBanner } from "../components/error-banner.jsx";
import { EmptyState } from "../components/empty-state.jsx";
import { FilterInput } from "../components/filter-input.jsx";
import { TableSortHeader } from "../components/table-sort-header.jsx";
import { Page, Section } from "../components/page.jsx";
import { Amount } from "../components/amount.jsx";
import { Account } from "../components/account.jsx";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

const colHelper = createColumnHelper();

// Lot amounts are shown without their own cost annotations: the row already
// is the lot.
function plain(amount) {
  return amount ? { quantity: amount.quantity, commodity: amount.commodity } : null;
}

function Units({ lot }) {
  return lot.units ? <Amount value={plain(lot.units)} /> : null;
}

function UnitCost({ lot }) {
  return lot.unitCost ? <Amount value={plain(lot.unitCost)} /> : null;
}

function TotalCost({ lot }) {
  return <Amount value={(lot.totalCost || []).map(plain)} />;
}

const lotColumns = [
  colHelper.accessor("account", {
    id: "account",
    header: ({ column }) => <TableSortHeader column={column}>Account</TableSortHeader>,
    cell: ({ getValue, table }) => (
      <button
        type="button"
        className="touch-target cursor-pointer rounded-sm text-left outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
        onClick={() => table.options.meta.onSelectAccount(getValue())}
      >
        <Account name={getValue()} variant="path" />
      </button>
    ),
    sortingFn: "alphanumeric",
  }),
  colHelper.accessor((lot) => lot.units?.commodity ?? "", {
    id: "commodity",
    header: ({ column }) => <TableSortHeader column={column}>Commodity</TableSortHeader>,
    cell: ({ getValue }) => <span className="font-mono text-code font-medium">{getValue()}</span>,
    sortingFn: "alphanumeric",
  }),
  colHelper.accessor("date", {
    id: "date",
    header: ({ column }) => <TableSortHeader column={column}>Acquired</TableSortHeader>,
    cell: ({ getValue }) => <span className="text-muted-foreground">{formatDate(getValue())}</span>,
    sortingFn: "alphanumeric",
  }),
  colHelper.accessor("label", {
    id: "label",
    header: "Label",
    cell: ({ getValue }) => getValue() || <span className="text-faint">—</span>,
    enableSorting: false,
  }),
  colHelper.accessor((lot) => parseFloat(lot.units?.quantity ?? "0"), {
    id: "units",
    header: ({ column }) => <TableSortHeader column={column} align="right">Units</TableSortHeader>,
    cell: ({ row }) => <span className="block text-right"><Units lot={row.original} /></span>,
    meta: { headerClass: "text-right", cellClass: "text-right" },
  }),
  colHelper.accessor((lot) => parseFloat(lot.unitCost?.quantity ?? "0"), {
    id: "unitCost",
    header: ({ column }) => <TableSortHeader column={column} align="right">Unit cost</TableSortHeader>,
    cell: ({ row }) => <span className="block text-right text-muted-foreground"><UnitCost lot={row.original} /></span>,
    meta: { headerClass: "text-right", cellClass: "text-right" },
  }),
  colHelper.accessor((lot) => parseFloat(lot.totalCost?.[0]?.quantity ?? "0"), {
    id: "totalCost",
    header: ({ column }) => <TableSortHeader column={column} align="right">Total cost</TableSortHeader>,
    cell: ({ row }) => <span className="block text-right font-medium"><TotalCost lot={row.original} /></span>,
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

      <ul data-slot="lot-list" className="flex flex-col sm:hidden">
        {rows.map((row) => {
          const lot = row.original;
          return (
            <li key={row.id} className="border-b border-border last:border-b-0">
              <button
                type="button"
                className="flex w-full min-w-0 flex-col gap-0.5 py-2.5 text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
                onClick={() => table.options.meta.onSelectAccount(lot.account)}
              >
                <span className="flex items-baseline justify-between gap-3">
                  <span className="flex min-w-0 items-baseline gap-2">
                    <span className="font-mono text-code font-medium">{lot.units?.commodity}</span>
                    <Account name={lot.account} className="min-w-0 truncate text-muted-foreground" />
                  </span>
                  <span className="font-medium"><TotalCost lot={lot} /></span>
                </span>
                <span className="flex items-baseline justify-between gap-3 text-[12.5px] text-muted-foreground">
                  <span className="min-w-0 truncate">
                    {formatDate(lot.date)}
                    {lot.label && <span className="text-faint"> · {lot.label}</span>}
                  </span>
                  <span className="whitespace-nowrap">
                    <Units lot={lot} /> @ <UnitCost lot={lot} />
                  </span>
                </span>
              </button>
            </li>
          );
        })}
      </ul>
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
      <Section
        title="Open lots"
        description={data ? `${lots.length} ${lots.length === 1 ? "lot" : "lots"}` : undefined}
      >
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
      </Section>
    </Page>
  );
}
