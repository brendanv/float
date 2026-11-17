import { createColumnHelper } from "@tanstack/react-table";
import { Scale } from "lucide-react";
import { amountSortValue, accountSplit, displayAmounts, isIncome, registerDisplay } from "@/lib/transactions.js";
import { Amount } from "./amount.jsx";
import { StatusMark } from "./status.jsx";
import { TagChip, TagEditor, tagLabel } from "./tag-editor.jsx";
import { TableSortHeader } from "./table-sort-header.jsx";
import {
  EditableAccountSideCell,
  EditableDescriptionCell,
  EditableOtherAccountCell,
  StripeIndicator,
} from "./transaction-editors.jsx";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn, ROW_ACTION } from "@/lib/utils";

// Column definitions shared by the general transactions table and the
// account register. Cell renderers read mutable state from
// table.options.meta to avoid stale closures when useMemo deps are unchanged
// between renders.

const col = createColumnHelper();

const selectColumn = col.display({
  id: "select",
  header: ({ table }) => {
    const { allSelected, someSelected, toggleSelectAll } = table.options.meta;
    return (
      <Checkbox
        checked={allSelected}
        indeterminate={!allSelected && someSelected}
        onCheckedChange={toggleSelectAll}
        onClick={(e) => e.stopPropagation()}
        title={allSelected ? "Deselect all" : "Select all"}
        aria-label={allSelected ? "Deselect all" : "Select all"}
        className={cn(
          !someSelected && "[@media(hover:hover)]:opacity-0 [@media(hover:hover)]:group-hover/head:opacity-100 focus-visible:opacity-100",
        )}
      />
    );
  },
  cell: ({ row, table }) => {
    const { selectedFids, onSelectionChange, someSelected } = table.options.meta;
    const checked = selectedFids?.has(row.original.fid) ?? false;
    return (
      <span className="flex" onClick={(e) => e.stopPropagation()}>
        <Checkbox
          checked={checked}
          aria-label="Select transaction"
          className={cn(!someSelected && !checked && ROW_ACTION)}
          onCheckedChange={() => {
            if (!row.original.fid) return;
            const next = new Set(selectedFids);
            if (next.has(row.original.fid)) next.delete(row.original.fid);
            else next.add(row.original.fid);
            onSelectionChange(next);
          }}
        />
      </span>
    );
  },
  meta: { headerClass: "w-7 pr-0", cellClass: "pr-0" },
});

const dateColumn = col.accessor("date", {
  id: "date",
  // Use original array index as a tiebreaker so that descending sort also
  // reverses within-day order (later transactions in file = shown first).
  sortingFn: (rowA, rowB) => {
    const dateA = rowA.original.date;
    const dateB = rowB.original.date;
    if (dateA < dateB) return -1;
    if (dateA > dateB) return 1;
    return rowA.index - rowB.index;
  },
  header: ({ column }) => <TableSortHeader column={column}>Date</TableSortHeader>,
  cell: ({ getValue }) => (
    <span className="whitespace-nowrap text-muted-foreground">{getValue()}</span>
  ),
  meta: { headerClass: "w-24" },
});

const statusColumn = col.display({
  id: "status",
  header: () => <span className="sr-only">Status</span>,
  cell: ({ row, table }) => {
    const { onStatusChange } = table.options.meta;
    return <StatusMark fid={row.original.fid} status={row.original.status} onStatusChange={onStatusChange} />;
  },
  meta: { headerClass: "w-7 px-0", cellClass: "px-0 text-center" },
});

function RowTags({ tx, onChanged }) {
  const tags = tx.tags || {};
  if (tx.fid) {
    return (
      <span className="min-w-0 max-w-[55%] shrink-0 overflow-hidden has-[input]:max-w-none" onClick={(e) => e.stopPropagation()}>
        <TagEditor quiet fid={tx.fid} tags={tags} onChanged={onChanged} addButtonClassName={ROW_ACTION} />
      </span>
    );
  }
  const entries = Object.entries(tags);
  if (entries.length === 0) return null;
  return (
    <span className="flex min-w-0 shrink-0 gap-1 overflow-hidden">
      {entries.map(([k, v]) => <TagChip key={k}>{tagLabel(k, v)}</TagChip>)}
    </span>
  );
}

// Description cell: status-free single line with the (inline-editable)
// payee + note, quiet #tags (editable on click), and a hover "Edit" button
// that opens the full editor (general list only).
const descriptionColumn = col.accessor((tx) => tx.description, {
  id: "description",
  header: ({ column, table }) => {
    // Grouped by day the Date column is hidden; its sort toggle lives here.
    const date = table.getColumn("date");
    return (
      <span className="inline-flex items-center gap-4">
        {date && !date.getIsVisible() && <TableSortHeader column={date}>Date</TableSortHeader>}
        <TableSortHeader column={column}>Description</TableSortHeader>
      </span>
    );
  },
  cell: ({ row, table }) => {
    const { onStatusChange, isRegisterMode } = table.options.meta;
    const tx = row.original;
    const canExpand = row.getCanExpand();
    return (
      <span className="flex min-w-0 items-center gap-2">
        <span className="min-w-0 flex-1">
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
        {tx.stripeTransactionId && <StripeIndicator id={tx.stripeTransactionId} />}
        <RowTags tx={tx} onChanged={onStatusChange} />
        {canExpand && !isRegisterMode && (
          <Button
            variant="outline"
            size="xs"
            className={cn("h-5 shrink-0 px-1.5 text-[12px] font-normal text-muted-foreground aria-expanded:opacity-100", ROW_ACTION)}
            aria-expanded={row.getIsExpanded()}
            onClick={(e) => { e.stopPropagation(); row.toggleExpanded(); }}
          >
            Edit
          </Button>
        )}
      </span>
    );
  },
  meta: { cellClass: "overflow-hidden" },
});

// "From" and "To" for the general table: each side is inline-editable via
// the account typeahead when the row has exactly one posting on that side;
// otherwise it reads "N accounts". Together they read as one flow,
// "checking → dining", with the arrows lined up down the list.
const fromColumn = col.accessor((tx) => accountSplit(tx).from || "", {
  id: "from",
  header: ({ column }) => <TableSortHeader column={column}>From</TableSortHeader>,
  cell: ({ row, table }) => {
    const { accounts, onStatusChange } = table.options.meta;
    return <EditableAccountSideCell tx={row.original} side="from" accounts={accounts} onSaved={onStatusChange} />;
  },
  meta: { headerClass: "w-[14%] min-w-0", cellClass: "overflow-hidden" },
});

const toColumn = col.accessor((tx) => accountSplit(tx).to || "", {
  id: "to",
  header: ({ column }) => <span className="pl-[18px]"><TableSortHeader column={column}>To</TableSortHeader></span>,
  cell: ({ row, table }) => {
    const { accounts, onStatusChange } = table.options.meta;
    return (
      <span className="flex min-w-0 items-center gap-[7px]">
        <span aria-hidden="true" className="text-faint">→</span>
        <span className="sr-only">to</span>
        <span className="min-w-0 flex-1">
          <EditableAccountSideCell tx={row.original} side="to" accounts={accounts} onSaved={onStatusChange} />
        </span>
      </span>
    );
  },
  meta: { headerClass: "w-[16%]", cellClass: "overflow-hidden" },
});

const amountColumn = col.accessor(amountSortValue, {
  id: "amount",
  header: ({ column }) => <TableSortHeader column={column} align="right">Amount</TableSortHeader>,
  cell: ({ row }) => {
    const tx = row.original;
    const income = isIncome(tx);
    const amounts = displayAmounts(tx);
    const hasAssertion = (tx.postings || []).some((p) => p.balanceAssertion);
    return (
      <span className="inline-flex items-center justify-end gap-1">
        {hasAssertion && (
          <Tooltip>
            <TooltipTrigger render={<Scale aria-label="Has balance assertion" className="size-3 shrink-0 text-faint" />} />
            <TooltipContent>Has balance assertion</TooltipContent>
          </Tooltip>
        )}
        <Amount value={amounts} tone={income ? "inflow" : "auto"} signed={income} />
      </span>
    );
  },
  meta: { headerClass: "w-36 text-right", cellClass: "text-right" },
});

// Register-only columns: editable Other accounts, signed Change, running
// Balance.
const otherAccountsColumn = col.display({
  id: "otherAccounts",
  header: "Other accounts",
  cell: ({ row, table }) => {
    const { accounts, onStatusChange } = table.options.meta;
    const tx = row.original;
    return (
      <EditableOtherAccountCell
        fid={tx.fid}
        otherAccounts={tx.otherAccounts}
        accounts={accounts}
        onSaved={onStatusChange}
      />
    );
  },
  meta: { headerClass: "w-[22%]", cellClass: "overflow-hidden" },
});

function firstChange(amounts) {
  return amounts?.length > 0 ? parseFloat(amounts[0].quantity) || 0 : 0;
}

const changeColumn = col.accessor((row) => firstChange(row.change), {
  id: "change",
  header: ({ column }) => <TableSortHeader column={column} align="right">Change</TableSortHeader>,
  cell: ({ row, table }) => {
    const { negate, changeTone } = registerDisplay(table.options.meta.registerAccount);
    const change = row.original.change || [];
    return <Amount value={change} tone={changeTone(firstChange(change))} negate={negate} signed />;
  },
  meta: { headerClass: "w-32 text-right", cellClass: "text-right" },
});

const balanceColumn = col.accessor((row) => firstChange(row.runningTotal), {
  id: "balance",
  header: ({ column }) => <TableSortHeader column={column} align="right">Balance</TableSortHeader>,
  cell: ({ row, table }) => {
    const { negate, balanceTone } = registerDisplay(table.options.meta.registerAccount);
    const total = row.original.runningTotal || [];
    const tone = balanceTone(firstChange(total));
    return <Amount value={total} tone={tone} negate={negate} className={tone === "below" ? undefined : "text-muted-foreground"} />;
  },
  meta: { headerClass: "w-32 text-right", cellClass: "text-right" },
});

export const transactionColumns = [
  selectColumn,
  dateColumn,
  statusColumn,
  descriptionColumn,
  fromColumn,
  toColumn,
  amountColumn,
];

export const registerColumns = [
  selectColumn,
  dateColumn,
  statusColumn,
  descriptionColumn,
  otherAccountsColumn,
  changeColumn,
  balanceColumn,
];
