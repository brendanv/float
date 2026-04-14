import { useQuery } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { formatAmounts } from "../format.js";

function parseAmount(amounts) {
  if (!amounts || amounts.length === 0) return 0;
  return Math.abs(parseFloat(amounts[0].quantity) || 0);
}

function BarChart({ rows, colorClass }) {
  if (!rows || rows.length === 0) return null;

  const maxVal = Math.max(...rows.map((r) => parseAmount(r.amounts)), 1);

  return (
    <div className="space-y-1">
      {rows.map((row) => {
        const val = parseAmount(row.amounts);
        const pct = (val / maxVal) * 100;
        return (
          <div key={row.fullName} className="flex items-center gap-2">
            <span
              className="w-2/5 flex-none truncate pr-1 text-right text-xs text-muted-foreground"
              title={row.fullName}
            >
              {row.displayName || row.fullName}
            </span>
            <div className="h-5 flex-1 overflow-hidden rounded bg-muted">
              <div
                className={`h-full rounded transition-all duration-300 ${colorClass}`}
                style={{ width: pct + "%" }}
              />
            </div>
            <span className="w-20 flex-none text-right font-mono text-xs sm:w-24">
              {formatAmounts(row.amounts)}
            </span>
          </div>
        );
      })}
    </div>
  );
}

export function InsightsChart({ periodQuery }) {
  const expenseParams = { depth: 2, query: [...periodQuery, "type:X"] };
  const revenueParams = { depth: 2, query: [...periodQuery, "type:R"] };

  const { data: expensesData, isLoading: expensesLoading } = useQuery({
    queryKey: queryKeys.balances(expenseParams),
    queryFn: () => ledgerClient.getBalances(expenseParams),
  });

  const { data: revenueData, isLoading: revenueLoading } = useQuery({
    queryKey: queryKeys.balances(revenueParams),
    queryFn: () => ledgerClient.getBalances(revenueParams),
  });

  const expenseRows = expensesData?.report?.rows || [];
  const revenueRows = revenueData?.report?.rows || [];

  return (
    <div>
      {expenseRows.length > 0 && (
        <div className="mb-6">
          <h5 className="mb-2 text-sm font-semibold uppercase tracking-wide text-muted-foreground">
            Expenses
          </h5>
          <BarChart rows={expenseRows} colorClass="bg-destructive" />
        </div>
      )}
      {revenueRows.length > 0 && (
        <div className="mb-6">
          <h5 className="mb-2 text-sm font-semibold uppercase tracking-wide text-muted-foreground">
            Revenue
          </h5>
          <BarChart rows={revenueRows} colorClass="bg-success" />
        </div>
      )}
      {expenseRows.length === 0 && revenueRows.length === 0 && !expensesLoading && !revenueLoading && (
        <p className="text-sm text-muted-foreground">No expense or revenue data for this period.</p>
      )}
    </div>
  );
}
