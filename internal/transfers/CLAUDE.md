# internal/transfers

Finds transfers through a clearing account (default `equity:transfers`, config `transfers.account`) that have only one side in the journal, which usually means a transaction is missing entirely.

Each side of a transfer is its own transaction posting between its account and the clearing account, so a complete pair nets the clearing account to zero.

## Routes

The clearing account's direct children are **routes**, one per pair of counterparties (e.g. `equity:transfers:Schwab-Amex`). `RouteOf(name, account)` maps a posting account to its route: deeper sub-accounts belong to their route, and direct postings to the clearing account form a route of their own. Route names are never parsed (`Wells-Fargo-Amex` would be ambiguous, and labels aren't hledger accounts); instead each route learns from history which source accounts post to it and in which direction.

The package only pairs transactions hledger has already parsed; the sum of a route's unmatched sides equals hledger's balance of that route (`TestFindUnmatchedMatchesHledgerBalance` checks this against real hledger, reading routes from `bal --depth 3`).

## API

- `FindUnmatched(txns, Options) []Unmatched` — pure function over `[]hledger.Transaction`; returns unmatched sides in input order.
- `Options` — `Account` (empty = `DefaultAccount`), `WindowDays` (≤0 = `DefaultWindowDays` = 5), `Today` (date in the ledger timezone).
- `Unmatched` — the transaction, the amount posted to the clearing account, `Status` (`missing`: dated more than the window before today; `recent`: the other side may not be imported yet), `Route`, and `ExpectedAccounts`.
- `RouteOf(name, account)` — route for a posting account.

## Matching Rules

- A side is a transaction with exactly one posting to the clearing account or a sub-account, carrying exactly one amount. Transactions touching it twice, or with a multi-commodity clearing posting, are ignored.
- Two sides match when they post to the same route, the clearing amounts are equal and opposite in the same commodity (decimal places normalized, costs ignored), their source accounts (first posting outside the clearing account) differ, and their dates are at most `WindowDays` apart.
- Pairing is one-to-one, closest dates first, ties to the earliest transactions, so repeated identical transfers each need their own counterpart.
- `ExpectedAccounts` for an unmatched side: the route's other accounts that have moved money the opposite way (so in a route shared by checking and two cards, a lone card payment expects checking, not the other card); if none has, every other account in the route; empty if the route has only seen this side's account.

Read-only: nothing here writes, and there is no dismiss/link state.
