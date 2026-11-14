// Helpers for turning literal UI values into hledger query tokens. hledger
// (and floatd's txfilter, which mirrors it) treats the value of payee:, desc:,
// tag: and acct: as a case-insensitive infix regex, so a payee such as
// "SQ *COFFEE" must be escaped, and anchored when an exact match is meant.

export function escapeRegex(value) {
  return String(value).replace(/[\\^$.*+?()[\]{}|]/g, "\\$&");
}

// exactRegex matches the whole value literally.
export function exactRegex(value) {
  return `^${escapeRegex(value)}$`;
}

// dateRangeQuery returns the date: token for a [from, to) range where either
// bound may be empty (open-ended), or no token when both are empty.
export function dateRangeQuery(from, to) {
  if (!from && !to) return [];
  return [`date:${from || ""}..${to || ""}`];
}
