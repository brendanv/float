import { useState } from "preact/hooks";
import { ledgerClient } from "../client.js";
import { formatAmounts, formatDate } from "../format.js";

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

function PostingDetail({ postings }) {
  return (
    <table class="table table-xs mt-2">
      <tbody>
        {postings.map((p, i) => (
          <tr key={i}>
            <td class="pl-6 text-xs text-base-content/70">{p.account}</td>
            <td class="text-right text-xs font-mono whitespace-nowrap">{formatAmounts(p.amounts)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export function TransactionTable({ transactions, focusedAccount, onStatusChange }) {
  const [expanded, setExpanded] = useState(null);

  if (!transactions || transactions.length === 0) {
    return <p class="text-base-content/60 py-4">No transactions for this period.</p>;
  }

  function toggle(fid) {
    setExpanded(expanded === fid ? null : fid);
  }

  const isAccountRegister = !!focusedAccount;

  function resolveDisplay(tx) {
    if (isAccountRegister) {
      return accountRegisterDisplay(tx, focusedAccount);
    }
    return generalDisplay(tx);
  }

  return (
    <div>
      {/* Desktop table */}
      <div class="hidden sm:block overflow-x-auto">
        <table class="table table-zebra table-sm w-full">
          <thead>
            <tr>
              <th class="w-8"></th>
              <th>Date</th>
              <th>Description</th>
              <th>{isAccountRegister ? "Other accounts" : "From \u2192 To"}</th>
              <th class="text-right">Amount</th>
            </tr>
          </thead>
          <tbody>
            {transactions.map((tx) => {
              const display = resolveDisplay(tx);
              const accountCell = isAccountRegister
                ? (display?.otherAccounts || "")
                : display ? (display.from === "various accounts" && display.to === "various accounts" ? "various accounts" : `${display.from} \u2192 ${display.to}`) : "";
              const amountCell = display?.amount || "";
              const key = tx.fid || tx.date + tx.description;
              return [
                <tr
                  key={key}
                  onClick={() => toggle(tx.fid)}
                  class="cursor-pointer hover"
                >
                  <td class="w-8 pr-0">
                    <StatusButton fid={tx.fid} status={tx.status} onStatusChange={onStatusChange} />
                  </td>
                  <td class="whitespace-nowrap font-mono text-xs">{formatDate(tx.date)}</td>
                  <td>{tx.description}</td>
                  <td class="text-sm text-base-content/70">{accountCell}</td>
                  <td class="text-right whitespace-nowrap font-mono text-sm">{amountCell}</td>
                </tr>,
                expanded === tx.fid && tx.postings && (
                  <tr key={key + "-detail"} class="bg-base-200">
                    <td colSpan={5} class="p-0">
                      <PostingDetail postings={tx.postings} />
                    </td>
                  </tr>
                ),
              ];
            })}
          </tbody>
        </table>
      </div>

      {/* Mobile cards */}
      <div class="sm:hidden space-y-2">
        {transactions.map((tx) => {
          const display = resolveDisplay(tx);
          const accountCell = isAccountRegister
            ? (display?.otherAccounts || "")
            : display ? (display.from === "various accounts" && display.to === "various accounts" ? "various accounts" : `${display.from} \u2192 ${display.to}`) : "";
          const amountCell = display?.amount || "";
          return (
            <div
              key={tx.fid || tx.date + tx.description}
              class="card card-compact bg-base-100 shadow-sm border border-base-200 cursor-pointer"
              onClick={() => toggle(tx.fid)}
            >
              <div class="card-body">
                <div class="flex justify-between items-center gap-2">
                  <span class="font-medium truncate">{tx.description}</span>
                  <div class="flex items-center gap-1 shrink-0">
                    <span class="whitespace-nowrap font-mono text-sm">{amountCell}</span>
                    <StatusButton fid={tx.fid} status={tx.status} onStatusChange={onStatusChange} />
                  </div>
                </div>
                <div class="text-xs text-base-content/60">{formatDate(tx.date)}</div>
                <div class="text-xs text-base-content/60 truncate">{accountCell}</div>
                {expanded === tx.fid && tx.postings && (
                  <PostingDetail postings={tx.postings} />
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
