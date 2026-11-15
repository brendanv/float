import { MetricCard } from "./page.jsx";
import { cn } from "@/lib/utils";
import { formatAmounts } from "../format.js";

// BalanceSummary shows assets, liabilities and net worth. Liabilities are
// negative in hledger; they are shown as a positive "owed" amount. Only net
// worth is colored, since its sign is the one that carries meaning.
export function BalanceSummary({ balanceRows, className }) {
  if (!balanceRows || balanceRows.length === 0) return null;

  const assets = balanceRows.find((r) => r.fullName === "assets");
  const liabilities = balanceRows.find((r) => r.fullName === "liabilities");

  const assetVal = parseFloat(assets?.amounts?.[0]?.quantity || 0);
  const liabVal = parseFloat(liabilities?.amounts?.[0]?.quantity || 0);
  const netWorth = assetVal + liabVal;
  const commodity = assets?.amounts?.[0]?.commodity ?? liabilities?.amounts?.[0]?.commodity ?? "USD";

  return (
    <div className={cn("grid gap-6 md:grid-cols-3", className)}>
      {assets && (
        <MetricCard
          title="Assets"
          value={formatAmounts(assets.amounts)}
          description="Current value"
          className="min-w-0"
        />
      )}
      {liabilities && (
        <MetricCard
          title="Liabilities"
          value={formatAmounts(liabilities.amounts.map((a) => ({ ...a, quantity: String(-parseFloat(a.quantity)) })))}
          description="Owed"
          className="min-w-0"
        />
      )}
      {(assets || liabilities) && (
        <MetricCard
          title="Net Worth"
          value={formatAmounts([{ commodity, quantity: String(netWorth) }])}
          valueClassName={netWorth >= 0 ? "text-success" : "text-destructive"}
          description="Assets minus liabilities"
          className="min-w-0"
        />
      )}
    </div>
  );
}
