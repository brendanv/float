import { useState, useRef } from "react";
import { ledgerClient } from "../client.js";
import { formatAmounts, formatDate } from "../format.js";
import { PostingFields } from "./posting-fields.jsx";
import { useNavigate } from "@tanstack/react-router";

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

function CheckIcon() {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
      <polyline points="20 6 9 17 4 12" />
    </svg>
  );
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
    <button
      class="btn btn-ghost btn-xs btn-circle"
      onClick={handleClick}
      disabled={updating}
      title={title}
    >
      {updating
        ? <span class="loading loading-spinner loading-xs" />
        : <span class={isReviewed ? "text-success" : "text-base-content/25"}><CheckIcon /></span>
      }
    </button>
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
        {saving
          ? <span class="loading loading-spinner loading-xs" />
          : (
            <input
              class="input input-bordered input-xs w-full"
              value={draft}
              onInput={(e) => setDraft(e.target.value)}
              onBlur={save}
              onKeyDown={handleKeyDown}
              autoFocus
            />
          )
        }
        {error && <span class="text-error text-xs block mt-1">{error}</span>}
      </span>
    );
  }

  return (
    <span
      onClick={(e) => { e.stopPropagation(); setDraft(description); setEditing(true); }}
      class="cursor-text hover:underline decoration-dotted"
      title="Click to edit description"
    >
      {payee ? (
        <>
          <strong
            class="cursor-pointer hover:underline"
            onClick={(e) => { e.stopPropagation(); navigate({ to: "/transactions", search: { payee } }); }}
            title={"Show all transactions for " + payee}
          >{payee}</strong>
          {note && <span class="text-base-content/60"> · {note}</span>}
        </>
      ) : (
        description
      )}
    </span>
  );
}

function EditableDetailRow({ tx, accounts, onSaved }) {
  function toFields(ps) {
    return (ps || []).map((p) => ({ account: p.account, amount: formatAmounts(p.amounts) }));
  }

  const [postings, setPostings] = useState(() => toFields(tx.postings));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);
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

  return (
    <div
      ref={containerRef}
      class="p-3"
      onClick={(e) => e.stopPropagation()}
      onFocusOut={handleFocusOut}
    >
      {saving
        ? <span class="loading loading-spinner loading-xs" />
        : <PostingFields postings={postings} onChange={setPostings} accounts={accounts} />
      }
      {error && <p class="text-error text-xs mt-2">{error}</p>}
    </div>
  );
}

export function TransactionTable({ transactions, focusedAccount, onStatusChange, accounts = [], selectedFids, onSelectionChange }) {
  const [expanded, setExpanded] = useState(null);

  const selectable = selectedFids !== undefined && onSelectionChange !== undefined;

  if (!transactions || transactions.length === 0) {
    return <p class="text-base-content/60 py-4">No transactions for this period.</p>;
  }

  function toggle(fid) {
    setExpanded(expanded === fid ? null : fid);
  }

  function toggleSelect(e, fid) {
    e.stopPropagation();
    if (!selectable || !fid) return;
    const next = new Set(selectedFids);
    if (next.has(fid)) {
      next.delete(fid);
    } else {
      next.add(fid);
    }
    onSelectionChange(next);
  }

  const allFids = transactions.filter((tx) => tx.fid).map((tx) => tx.fid);
  const allSelected = selectable && allFids.length > 0 && allFids.every((fid) => selectedFids.has(fid));
  const someSelected = selectable && allFids.some((fid) => selectedFids.has(fid));

  function toggleSelectAll(e) {
    e.stopPropagation();
    if (!selectable) return;
    if (allSelected) {
      // Deselect all visible
      const next = new Set(selectedFids);
      for (const fid of allFids) next.delete(fid);
      onSelectionChange(next);
    } else {
      // Select all visible
      const next = new Set(selectedFids);
      for (const fid of allFids) next.add(fid);
      onSelectionChange(next);
    }
  }

  const isAccountRegister = !!focusedAccount;

  function resolveDisplay(tx) {
    if (isAccountRegister) {
      return accountRegisterDisplay(tx, focusedAccount);
    }
    return generalDisplay(tx);
  }

  // Group transactions by date for DaisyUI alternating thead/tbody pattern
  const dateGroups = [];
  let currentGroup = null;
  let txIndex = 0;
  for (const tx of transactions) {
    if (!currentGroup || tx.date !== currentGroup.date) {
      currentGroup = { date: tx.date, txs: [] };
      dateGroups.push(currentGroup);
    }
    currentGroup.txs.push({ tx, index: txIndex++ });
  }

  const colSpan = selectable ? 5 : 4;

  return (
    <div>
      {/* Desktop table */}
      <div class="hidden sm:block overflow-x-auto overflow-y-auto max-h-[calc(100vh-14rem)]">
        <table class="table table-pin-rows table-zebra table-sm w-full">
          <thead>
            <tr>
              {selectable && (
                <th class="w-6 pr-0">
                  <input
                    type="checkbox"
                    class="checkbox checkbox-xs"
                    checked={allSelected}
                    indeterminate={!allSelected && someSelected}
                    onChange={toggleSelectAll}
                    onClick={(e) => e.stopPropagation()}
                    title={allSelected ? "Deselect all" : "Select all"}
                  />
                </th>
              )}
              <th class="w-8"></th>
              <th>Description</th>
              <th>{isAccountRegister ? "Other accounts" : "From \u2192 To"}</th>
              <th class="text-right">Amount</th>
            </tr>
          </thead>
          {dateGroups.map((group) => [
            <thead key={"date-" + group.date}>
              <tr>
                <th colSpan={colSpan} class="font-mono text-[10px] font-normal text-base-content/60 py-0.5">
                  {formatDate(group.date)}
                </th>
              </tr>
            </thead>,
            <tbody key={"body-" + group.date}>
              {group.txs.map(({ tx, index }) => {
                const display = resolveDisplay(tx);
                const accountCell = isAccountRegister
                  ? (display?.otherAccounts || "")
                  : display ? (display.from === "various accounts" && display.to === "various accounts" ? "various accounts" : `${display.from} \u2192 ${display.to}`) : "";
                const amountCell = display?.amount || "";
                const key = tx.fid ? tx.fid + "-" + index : tx.date + tx.description + index;
                const isSelected = selectable && tx.fid && selectedFids.has(tx.fid);
                return [
                  <tr
                    key={key}
                    onClick={() => toggle(tx.fid)}
                    class={"cursor-pointer hover" + (isSelected ? " !bg-primary/10" : "")}
                  >
                    {selectable && (
                      <td class="w-6 pr-0" onClick={(e) => e.stopPropagation()}>
                        <input
                          type="checkbox"
                          class="checkbox checkbox-xs"
                          checked={isSelected}
                          onChange={(e) => toggleSelect(e, tx.fid)}
                        />
                      </td>
                    )}
                    <td class="w-8 pr-0">
                      <StatusButton fid={tx.fid} status={tx.status} onStatusChange={onStatusChange} />
                    </td>
                    <td>
                      <EditableDescriptionCell
                        fid={tx.fid}
                        description={tx.description}
                        date={tx.date}
                        postings={tx.postings}
                        payee={tx.payee}
                        note={tx.note}
                        onSaved={onStatusChange}
                      />
                      {tx.tags && Object.keys(tx.tags).length > 0 && (
                        <span class="ml-2 inline-flex flex-wrap gap-1">
                          {Object.entries(tx.tags).map(([k, v]) => (
                            <span key={k} class="badge badge-soft badge-sm badge-primary">
                              {v ? `${k}:${v}` : k}
                            </span>
                          ))}
                        </span>
                      )}
                    </td>
                    <td class="text-sm text-base-content/70">{accountCell}</td>
                    <td class="text-right whitespace-nowrap font-mono text-sm">{amountCell}</td>
                  </tr>,
                  expanded === tx.fid && (
                    <tr key={key + "-detail"} class="bg-base-200">
                      <td colSpan={colSpan} class="p-0">
                        <EditableDetailRow tx={tx} accounts={accounts} onSaved={onStatusChange} />
                      </td>
                    </tr>
                  ),
                ];
              })}
            </tbody>,
          ])}
        </table>
      </div>

      {/* Mobile cards */}
      <div class="sm:hidden space-y-2 overflow-y-auto max-h-[calc(100vh-14rem)]">
        {dateGroups.map((group) => [
          <div key={"date-" + group.date} class="sticky top-0 z-[1] py-0.5 px-1 font-mono text-[10px] font-semibold text-base-content/60 bg-base-100">
            {formatDate(group.date)}
          </div>,
          ...group.txs.map(({ tx, index }) => {
            const display = resolveDisplay(tx);
            const accountCell = isAccountRegister
              ? (display?.otherAccounts || "")
              : display ? (display.from === "various accounts" && display.to === "various accounts" ? "various accounts" : `${display.from} \u2192 ${display.to}`) : "";
            const amountCell = display?.amount || "";
            const isSelected = selectable && tx.fid && selectedFids.has(tx.fid);
            return (
              <div
                key={tx.fid ? tx.fid + "-" + index : tx.date + tx.description + index}
                class={"card card-compact bg-base-100 shadow-sm border cursor-pointer" + (isSelected ? " border-primary bg-primary/5" : " border-base-200")}
                onClick={() => toggle(tx.fid)}
              >
                <div class="card-body">
                  <div class="flex justify-between items-center gap-2">
                    {selectable && (
                      <input
                        type="checkbox"
                        class="checkbox checkbox-xs shrink-0"
                        checked={isSelected}
                        onChange={(e) => toggleSelect(e, tx.fid)}
                        onClick={(e) => e.stopPropagation()}
                      />
                    )}
                    <span class="font-medium truncate" onClick={(e) => e.stopPropagation()}>
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
                    <div class="flex items-center gap-1 shrink-0">
                      <span class="whitespace-nowrap font-mono text-sm">{amountCell}</span>
                      <StatusButton fid={tx.fid} status={tx.status} onStatusChange={onStatusChange} />
                    </div>
                  </div>
                  <div class="text-xs text-base-content/60 truncate">{accountCell}</div>
                  {tx.tags && Object.keys(tx.tags).length > 0 && (
                    <div class="flex flex-wrap gap-1 mt-1">
                      {Object.entries(tx.tags).map(([k, v]) => (
                        <span key={k} class="badge badge-soft badge-sm badge-primary">
                          {v ? `${k}:${v}` : k}
                        </span>
                      ))}
                    </div>
                  )}
                  {expanded === tx.fid && (
                    <EditableDetailRow tx={tx} accounts={accounts} onSaved={onStatusChange} />
                  )}
                </div>
              </div>
            );
          }),
        ])}
      </div>
    </div>
  );
}
