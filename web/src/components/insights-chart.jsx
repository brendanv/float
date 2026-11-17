import { useQuery } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { cn } from "@/lib/utils";
import { Amount } from "./amount.jsx";
import { Account } from "./account.jsx";

function parseAmount(amounts) {
  if (!amounts || amounts.length === 0) return 0;
  return Math.abs(parseFloat(amounts[0].quantity) || 0);
}

// BarChart draws one bar per account. `flip` negates the displayed amounts
// for revenue, which hledger reports as negative balances.
function BarChart({ rows, colorClass, flip }) {
  if (!rows || rows.length === 0) return null;
  rows = [...rows].sort((a, b) => parseAmount(b.amounts) - parseAmount(a.amounts));

  const maxVal = Math.max(...rows.map((r) => parseAmount(r.amounts)), 1);

  return (
    <div className="flex flex-col gap-1.5">
      {rows.map((row) => {
        const val = parseAmount(row.amounts);
        const pct = (val / maxVal) * 100;
        return (
          <div key={row.fullName} className="flex items-center gap-2">
            <Account
              name={row.fullName}
              variant="leaf"
              glyph={false}
              className="flex w-2/5 flex-none justify-end pr-1 text-muted-foreground"
            />
            <div className="h-2 flex-1 overflow-hidden rounded-[2px]">
              <div
                className={cn("h-full rounded-[2px] transition-all duration-300", colorClass)}
                style={{ width: pct + "%" }}
              />
            </div>
            <Amount
              value={row.amounts}
              negate={flip}
              className="w-24 flex-none text-right sm:w-28"
            />
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
          <h3 className="mb-2 text-label uppercase text-faint">Expenses</h3>
          <BarChart rows={expenseRows} colorClass="bg-primary" />
        </div>
      )}
      {revenueRows.length > 0 && (
        <div className="mb-6">
          <h3 className="mb-2 text-label uppercase text-faint">Revenue</h3>
          <BarChart rows={revenueRows} colorClass="bg-inflow" flip />
        </div>
      )}
      {expenseRows.length === 0 && revenueRows.length === 0 && !expensesLoading && !revenueLoading && (
        <p className="text-body text-muted-foreground">No expense or revenue data for this period.</p>
      )}
    </div>
  );
}
