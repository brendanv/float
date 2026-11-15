import { useQuery } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { cn } from "@/lib/utils";
import { formatAmounts } from "../format.js";

function parseAmount(amounts) {
  if (!amounts || amounts.length === 0) return 0;
  return Math.abs(parseFloat(amounts[0].quantity) || 0);
}

// leafLabel drops the top-level segment ("expenses:groceries" → "groceries").
function leafLabel(fullName) {
  const i = fullName.indexOf(":");
  return i === -1 ? fullName : fullName.slice(i + 1);
}

// BarChart draws one bar per account. `flip` negates the displayed amounts
// for revenue, which hledger reports as negative balances.
function BarChart({ rows, colorClass, flip }) {
  if (!rows || rows.length === 0) return null;
  rows = [...rows].sort((a, b) => parseAmount(b.amounts) - parseAmount(a.amounts));

  const maxVal = Math.max(...rows.map((r) => parseAmount(r.amounts)), 1);

  return (
    <div className="flex flex-col gap-1">
      {rows.map((row) => {
        const val = parseAmount(row.amounts);
        const pct = (val / maxVal) * 100;
        return (
          <div key={row.fullName} className="flex items-center gap-2">
            <span
              className="w-2/5 flex-none truncate pr-1 text-right text-xs text-muted-foreground"
              title={row.fullName}
            >
              {leafLabel(row.fullName)}
            </span>
            <div className="h-4 flex-1 overflow-hidden rounded-sm bg-muted">
              <div
                className={cn("h-full rounded-sm transition-all duration-300", colorClass)}
                style={{ width: pct + "%" }}
              />
            </div>
            <span className="w-20 flex-none text-right font-mono text-xs tabular-nums sm:w-24">
              {formatAmounts(flip ? row.amounts.map((a) => ({ ...a, quantity: String(-parseFloat(a.quantity)) })) : row.amounts)}
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
          <BarChart rows={expenseRows} colorClass="bg-chart-1" />
        </div>
      )}
      {revenueRows.length > 0 && (
        <div className="mb-6">
          <h5 className="mb-2 text-sm font-semibold uppercase tracking-wide text-muted-foreground">
            Revenue
          </h5>
          <BarChart rows={revenueRows} colorClass="bg-chart-3" flip />
        </div>
      )}
      {expenseRows.length === 0 && revenueRows.length === 0 && !expensesLoading && !revenueLoading && (
        <p className="text-sm text-muted-foreground">No expense or revenue data for this period.</p>
      )}
    </div>
  );
}
