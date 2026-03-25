import { formatAmounts } from "../format.js";
import { navigate } from "../router.jsx";

const TYPE_ORDER = ["A", "L", "R", "E", "X"];
const TYPE_LABELS = {
  A: "Assets",
  L: "Liabilities",
  R: "Revenue",
  E: "Expenses",
  X: "Equity",
};

export function AccountList({ accounts, balanceRows }) {
  if (!accounts || accounts.length === 0) {
    return <p>No accounts found.</p>;
  }

  // Build a map of fullName -> balance amounts from the balance report
  const balanceMap = {};
  if (balanceRows) {
    for (const row of balanceRows) {
      balanceMap[row.fullName] = row.amounts;
    }
  }

  // Group accounts by type
  const groups = {};
  for (const acct of accounts) {
    const t = acct.type || "X";
    if (!groups[t]) groups[t] = [];
    groups[t].push(acct);
  }

  return (
    <div>
      {TYPE_ORDER.map((type) => {
        const group = groups[type];
        if (!group || group.length === 0) return null;
        return (
          <section key={type} style={{ marginBottom: "1.5rem" }}>
            <h4 style={{ marginBottom: "0.5rem" }}>{TYPE_LABELS[type] || type}</h4>
            <table role="grid">
              <tbody>
                {group.map((acct) => (
                  <tr key={acct.fullName}>
                    <td>
                      <a
                        href={"#/transactions?account=" + encodeURIComponent(acct.fullName)}
                        onClick={(e) => {
                          e.preventDefault();
                          navigate("/transactions?account=" + encodeURIComponent(acct.fullName));
                        }}
                      >
                        {acct.fullName}
                      </a>
                    </td>
                    <td style={{ textAlign: "right", whiteSpace: "nowrap" }}>
                      {formatAmounts(balanceMap[acct.fullName])}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>
        );
      })}
    </div>
  );
}
