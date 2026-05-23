import { MetricCard } from "./page.jsx";
import { cn } from "@/lib/utils";
import { formatAmounts } from "../format.js";

function StatItem({ title, value, valueClass }) {
  return (
    <MetricCard
      title={title}
      value={value}
      valueClassName={valueClass}
      className="min-w-0"
    />
  );
}

export function BalanceSummary({ balanceRows, className }) {
  if (!balanceRows || balanceRows.length === 0) return null;

  const assets = balanceRows.find((r) => r.fullName === "assets");
  const liabilities = balanceRows.find((r) => r.fullName === "liabilities");

  const assetVal = parseFloat(assets?.amounts?.[0]?.quantity || 0);
  const liabVal = parseFloat(liabilities?.amounts?.[0]?.quantity || 0);
  const netWorth = assetVal + liabVal;
  const netPositive = netWorth >= 0;

  return (
    <div className={cn("grid gap-6 md:grid-cols-3", className)}>
      {assets && (
        <StatItem
          title="Assets"
          value={formatAmounts(assets.amounts)}
          valueClass="text-success"
        />
      )}
      {liabilities && (
        <StatItem
          title="Liabilities"
          value={formatAmounts(liabilities.amounts)}
          valueClass="text-destructive"
        />
      )}
      {assets && liabilities && (
        <StatItem
          title="Net Worth"
          value={formatAmounts([
            { commodity: assets.amounts[0].commodity, quantity: String(netWorth) },
          ])}
          valueClass={netPositive ? "text-success" : "text-destructive"}
        />
      )}
    </div>
  );
}
