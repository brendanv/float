import { useState, useRef } from "react";
import { Check, Loader2, Trash2 } from "lucide-react";
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
import { formatAmounts, formatDate } from "../format.js";
import { PostingFields } from "./posting-fields.jsx";
import { useNavigate } from "@tanstack/react-router";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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
import { cn } from "@/lib/utils";

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
    amount = Object.entries(sumByCommodity).map(([c, q]) => `${c}${q}`).join(", ");
  }
  return { otherAccounts, amount };
}

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
    <Button
      variant="ghost"
      size="icon-xs"
      onClick={handleClick}
      disabled={updating}
      title={title}
      className="rounded-full"
    >
      {updating ? (
        <Loader2 className="h-3 w-3 animate-spin" />
      ) : (
        <Check className={isReviewed ? "text-success" : "text-muted-foreground/40"} />
      )}
    </Button>
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
        postings: postings.map((p) => ({ account: p.account, amount: formatAmounts(p.amounts) })),
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
          <Loader2 className="h-3 w-3 animate-spin" />
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

function EditableDetailRow({ tx, accounts, onSaved, onDeleted }) {
  function toFields(ps) {
    return (ps || []).map((p) => ({ account: p.account, amount: formatAmounts(p.amounts) }));
  }

  const [postings, setPostings] = useState(() => toFields(tx.postings));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);
  const [deleting, setDeleting] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const containerRef = useRef(null);
  // Track current postings in a ref so the focusout handler always sees the latest value
  const postingsRef = useRef(postings);
  postingsRef.current = postings;

  async function save(currentPostings) {
    setSaving(true);
    setError(null);
    try {
      await ledgerClient.updateTransaction({
        fid: tx.fid,
        description: tx.description,
        date: tx.date,
        postings: currentPostings.map((p) => ({ account: p.account.trim(), amount: p.amount.trim() })),
      });
      if (onSaved) onSaved();
    } catch (err) {
      setError(err.message || String(err));
      setSaving(false);
    }
  }

  function handleFocusOut(e) {
    // Only save when focus leaves the entire container (not when moving between fields within it)
    if (containerRef.current && !containerRef.current.contains(e.relatedTarget)) {
      save(postingsRef.current);
    }
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
      onFocusOut={handleFocusOut}
    >
      {saving ? (
        <Loader2 className="h-3 w-3 animate-spin" />
      ) : (
        <PostingFields postings={postings} onChange={setPostings} accounts={accounts} />
      )}
      {error && <p className="mt-2 text-xs text-destructive">{error}</p>}
      <div className="mt-3 flex justify-end">
        <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
          <DialogTrigger asChild>
            <Button variant="ghost" size="xs" className="text-destructive hover:text-destructive" disabled={saving || deleting}>
              <Trash2 className="h-3 w-3" /> Delete
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
                {deleting ? <Loader2 className="h-3 w-3 animate-spin" /> : "Delete"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </div>
  );
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

export function TransactionTable({ transactions, registerRows, focusedAccount, onStatusChange, onDeleted, accounts = [], selectedFids, onSelectionChange }) {
  const [expanded, setExpanded] = useState(null);

  const selectable = selectedFids !== undefined && onSelectionChange !== undefined;
  const isRegisterMode = !!registerRows;

  const rows = isRegisterMode ? registerRows : (transactions || []);

  if (!rows || rows.length === 0) {
    return <p className="py-4 text-muted-foreground">No transactions for this period.</p>;
  }

  function toggle(fid) {
    if (isRegisterMode) return;
    setExpanded(expanded === fid ? null : fid);
  }

  function toggleSelect(fid) {
    if (!selectable || !fid) return;
    const next = new Set(selectedFids);
    if (next.has(fid)) {
      next.delete(fid);
    } else {
      next.add(fid);
    }
    onSelectionChange(next);
  }

  const allFids = rows.filter((r) => r.fid).map((r) => r.fid);
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

  const isAccountRegister = isRegisterMode || !!focusedAccount;

  function resolveDisplay(tx) {
    if (focusedAccount) return accountRegisterDisplay(tx, focusedAccount);
    return generalDisplay(tx);
  }

  // Group rows by date
  const dateGroups = [];
  let currentGroup = null;
  let rowIndex = 0;
  for (const row of rows) {
    if (!currentGroup || row.date !== currentGroup.date) {
      currentGroup = { date: row.date, rows: [] };
      dateGroups.push(currentGroup);
    }
    currentGroup.rows.push({ row, index: rowIndex++ });
  }

  const extraCols = isRegisterMode ? 1 : 0;
  const colSpan = (selectable ? 5 : 4) + extraCols;

  return (
    <div>
      {/* Desktop table */}
      <div className="hidden max-h-[calc(100vh-14rem)] overflow-x-auto overflow-y-auto sm:block">
        <Table>
          <TableHeader className="sticky top-0 z-10 bg-background">
            <TableRow>
              {selectable && (
                <TableHead className="w-6 pr-0">
                  <Checkbox
                    checked={allSelected}
                    indeterminate={!allSelected && someSelected}
                    onCheckedChange={toggleSelectAll}
                    onClick={(e) => e.stopPropagation()}
                    title={allSelected ? "Deselect all" : "Select all"}
                  />
                </TableHead>
              )}
              <TableHead className="w-8"></TableHead>
              <TableHead>Description</TableHead>
              <TableHead>{isAccountRegister ? "Other accounts" : "From \u2192 To"}</TableHead>
              <TableHead className="text-right">{isRegisterMode ? "Change" : "Amount"}</TableHead>
              {isRegisterMode && <TableHead className="text-right">Balance</TableHead>}
            </TableRow>
          </TableHeader>
          {dateGroups.map((group) => [
            <TableHeader key={"date-" + group.date}>
              <TableRow>
                <TableHead colSpan={colSpan} className="py-1 font-mono text-[10px] font-normal text-muted-foreground">
                  {formatDate(group.date)}
                </TableHead>
              </TableRow>
            </TableHeader>,
            <TableBody key={"body-" + group.date}>
              {group.rows.map(({ row, index }) => {
                let accountCell, amountCell, balanceCell, changePositive, changeNegative;
                if (isRegisterMode) {
                  const cells = resolveRegisterCells(row);
                  accountCell = cells.otherAccounts;
                  amountCell = cells.change;
                  balanceCell = cells.balance;
                  changePositive = cells.changePositive;
                  changeNegative = cells.changeNegative;
                } else {
                  const display = resolveDisplay(row);
                  accountCell = isAccountRegister
                    ? (display?.otherAccounts || "")
                    : display ? (display.from === "various accounts" && display.to === "various accounts" ? "various accounts" : `${display.from} \u2192 ${display.to}`) : "";
                  amountCell = display?.amount || "";
                }
                const key = row.fid ? row.fid + "-" + index : row.date + row.description + index;
                const isSelected = selectable && row.fid && selectedFids.has(row.fid);
                return [
                  <TableRow
                    key={key}
                    onClick={() => toggle(row.fid)}
                    className={cn(
                      !isRegisterMode && "cursor-pointer",
                      isSelected && "bg-primary/10 hover:bg-primary/15",
                    )}
                  >
                    {selectable && (
                      <TableCell className="w-6 pr-0" onClick={(e) => e.stopPropagation()}>
                        <Checkbox
                          checked={isSelected}
                          onCheckedChange={() => toggleSelect(row.fid)}
                        />
                      </TableCell>
                    )}
                    <TableCell className="w-8 pr-0">
                      <StatusButton fid={row.fid} status={row.status} onStatusChange={onStatusChange} />
                    </TableCell>
                    <TableCell className="whitespace-normal">
                      <EditableDescriptionCell
                        fid={row.fid}
                        description={row.description}
                        date={row.date}
                        postings={row.postings}
                        payee={row.payee}
                        note={row.note}
                        onSaved={onStatusChange}
                      />
                      {row.tags && Object.keys(row.tags).length > 0 && (
                        <span className="ml-2 inline-flex flex-wrap gap-1">
                          {Object.entries(row.tags).map(([k, v]) => (
                            <Badge key={k} variant="secondary" className="text-xs">
                              {v ? `${k}:${v}` : k}
                            </Badge>
                          ))}
                        </span>
                      )}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">{accountCell}</TableCell>
                    <TableCell className={cn(
                      "whitespace-nowrap text-right font-mono text-sm",
                      isRegisterMode && changePositive && "text-success",
                      isRegisterMode && changeNegative && "text-destructive",
                    )}>{amountCell}</TableCell>
                    {isRegisterMode && (
                      <TableCell className="whitespace-nowrap text-right font-mono text-sm text-muted-foreground">{balanceCell}</TableCell>
                    )}
                  </TableRow>,
                  !isRegisterMode && expanded === row.fid && (
                    <TableRow key={key + "-detail"} className="bg-muted/30 hover:bg-muted/30">
                      <TableCell colSpan={colSpan} className="p-0">
                        <EditableDetailRow tx={row} accounts={accounts} onSaved={onStatusChange} onDeleted={() => { setExpanded(null); if (onDeleted) onDeleted(); }} />
                      </TableCell>
                    </TableRow>
                  ),
                ];
              })}
            </TableBody>,
          ])}
        </Table>
      </div>

      {/* Mobile cards */}
      <div className="max-h-[calc(100vh-14rem)] space-y-2 overflow-y-auto sm:hidden">
        {dateGroups.map((group) => [
          <div key={"date-" + group.date} className="sticky top-0 z-[1] bg-background px-1 py-0.5 font-mono text-[10px] font-semibold text-muted-foreground">
            {formatDate(group.date)}
          </div>,
          ...group.rows.map(({ row, index }) => {
            let accountCell, amountCell, balanceCell, changePositive, changeNegative;
            if (isRegisterMode) {
              const cells = resolveRegisterCells(row);
              accountCell = cells.otherAccounts;
              amountCell = cells.change;
              balanceCell = cells.balance;
              changePositive = cells.changePositive;
              changeNegative = cells.changeNegative;
            } else {
              const display = resolveDisplay(row);
              accountCell = isAccountRegister
                ? (display?.otherAccounts || "")
                : display ? (display.from === "various accounts" && display.to === "various accounts" ? "various accounts" : `${display.from} \u2192 ${display.to}`) : "";
              amountCell = display?.amount || "";
            }
            const isSelected = selectable && row.fid && selectedFids.has(row.fid);
            return (
              <Card
                key={row.fid ? row.fid + "-" + index : row.date + row.description + index}
                size="sm"
                className={cn(
                  !isRegisterMode && "cursor-pointer",
                  isSelected && "bg-primary/5 ring-primary",
                )}
                onClick={() => toggle(row.fid)}
              >
                <CardContent className="space-y-1.5">
                  <div className="flex items-center justify-between gap-2">
                    {selectable && (
                      <span onClick={(e) => e.stopPropagation()}>
                        <Checkbox
                          checked={isSelected}
                          onCheckedChange={() => toggleSelect(row.fid)}
                        />
                      </span>
                    )}
                    <span className="truncate font-medium" onClick={(e) => e.stopPropagation()}>
                      <EditableDescriptionCell
                        fid={row.fid}
                        description={row.description}
                        date={row.date}
                        postings={row.postings}
                        payee={row.payee}
                        note={row.note}
                        onSaved={onStatusChange}
                      />
                    </span>
                    <div className="flex shrink-0 items-center gap-1">
                      <span className={cn(
                        "whitespace-nowrap font-mono text-sm",
                        isRegisterMode && changePositive && "text-success",
                        isRegisterMode && changeNegative && "text-destructive",
                      )}>{amountCell}</span>
                      <StatusButton fid={row.fid} status={row.status} onStatusChange={onStatusChange} />
                    </div>
                  </div>
                  <div className="flex items-center justify-between gap-2">
                    <div className="truncate text-xs text-muted-foreground">{accountCell}</div>
                    {isRegisterMode && balanceCell && (
                      <div className="shrink-0 font-mono text-xs text-muted-foreground">{balanceCell}</div>
                    )}
                  </div>
                  {row.tags && Object.keys(row.tags).length > 0 && (
                    <div className="mt-1 flex flex-wrap gap-1">
                      {Object.entries(row.tags).map(([k, v]) => (
                        <Badge key={k} variant="secondary" className="text-xs">
                          {v ? `${k}:${v}` : k}
                        </Badge>
                      ))}
                    </div>
                  )}
                  {!isRegisterMode && expanded === row.fid && (
                    <EditableDetailRow tx={row} accounts={accounts} onSaved={onStatusChange} onDeleted={() => { setExpanded(null); if (onDeleted) onDeleted(); }} />
                  )}
                </CardContent>
              </Card>
            );
          }),
        ])}
      </div>
    </div>
  );
}
