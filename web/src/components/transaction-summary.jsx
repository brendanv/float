import { cn } from "@/lib/utils";

// TransactionSummary is the list's one-line summary: how many transactions
// (or register rows) match. Totals are left to hledger's reports; the list
// doesn't add up postings itself.
export function TransactionSummary({ transactions, registerRows, className }) {
  const rows = registerRows ?? transactions ?? [];
  const count = rows.length;
  return (
    <p data-slot="transaction-summary" className={cn("text-[12.5px] text-faint", className)}>
      <b className="font-medium text-muted-foreground">{count}</b> transaction{count === 1 ? "" : "s"}
    </p>
  );
}
