import { useState } from "preact/hooks";
import { formatAmounts, formatDate } from "../format.js";

function txPrimaryAmount(tx) {
  // Show the first posting's amounts as the "primary" display amount
  if (tx.postings && tx.postings.length > 0) {
    return formatAmounts(tx.postings[0].amounts);
  }
  return "";
}

function txPrimaryAccount(tx) {
  if (tx.postings && tx.postings.length > 0) {
    return tx.postings[0].account;
  }
  return "";
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

export function TransactionTable({ transactions }) {
  const [expanded, setExpanded] = useState(null);

  if (!transactions || transactions.length === 0) {
    return <p>No transactions for this period.</p>;
  }

  function toggle(fid) {
    setExpanded(expanded === fid ? null : fid);
  }

  return (
    <div>
      {/* Desktop table */}
      <table role="grid" class="transaction-table-desktop">
        <thead>
          <tr>
            <th>Date</th>
            <th>Description</th>
            <th>Account</th>
            <th style={{ textAlign: "right" }}>Amount</th>
          </tr>
        </thead>
        <tbody>
          {transactions.map((tx) => (
            <tr
              key={tx.fid || tx.date + tx.description}
              onClick={() => toggle(tx.fid)}
              style={{ cursor: "pointer" }}
            >
              <td style={{ whiteSpace: "nowrap" }}>{formatDate(tx.date)}</td>
              <td>{tx.description}</td>
              <td>{txPrimaryAccount(tx)}</td>
              <td style={{ textAlign: "right", whiteSpace: "nowrap" }}>{txPrimaryAmount(tx)}</td>
            </tr>
          ))}
        </tbody>
      </table>

      {/* Mobile cards */}
      <div class="transaction-cards-mobile">
        {transactions.map((tx) => (
          <article
            key={tx.fid || tx.date + tx.description}
            onClick={() => toggle(tx.fid)}
            style={{ cursor: "pointer", padding: "0.75rem", marginBottom: "0.5rem" }}
          >
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline" }}>
              <strong>{tx.description}</strong>
              <span style={{ whiteSpace: "nowrap" }}>{txPrimaryAmount(tx)}</span>
            </div>
            <small class="secondary">{formatDate(tx.date)}</small>
            {expanded === tx.fid && tx.postings && (
              <PostingDetail postings={tx.postings} />
            )}
          </article>
        ))}
      </div>
    </div>
  );
}
