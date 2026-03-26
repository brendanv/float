import { ledgerClient } from "../client.js";
import { useRpc } from "../hooks/use-rpc.js";
import { formatAmounts } from "../format.js";

const COLORS = {
  expenses: "var(--pico-del-color, #e53935)",
  revenue: "var(--pico-ins-color, #43a047)",
};

function parseAmount(amounts) {
  if (!amounts || amounts.length === 0) return 0;
  // Parse the first commodity's quantity as a number
  return Math.abs(parseFloat(amounts[0].quantity) || 0);
}

function BarChart({ rows, color }) {
  if (!rows || rows.length === 0) return null;

  const maxVal = Math.max(...rows.map((r) => parseAmount(r.amounts)), 1);

  return (
    <div>
      {rows.map((row) => {
        const val = parseAmount(row.amounts);
        const pct = (val / maxVal) * 100;
        return (
          <div class="insights-bar" key={row.fullName}>
            <span class="insights-bar-label" title={row.fullName}>
              {row.displayName || row.fullName}
            </span>
            <div class="insights-bar-track">
              <div
                class="insights-bar-fill"
                style={{ width: pct + "%", background: color }}
              />
            </div>
            <span class="insights-bar-value">{formatAmounts(row.amounts)}</span>
          </div>
        );
      })}
    </div>
  );
}

export function InsightsChart({ periodQuery }) {
  const expenses = useRpc(
    () => ledgerClient.getBalances({ depth: 2, query: [...periodQuery, "type:X"] }),
    [periodQuery.join(",")]
  );
  const revenue = useRpc(
    () => ledgerClient.getBalances({ depth: 2, query: [...periodQuery, "type:R"] }),
    [periodQuery.join(",")]
  );

  const expenseRows = expenses.data?.report?.rows || [];
  const revenueRows = revenue.data?.report?.rows || [];

  return (
    <div>
      {expenseRows.length > 0 && (
        <section style={{ marginBottom: "1.5rem" }}>
          <h5>Expenses</h5>
          <BarChart rows={expenseRows} color={COLORS.expenses} />
        </section>
      )}
      {revenueRows.length > 0 && (
        <section style={{ marginBottom: "1.5rem" }}>
          <h5>Revenue</h5>
          <BarChart rows={revenueRows} color={COLORS.revenue} />
        </section>
      )}
      {expenseRows.length === 0 && revenueRows.length === 0 && !expenses.loading && !revenue.loading && (
        <p class="secondary">No expense or revenue data for this period.</p>
      )}
    </div>
  );
}
