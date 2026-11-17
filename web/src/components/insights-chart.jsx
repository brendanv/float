import { useQuery } from "@tanstack/react-query";
import { ledgerClient } from "../client.js";
import { queryKeys } from "../query-keys.js";
import { Amount } from "./amount.jsx";
import { Account } from "./account.jsx";
import { ErrorBanner } from "./error-banner.jsx";

function quantity(amounts) {
  if (!amounts || amounts.length === 0) return 0;
  return parseFloat(amounts[0].quantity) || 0;
}

// SpendingBars draws one single-hue bar per expense account, scaled to the
// largest, with its amount and its share of `total`.
function SpendingBars({ rows, total }) {
  const sorted = [...rows].sort((a, b) => Math.abs(quantity(b.amounts)) - Math.abs(quantity(a.amounts)));
  const max = Math.max(...sorted.map((r) => Math.abs(quantity(r.amounts))), 1);

  return (
    <ul
      data-slot="spending-bars"
      className="grid grid-cols-[minmax(4.5rem,6rem)_minmax(0,1fr)_auto_2.25rem] items-center gap-x-3 gap-y-[7px]"
    >
      {sorted.map((row) => {
        const val = Math.abs(quantity(row.amounts));
        return (
          <li key={row.fullName} className="contents">
            <Account
              name={row.fullName}
              variant="leaf"
              glyph={false}
              className="flex min-w-0 justify-end text-muted-foreground"
            />
            <div className="h-2" aria-hidden="true">
              <div className="h-full rounded-[2px] bg-primary" style={{ width: `${(val / max) * 100}%` }} />
            </div>
            <Amount value={row.amounts} className="text-right" />
            <span className="text-right text-[12px] text-faint">
              {total > 0 ? `${Math.round((val / total) * 100)}%` : ""}
            </span>
          </li>
        );
      })}
    </ul>
  );
}

// InsightsChart is the Overview's spending block for `periodQuery`: an
// In / Out / Kept line (hledger's report totals for revenue and expenses)
// over one bar per expense category.
export function InsightsChart({ periodQuery }) {
  const expenseParams = { depth: 2, query: [...periodQuery, "type:X"] };
  const revenueParams = { depth: 2, query: [...periodQuery, "type:R"] };

  const { data: expensesData, isLoading: expensesLoading, error: expensesError } = useQuery({
    queryKey: queryKeys.balances(expenseParams),
    queryFn: () => ledgerClient.getBalances(expenseParams),
  });

  const { data: revenueData, isLoading: revenueLoading, error: revenueError } = useQuery({
    queryKey: queryKeys.balances(revenueParams),
    queryFn: () => ledgerClient.getBalances(revenueParams),
  });

  const error = expensesError || revenueError;
  if (error) return <ErrorBanner error={error} />;

  const expenseRows = expensesData?.report?.rows || [];
  const revenueRows = revenueData?.report?.rows || [];
  const outTotal = expensesData?.report?.total || [];
  const inTotal = revenueData?.report?.total || [];
  const outVal = quantity(outTotal);
  // hledger reports revenue as a negative balance.
  const inVal = -quantity(inTotal);
  const kept = inVal > 0 ? Math.round(((inVal - outVal) / inVal) * 100) : null;

  if (expenseRows.length === 0 && revenueRows.length === 0) {
    if (expensesLoading || revenueLoading) return null;
    return <p className="text-body text-muted-foreground">No income or spending in this period.</p>;
  }

  return (
    <div className="flex flex-col gap-4">
      <div data-slot="summary-line" className="flex flex-wrap gap-x-5 gap-y-1 text-[12.5px] text-muted-foreground">
        <span>
          In{" "}
          {inTotal.length > 0
            ? <Amount value={inTotal} negate signed tone={inVal > 0 ? "inflow" : "auto"} className="font-medium" />
            : <span className="text-faint">—</span>}
        </span>
        <span>
          Out{" "}
          {outTotal.length > 0
            ? <Amount value={outTotal} className="font-medium text-foreground" />
            : <span className="text-faint">—</span>}
        </span>
        {kept !== null && (
          <span>
            Kept <span className={kept < 0 ? "font-medium text-below" : "font-medium text-foreground"}>{kept}%</span>
          </span>
        )}
      </div>
      {expenseRows.length > 0 && <SpendingBars rows={expenseRows} total={outVal} />}
    </div>
  );
}
