import {
  useReactTable,
  getCoreRowModel,
  getSortedRowModel,
  flexRender,
  createColumnHelper,
} from "@tanstack/react-table";
import { ChevronUp, ChevronDown, ChevronsUpDown } from "lucide-react";
import { XIcon } from "@phosphor-icons/react";
import { Amount } from "@/components/amount.jsx";
import { Account } from "@/components/account.jsx";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import {
  STALE_PRICE_DAYS,
  VERY_STALE_PRICE_DAYS,
  daysSinceDate,
  formatPct,
  gainTone,
} from "./performance.js";

const TONE_TEXT = { inflow: "text-inflow", below: "text-below", ink: "" };

function SortIcon({ column }) {
  const sorted = column.getIsSorted();
  if (sorted === "asc") return <ChevronUp className="inline ml-1 h-3 w-3" />;
  if (sorted === "desc") return <ChevronDown className="inline ml-1 h-3 w-3" />;
  return <ChevronsUpDown className="inline ml-1 h-3 w-3 opacity-40" />;
}

function SortableHeader({ column, children, className }) {
  return (
    <button
      className={cn(
        "flex items-center gap-0.5 hover:text-foreground transition-colors",
        className,
      )}
      onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
    >
      {children}
      <SortIcon column={column} />
    </button>
  );
}

function Dash() {
  return <span className="text-faint">—</span>;
}

// byAmount sorts rows by an amount field, missing values last (ascending).
function byAmount(key) {
  return (a, b) => {
    const av = a.original[key] ? parseFloat(a.original[key].quantity) : -Infinity;
    const bv = b.original[key] ? parseFloat(b.original[key].quantity) : -Infinity;
    return av - bv;
  };
}

function hasGain(holding) {
  return holding.unrealizedGain !== null && holding.unrealizedGain !== undefined;
}

const columnHelper = createColumnHelper();

const columns = [
  columnHelper.accessor("symbol", {
    header: ({ column }) => (
      <SortableHeader column={column}>Symbol</SortableHeader>
    ),
    cell: (info) => (
      <span className="font-mono text-code font-medium">{info.getValue()}</span>
    ),
    sortingFn: "alphanumeric",
  }),
  columnHelper.accessor("account", {
    header: ({ column }) => (
      <SortableHeader column={column}>Account</SortableHeader>
    ),
    cell: (info) => <Account name={info.getValue()} variant="path" />,
    sortingFn: "alphanumeric",
  }),
  columnHelper.accessor("quantity", {
    header: ({ column }) => (
      <SortableHeader column={column} className="ml-auto">
        Quantity
      </SortableHeader>
    ),
    cell: (info) => (
      <span className="block text-right tabular-nums">{info.getValue()}</span>
    ),
    sortingFn: (a, b) =>
      parseFloat(a.original.quantity) - parseFloat(b.original.quantity),
  }),
  columnHelper.accessor("latestPrice", {
    header: ({ column }) => (
      <SortableHeader column={column} className="ml-auto">
        Latest price
      </SortableHeader>
    ),
    cell: (info) => {
      const v = info.getValue();
      return <span className="block text-right">{v ? <Amount value={v} /> : <Dash />}</span>;
    },
    sortingFn: byAmount("latestPrice"),
  }),
  columnHelper.accessor("priceDate", {
    header: ({ column }) => (
      <SortableHeader column={column} className="ml-auto">
        Price date
      </SortableHeader>
    ),
    cell: (info) => {
      const dateStr = info.getValue();
      if (!dateStr) {
        return <span className="block text-right"><Dash /></span>;
      }
      // Stale prices need a look (warning), not an error.
      const days = daysSinceDate(dateStr);
      const stale = days !== null && days > STALE_PRICE_DAYS;
      const veryStale = days !== null && days > VERY_STALE_PRICE_DAYS;
      return (
        <span
          className={cn(
            "block text-right tabular-nums",
            stale ? "text-warning" : "text-muted-foreground",
            veryStale && "font-medium",
          )}
          title={stale ? `Price is ${days} days old` : undefined}
        >
          {dateStr}
        </span>
      );
    },
    sortingFn: "alphanumeric",
  }),
  columnHelper.accessor("currentValue", {
    header: ({ column }) => (
      <SortableHeader column={column} className="ml-auto">
        Value
      </SortableHeader>
    ),
    cell: (info) => {
      const v = info.getValue();
      return (
        <span className="block text-right font-medium">
          {v ? <Amount value={v} /> : <Dash />}
        </span>
      );
    },
    sortingFn: byAmount("currentValue"),
  }),
  columnHelper.accessor("bookValue", {
    header: ({ column }) => (
      <SortableHeader column={column} className="ml-auto">
        Book value
      </SortableHeader>
    ),
    cell: (info) => {
      const v = info.getValue();
      return (
        <span className="block text-right text-muted-foreground">
          {v ? <Amount value={v} /> : <Dash />}
        </span>
      );
    },
    sortingFn: byAmount("bookValue"),
  }),
  columnHelper.accessor("unrealizedGain", {
    header: ({ column }) => (
      <SortableHeader column={column} className="ml-auto">
        Unrealized gain
      </SortableHeader>
    ),
    cell: (info) => {
      const v = info.getValue();
      return (
        <span className="block text-right">
          {v ? <Amount value={v} signed tone={gainTone(parseFloat(v.quantity))} /> : <Dash />}
        </span>
      );
    },
    sortingFn: byAmount("unrealizedGain"),
  }),
  columnHelper.accessor("unrealizedGainPct", {
    header: ({ column }) => (
      <SortableHeader column={column} className="ml-auto">
        Gain %
      </SortableHeader>
    ),
    cell: (info) => {
      const v = info.getValue();
      if (!hasGain(info.row.original)) {
        return <span className="block text-right"><Dash /></span>;
      }
      return (
        <span className={cn("block text-right tabular-nums", TONE_TEXT[gainTone(v)])}>
          {formatPct(v ?? 0, 2)}
        </span>
      );
    },
    sortingFn: (a, b) => {
      const av = hasGain(a.original) ? (a.original.unrealizedGainPct ?? 0) : -Infinity;
      const bv = hasGain(b.original) ? (b.original.unrealizedGainPct ?? 0) : -Infinity;
      return av - bv;
    },
  }),
  columnHelper.accessor("portfolioPct", {
    header: ({ column }) => (
      <SortableHeader column={column} className="ml-auto">
        Share
      </SortableHeader>
    ),
    cell: (info) => {
      const v = info.getValue();
      return (
        <span className="block text-right tabular-nums text-muted-foreground">
          {v > 0 ? `${v.toFixed(1)}%` : <Dash />}
        </span>
      );
    },
    sortingFn: "basic",
  }),
  columnHelper.display({
    id: "actions",
    cell: (info) => {
      const { onExcludeSymbol } = info.table.options.meta ?? {};
      const symbol = info.row.original.symbol;
      if (!onExcludeSymbol) return null;
      return (
        <Button
          variant="ghost"
          size="xs"
          className="opacity-0 group-hover/row:opacity-100 focus-visible:opacity-100 transition-opacity text-muted-foreground hover:text-below"
          title={`Exclude ${symbol} from portfolio`}
          onClick={() => onExcludeSymbol(symbol)}
        >
          <XIcon size={12} className="mr-1" />
          Exclude
        </Button>
      );
    },
  }),
];

// HoldingsTable is the sortable holdings list. `onExcludeSymbol` adds a
// hover "Exclude" action per row.
export function HoldingsTable({ holdings, sorting, onSortingChange, onExcludeSymbol }) {
  const table = useReactTable({
    data: holdings,
    columns,
    state: { sorting },
    onSortingChange,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    meta: { onExcludeSymbol },
  });

  return (
    <div className="overflow-x-auto">
      <Table>
        <TableHeader>
          {table.getHeaderGroups().map((headerGroup) => (
            <TableRow key={headerGroup.id} className="hover:bg-transparent">
              {headerGroup.headers.map((header) => (
                <TableHead key={header.id}>
                  {header.isPlaceholder
                    ? null
                    : flexRender(header.column.columnDef.header, header.getContext())}
                </TableHead>
              ))}
            </TableRow>
          ))}
        </TableHeader>
        <TableBody>
          {table.getRowModel().rows.map((row) => (
            <TableRow key={row.id} className="group/row">
              {row.getVisibleCells().map((cell) => (
                <TableCell key={cell.id}>
                  {flexRender(cell.column.columnDef.cell, cell.getContext())}
                </TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
