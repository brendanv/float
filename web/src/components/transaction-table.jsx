import { useState, useRef, useMemo } from "react";
import {
  useReactTable,
  getCoreRowModel,
  getPaginationRowModel,
  getExpandedRowModel,
  getSortedRowModel,
  createColumnHelper,
  flexRender,
} from "@tanstack/react-table";
import { Check, Loader2, Trash2, ChevronLeft, ChevronRight, X, ArrowUp, ArrowDown, ArrowUpDown, Scale } from "lucide-react";
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
import { useNavigate } from "@tanstack/react-router";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
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
import { Separator } from "@/components/ui/separator";
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
            disabled={updating}
            className="rounded-full"
          >
            {updating ? (
              <Loader2 className="size-3 animate-spin" />
            ) : (
              <Check className={isReviewed ? "text-success" : "text-muted-foreground/40"} />
            )}
          </Button>
        }
      />
      <TooltipContent>{title}</TooltipContent>
    </Tooltip>
  );
}

function EditableDescriptionCell({ fid, description, date, postings, payee, note, onSaved }) {
  const navigate = useNavigate();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(description);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);

  async function save() {
    if (draft.trim() === description) { setEditing(false); return; }
    setSaving(true);
    setError(null);
    try {
      await ledgerClient.updateTransaction({
        fid,
        description: draft.trim(),
        date,
        postings: postings.map((p) => toPostingInput({
          account: p.account,
          commodity: p.amounts?.[0]?.commodity ?? "",
          quantity: p.amounts?.[0]?.quantity ?? "",
          cost: p.amounts?.[0]?.cost,
        })),
      });
      setEditing(false);
      if (onSaved) onSaved();
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setSaving(false);
    }
  }

  function handleKeyDown(e) {
    if (e.key === "Enter") { e.preventDefault(); save(); }
    if (e.key === "Escape") { setDraft(description); setEditing(false); setError(null); }
  }

  if (editing) {
    return (
      <span onClick={(e) => e.stopPropagation()}>
        {saving ? (
          <Loader2 className="size-3 animate-spin" />
        ) : (
          <Input
            className="h-6 w-full"
            value={draft}
            onInput={(e) => setDraft(e.target.value)}
            onBlur={save}
            onKeyDown={handleKeyDown}
            autoFocus
          />
        )}
        {error && <span className="mt-1 block text-xs text-destructive">{error}</span>}
      </span>
    );
  }

  return (
    <span
      onClick={(e) => { e.stopPropagation(); setDraft(description); setEditing(true); }}
      className="cursor-text decoration-dotted hover:underline"
      title="Click to edit description"
    >
      {payee ? (
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
      )}
    </span>
  );
}

function EditableOtherAccountCell({ fid, otherAccounts, accounts, onSaved }) {
  const [editing, setEditing] = useState(false);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [txData, setTxData] = useState(null);
  const [newAccount, setNewAccount] = useState("");
  const [error, setError] = useState(null);

  const canEdit = !!fid && otherAccounts.length === 1;
  const displayText = otherAccounts.length === 0 ? ""
    : otherAccounts.length === 1 ? otherAccounts[0]
    : "various accounts";

  async function startEdit(e) {
    e.stopPropagation();
    setLoading(true);
    setError(null);
    try {
      const resp = await ledgerClient.listTransactions({ query: [`code:${fid}`], limit: 1 });
      const tx = resp.transactions?.[0];
      if (!tx) throw new Error("Transaction not found");
      setTxData(tx);
      setNewAccount(otherAccounts[0]);
      setEditing(true);
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setLoading(false);
    }
  }

  async function save(e) {
    e.stopPropagation();
    if (!txData || !newAccount) return;
    setSaving(true);
    setError(null);
    try {
      const oldAccount = otherAccounts[0];
      const newPostings = (txData.postings || []).map((p) => {
        const a = p.amounts?.[0];
        const ba = p.balanceAssertion;
        return toPostingInput({
          account: p.account === oldAccount ? newAccount : p.account,
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
      setEditing(false);
      setTxData(null);
      if (onSaved) onSaved();
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setSaving(false);
    }
  }

  function cancel(e) {
    e.stopPropagation();
    setEditing(false);
    setTxData(null);
    setError(null);
  }

  if (!canEdit) {
    return <span className="text-sm text-muted-foreground">{displayText}</span>;
  }

  if (loading) {
    return <Loader2 className="size-3 animate-spin" />;
  }

  if (editing) {
    return (
      <span onClick={(e) => e.stopPropagation()} className="flex items-center gap-1">
        <span className="min-w-40 flex-1">
          <AccountInput value={newAccount} onChange={setNewAccount} accounts={accounts} />
        </span>
        <Button variant="ghost" size="icon-xs" onClick={save} disabled={saving} title="Save">
          {saving ? <Loader2 className="size-3 animate-spin" /> : <Check className="size-3" />}
        </Button>
        <Button variant="ghost" size="icon-xs" onClick={cancel} disabled={saving} title="Cancel">
          <X className="size-3" />
        </Button>
        {error && <span className="ml-1 text-xs text-destructive">{error}</span>}
      </span>
    );
  }

  return (
    <span
      onClick={startEdit}
      className="cursor-text text-sm text-muted-foreground decoration-dotted hover:underline"
      title="Click to change account"
    >
      {displayText}
    </span>
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
  const containerRef = useRef(null);

  const isDirty = JSON.stringify(postings) !== JSON.stringify(initialPostings);

  async function save() {
    setSaving(true);
    setError(null);
    const trimmed = postings.map(toPostingInput);
    const autoReview = tx.status !== "Cleared" && trimmed.every((p) => !p.account.toLowerCase().includes("unknown"));
    try {
      await ledgerClient.updateTransaction({
        fid: tx.fid,
        description: tx.description,
        date: tx.date,
        postings: trimmed,
        status: autoReview ? "Cleared" : "",
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
    <div
      ref={containerRef}
      className="p-3"
      onClick={(e) => e.stopPropagation()}
    >
      {saving ? (
        <Loader2 className="size-3 animate-spin" />
      ) : (
        <PostingFields postings={postings} onChange={setPostings} accounts={accounts} />
      )}
      <TagEditor fid={tx.fid} tags={tx.tags} onChanged={onTagsChanged} />
      {error && <p className="mt-2 whitespace-pre-wrap break-words text-xs text-destructive">{error}</p>}
      <div className="mt-3 flex justify-between gap-2">
        <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
          <DialogTrigger asChild>
            <Button variant="ghost" size="xs" className="text-destructive hover:text-destructive" disabled={saving || deleting}>
              <Trash2 className="size-3" data-icon="inline-start" /> Delete
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
              <Button variant="destructive" onClick={handleDelete} disabled={deleting}>
                {deleting ? <Loader2 className="size-3 animate-spin" /> : "Delete"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
        <div className="flex gap-2">
          <Button variant="outline" size="xs" onClick={cancel} disabled={!isDirty || saving || deleting}>
            Cancel
          </Button>
          <Button size="xs" onClick={save} disabled={!isDirty || saving || deleting}>
            {saving ? <Loader2 className="size-3 animate-spin" /> : "Save"}
          </Button>
        </div>
      </div>
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

  return (
    <div className={cn("mt-3", className)}>
      <div className="flex flex-wrap items-center gap-1">
        {Object.entries(tags || {}).map(([k, v]) => (
          <Badge key={k} variant="secondary" className="text-xs gap-1 pr-1">
            {v ? `${k}:${v}` : k}
            {removingKey === k ? (
              <Loader2 className="size-2.5 animate-spin" />
            ) : (
              <button
                className="rounded-sm p-0.5 hover:bg-foreground/20 disabled:opacity-50"
                onClick={() => removeTag(k)}
                disabled={isBusy}
                title={`Remove tag "${k}"`}
              >
                <X className="size-2.5" />
              </button>
            )}
          </Badge>
        ))}
        {adding ? (
          <>
            <Input
              className="h-6 w-24"
              placeholder="key"
              value={tagKey}
              onChange={(e) => setTagKey(e.target.value)}
              onKeyDown={(e) => { if (e.key === "Enter") addTag(); if (e.key === "Escape") cancelAdd(); }}
              autoFocus
            />
            <Input
              className="h-6 w-28"
              placeholder="value (optional)"
              value={tagValue}
              onChange={(e) => setTagValue(e.target.value)}
              onKeyDown={(e) => { if (e.key === "Enter") addTag(); if (e.key === "Escape") cancelAdd(); }}
            />
            <Button size="xs" disabled={working || !tagKey.trim()} onClick={addTag}>
              {working ? <Loader2 className="size-3 animate-spin" /> : "Add"}
            </Button>
            <Button variant="ghost" size="xs" disabled={working} onClick={cancelAdd}>Cancel</Button>
          </>
        ) : (
          <Button
            variant="ghost"
            size="xs"
            className="text-muted-foreground"
            onClick={() => setAdding(true)}
            disabled={isBusy}
          >
            + Tag
          </Button>
        )}
      </div>
      {error && <p className="mt-1 text-xs text-destructive">{error}</p>}
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

// ── sort header ────────────────────────────────────────────────────────────

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

// ── column definitions ─────────────────────────────────────────────────────
// Cell renderers read mutable state from table.options.meta to avoid stale
// closures when useMemo deps are unchanged between renders.

const txHelper = createColumnHelper();
const regHelper = createColumnHelper();

// General transactions columns (also used for focusedAccount / non-register mode)
const transactionColumns = [
  txHelper.display({
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
  }),
  txHelper.accessor("date", {
    id: "date",
    header: ({ column }) => <SortableHeader column={column}>Date</SortableHeader>,
    cell: ({ getValue }) => (
      <span className="font-mono text-xs text-muted-foreground whitespace-nowrap">
        {formatDate(getValue())}
      </span>
    ),
    meta: { headerClass: "w-28", cellClass: "w-28" },
  }),
  txHelper.display({
    id: "status",
    header: "",
    cell: ({ row, table }) => {
      const { onStatusChange } = table.options.meta;
      return <StatusButton fid={row.original.fid} status={row.original.status} onStatusChange={onStatusChange} />;
    },
    meta: { headerClass: "w-8", cellClass: "w-8 pr-0" },
  }),
  txHelper.accessor((tx) => tx.description, {
    id: "description",
    header: ({ column }) => <SortableHeader column={column}>Description</SortableHeader>,
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
  }),
  txHelper.display({
    id: "tags",
    header: "Tags",
    cell: ({ row }) => {
      const tags = row.original.tags;
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
  }),
  txHelper.display({
    id: "accounts",
    header: ({ table }) => table.options.meta.focusedAccount ? "Other accounts" : "From \u2192 To",
    cell: ({ row, table }) => {
      const { focusedAccount } = table.options.meta;
      const tx = row.original;
      if (focusedAccount) {
        const display = accountRegisterDisplay(tx, focusedAccount);
        return <span className="text-sm text-muted-foreground">{display?.otherAccounts || ""}</span>;
      }
      const display = generalDisplay(tx);
      if (!display) return null;
      const accountText = display.from === "various accounts" && display.to === "various accounts"
        ? "various accounts"
        : `${display.from} \u2192 ${display.to}`;
      return <span className="text-sm text-muted-foreground">{accountText}</span>;
    },
  }),
  txHelper.accessor(
    (tx) => {
      const postings = tx.postings || [];
      if (postings.length === 0) return 0;
      const pos = postings.find((p) => firstQuantity(p) > 0);
      return pos ? firstQuantity(pos) : firstQuantity(postings[0]);
    },
    {
      id: "amount",
      header: ({ column }) => <SortableHeader column={column} align="right">Amount</SortableHeader>,
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
      meta: { headerClass: "text-right", cellClass: "text-right" },
    },
  ),
];

// Account register columns (register mode)
const registerColumns = [
  regHelper.display({
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
  }),
  regHelper.accessor("date", {
    id: "date",
    header: ({ column }) => <SortableHeader column={column}>Date</SortableHeader>,
    cell: ({ getValue }) => (
      <span className="font-mono text-xs text-muted-foreground whitespace-nowrap">
        {formatDate(getValue())}
      </span>
    ),
    meta: { headerClass: "w-28", cellClass: "w-28" },
  }),
  regHelper.display({
    id: "status",
    header: "",
    cell: ({ row, table }) => {
      const { onStatusChange } = table.options.meta;
      return <StatusButton fid={row.original.fid} status={row.original.status} onStatusChange={onStatusChange} />;
    },
    meta: { headerClass: "w-8", cellClass: "w-8 pr-0" },
  }),
  regHelper.accessor((tx) => tx.description, {
    id: "description",
    header: ({ column }) => <SortableHeader column={column}>Description</SortableHeader>,
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
  }),
  regHelper.display({
    id: "tags",
    header: "Tags",
    cell: ({ row, table }) => {
      const tx = row.original;
      const { onStatusChange } = table.options.meta;
      if (tx.fid) {
        return (
          <span onClick={(e) => e.stopPropagation()}>
            <TagEditor fid={tx.fid} tags={tx.tags} onChanged={onStatusChange} className="mt-0" />
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
  }),
  regHelper.display({
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
  }),
  regHelper.accessor(
    (row) => (row.change?.length > 0 ? parseFloat(row.change[0].quantity) || 0 : 0),
    {
      id: "change",
      header: ({ column }) => <SortableHeader column={column} align="right">Change</SortableHeader>,
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
      meta: { cellClass: "text-right" },
    },
  ),
  regHelper.accessor(
    (row) => (row.runningTotal?.length > 0 ? parseFloat(row.runningTotal[0].quantity) || 0 : 0),
    {
      id: "balance",
      header: ({ column }) => <SortableHeader column={column} align="right">Balance</SortableHeader>,
      cell: ({ row }) => {
        const cells = resolveRegisterCells(row.original);
        return (
          <span className="block whitespace-nowrap text-right font-mono text-sm text-muted-foreground">
            {cells.balance}
          </span>
        );
      },
      meta: { cellClass: "text-right" },
    },
  ),
];

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

      {/* Mobile cards */}
      <div className="flex flex-col gap-2 sm:hidden">
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

  return (
    <>
      <TableRow
        onClick={() => { if (!isRegisterMode && tx.fid) row.toggleExpanded(); }}
        className={cn(
          !isRegisterMode && "cursor-pointer",
          isSelected && "bg-primary/10 hover:bg-primary/15",
        )}
      >
        {row.getVisibleCells().map((cell) => (
          <TableCell
            key={cell.id}
            className={cell.column.columnDef.meta?.cellClass}
          >
            {flexRender(cell.column.columnDef.cell, cell.getContext())}
          </TableCell>
        ))}
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
  let accountCell = "";
  let amountCell = "";
  let balanceCell = "";
  let changePositive = false;
  let changeNegative = false;

  if (isRegisterMode) {
    const cells = resolveRegisterCells(tx);
    accountCell = cells.otherAccounts;
    amountCell = cells.change;
    balanceCell = cells.balance;
    changePositive = cells.changePositive;
    changeNegative = cells.changeNegative;
  } else if (focusedAccount) {
    const display = accountRegisterDisplay(tx, focusedAccount);
    accountCell = display?.otherAccounts || "";
    amountCell = display?.amount || "";
  } else {
    const display = generalDisplay(tx);
    if (display) {
      accountCell = display.from === "various accounts" && display.to === "various accounts"
        ? "various accounts"
        : `${display.from} \u2192 ${display.to}`;
      amountCell = display.amount;
    }
  }

  const isSelected = selectable && tx.fid && selectedFids?.has(tx.fid);

  return (
    <Card
      size="sm"
      className={cn(
        !isRegisterMode && "cursor-pointer",
        isSelected && "bg-primary/5 ring-primary",
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
            ) : (
              <span className="truncate text-muted-foreground">{accountCell}</span>
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
