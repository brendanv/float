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

export function filterAndSortAccountNames(accounts, { includePrefixes = [], excludePrefixes = [] } = {}) {
  const normalizedIncludePrefixes = includePrefixes.map(normalizePrefix).filter(Boolean);
  const normalizedExcludePrefixes = excludePrefixes.map(normalizePrefix).filter(Boolean);
  const names = new Set();

  for (const account of accounts ?? []) {
    const name = accountName(account);
    if (!name) continue;

    const included = normalizedIncludePrefixes.length === 0
      || normalizedIncludePrefixes.some((prefix) => accountMatchesPrefix(name, prefix));
    const excluded = normalizedExcludePrefixes.some((prefix) => accountMatchesPrefix(name, prefix));

    if (included && !excluded) names.add(name);
  }

  return [...names].sort((a, b) => a.localeCompare(b, undefined, { sensitivity: "base" }));
}
