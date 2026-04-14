import { Link } from "@tanstack/react-router";
import { Table, TableBody, TableCell, TableRow } from "@/components/ui/table";
import { formatAmounts } from "../format.js";

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
    return <p className="text-muted-foreground">No accounts found.</p>;
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
          <div key={type} className="mb-6">
            <h4 className="mb-2 text-sm font-semibold uppercase tracking-wide text-muted-foreground">
              {TYPE_LABELS[type] || type}
            </h4>
            <Table>
              <TableBody>
                {group.map((acct) => (
                  <TableRow key={acct.fullName}>
                    <TableCell className="py-1.5">
                      <Link
                        to="/transactions"
                        search={{ account: acct.fullName }}
                        className="text-sm hover:underline"
                      >
                        {acct.fullName}
                      </Link>
                    </TableCell>
                    <TableCell className="py-1.5 text-right font-mono text-sm">
                      {formatAmounts(balanceMap[acct.fullName])}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        );
      })}
    </div>
  );
}
