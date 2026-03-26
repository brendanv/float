import { formatAmounts } from "../format.js";

export function BalanceSummary({ balanceRows }) {
  if (!balanceRows || balanceRows.length === 0) return null;

  const assets = balanceRows.find((r) => r.fullName === "assets");
  const liabilities = balanceRows.find((r) => r.fullName === "liabilities");

  return (
    <div class="stats shadow mb-6 w-full sm:w-auto">
      {assets && (
        <div class="stat">
          <div class="stat-title">Assets</div>
          <div class="stat-value text-success text-2xl">{formatAmounts(assets.amounts)}</div>
        </div>
      )}
      {liabilities && (
        <div class="stat">
          <div class="stat-title">Liabilities</div>
          <div class="stat-value text-error text-2xl">{formatAmounts(liabilities.amounts)}</div>
        </div>
      )}
    </div>
  );
}
