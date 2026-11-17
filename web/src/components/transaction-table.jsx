import { Fragment, useEffect, useMemo, useState } from "react";
import {
  useReactTable,
  getCoreRowModel,
  getPaginationRowModel,
  getExpandedRowModel,
  getSortedRowModel,
  flexRender,
} from "@tanstack/react-table";
import { Scale } from "lucide-react";
import { splitAccount } from "@/lib/accounts.js";
import {
  accountSplit,
  dayLabel,
  displayAmounts,
  focusedAmounts,
  isIncome,
} from "@/lib/transactions.js";
import { Amount } from "./amount.jsx";
import { StatusMark } from "./status.jsx";
import { TagChip, TagEditor, tagLabel } from "./tag-editor.jsx";
import { TablePagination } from "./table-pagination.jsx";
import { registerColumns, transactionColumns } from "./transaction-columns.jsx";
import {
  EditableDescriptionCell,
  EditableDetailRow,
  EditableOtherAccountCell,
  StripeIndicator,
} from "./transaction-editors.jsx";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  ResponsiveDialog,
  ResponsiveDialogContent,
  ResponsiveDialogHeader,
  ResponsiveDialogTitle,
} from "@/components/ui/responsive-dialog";
import { cn } from "@/lib/utils";

// ── day grouping ───────────────────────────────────────────────────────────

function DayTitle({ date }) {
  const { day, weekday } = dayLabel(date);
  return (
    <>
      <span className="mr-1.5 text-[12.5px] font-semibold text-foreground">{day}</span>
      {weekday}
    </>
  );
}

// ── main component ─────────────────────────────────────────────────────────

export function TransactionTable({
  transactions,
  registerRows,
  focusedAccount,
  onStatusChange,
  onDeleted,
  accounts = [],
  selectedFids,
  onSelectionChange,
  pageSize = 25,
  hiddenColumns = [],
}) {
  const [expanded, setExpanded] = useState({});
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize });
  const [sorting, setSorting] = useState([{ id: "date", desc: true }]);

  const selectable = selectedFids !== undefined && onSelectionChange !== undefined;
  const isRegisterMode = !!registerRows;
  // Sorted by date the list reads as days; any other sort is a flat list
  // with a Date column.
  const grouped = sorting[0]?.id === "date";

  const rows = isRegisterMode ? (registerRows || []) : (transactions || []);

  const allFids = useMemo(() => rows.filter((r) => r.fid).map((r) => r.fid), [rows]);
  const allSelected = selectable && allFids.length > 0 && allFids.every((fid) => selectedFids.has(fid));
  const someSelected = selectable && allFids.some((fid) => selectedFids.has(fid));

  function toggleSelectAll() {
    if (!selectable) return;
    const next = new Set(selectedFids);
    for (const fid of allFids) {
      if (allSelected) next.delete(fid);
      else next.add(fid);
    }
    onSelectionChange(next);
  }

  const columnVisibility = useMemo(() => {
    const vis = { select: selectable, date: !grouped };
    for (const col of hiddenColumns) vis[col] = false;
    return vis;
  }, [selectable, grouped, hiddenColumns]);

  const columns = isRegisterMode ? registerColumns : transactionColumns;

  const table = useReactTable({
    data: rows,
    columns,
    state: { expanded, pagination, columnVisibility, sorting },
    onExpandedChange: setExpanded,
    onPaginationChange: setPagination,
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getExpandedRowModel: getExpandedRowModel(),
    getRowCanExpand: (row) => !isRegisterMode && !!row.original.fid,
    // Keep the current page when data refetches after an edit; callers that
    // change the filter remount the table (key) to start over at page 1.
    autoResetPageIndex: false,
    getRowId: (row, idx) => row.fid ? row.fid : `${row.date}-${idx}`,
    // Pass mutable state through meta so column cell renderers always read current values
    meta: {
      selectedFids,
      onSelectionChange,
      selectable,
      onStatusChange,
      onDeleted,
      accounts,
      focusedAccount,
      isRegisterMode,
      allSelected,
      someSelected,
      toggleSelectAll,
    },
  });

  // If rows were removed (e.g. a delete) and the current page no longer
  // exists, step back to the last page that does.
  const pageCount = table.getPageCount();
  useEffect(() => {
    if (pagination.pageIndex > 0 && pagination.pageIndex >= pageCount) {
      setPagination((p) => ({ ...p, pageIndex: Math.max(0, pageCount - 1) }));
    }
  }, [pagination.pageIndex, pageCount]);

  if (rows.length === 0) {
    return <p className="py-4 text-muted-foreground">No transactions for this period.</p>;
  }

  const pageRows = table.getRowModel().rows;
  const visibleColumnCount = table.getVisibleLeafColumns().length;
  const startsDay = (i) => grouped && (i === 0 || pageRows[i - 1].original.date !== pageRows[i].original.date);

  return (
    <div>
      {/* Desktop table. Fixed layout: Description takes the remaining width
          and long cells truncate. Narrow widths scroll sideways; wide ones
          don't clip, so day headers can stick to the top while scrolling. */}
      <div className="hidden overflow-x-auto [container-type:inline-size] sm:block lg:overflow-x-visible">
        <table data-slot="table" className="w-full min-w-[34rem] table-fixed text-body">
          <TableHeader className="group/head [&_tr]:border-border">
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
            {pageRows.map((row, i) => (
              <Fragment key={row.id}>
                {startsDay(i) && (
                  <tr data-slot="day-header" className="sticky top-0 z-[1] bg-background text-[12px] text-faint shadow-[inset_0_-1px_0_var(--input)]">
                    <td colSpan={visibleColumnCount} className="px-2 pt-5 pb-1.5">
                      <DayTitle date={row.original.date} />
                    </td>
                  </tr>
                )}
                <TransactionRow
                  row={row}
                  isRegisterMode={isRegisterMode}
                  selectable={selectable}
                  selectedFids={selectedFids}
                  accounts={accounts}
                  onStatusChange={onStatusChange}
                  onDeleted={onDeleted}
                  visibleColumnCount={visibleColumnCount}
                />
              </Fragment>
            ))}
          </TableBody>
        </table>
      </div>

      <MobileList
        pageRows={pageRows}
        grouped={grouped}
        isRegisterMode={isRegisterMode}
        focusedAccount={focusedAccount}
        selectable={selectable}
        someSelected={someSelected}
        selectedFids={selectedFids}
        onSelectionChange={onSelectionChange}
        onStatusChange={onStatusChange}
        accounts={accounts}
        onDeleted={onDeleted}
      />

      <TablePagination table={table} />
    </div>
  );
}

// ── desktop row (with optional expansion) ─────────────────────────────────

function TransactionRow({ row, isRegisterMode, selectable, selectedFids, accounts, onStatusChange, onDeleted, visibleColumnCount }) {
  const tx = row.original;
  const isSelected = selectable && tx.fid && selectedFids?.has(tx.fid);

  return (
    <>
      <TableRow
        data-slot="transaction-row"
        data-fid={tx.fid || undefined}
        onClick={() => { if (row.getCanExpand()) row.toggleExpanded(); }}
        className={cn(
          "group/row border-border",
          !isRegisterMode && "cursor-pointer",
          isSelected && "bg-accent hover:bg-accent",
        )}
      >
        {row.getVisibleCells().map((cell) => (
          <TableCell
            key={cell.id}
            className={cn("py-1", cell.column.columnDef.meta?.cellClass)}
          >
            {flexRender(cell.column.columnDef.cell, cell.getContext())}
          </TableCell>
        ))}
      </TableRow>
      {row.getIsExpanded() && (
        <TableRow className="bg-muted/40 hover:bg-muted/40">
          <TableCell colSpan={visibleColumnCount} className="p-0 whitespace-normal">
            <EditableDetailRow
              tx={tx}
              accounts={accounts}
              onSaved={() => { row.toggleExpanded(); if (onStatusChange) onStatusChange(); }}
              onDeleted={() => { row.toggleExpanded(); if (onDeleted) onDeleted(); }}
              onCancel={() => row.toggleExpanded()}
            />
          </TableCell>
        </TableRow>
      )}
    </>
  );
}

// ── mobile: stacked rows grouped by day ───────────────────────────────────

function MobileList({ pageRows, grouped, isRegisterMode, focusedAccount, selectable, someSelected, selectedFids, onSelectionChange, onStatusChange, accounts, onDeleted }) {
  // Checkboxes would crowd 48px rows, so phones pick rows in a select mode
  // (also on whenever something is selected, e.g. from a wider window).
  const [selecting, setSelecting] = useState(false);
  const showChecks = selectable && (selecting || someSelected);

  function toggleSelecting() {
    if (showChecks) {
      setSelecting(false);
      onSelectionChange(new Set());
    } else {
      setSelecting(true);
    }
  }

  return (
    <div data-slot="transaction-list" className="flex flex-col sm:hidden">
      {selectable && (
        <div className="-mb-1 flex justify-end">
          <Button variant="ghost" size="xs" className="text-muted-foreground" onClick={toggleSelecting}>
            {showChecks ? "Done" : "Select"}
          </Button>
        </div>
      )}
      {pageRows.map((row, i) => {
        const date = row.original.date;
        const startsDay = grouped && (i === 0 || pageRows[i - 1].original.date !== date);
        return (
          <Fragment key={row.id}>
            {startsDay && (
              <div data-slot="day-header" className="pt-3 pb-1 text-[11.5px] text-faint">
                <DayTitle date={date} />
              </div>
            )}
            <MobileRow
              row={row}
              isRegisterMode={isRegisterMode}
              focusedAccount={focusedAccount}
              showDate={!grouped}
              showCheck={showChecks}
              selectedFids={selectedFids}
              onSelectionChange={onSelectionChange}
              onStatusChange={onStatusChange}
              accounts={accounts}
              onDeleted={onDeleted}
            />
          </Fragment>
        );
      })}
    </div>
  );
}

function leaf(name) {
  return /^\d+ accounts$/.test(name) ? name : splitAccount(name).leaf;
}

function MobileRow({ row, isRegisterMode, focusedAccount, showDate, showCheck, selectedFids, onSelectionChange, onStatusChange, accounts, onDeleted }) {
  const tx = row.original;
  const isSelected = !!tx.fid && !!selectedFids?.has(tx.fid);
  // Outside register mode the whole row is one tap target that opens the
  // editor; there is no inline editing on rows (small targets that each did
  // something different were too easy to mis-tap on a phone).
  const canOpen = !isRegisterMode && !!tx.fid;
  const [editing, setEditing] = useState(false);

  let amount;
  if (isRegisterMode) {
    const change = tx.change || [];
    const increase = change.length > 0 && parseFloat(change[0].quantity) > 0;
    amount = <Amount value={change} tone={increase ? "inflow" : "auto"} signed />;
  } else if (focusedAccount) {
    amount = <Amount value={focusedAmounts(tx, focusedAccount)} />;
  } else {
    const income = isIncome(tx);
    amount = <Amount value={displayAmounts(tx)} tone={income ? "inflow" : "auto"} signed={income} />;
  }

  function toggleSelected() {
    if (!tx.fid) return;
    const next = new Set(selectedFids);
    if (next.has(tx.fid)) next.delete(tx.fid);
    else next.add(tx.fid);
    onSelectionChange(next);
  }

  const split = isRegisterMode ? null : accountSplit(tx);
  const tags = Object.entries(tx.tags || {});
  const stop = (e) => e.stopPropagation();

  return (
    <>
      <div
        data-slot="transaction-card"
        data-fid={tx.fid || undefined}
        className={cn(
          "grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-2.5 gap-y-0.5 border-t border-border py-2",
          showCheck && "grid-cols-[auto_auto_minmax(0,1fr)_auto]",
          canOpen && "cursor-pointer active:bg-muted/50",
          isSelected && "-mx-2 rounded-md border-transparent bg-accent px-2",
        )}
        onClick={() => { if (canOpen) setEditing(true); }}
      >
        {showCheck && (
          // Padded label enlarges the tap target around the 16px box.
          <label className="row-span-2 -m-2.5 flex p-2.5" onClick={stop}>
            <Checkbox checked={isSelected} onCheckedChange={toggleSelected} aria-label="Select transaction" />
          </label>
        )}
        <span className="row-span-2 flex w-2 justify-center self-start pt-1.5">
          <StatusMark
            fid={tx.fid}
            status={tx.status}
            onStatusChange={onStatusChange}
            className="-m-4 size-10 shrink-0"
          />
        </span>
        <span className="min-w-0 truncate">
          {isRegisterMode ? (
            <span className="block truncate" onClick={stop}>
              <EditableDescriptionCell
                fid={tx.fid}
                description={tx.description}
                date={tx.date}
                postings={tx.postings}
                payee={tx.payee}
                note={tx.note}
                onSaved={onStatusChange}
              />
            </span>
          ) : (
            <>
              <span className="font-medium">{tx.payee || tx.description}</span>
              {tx.payee && tx.note && <span className="ml-1.5 text-faint">{tx.note}</span>}
            </>
          )}
        </span>
        <span className="flex items-center justify-end gap-1 text-right">
          {(tx.postings || []).some((p) => p.balanceAssertion) && (
            <Scale aria-label="Has balance assertion" className="size-3 shrink-0 text-faint" />
          )}
          {tx.stripeTransactionId && <StripeIndicator id={tx.stripeTransactionId} />}
          {amount}
        </span>
        <span className="flex min-w-0 items-center gap-1.5 text-[11.5px] text-faint">
          {showDate && <span className="shrink-0">{tx.date}</span>}
          {isRegisterMode ? (
            <>
              <span className="min-w-0 shrink" onClick={stop}>
                <EditableOtherAccountCell
                  fid={tx.fid}
                  otherAccounts={tx.otherAccounts}
                  accounts={accounts}
                  onSaved={onStatusChange}
                />
              </span>
              {tx.fid && (
                <span className="min-w-0 has-[input]:basis-full" onClick={stop}>
                  <TagEditor quiet fid={tx.fid} tags={tx.tags} onChanged={onStatusChange} />
                </span>
              )}
            </>
          ) : (
            <>
              <span className="min-w-0 truncate" title={`${split.from} → ${split.to}`}>
                {leaf(split.from)} → {leaf(split.to)}
              </span>
              {tags.map(([k, v]) => <TagChip key={k} className="h-4 text-[11px]">{tagLabel(k, v)}</TagChip>)}
            </>
          )}
        </span>
        {isRegisterMode && (
          <span className="text-right text-[11.5px] text-faint">
            <Amount value={tx.runningTotal || []} />
          </span>
        )}
      </div>
      {/* A sibling of the row, not a child: React events bubble through
          portals, so taps inside the drawer would otherwise reach the row's
          onClick and reopen it. */}
      {canOpen && (
        <ResponsiveDialog open={editing} onOpenChange={setEditing}>
          <ResponsiveDialogContent size="lg">
            <ResponsiveDialogHeader>
              <ResponsiveDialogTitle>Edit Transaction</ResponsiveDialogTitle>
            </ResponsiveDialogHeader>
            <EditableDetailRow
              inDialog
              tx={tx}
              accounts={accounts}
              onSaved={() => { setEditing(false); if (onStatusChange) onStatusChange(); }}
              onDeleted={() => { setEditing(false); if (onDeleted) onDeleted(); }}
              onCancel={() => setEditing(false)}
            />
          </ResponsiveDialogContent>
        </ResponsiveDialog>
      )}
    </>
  );
}
