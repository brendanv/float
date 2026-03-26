import { formatAmounts } from "../format.js";
import { navigate } from "../router.jsx";

const TYPE_ORDER = ["A", "L", "E", "R", "X"];
const TYPE_LABELS = {
  A: "Assets",
  L: "Liabilities",
  R: "Revenue",
  E: "Equity",
  X: "Expenses",
};

export function AccountList({ accounts, balanceRows }) {
  if (!accounts || accounts.length === 0) {
    return <p class="text-base-content/60">No accounts found.</p>;
  }

  const balanceMap = {};
  if (balanceRows) {
    for (const row of balanceRows) {
      balanceMap[row.fullName] = row.amounts;
    }
  }

  const groups = {};
  for (const acct of accounts) {
    const t = acct.type === "C" ? "A" : (acct.type || "X");
    if (!groups[t]) groups[t] = [];
    groups[t].push(acct);
  }

  return (
    <div>
      {TYPE_ORDER.map((type) => {
        const group = groups[type];
        if (!group || group.length === 0) return null;
        return (
          <div key={type} class="mb-6">
            <h4 class="font-semibold text-sm uppercase tracking-wide text-base-content/60 mb-2">
              {TYPE_LABELS[type] || type}
            </h4>
            <table class="table table-sm w-full">
              <tbody>
                {group.map((acct) => (
                  <tr key={acct.fullName} class="hover">
                    <td>
                      <a
                        class="link link-hover text-sm"
                        href={"#/transactions?account=" + encodeURIComponent(acct.fullName)}
                        onClick={(e) => {
                          e.preventDefault();
                          navigate("/transactions?account=" + encodeURIComponent(acct.fullName));
                        }}
                      >
                        {acct.fullName}
                      </a>
                    </td>
                    <td class="text-right whitespace-nowrap text-sm font-mono">
                      {formatAmounts(balanceMap[acct.fullName])}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        );
      })}
    </div>
  );
}
