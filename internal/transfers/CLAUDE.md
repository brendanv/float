# internal/transfers

Finds transfers through a clearing account (default `equity:transfers`, config `transfers.account`) that have only one side in the journal, which usually means a transaction is missing entirely.

Each side of a transfer is its own transaction posting between its account and the clearing account, so a complete pair nets the clearing account to zero. The package only pairs transactions hledger has already parsed; the sum of the unmatched sides equals hledger's balance of the clearing account (`TestFindUnmatchedMatchesHledgerBalance` checks this against real hledger).

## API

- `FindUnmatched(txns, Options) []Unmatched` — pure function over `[]hledger.Transaction`; returns unmatched sides in input order.
- `Options` — `Account` (empty = `DefaultAccount`), `WindowDays` (≤0 = `DefaultWindowDays` = 5), `Today` (date in the ledger timezone).
- `Unmatched` — the transaction, the amount posted to the clearing account, and `Status`: `missing` (dated more than the window before today) or `recent` (the other side may not be imported yet).

## Matching Rules

- A side is a transaction with exactly one posting to the clearing account or a sub-account, carrying exactly one amount. Transactions touching it twice, or with a multi-commodity clearing posting, are ignored.
- Two sides match when the clearing amounts are equal and opposite in the same commodity (decimal places normalized, costs ignored), their source accounts (first posting outside the clearing account) differ, and their dates are at most `WindowDays` apart.
- Pairing is one-to-one, closest dates first, ties to the earliest transactions, so repeated identical transfers each need their own counterpart.

Read-only: nothing here writes, and there is no dismiss/link state.
