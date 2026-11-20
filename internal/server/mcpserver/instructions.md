float is a personal finance ledger. hledger (plain-text double-entry accounting) does the accounting; these tools call float's API.

## Data model
- Accounts are colon paths under five roots: assets, liabilities, equity, income, expenses (e.g. `assets:checking`, `expenses:food:groceries`, `liabilities:visa`). Call list_accounts and reuse existing accounts; a new name creates a new account.
- Postings in a transaction sum to zero. Spending is a positive amount on an expenses account balanced by a negative amount on the asset/liability it was paid from. One posting may leave quantity and commodity empty to auto-balance.
- Amounts are decimal strings plus a commodity, e.g. quantity "45.50", commodity "USD".
- Every transaction has an 8-character `fid`; write tools take fids.
- status: "" = Unmarked (not yet reviewed), "Pending", "Cleared" (reviewed).
- Output is JSON with proto field names. Fields with zero values (0, false, "", empty lists) are omitted.

## Queries
Tools with a `query` field take hledger query tokens, one per array element:
`acct:REGEX`, `desc:REGEX`, `payee:REGEX`, `date:2026-01`, `date:2026-01-01..2026-03-01` (end exclusive), `date:lastmonth`, `date:thisyear`, `status:` (unmarked), `status:*` (cleared), `status:!` (pending), `tag:NAME` or `tag:NAME=REGEX`, `amt:>100`, `code:FID`, and `not:` before any of them. A bare word matches account names. Tokens of different types are ANDed; several `acct:`/`desc:` tokens are ORed.

list_transactions returns transactions oldest first. Narrow it with a `date:` token, or page from `offset = total - limit` for the newest.

## Workflows
- Recording transactions from a statement, export, email or receipt: call import_structured_transactions with `dry_run: true`, check the candidates (and show the user), then send the same request with `dry_run: false`. Duplicates already in the ledger are skipped, so re-sending is safe. Set `source` (e.g. "agent") and usually `apply_rules: true`. Imported transactions stay Unmarked for review.
- Categorizing: find candidates with list_transactions (e.g. query `["status:"]` or `["acct:unknown"]`). For recurring payees, add a rule (add_rule) and run preview_apply_rules, then apply_rules. For one-off changes use bulk_edit_transactions (tags, payee, reviewed status, replacing an `unknown` account) or update_transaction (replaces the postings).
- Reports: get_balances (`value: "now,USD"` for market value), get_income_statement_timeseries, get_net_worth_timeseries, get_portfolio_holdings. For anything else, run_hledger_query runs a read-only hledger command, e.g. `bal expenses -M date:2026 --depth 2`.

## Undo
Every write is committed as a snapshot. list_snapshots is newest first, and its first entry is the current state. get_snapshot_diff shows what a snapshot changed. restore_snapshot resets everything to an earlier snapshot and discards all later ones (including changes the user made meanwhile), so it is a last resort that needs the user's go-ahead. To remove one import batch, get its transactions with get_imported_transactions and pass their fids to bulk_delete_transactions.

Confirm with the user before destructive tools (bulk_delete_transactions, bulk_edit_transactions with delete, update_transaction, delete_rule, restore_snapshot).
