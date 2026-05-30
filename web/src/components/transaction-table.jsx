import { useState, useMemo } from "react";
import {
  useReactTable,
  getCoreRowModel,
  getPaginationRowModel,
  getExpandedRowModel,
  getSortedRowModel,
  createColumnHelper,
  flexRender,
} from "@tanstack/react-table";
import { Check, Loader2, Trash2, ChevronLeft, ChevronRight, X, Scale, Plus, ArrowUp, ArrowDown } from "lucide-react";
import { ledgerClient } from "../client.js";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { formatAmounts, formatCurrency, formatDate } from "../format.js";
import { AccountInput, PostingFields, toPostingInput } from "./posting-fields.jsx";
import { InlineEdit, inlineEditKeyHandler, useInlineEditState } from "./inline-edit.jsx";
import { TableSortHeader } from "./table-sort-header.jsx";
import { useNavigate } from "@tanstack/react-router";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import {
  Form,
  FormActions,
  FormField,
} from "@/components/ui/form";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Card, CardContent } from "@/components/ui/card";
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

// ── helpers ────────────────────────────────────────────────────────────────

function firstQuantity(posting) {
  if (!posting.amounts || posting.amounts.length === 0) return 0;
  return parseFloat(posting.amounts[0].quantity) || 0;
}

function generalDisplay(tx) {
  const postings = tx.postings || [];
  if (postings.length === 0) return null;
  if (postings.length === 1) {
    return { from: postings[0].account, to: postings[0].account, amount: formatAmounts(postings[0].amounts) };
  }
  if (postings.length > 2) {
    const positives = postings.filter((p) => firstQuantity(p) > 0);
    const negatives = postings.filter((p) => firstQuantity(p) < 0);
    const amount = positives.length > 0 ? formatAmounts(positives[0].amounts) : formatAmounts(postings[0].amounts);
    const from = negatives.length === 1 ? negatives[0].account : "various accounts";
    const to = positives.length === 1 ? positives[0].account : "various accounts";
    return { from, to, amount };
  }
  const neg = postings.find((p) => firstQuantity(p) < 0);
  const pos = postings.find((p) => firstQuantity(p) > 0);
  if (!neg || !pos) {
    return { from: postings[0].account, to: postings[1].account, amount: formatAmounts(postings[0].amounts) };
  }
  return { from: neg.account, to: pos.account, amount: formatAmounts(pos.amounts) };
}

// accountSplit returns the singular from/to accounts for a transaction, or
// flags isMultiple when either side has more than one posting and the cells
// should be merged into a single "multiple accounts" view.
function accountSplit(tx) {
  const postings = tx.postings || [];
  if (postings.length === 0) return { from: "", to: "", isMultiple: false };
  if (postings.length === 1) {
    return { from: postings[0].account, to: postings[0].account, isMultiple: false };
  }
  const positives = postings.filter((p) => firstQuantity(p) > 0);
  const negatives = postings.filter((p) => firstQuantity(p) < 0);
  if (postings.length === 2 && (negatives.length !== 1 || positives.length !== 1)) {
    return { from: postings[0].account, to: postings[1].account, isMultiple: false };
  }
  if (negatives.length === 1 && positives.length === 1) {
    return { from: negatives[0].account, to: positives[0].account, isMultiple: false };
  }
  return {
    from: negatives.length === 1 ? negatives[0].account : "",
    to: positives.length === 1 ? positives[0].account : "",
    isMultiple: true,
  };
}

function accountRegisterDisplay(tx, focusedAccount) {
  const postings = tx.postings || [];
  if (postings.length === 0) return null;
  const focused = postings.filter((p) => p.account === focusedAccount || p.account.startsWith(focusedAccount + ":"));
  if (focused.length === 0) {
    return { otherAccounts: postings[0].account, amount: formatAmounts(postings[0].amounts) };
  }
  const others = postings.filter((p) => p.account !== focusedAccount && !p.account.startsWith(focusedAccount + ":"));
  const otherAccounts = others.length === 0 ? focusedAccount : others.length === 1 ? others[0].account : "various accounts";
  let amount;
  if (focused.length === 1) {
    amount = formatAmounts(focused[0].amounts);
  } else {
    const sumByCommodity = {};
    for (const p of focused) {
      for (const a of (p.amounts || [])) {
        sumByCommodity[a.commodity] = (sumByCommodity[a.commodity] || 0) + (parseFloat(a.quantity) || 0);
      }
    }
    amount = Object.entries(sumByCommodity).map(([c, q]) => formatCurrency(q, c)).join(", ");
  }
  return { otherAccounts, amount };
}

function resolveRegisterCells(row) {
  const otherAccounts = row.otherAccounts.length === 0 ? ""
    : row.otherAccounts.length === 1 ? row.otherAccounts[0]
    : "various accounts";
  const change = formatAmounts(row.change);
  const balance = formatAmounts(row.runningTotal);
  const changePositive = row.change.length > 0 && (parseFloat(row.change[0].quantity) || 0) > 0;
  const changeNegative = row.change.length > 0 && (parseFloat(row.change[0].quantity) || 0) < 0;
  return { otherAccounts, change, balance, changePositive, changeNegative };
}

// ── sub-components ─────────────────────────────────────────────────────────

function StatusButton({ fid, status, onStatusChange }) {
  const [updating, setUpdating] = useState(false);

  async function handleClick(e) {
    e.stopPropagation();
    if (!fid || updating) return;
    const newStatus = status === "Cleared" ? "Pending" : "Cleared";
    setUpdating(true);
    try {
      await ledgerClient.updateTransactionStatus({ fid, status: newStatus });
      if (onStatusChange) onStatusChange();
    } finally {
      setUpdating(false);
    }
  }

  const isReviewed = status === "Cleared";
  const title = isReviewed ? "Reviewed — click to mark pending" : "Unreviewed — click to mark reviewed";

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={handleClick}
            isLoading={updating}
            className="rounded-full"
          >
            <Check className={isReviewed ? "text-success" : "text-muted-foreground/40"} />
          </Button>
        }
      />
      <TooltipContent>{title}</TooltipContent>
    </Tooltip>
  );
}

function EditableDescriptionCell({ fid, description, date, postings, payee, note, onSaved }) {
  const navigate = useNavigate();
  const state = useInlineEditState(description);

  async function save() {
    if (state.draft.trim() === description) { state.cancel(); return; }
    await state.run(async () => {
      await ledgerClient.updateTransaction({
        fid,
        description: state.draft.trim(),
        date,
        postings: postings.map((p) => toPostingInput({
          account: p.account,
          commodity: p.amounts?.[0]?.commodity ?? "",
          quantity: p.amounts?.[0]?.quantity ?? "",
          cost: p.amounts?.[0]?.cost,
        })),
      });
      if (onSaved) onSaved();
    });
  }

  const display = payee ? (
    <>
      <strong
        className="cursor-pointer hover:underline"
        onClick={(e) => { e.stopPropagation(); navigate({ to: "/transactions", search: { payee } }); }}
        title={"Show all transactions for " + payee}
      >{payee}</strong>
      {note && <span className="text-muted-foreground"> · {note}</span>}
    </>
  ) : (
    description
  );

  return (
    <InlineEdit
      display={display}
      canEdit={!!fid}
      editing={state.editing}
      onActivate={() => state.start(description)}
      onCancel={state.cancel}
      onSave={save}
      saving={state.saving}
      error={state.error}
      title="Click to edit description"
      editor={
        <Input
          className="h-6 w-full"
          value={state.draft}
          onInput={(e) => state.setDraft(e.target.value)}
          onKeyDown={inlineEditKeyHandler({ onSave: save, onCancel: state.cancel })}
          autoFocus
        />
      }
    />
  );
}

// EditableAccountSideCell handles the from/to columns in the general
// transactions view. `side` is "from" or "to" and selects which posting on
// the row to replace on save (the single negative or single positive
// posting). When the row has zero or multiple postings on that side, the
// cell is not editable and falls back to the full edit view.
function EditableAccountSideCell({ tx, side, accounts, onSaved }) {
  const postings = tx.postings || [];
  const matching = postings.filter((p) => side === "from" ? firstQuantity(p) < 0 : firstQuantity(p) > 0);
  const account = matching.length === 1 ? matching[0].account : "";
  const state = useInlineEditState(account);
  const canEdit = !!tx.fid && matching.length === 1;

  async function save() {
    if (!state.draft || state.draft === account) { state.cancel(); return; }
    const oldAccount = matching[0].account;
    await state.run(async () => {
      const newPostings = postings.map((p) => {
        const a = p.amounts?.[0];
        const ba = p.balanceAssertion;
        return toPostingInput({
          account: p.account === oldAccount ? state.draft : p.account,
          commodity: a?.commodity ?? "",
          quantity: a?.quantity ?? "",
          cost: a?.cost,
          balanceAssertion: ba?.amount ? { commodity: ba.amount.commodity, quantity: ba.amount.quantity } : undefined,
        });
      });
      await ledgerClient.updateTransaction({
        fid: tx.fid,
        description: tx.description,
        date: tx.date,
        postings: newPostings,
        status: "Cleared",
      });
      if (onSaved) onSaved();
    });
  }

  return (
    <InlineEdit
      display={account}
      canEdit={canEdit}
      editing={state.editing}
      onActivate={() => state.start(account)}
      onCancel={state.cancel}
      onSave={save}
      saving={state.saving}
      error={state.error}
      title={`Click to change ${side} account`}
      displayClassName="text-sm text-muted-foreground"
      editor={
        <AccountInput
          value={state.draft}
          onChange={state.setDraft}
          accounts={accounts}
        />
      }
    />
  );
}

function EditableOtherAccountCell({ fid, otherAccounts, accounts, onSaved }) {
  const state = useInlineEditState("");
  const [loading, setLoading] = useState(false);
  const [txData, setTxData] = useState(null);

  const canEdit = !!fid && otherAccounts.length === 1;
  const displayText = otherAccounts.length === 0 ? ""
    : otherAccounts.length === 1 ? otherAccounts[0]
    : "various accounts";

  async function activate() {
    setLoading(true);
    state.setError?.(null);
    try {
      const resp = await ledgerClient.listTransactions({ query: [`code:${fid}`], limit: 1 });
      const tx = resp.transactions?.[0];
      if (!tx) throw new Error("Transaction not found");
      setTxData(tx);
      state.start(otherAccounts[0]);
    } catch (err) {
      state.setError(err.message || String(err));
    } finally {
      setLoading(false);
    }
  }

  async function save() {
    if (!txData || !state.draft) return;
    await state.run(async () => {
      const oldAccount = otherAccounts[0];
      const newPostings = (txData.postings || []).map((p) => {
        const a = p.amounts?.[0];
        const ba = p.balanceAssertion;
        return toPostingInput({
          account: p.account === oldAccount ? state.draft : p.account,
          commodity: a?.commodity ?? "",
          quantity: a?.quantity ?? "",
          cost: a?.cost,
          balanceAssertion: ba?.amount ? { commodity: ba.amount.commodity, quantity: ba.amount.quantity } : undefined,
        });
      });
      await ledgerClient.updateTransaction({
        fid: txData.fid,
        description: txData.description,
        date: txData.date,
        postings: newPostings,
        status: "Cleared",
      });
      setTxData(null);
      if (onSaved) onSaved();
    });
  }

  function cancel() {
    state.cancel();
    setTxData(null);
  }

  return (
    <InlineEdit
      display={displayText}
      canEdit={canEdit}
      editing={state.editing}
      loading={loading}
      onActivate={activate}
      onCancel={cancel}
      onSave={save}
      saving={state.saving}
      error={state.error}
      title="Click to change account"
      displayClassName="text-sm text-muted-foreground"
      editor={
        <AccountInput
          value={state.draft}
          onChange={state.setDraft}
          accounts={accounts}
        />
      }
    />
  );
}

function EditableDetailRow({ tx, accounts, onSaved, onDeleted, onTagsChanged }) {
  function toFields(ps) {
    return (ps || []).map((p) => {
      const a = p.amounts && p.amounts[0];
      const ba = p.balanceAssertion;
      return {
        account: p.account,
        commodity: a ? a.commodity : "",
        quantity: a ? a.quantity : "",
        cost: a ? a.cost : undefined,
        balanceAssertion: ba?.amount ? { commodity: ba.amount.commodity, quantity: ba.amount.quantity } : undefined,
      };
    });
  }

  const initialPostings = toFields(tx.postings);
  const [postings, setPostings] = useState(initialPostings);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);
  const [deleting, setDeleting] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);

  const isDirty = JSON.stringify(postings) !== JSON.stringify(initialPostings);

  async function handleSubmit(e) {
    e.preventDefault();
    setSaving(true);
    setError(null);
    const trimmed = postings.map(toPostingInput);
    const autoReview = trimmed.every((p) => !p.account.toLowerCase().includes("unknown"));
    try {
      await ledgerClient.updateTransaction({
        fid: tx.fid,
        description: tx.description,
        date: tx.date,
        postings: trimmed,
        status: autoReview ? "Cleared" : (tx.status ?? ""),
      });
      if (onSaved) onSaved();
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setSaving(false);
    }
  }

  function cancel() {
    setPostings(initialPostings);
    setError(null);
  }

  async function handleDelete() {
    setDeleting(true);
    try {
      await ledgerClient.deleteTransaction({ fid: tx.fid });
      setDeleteOpen(false);
      if (onDeleted) onDeleted();
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setDeleting(false);
    }
  }

  return (
    <div className="p-3" onClick={(e) => e.stopPropagation()}>
      <Form onSubmit={handleSubmit}>
        <FormField label="Postings" error={error}>
          <PostingFields postings={postings} onChange={setPostings} accounts={accounts} />
        </FormField>
        <FormField label="Tags">
          <TagEditor fid={tx.fid} tags={tx.tags} onChanged={onTagsChanged} />
        </FormField>
        <FormActions align="between">
          <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
            <DialogTrigger asChild>
              <Button variant="destructive-ghost" size="xs" disabled={saving || deleting}>
                <Trash2 data-icon="inline-start" /> Delete
              </Button>
            </DialogTrigger>
            <DialogContent showCloseButton={false}>
              <DialogHeader>
                <DialogTitle>Delete transaction?</DialogTitle>
                <DialogDescription>
                  This will permanently remove &ldquo;{tx.description}&rdquo; from the journal. This cannot be undone.
                </DialogDescription>
              </DialogHeader>
              <DialogFooter>
                <Button variant="outline" onClick={() => setDeleteOpen(false)} disabled={deleting}>Cancel</Button>
                <Button variant="destructive" onClick={handleDelete} isLoading={deleting}>
                  Delete
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
          <div className="flex flex-col-reverse gap-2 sm:flex-row sm:items-center">
            <Button type="button" variant="outline" size="xs" onClick={cancel} disabled={!isDirty || saving || deleting}>
              Cancel
            </Button>
            <Button type="submit" size="xs" disabled={!isDirty || deleting} isLoading={saving}>
              Save
            </Button>
          </div>
        </FormActions>
      </Form>
    </div>
  );
}

function TagEditor({ fid, tags, onChanged, className }) {
  const [adding, setAdding] = useState(false);
  const [tagKey, setTagKey] = useState("");
  const [tagValue, setTagValue] = useState("");
  const [working, setWorking] = useState(false);
  const [removingKey, setRemovingKey] = useState(null);
  const [error, setError] = useState(null);

  const isBusy = working || removingKey !== null;

  async function removeTag(key) {
    setRemovingKey(key);
    setError(null);
    try {
      await ledgerClient.bulkEditTransactions({
        fids: [fid],
        operations: [{ operation: { case: "removeTag", value: { key } } }],
      });
      if (onChanged) onChanged();
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setRemovingKey(null);
    }
  }

  async function addTag() {
    if (!tagKey.trim()) return;
    setWorking(true);
    setError(null);
    try {
      await ledgerClient.bulkEditTransactions({
        fids: [fid],
        operations: [{ operation: { case: "addTag", value: { key: tagKey.trim(), value: tagValue.trim() } } }],
      });
      setTagKey("");
      setTagValue("");
      setAdding(false);
      if (onChanged) onChanged();
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setWorking(false);
    }
  }

  function cancelAdd() {
    setAdding(false);
    setTagKey("");
    setTagValue("");
    setError(null);
  }

  const onKey = inlineEditKeyHandler({ onSave: addTag, onCancel: cancelAdd });

  return (
    <div className={className}>
      <div className="flex flex-wrap items-center gap-1">
        {Object.entries(tags || {}).map(([k, v]) => (
          <Badge key={k} variant="secondary" className="text-xs gap-1 pr-1">
            {v ? `${k}:${v}` : k}
            {removingKey === k ? (
              <Loader2 className="size-2.5 animate-spin" />
            ) : (
              <button
                type="button"
                className="rounded-sm p-0.5 hover:bg-foreground/20 disabled:opacity-50"
                onClick={(e) => { e.stopPropagation(); removeTag(k); }}
                disabled={isBusy}
                title={`Remove tag "${k}"`}
              >
                <X className="size-2.5" />
              </button>
            )}
          </Badge>
        ))}
        {adding ? (
          <span className="flex flex-wrap items-center gap-1">
            <Input
              className="h-6 w-24"
              placeholder="key"
              value={tagKey}
              onChange={(e) => setTagKey(e.target.value)}
              onKeyDown={onKey}
              autoFocus
            />
            <Input
              className="h-6 w-28"
              placeholder="value (optional)"
              value={tagValue}
              onChange={(e) => setTagValue(e.target.value)}
              onKeyDown={onKey}
            />
            <Button type="button" variant="ghost" size="icon-xs" onClick={addTag} disabled={!tagKey.trim()} isLoading={working} title="Add tag">
              <Check className="size-3" />
            </Button>
            <Button type="button" variant="ghost" size="icon-xs" onClick={cancelAdd} disabled={working} title="Cancel">
              <X className="size-3" />
            </Button>
          </span>
        ) : (
          <Button
            type="button"
            variant="ghost"
            size="xs"
            className="text-muted-foreground"
            onClick={(e) => { e.stopPropagation(); setAdding(true); }}
            disabled={isBusy}
          >
            <Plus data-icon="inline-start" /> Tag
          </Button>
        )}
      </div>
      {error && <p className="mt-1 text-[11px] leading-snug text-destructive">{error}</p>}
    </div>
  );
}

// ── stripe indicator ───────────────────────────────────────────────────────

function StripeIndicator({ id }) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span className="inline-flex shrink-0 cursor-default items-center rounded-sm border px-1 py-0.5">
            <svg role="img" viewBox="0 0 24 24" className="size-2.5 fill-muted-foreground" xmlns="http://www.w3.org/2000/svg">
              <path d="M13.976 9.15c-2.172-.806-3.356-1.426-3.356-2.409 0-.831.683-1.305 1.901-1.305 2.227 0 4.515.858 6.09 1.631l.89-5.494C18.252.975 15.697 0 12.165 0 9.667 0 7.589.654 6.104 1.872 4.56 3.147 3.757 4.992 3.757 7.218c0 4.039 2.467 5.76 6.476 7.219 2.585.92 3.445 1.574 3.445 2.583 0 .98-.84 1.545-2.354 1.545-1.875 0-4.965-.921-6.99-2.109l-.9 5.555C5.175 22.99 8.385 24 11.714 24c2.641 0 4.843-.624 6.328-1.813 1.664-1.305 2.525-3.236 2.525-5.732 0-4.128-2.524-5.851-6.594-7.305h.003z" />
            </svg>
          </span>
        }
      />
      <TooltipContent>Stripe transaction: {id}</TooltipContent>
    </Tooltip>
  );
}

// ── column definitions ─────────────────────────────────────────────────────
// Cell renderers read mutable state from table.options.meta to avoid stale
// closures when useMemo deps are unchanged between renders. The general
// transactions table and the account-register table share one set of column
// definitions; register-only columns are appended after the shared ones.

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
      />
    );
  },
  cell: ({ row, table }) => {
    const { selectedFids, onSelectionChange } = table.options.meta;
    return (
      <span onClick={(e) => e.stopPropagation()}>
        <Checkbox
          checked={selectedFids?.has(row.original.fid) ?? false}
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
  meta: { headerClass: "w-6 pr-0", cellClass: "w-6 pr-0" },
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
    <span className="font-mono text-xs text-muted-foreground whitespace-nowrap">
      {formatDate(getValue())}
    </span>
  ),
  meta: { headerClass: "w-28", cellClass: "w-28", label: "Date" },
});

const statusColumn = col.display({
  id: "status",
  header: "",
  cell: ({ row, table }) => {
    const { onStatusChange } = table.options.meta;
    return <StatusButton fid={row.original.fid} status={row.original.status} onStatusChange={onStatusChange} />;
  },
  meta: { headerClass: "w-8", cellClass: "w-8 pr-0" },
});

const descriptionColumn = col.accessor((tx) => tx.description, {
  id: "description",
  meta: { label: "Description" },
  header: ({ column }) => <TableSortHeader column={column}>Description</TableSortHeader>,
  cell: ({ row, table }) => {
    const { onStatusChange } = table.options.meta;
    const tx = row.original;
    return (
      <span className="flex items-center gap-1.5">
        <EditableDescriptionCell
          fid={tx.fid}
          description={tx.description}
          date={tx.date}
          postings={tx.postings}
          payee={tx.payee}
          note={tx.note}
          onSaved={onStatusChange}
        />
        {tx.stripeTransactionId && <StripeIndicator id={tx.stripeTransactionId} />}
      </span>
    );
  },
});

// Tags cell uses the same inline editor in both modes \u2014 falls back to a
// static badge list when the row has no fid to edit.
const tagsColumn = col.display({
  id: "tags",
  header: "Tags",
  cell: ({ row, table }) => {
    const tx = row.original;
    const { onStatusChange } = table.options.meta;
    if (tx.fid) {
      return (
        <span onClick={(e) => e.stopPropagation()}>
          <TagEditor fid={tx.fid} tags={tx.tags} onChanged={onStatusChange} />
        </span>
      );
    }
    const tags = tx.tags;
    if (!tags || Object.keys(tags).length === 0) return null;
    return (
      <span className="inline-flex flex-wrap gap-1">
        {Object.entries(tags).map(([k, v]) => (
          <Badge key={k} variant="secondary" className="text-xs">
            {v ? `${k}:${v}` : k}
          </Badge>
        ))}
      </span>
    );
  },
});

// "From" and "To" columns for the general transactions table. Each cell is
// inline-editable via the account typeahead when the row has exactly one
// posting on that side. Rows with multiple postings on either side render
// as a single merged "multiple accounts" cell (handled by TableRowGroup);
// the sort accessor still returns a stable string so the column header
// remains sortable.
const fromColumn = col.accessor((tx) => accountSplit(tx).from || "", {
  id: "from",
  meta: { label: "From account" },
  header: ({ column }) => <TableSortHeader column={column}>From</TableSortHeader>,
  cell: ({ row, table }) => {
    const { accounts, onStatusChange } = table.options.meta;
    return (
      <EditableAccountSideCell
        tx={row.original}
        side="from"
        accounts={accounts}
        onSaved={onStatusChange}
      />
    );
  },
});

const toColumn = col.accessor((tx) => accountSplit(tx).to || "", {
  id: "to",
  meta: { label: "To account" },
  header: ({ column }) => <TableSortHeader column={column}>To</TableSortHeader>,
  cell: ({ row, table }) => {
    const { accounts, onStatusChange } = table.options.meta;
    return (
      <EditableAccountSideCell
        tx={row.original}
        side="to"
        accounts={accounts}
        onSaved={onStatusChange}
      />
    );
  },
});

const amountColumn = col.accessor(
  (tx) => {
    const postings = tx.postings || [];
    if (postings.length === 0) return 0;
    const pos = postings.find((p) => firstQuantity(p) > 0);
    return pos ? firstQuantity(pos) : firstQuantity(postings[0]);
  },
  {
    id: "amount",
    header: ({ column }) => <TableSortHeader column={column} align="right">Amount</TableSortHeader>,
    cell: ({ row, table }) => {
      const { focusedAccount } = table.options.meta;
      const tx = row.original;
      let amount = "";
      if (focusedAccount) {
        const display = accountRegisterDisplay(tx, focusedAccount);
        amount = display?.amount || "";
      } else {
        const display = generalDisplay(tx);
        amount = display?.amount || "";
      }
      const hasAssertion = (tx.postings || []).some((p) => p.balanceAssertion);
      return (
        <span className="flex items-center justify-end gap-1 whitespace-nowrap font-mono text-sm">
          {hasAssertion && (
            <Tooltip>
              <TooltipTrigger render={<Scale className="size-3 shrink-0 text-muted-foreground" />} />
              <TooltipContent>Has balance assertion</TooltipContent>
            </Tooltip>
          )}
          {amount}
        </span>
      );
    },
    meta: { headerClass: "text-right", cellClass: "text-right", label: "Amount" },
  },
);

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
});

const changeColumn = col.accessor(
  (row) => (row.change?.length > 0 ? parseFloat(row.change[0].quantity) || 0 : 0),
  {
    id: "change",
    header: ({ column }) => <TableSortHeader column={column} align="right">Change</TableSortHeader>,
    cell: ({ row }) => {
      const cells = resolveRegisterCells(row.original);
      return (
        <span className={cn(
          "block whitespace-nowrap text-right font-mono text-sm",
          cells.changePositive && "text-success",
          cells.changeNegative && "text-destructive",
        )}>
          {cells.change}
        </span>
      );
    },
    meta: { cellClass: "text-right", label: "Change" },
  },
);

const balanceColumn = col.accessor(
  (row) => (row.runningTotal?.length > 0 ? parseFloat(row.runningTotal[0].quantity) || 0 : 0),
  {
    id: "balance",
    header: ({ column }) => <TableSortHeader column={column} align="right">Balance</TableSortHeader>,
    cell: ({ row }) => {
      const cells = resolveRegisterCells(row.original);
      return (
        <span className="block whitespace-nowrap text-right font-mono text-sm text-muted-foreground">
          {cells.balance}
        </span>
      );
    },
    meta: { cellClass: "text-right", label: "Balance" },
  },
);

const transactionColumns = [
  selectColumn,
  dateColumn,
  statusColumn,
  descriptionColumn,
  tagsColumn,
  fromColumn,
  toColumn,
  amountColumn,
];

const registerColumns = [
  selectColumn,
  dateColumn,
  statusColumn,
  descriptionColumn,
  tagsColumn,
  otherAccountsColumn,
  changeColumn,
  balanceColumn,
];

// ── mobile sort control ─────────────────────────────────────────────────────
// The card layout has no column headers, so mobile users have no way to sort.
// This compact bar drives the same TanStack `sorting` state the desktop header
// toggles: a field picker plus an ascending/descending toggle. Sortable
// columns are those that opt in via `meta.label`.

const NO_SORT = "__default__";

function MobileSortControl({ table }) {
  const sortableColumns = table
    .getAllLeafColumns()
    .filter((c) => c.getCanSort() && c.columnDef.meta?.label);
  const active = table.getState().sorting[0];
  const activeColumn = active ? table.getColumn(active.id) : null;

  return (
    <div className="mb-2 flex items-center gap-2 sm:hidden">
      <span className="shrink-0 text-xs text-muted-foreground">Sort by</span>
      <Select
        value={active?.id ?? NO_SORT}
        onValueChange={(val) => {
          if (val === NO_SORT) table.setSorting([]);
          else table.setSorting([{ id: val, desc: false }]);
        }}
      >
        <SelectTrigger size="sm" className="h-8 flex-1">
          <SelectValue placeholder="Default order" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={NO_SORT}>Default order</SelectItem>
          {sortableColumns.map((c) => (
            <SelectItem key={c.id} value={c.id}>
              {c.columnDef.meta.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Button
        variant="outline"
        size="icon"
        className="size-8 shrink-0"
        disabled={!activeColumn}
        onClick={() => activeColumn?.toggleSorting(activeColumn.getIsSorted() === "asc")}
        title={active?.desc ? "Descending — tap for ascending" : "Ascending — tap for descending"}
        aria-label="Toggle sort direction"
      >
        {active?.desc ? <ArrowDown className="size-4" /> : <ArrowUp className="size-4" />}
      </Button>
    </div>
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
  const [sorting, setSorting] = useState([]);

  const selectable = selectedFids !== undefined && onSelectionChange !== undefined;
  const isRegisterMode = !!registerRows;

  const rows = isRegisterMode ? (registerRows || []) : (transactions || []);

  const allFids = useMemo(() => rows.filter((r) => r.fid).map((r) => r.fid), [rows]);
  const allSelected = selectable && allFids.length > 0 && allFids.every((fid) => selectedFids.has(fid));
  const someSelected = selectable && allFids.some((fid) => selectedFids.has(fid));

  function toggleSelectAll() {
    if (!selectable) return;
    if (allSelected) {
      const next = new Set(selectedFids);
      for (const fid of allFids) next.delete(fid);
      onSelectionChange(next);
    } else {
      const next = new Set(selectedFids);
      for (const fid of allFids) next.add(fid);
      onSelectionChange(next);
    }
  }

  const columnVisibility = useMemo(() => {
    const vis = { select: selectable };
    for (const col of hiddenColumns) vis[col] = false;
    return vis;
  }, [selectable, hiddenColumns]);

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
      allSelected,
      someSelected,
      toggleSelectAll,
    },
  });

  if (rows.length === 0) {
    return <p className="py-4 text-muted-foreground">No transactions for this period.</p>;
  }

  const pageRows = table.getRowModel().rows;
  const { pageIndex } = table.getState().pagination;
  const pageCount = table.getPageCount();
  const total = rows.length;
  const rangeStart = total === 0 ? 0 : pageIndex * pagination.pageSize + 1;
  const rangeEnd = Math.min((pageIndex + 1) * pagination.pageSize, total);
  const showPagination = total > 0;

  const visibleColumnCount = table.getVisibleLeafColumns().length;

  return (
    <div>
      {/* Desktop table */}
      <div className="hidden overflow-x-auto sm:block">
        <Table>
          <TableHeader className="sticky top-0 z-10 bg-background">
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
            {pageRows.map((row) => (
              <TableRowGroup
                key={row.id}
                row={row}
                isRegisterMode={isRegisterMode}
                selectable={selectable}
                selectedFids={selectedFids}
                accounts={accounts}
                onStatusChange={onStatusChange}
                onTagsChanged={onStatusChange}
                onDeleted={onDeleted}
                visibleColumnCount={visibleColumnCount}
              />
            ))}
          </TableBody>
        </Table>
      </div>

      {/* Mobile sort + cards */}
      <MobileSortControl table={table} />
      <div className="-mx-4 flex flex-col gap-2 sm:hidden">
        {pageRows.map((row) => (
          <MobileCard
            key={row.id}
            row={row}
            isRegisterMode={isRegisterMode}
            focusedAccount={focusedAccount}
            selectable={selectable}
            selectedFids={selectedFids}
            onSelectionChange={onSelectionChange}
            onStatusChange={onStatusChange}
            accounts={accounts}
            onDeleted={onDeleted}
          />
        ))}
      </div>

      {/* Pagination */}
      {showPagination && (
        <div className="mt-3 flex w-full flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <Label className="whitespace-nowrap text-sm text-muted-foreground">Rows per page:</Label>
            <Select
              value={String(table.getState().pagination.pageSize)}
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

// ── desktop row (with optional expansion) ─────────────────────────────────

function TableRowGroup({ row, isRegisterMode, selectable, selectedFids, accounts, onStatusChange, onTagsChanged, onDeleted, visibleColumnCount }) {
  const tx = row.original;
  const isSelected = selectable && tx.fid && selectedFids?.has(tx.fid);
  const split = isRegisterMode ? null : accountSplit(tx);

  return (
    <>
      <TableRow
        onClick={() => { if (!isRegisterMode && tx.fid) row.toggleExpanded(); }}
        className={cn(
          !isRegisterMode && "cursor-pointer",
          isSelected && "bg-primary/10 hover:bg-primary/15",
        )}
      >
        {row.getVisibleCells().map((cell) => {
          if (split?.isMultiple && cell.column.id === "from") {
            return (
              <TableCell
                key={cell.id}
                colSpan={2}
                className="text-sm italic text-muted-foreground"
              >
                multiple accounts
              </TableCell>
            );
          }
          if (split?.isMultiple && cell.column.id === "to") {
            return null;
          }
          return (
            <TableCell
              key={cell.id}
              className={cell.column.columnDef.meta?.cellClass}
            >
              {flexRender(cell.column.columnDef.cell, cell.getContext())}
            </TableCell>
          );
        })}
      </TableRow>
      {row.getIsExpanded() && (
        <TableRow className="bg-muted/30 hover:bg-muted/30">
          <TableCell colSpan={visibleColumnCount} className="p-0">
            <EditableDetailRow
              tx={tx}
              accounts={accounts}
              onSaved={() => { row.toggleExpanded(); if (onStatusChange) onStatusChange(); }}
              onDeleted={() => { row.toggleExpanded(); if (onDeleted) onDeleted(); }}
              onTagsChanged={onTagsChanged}
            />
          </TableCell>
        </TableRow>
      )}
    </>
  );
}

// ── mobile card ────────────────────────────────────────────────────────────

function MobileCard({ row, isRegisterMode, focusedAccount, selectable, selectedFids, onSelectionChange, onStatusChange, accounts, onDeleted }) {
  const tx = row.original;
  let amountCell = "";
  let balanceCell = "";
  let changePositive = false;
  let changeNegative = false;
  const split = isRegisterMode ? null : accountSplit(tx);

  if (isRegisterMode) {
    const cells = resolveRegisterCells(tx);
    amountCell = cells.change;
    balanceCell = cells.balance;
    changePositive = cells.changePositive;
    changeNegative = cells.changeNegative;
  } else if (focusedAccount) {
    const display = accountRegisterDisplay(tx, focusedAccount);
    amountCell = display?.amount || "";
  } else {
    const display = generalDisplay(tx);
    amountCell = display?.amount || "";
  }

  const isSelected = selectable && tx.fid && selectedFids?.has(tx.fid);

  return (
    <Card
      size="sm"
      className={cn(
        "ring-0",
        !isRegisterMode && "cursor-pointer",
        isSelected && "bg-primary/5 ring-1 ring-primary",
      )}
      onClick={() => { if (!isRegisterMode && tx.fid) row.toggleExpanded(); }}
    >
      <CardContent className="flex flex-col gap-1.5">
        <div className="flex items-center justify-between gap-2">
          <span className="shrink-0 font-mono text-xs text-muted-foreground">
            {formatDate(tx.date)}
          </span>
          {selectable && (
            <span onClick={(e) => e.stopPropagation()}>
              <Checkbox
                checked={isSelected}
                onCheckedChange={() => {
                  if (!tx.fid) return;
                  const next = new Set(selectedFids);
                  if (next.has(tx.fid)) next.delete(tx.fid);
                  else next.add(tx.fid);
                  onSelectionChange(next);
                }}
              />
            </span>
          )}
          <span className="flex-1 truncate font-medium" onClick={(e) => e.stopPropagation()}>
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
          <div className="flex shrink-0 items-center gap-1">
            {(tx.postings || []).some((p) => p.balanceAssertion) && (
              <Scale className="size-3 shrink-0 text-muted-foreground" />
            )}
            {tx.stripeTransactionId && <StripeIndicator id={tx.stripeTransactionId} />}
            <span className={cn(
              "whitespace-nowrap font-mono text-sm",
              isRegisterMode && changePositive && "text-success",
              isRegisterMode && changeNegative && "text-destructive",
            )}>{amountCell}</span>
            <StatusButton fid={tx.fid} status={tx.status} onStatusChange={onStatusChange} />
          </div>
        </div>
        <div className="flex items-center justify-between gap-2">
          <div className="min-w-0 flex-1 text-xs">
            {isRegisterMode ? (
              <EditableOtherAccountCell
                fid={tx.fid}
                otherAccounts={tx.otherAccounts}
                accounts={accounts}
                onSaved={onStatusChange}
              />
            ) : split?.isMultiple ? (
              <span className="italic text-muted-foreground">multiple accounts</span>
            ) : (
              <span className="flex flex-wrap items-center gap-1 text-muted-foreground">
                <EditableAccountSideCell
                  tx={tx}
                  side="from"
                  accounts={accounts}
                  onSaved={onStatusChange}
                />
                <span className="text-muted-foreground/60">→</span>
                <EditableAccountSideCell
                  tx={tx}
                  side="to"
                  accounts={accounts}
                  onSaved={onStatusChange}
                />
              </span>
            )}
          </div>
          {isRegisterMode && balanceCell && (
            <div className="shrink-0 font-mono text-xs text-muted-foreground">{balanceCell}</div>
          )}
        </div>
        {isRegisterMode && tx.fid ? (
          <div onClick={(e) => e.stopPropagation()}>
            <TagEditor fid={tx.fid} tags={tx.tags} onChanged={onStatusChange} className="mt-1" />
          </div>
        ) : (tx.tags && Object.keys(tx.tags).length > 0 && (
          <div className="mt-1 flex flex-wrap gap-1">
            {Object.entries(tx.tags).map(([k, v]) => (
              <Badge key={k} variant="secondary" className="text-xs">
                {v ? `${k}:${v}` : k}
              </Badge>
            ))}
          </div>
        ))}
        {row.getIsExpanded() && (
          <EditableDetailRow
            tx={tx}
            accounts={accounts}
            onSaved={onStatusChange}
            onDeleted={() => { row.toggleExpanded(); if (onDeleted) onDeleted(); }}
            onTagsChanged={onStatusChange}
          />
        )}
      </CardContent>
    </Card>
  );
}
