import { Link } from "@tanstack/react-router";
import { cn } from "@/lib/utils";
import { accountTypeFromCode } from "@/lib/accounts.js";
import { Amount } from "./amount.jsx";
import { AccountGlyph } from "./account.jsx";

const SECTION_LABELS = { A: "Assets", L: "Liabilities" };

// groupTreeRows splits `hledger bal --tree` rows into one section per
// top-level account. hledger emits exactly one indent-0 row per top-level
// account: either the root itself, or (when the root has a single child) an
// elided "root:child" row. Either way its amount is the section total.
function groupTreeRows(rows, typeOf) {
  const sections = [];
  let current = null;
  for (const row of rows) {
    if (row.indent === 0) {
      const root = row.fullName.split(":")[0];
      current = {
        root,
        type: typeOf(row.fullName) || typeOf(root),
        total: row.amounts,
        rows: [],
      };
      sections.push(current);
      if (row.fullName === root) continue;
      // Elided "root:child": show the child part as a top-level entry.
      current.rows.push({ ...row, label: row.displayName.slice(root.length + 1) || row.displayName, level: 0 });
      continue;
    }
    if (!current) continue;
    current.rows.push({ ...row, label: row.displayName, level: row.indent - 1 });
  }
  // Undeclared parents aren't in ListAccounts; take the type from a child.
  for (const s of sections) {
    s.type ??= s.rows.map((r) => typeOf(r.fullName)).find(Boolean)
      ?? (/^liab/i.test(s.root) ? "L" : "A");
  }
  return sections;
}

// AccountList renders asset and liability balances as an indented tree. Rows
// are `hledger bal --tree` output, so parent totals come from hledger.
export function AccountList({ rows, accounts }) {
  const typeByName = new Map((accounts || []).map((a) => [a.fullName, a.type === "C" ? "A" : a.type]));
  const typeOf = (name) => typeByName.get(name);
  const sections = groupTreeRows(rows || [], typeOf)
    .filter((s) => s.type === "A" || s.type === "L")
    .sort((a, b) => (a.type === b.type ? 0 : a.type === "A" ? -1 : 1));

  if (sections.length === 0) {
    return <p className="text-sm text-muted-foreground">No balances yet.</p>;
  }

  return (
    <div className="flex flex-col gap-4">
      {sections.map((section) => {
        // Liabilities are stored negative but read as "amount owed".
        const flip = section.type === "L";
        const sectionType = section.type === "L" ? "liability" : "asset";
        return (
          <section key={section.root} className="flex flex-col">
            <div className="flex items-baseline justify-between gap-3 pb-1">
              <Link
                to="/transactions"
                search={{ account: section.root }}
                className="text-label uppercase text-faint hover:text-foreground"
              >
                {SECTION_LABELS[section.type]}
                {flip && <span className="ml-1.5 normal-case tracking-normal">(owed)</span>}
              </Link>
              <Amount value={section.total} negate={flip} className="shrink-0 text-[12.5px] text-muted-foreground" />
            </div>
            <ul className="flex flex-col">
              {section.rows.map((row) => (
                <li
                  key={row.fullName}
                  className="flex items-baseline justify-between gap-3 py-1.5"
                >
                  <Link
                    to="/transactions"
                    search={{ account: row.fullName }}
                    title={row.fullName}
                    className={cn(
                      "flex min-w-0 items-center gap-1.5 hover:underline",
                      row.level > 0 && "text-muted-foreground",
                    )}
                    style={{ paddingLeft: `${row.level * 0.9375}rem` }}
                  >
                    {row.level === 0 && (
                      <AccountGlyph type={accountTypeFromCode(typeOf(row.fullName)) ?? sectionType} />
                    )}
                    <span className="min-w-0 truncate">{row.label}</span>
                  </Link>
                  <Amount
                    value={row.amounts}
                    negate={flip}
                    className={cn("shrink-0", row.level > 0 && "text-muted-foreground")}
                  />
                </li>
              ))}
            </ul>
          </section>
        );
      })}
    </div>
  );
}
