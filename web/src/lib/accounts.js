// hledger query excluding postings to closed accounts (the float-closed
// account tag, which hledger inherits to sub-accounts). Keep in sync with
// openAccountsQuery in internal/server/ledger/warm.go (it is pre-warmed).
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
