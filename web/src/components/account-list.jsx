import { Link } from "@tanstack/react-router";
import { cn } from "@/lib/utils";
import { formatCurrency } from "../format.js";

const SECTION_LABELS = { A: "Assets", L: "Liabilities" };

// negate flips an amount's sign for display (liabilities are stored negative
// in hledger but read as "amount owed").
function negate(a) {
  const q = String(a.quantity ?? "").trim();
  return { ...a, quantity: q.startsWith("-") ? q.slice(1) : `-${q}` };
}

function Amounts({ amounts, flip, className }) {
  const shown = (amounts || []).filter((a) => parseFloat(a.quantity) !== 0);
  if (shown.length === 0) {
    return <span className={cn("text-muted-foreground/60", className)}>—</span>;
  }
  return (
    <span className={cn("flex flex-col items-end font-mono tabular-nums", className)}>
      {shown.map((a, i) => {
        const v = flip ? negate(a) : a;
        return <span key={i}>{formatCurrency(v.quantity, v.commodity)}</span>;
      })}
    </span>
  );
}

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
    <div className="flex flex-col gap-5">
      {sections.map((section) => {
        const flip = section.type === "L";
        return (
          <section key={section.root} className="flex flex-col">
            <div className="flex items-baseline justify-between gap-3 border-b pb-1.5">
              <Link
                to="/transactions"
                search={{ account: section.root }}
                className="text-xs font-semibold uppercase tracking-wide text-muted-foreground hover:text-foreground"
              >
                {SECTION_LABELS[section.type]}
                {flip && <span className="ml-1.5 font-normal normal-case tracking-normal">(owed)</span>}
              </Link>
              <Amounts amounts={section.total} flip={flip} className="text-sm font-semibold" />
            </div>
            <ul className="flex flex-col">
              {section.rows.map((row) => (
                <li
                  key={row.fullName}
                  className="flex items-baseline justify-between gap-3 border-b border-border/50 py-1.5 last:border-0"
                >
                  <Link
                    to="/transactions"
                    search={{ account: row.fullName }}
                    title={row.fullName}
                    className="min-w-0 truncate text-sm hover:underline"
                    style={{ paddingLeft: `${row.level * 1}rem` }}
                  >
                    {row.label}
                  </Link>
                  <Amounts
                    amounts={row.amounts}
                    flip={flip}
                    className={cn("shrink-0 text-sm", row.level > 0 && "text-muted-foreground")}
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
