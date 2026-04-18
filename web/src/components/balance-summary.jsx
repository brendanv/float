import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import { formatAmounts } from "../format.js";

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

  const assetVal = parseFloat(assets?.amounts?.[0]?.quantity || 0);
  const liabVal = parseFloat(liabilities?.amounts?.[0]?.quantity || 0);
  const netWorth = assetVal + liabVal;
  const netPositive = netWorth >= 0;

  return (
    <div className="mb-6 flex flex-col gap-3 sm:flex-row">
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
