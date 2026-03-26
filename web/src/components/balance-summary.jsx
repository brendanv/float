import { formatAmounts } from "../format.js";

export function BalanceSummary({ balanceRows }) {
  if (!balanceRows || balanceRows.length === 0) return null;

  // Find top-level totals for assets, liabilities
  const assets = balanceRows.find((r) => r.fullName === "assets");
  const liabilities = balanceRows.find((r) => r.fullName === "liabilities");

  return (
    <div style={{ display: "flex", gap: "2rem", flexWrap: "wrap", marginBottom: "1.5rem" }}>
      {assets && (
        <div>
          <small class="secondary">Assets</small>
          <div><strong>{formatAmounts(assets.amounts)}</strong></div>
        </div>
      )}
      {liabilities && (
        <div>
          <small class="secondary">Liabilities</small>
          <div><strong>{formatAmounts(liabilities.amounts)}</strong></div>
        </div>
      )}
    </div>
  );
}
