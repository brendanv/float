import { useMemo, useState } from "react";
import {
  useReactTable,
  getCoreRowModel,
  getSortedRowModel,
  flexRender,
} from "@tanstack/react-table";
import { accountSplit, amountSortValue, displayAmounts, isIncome } from "@/lib/transactions.js";
import { Amount } from "@/components/amount.jsx";
import { Flow } from "@/components/account.jsx";
import { StatusMark } from "@/components/status.jsx";
import { TagChip } from "@/components/tag-editor.jsx";
import { TableSortHeader } from "@/components/table-sort-header.jsx";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn } from "@/lib/utils";

// CandidateDescription is the Transactions-page description: payee (500)
// with the note in ink-3, or the raw description when there is no payee.
function CandidateDescription({ tx }) {
  if (tx?.payee) {
    return (
      <span className="min-w-0 truncate">
        <span className="font-medium">{tx.payee}</span>
        {tx.note && <span className="ml-1.5 text-faint">{tx.note}</span>}
      </span>
    );
  }
  return <span className="min-w-0 truncate font-medium">{tx?.description}</span>;
}

// CandidateTable previews transactions about to be imported (CSV preview,
// Stripe fetch) in the Transactions row style: status mark, payee/note,
// From → To flow and Amount. Duplicates and rule matches are quiet chips.
//
// Selection belongs to the caller: isSelected(row), isDisabled(row) and
// onToggle(row). `sorting`/`onSortingChange` are optional (controlled).
// `showAccount` adds the linked-account column (Fetch all).
export function CandidateTable({
  rows,
  isSelected,
  isDisabled,
  onToggle,
  showAccount = false,
  sorting: controlledSorting,
  onSortingChange,
  className,
}) {
  const [localSorting, setLocalSorting] = useState([]);
  const sorting = controlledSorting ?? localSorting;
  const setSorting = onSortingChange ?? setLocalSorting;

  const columns = useMemo(
    () => [
      {
        id: "select",
        header: () => <span className="sr-only">Select</span>,
        cell: ({ row }) => (
          <span className="flex">
            <Checkbox
              aria-label="Select transaction"
              checked={isSelected(row.original)}
              disabled={isDisabled ? isDisabled(row.original) : false}
              onCheckedChange={() => onToggle(row.original)}
            />
          </span>
        ),
        enableSorting: false,
        meta: { headerClass: "w-7 pr-0", cellClass: "pr-0" },
      },
      {
        id: "date",
        header: "Date",
        accessorFn: (row) => row.transaction?.date ?? "",
        cell: ({ getValue }) => <span className="whitespace-nowrap text-muted-foreground">{getValue()}</span>,
        meta: { headerClass: "w-24" },
      },
      {
        id: "status",
        header: () => <span className="sr-only">Status</span>,
        cell: ({ row }) => <StatusMark status={row.original.transaction?.status ?? ""} />,
        enableSorting: false,
        meta: { headerClass: "w-7 px-0", cellClass: "px-0 text-center" },
      },
      {
        id: "description",
        header: "Description",
        accessorFn: (row) => row.transaction?.description ?? "",
        cell: ({ row }) => (
          <span className="flex min-w-0 items-center gap-2">
            <CandidateDescription tx={row.original.transaction} />
            {row.original.isDuplicate && (
              <TagChip title="Already in the journal; skipped">duplicate</TagChip>
            )}
            {row.original.matchedRuleId && (
              <TagChip title="Categorized by a rule">rule</TagChip>
            )}
          </span>
        ),
        meta: { cellClass: "max-w-[28rem] overflow-hidden" },
      },
      ...(showAccount
        ? [
            {
              id: "account",
              header: "Linked account",
              accessorFn: (row) => row._accountName ?? "",
              cell: ({ getValue }) => (
                <span className="whitespace-nowrap text-muted-foreground">{getValue()}</span>
              ),
            },
          ]
        : []),
      {
        id: "flow",
        header: "From → To",
        cell: ({ row }) => {
          const { from, to } = accountSplit(row.original.transaction ?? {});
          return <Flow from={from} to={to} />;
        },
        enableSorting: false,
        meta: { cellClass: "overflow-hidden" },
      },
      {
        id: "amount",
        header: "Amount",
        accessorFn: (row) => amountSortValue(row.transaction ?? {}),
        cell: ({ row }) => {
          const tx = row.original.transaction ?? {};
          const income = isIncome(tx);
          return <Amount value={displayAmounts(tx)} tone={income ? "inflow" : "auto"} signed={income} />;
        },
        meta: { headerClass: "w-32 text-right", cellClass: "text-right", align: "right" },
      },
    ],
    [isSelected, isDisabled, onToggle, showAccount],
  );

  const table = useReactTable({
    data: rows,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  });

  return (
    <Table data-slot="candidate-table" className={className}>
      <TableHeader>
        {table.getHeaderGroups().map((headerGroup) => (
          <TableRow key={headerGroup.id} className="hover:bg-transparent">
            {headerGroup.headers.map((header) => {
              const meta = header.column.columnDef.meta ?? {};
              const content = flexRender(header.column.columnDef.header, header.getContext());
              return (
                <TableHead key={header.id} className={meta.headerClass}>
                  {header.column.getCanSort() ? (
                    <TableSortHeader column={header.column} align={meta.align}>{content}</TableSortHeader>
                  ) : (
                    content
                  )}
                </TableHead>
              );
            })}
          </TableRow>
        ))}
      </TableHeader>
      <TableBody>
        {table.getRowModel().rows.map((row) => (
          <TableRow
            key={row.id}
            data-slot="candidate-row"
            data-duplicate={row.original.isDuplicate ? "" : undefined}
            className={cn("group/row", row.original.isDuplicate && "text-muted-foreground [&_[data-slot=amount]]:text-faint")}
          >
            {row.getVisibleCells().map((cell) => (
              <TableCell key={cell.id} className={cn("py-1", cell.column.columnDef.meta?.cellClass)}>
                {flexRender(cell.column.columnDef.cell, cell.getContext())}
              </TableCell>
            ))}
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
