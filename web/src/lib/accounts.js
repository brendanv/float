// hledger query excluding postings to closed accounts (the float-closed
// account tag, which hledger inherits to sub-accounts). Keep in sync with
// openAccountsQuery in internal/server/ledger/warm.go (pre-warmed reports
// build on it).
export const OPEN_ACCOUNTS_QUERY = ["not:tag:float-closed"];

function normalizePrefix(prefix) {
  return String(prefix ?? "").trim().toLowerCase().replace(/:+$/, "");
}

function accountName(account) {
  if (typeof account === "string") return account;
  return account?.fullName ?? account?.name ?? "";
}

export function accountMatchesPrefix(account, prefix) {
  const name = accountName(account).toLowerCase();
  const normalizedPrefix = normalizePrefix(prefix);

  if (!name || !normalizedPrefix) return false;
  return name === normalizedPrefix || name.startsWith(`${normalizedPrefix}:`);
}

// Closed accounts are dropped unless includeClosed is set. Plain-string
// entries carry no closed flag and are always kept.
export function filterAndSortAccountNames(accounts, { includePrefixes = [], excludePrefixes = [], includeClosed = false } = {}) {
  const normalizedIncludePrefixes = includePrefixes.map(normalizePrefix).filter(Boolean);
  const normalizedExcludePrefixes = excludePrefixes.map(normalizePrefix).filter(Boolean);
  const names = new Set();

  for (const account of accounts ?? []) {
    const name = accountName(account);
    if (!name) continue;
    if (!includeClosed && account?.closed) continue;

    const included = normalizedIncludePrefixes.length === 0
      || normalizedIncludePrefixes.some((prefix) => accountMatchesPrefix(name, prefix));
    const excluded = normalizedExcludePrefixes.some((prefix) => accountMatchesPrefix(name, prefix));

    if (included && !excluded) names.add(name);
  }

  return [...names].sort((a, b) => a.localeCompare(b, undefined, { sensitivity: "base" }));
}

// Account types used for Waterline glyphs and tones.
export const ACCOUNT_TYPES = ["asset", "liability", "income", "expense", "equity", "other"];

const TYPE_BY_ROOT = {
  assets: "asset", asset: "asset",
  liabilities: "liability", liability: "liability",
  income: "income", revenue: "income", revenues: "income",
  expenses: "expense", expense: "expense",
  equity: "equity",
};

// accountType maps an account's top-level segment to its type
// ("assets:bank:checking" → "asset"). Unknown roots are "other".
// Credit-normal account types: shown with the sign flipped so balances
// read as positive amounts owed / earned.
export const NEGATE_TYPES = new Set(["liability", "income", "equity"]);

export function accountType(name) {
  const root = accountName(name).split(":")[0].trim().toLowerCase();
  return TYPE_BY_ROOT[root] ?? "other";
}

// hledger's account type codes (ListAccounts `type`): A asset, C cash,
// L liability, R revenue, X expense, E equity, V conversion.
const TYPE_BY_CODE = { A: "asset", C: "asset", L: "liability", R: "income", X: "expense", E: "equity", V: "equity" };

// accountTypeFromCode maps a declared hledger type code to an account type,
// or undefined when the code is missing or unknown.
export function accountTypeFromCode(code) {
  return TYPE_BY_CODE[String(code ?? "").toUpperCase()];
}

// splitAccount splits a full account path into its parents (with the
// trailing colon) and its leaf: "expenses:dining:coffee" →
// { parents: "expenses:dining:", leaf: "coffee" }.
export function splitAccount(name) {
  const full = accountName(name);
  const i = full.lastIndexOf(":");
  if (i === -1) return { parents: "", leaf: full };
  return { parents: full.slice(0, i + 1), leaf: full.slice(i + 1) };
}
