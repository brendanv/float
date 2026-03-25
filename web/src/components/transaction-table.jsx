import { useState } from "preact/hooks";
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
    const pos = postings.find((p) => firstQuantity(p) > 0);
    const amount = pos ? formatAmounts(pos.amounts) : formatAmounts(postings[0].amounts);
    return { from: "various accounts", to: "various accounts", amount };
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

function PostingDetail({ postings }) {
  return (
    <table style={{ fontSize: "0.875rem", marginTop: "0.5rem", marginBottom: 0 }}>
      <tbody>
        {postings.map((p, i) => (
          <tr key={i}>
            <td style={{ paddingLeft: "1.5rem", border: "none", padding: "0.15rem 0.5rem" }}>{p.account}</td>
            <td style={{ textAlign: "right", border: "none", padding: "0.15rem 0.5rem", whiteSpace: "nowrap" }}>
              {formatAmounts(p.amounts)}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export function TransactionTable({ transactions, focusedAccount }) {
  const [expanded, setExpanded] = useState(null);

  if (!transactions || transactions.length === 0) {
    return <p>No transactions for this period.</p>;
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
      <table role="grid" class="transaction-table-desktop">
        <thead>
          <tr>
            <th>Date</th>
            <th>Description</th>
            <th>{isAccountRegister ? "Other accounts" : "From \u2192 To"}</th>
            <th style={{ textAlign: "right" }}>Amount</th>
          </tr>
        </thead>
        <tbody>
          {transactions.map((tx) => {
            const display = resolveDisplay(tx);
            const accountCell = isAccountRegister
              ? (display?.otherAccounts || "")
              : display ? `${display.from} \u2192 ${display.to}` : "";
            const amountCell = display?.amount || "";
            return (
              <tr
                key={tx.fid || tx.date + tx.description}
                onClick={() => toggle(tx.fid)}
                style={{ cursor: "pointer" }}
              >
                <td style={{ whiteSpace: "nowrap" }}>{formatDate(tx.date)}</td>
                <td>{tx.description}</td>
                <td>{accountCell}</td>
                <td style={{ textAlign: "right", whiteSpace: "nowrap" }}>{amountCell}</td>
              </tr>
            );
          })}
        </tbody>
      </table>

      {/* Mobile cards */}
      <div class="transaction-cards-mobile">
        {transactions.map((tx) => {
          const display = resolveDisplay(tx);
          const accountCell = isAccountRegister
            ? (display?.otherAccounts || "")
            : display ? `${display.from} \u2192 ${display.to}` : "";
          const amountCell = display?.amount || "";
          return (
            <article
              key={tx.fid || tx.date + tx.description}
              onClick={() => toggle(tx.fid)}
              style={{ cursor: "pointer", padding: "0.75rem", marginBottom: "0.5rem" }}
            >
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline" }}>
                <strong>{tx.description}</strong>
                <span style={{ whiteSpace: "nowrap" }}>{amountCell}</span>
              </div>
              <small class="secondary">{formatDate(tx.date)}</small>
              <div><small class="secondary">{accountCell}</small></div>
              {expanded === tx.fid && tx.postings && (
                <PostingDetail postings={tx.postings} />
              )}
            </article>
          );
        })}
      </div>
    </div>
  );
}
