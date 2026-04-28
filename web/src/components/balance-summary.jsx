import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";

function sumUsd(amounts) {
  if (!amounts || amounts.length === 0) return null;
  let total = 0;
  let hasUsd = false;
  for (const a of amounts) {
    if (a.commodity === "USD" || a.commodity === "$") {
      total += parseFloat(a.quantity || 0);
      hasUsd = true;
    }
  }
  return hasUsd ? total : parseFloat(amounts[0]?.quantity || 0);
}

function formatUsd(value) {
  if (value === null || value === undefined) return "";
  const abs = Math.abs(value);
  const str = abs.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  return (value < 0 ? "-$" : "$") + str;
}

function StatItem({ title, value, valueClass }) {
  return (
    <Card className="flex-1">
      <CardHeader>
        <CardTitle className="text-xs font-normal uppercase tracking-wide text-muted-foreground">
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className={cn("font-mono text-2xl font-semibold", valueClass)}>
          {value}
        </div>
      </CardContent>
    </Card>
  );
}

export function BalanceSummary({ balanceRows }) {
  if (!balanceRows || balanceRows.length === 0) return null;

  const assets = balanceRows.find((r) => r.fullName === "assets");
  const liabilities = balanceRows.find((r) => r.fullName === "liabilities");

  const assetVal = sumUsd(assets?.amounts);
  const liabVal = sumUsd(liabilities?.amounts);
  const netWorth = (assetVal ?? 0) + (liabVal ?? 0);
  const netPositive = netWorth >= 0;

  return (
    <div className="mb-6 flex flex-col gap-3 sm:flex-row">
      {assets && (
        <StatItem
          title="Assets"
          value={formatUsd(assetVal)}
          valueClass="text-success"
        />
      )}
      {liabilities && (
        <StatItem
          title="Liabilities"
          value={formatUsd(liabVal)}
          valueClass="text-destructive"
        />
      )}
      {assets && liabilities && (
        <StatItem
          title="Net Worth"
          value={formatUsd(netWorth)}
          valueClass={netPositive ? "text-success" : "text-destructive"}
        />
      )}
    </div>
  );
}
