import { Fragment } from "react";
import { flowTotals, registerTotals } from "@/lib/transactions.js";
import { Amount } from "./amount.jsx";
import { cn } from "@/lib/utils";

function Totals({ amounts }) {
  if (amounts.length === 0) return <b className="font-medium text-muted-foreground">—</b>;
  return amounts.map((a, i) => (
    <Fragment key={a.commodity}>
      {i > 0 && ", "}
      <Amount value={a} className="font-medium text-muted-foreground" />
    </Fragment>
  ));
}

// TransactionSummary is the list's one-line summary: how many transactions
// match, and what came in and went out. For the general list that's income
// and expense postings (transfers between your own accounts count as
// neither); for an account register it's the account's increases and
// decreases.
export function TransactionSummary({ transactions, registerRows, className }) {
  const isRegister = !!registerRows;
  const rows = isRegister ? registerRows : (transactions || []);
  const { inflow, outflow } = isRegister ? registerTotals(rows) : flowTotals(rows);
  const count = rows.length;
  return (
    <p data-slot="transaction-summary" className={cn("flex flex-wrap items-baseline gap-x-4 gap-y-1 text-[12.5px] text-faint", className)}>
      <span>
        <b className="font-medium text-muted-foreground">{count}</b> transaction{count === 1 ? "" : "s"}
      </span>
      <span>In <Totals amounts={inflow} /></span>
      <span>Out <Totals amounts={outflow} /></span>
    </p>
  );
}
