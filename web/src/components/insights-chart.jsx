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
    <div class="space-y-1">
      {rows.map((row) => {
        const val = parseAmount(row.amounts);
        const pct = (val / maxVal) * 100;
        return (
          <div key={row.fullName} class="flex items-center gap-2">
            <span
              class="flex-none w-2/5 text-right text-xs text-base-content/70 truncate pr-1"
              title={row.fullName}
            >
              {row.displayName || row.fullName}
            </span>
            <div class="flex-1 bg-base-300 rounded h-5 overflow-hidden">
              <div
                class={`h-full rounded transition-all duration-300 ${colorClass}`}
                style={{ width: pct + "%" }}
              />
            </div>
            <span class="flex-none w-20 sm:w-24 text-right text-xs font-mono">
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
        <div class="mb-6">
          <h5 class="font-semibold text-sm uppercase tracking-wide text-base-content/60 mb-2">Expenses</h5>
          <BarChart rows={expenseRows} colorClass="bg-error" />
        </div>
      )}
      {revenueRows.length > 0 && (
        <div class="mb-6">
          <h5 class="font-semibold text-sm uppercase tracking-wide text-base-content/60 mb-2">Revenue</h5>
          <BarChart rows={revenueRows} colorClass="bg-success" />
        </div>
      )}
      {expenseRows.length === 0 && revenueRows.length === 0 && !expensesLoading && !revenueLoading && (
        <p class="text-base-content/60 text-sm">No expense or revenue data for this period.</p>
      )}
    </div>
  );
}
