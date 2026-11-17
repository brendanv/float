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

// HLEDGER_QUERY_TERM_RE matches one whitespace-separated search term that
// starts with an hledger query keyword (optionally negated), e.g. `acct:`,
// `not:tag:`. Search text with such a term is sent as raw query tokens (and
// shown in mono); anything else ("Re: invoice") is a literal description.
export const HLEDGER_QUERY_TERM_RE =
  /^(not:)?(acct|code|cur|desc|date|date2|tag|payee|note|status|real|depth|amt|type|sym|commodity|inacct|inaccteq):/;

// isHledgerQuery reports whether search text contains an hledger query term.
export function isHledgerQuery(text) {
  return (text || "").trim().split(/\s+/).some((t) => HLEDGER_QUERY_TERM_RE.test(t));
}
