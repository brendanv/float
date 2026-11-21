float is a personal finance ledger. hledger (plain-text double-entry accounting) does the accounting; these tools call float's API.

## Data model
- Accounts are colon paths under five roots: assets, liabilities, equity, income, expenses (e.g. `assets:checking`, `expenses:food:groceries`, `liabilities:visa`). List them with run_hledger_query `accounts` and reuse existing ones; a new name creates a new account.
- Postings in a transaction sum to zero. Spending is a positive amount on an expenses account balanced by a negative amount on the asset/liability it was paid from. One posting may leave quantity and commodity empty to auto-balance.
- Amounts are decimal strings plus a commodity, e.g. quantity "45.50", commodity "USD".
- Every transaction has an 8-character `fid`; write tools take fids.
- status: "" = Unmarked (not yet reviewed), "Pending", "Cleared" (reviewed).
- Output is JSON with proto field names. Fields with zero values (0, false, "", empty lists) are omitted.

## Queries
list_transactions, preview_apply_rules and hledger commands take hledger query terms (one per `query` array element for the tools):
`acct:REGEX`, `desc:REGEX`, `payee:REGEX`, `date:2026-01`, `date:2026-01-01..2026-03-01` (end exclusive), `date:lastmonth`, `date:thisyear`, `status:` (unmarked), `status:*` (cleared), `status:!` (pending), `tag:NAME` or `tag:NAME=REGEX`, `amt:>100`, `code:FID`, and `not:` before any of them. A bare word matches account names. Terms of different types are ANDed; several `acct:`/`desc:` terms are ORed.

list_transactions returns transactions oldest first. Narrow it with a `date:` term, or page from `offset = total - limit` for the newest.

## Workflows
- Reports: run_hledger_query, e.g. `bal expenses -M date:2026 --depth 2` (spending by month), `bal assets liabilities -V` (market value), `is -M` (income statement), `bs` (balance sheet), `reg acct:checking date:lastmonth`, `payees`.
- Recording transactions from a statement, export, email or receipt: call import_structured_transactions with `dry_run: true`, check the summary (duplicates, rule_matches; show the user), then send the same request with `dry_run: false`. Duplicates already in the ledger are skipped, so re-sending is safe. Set `source` (e.g. "agent") and usually `apply_rules: true`. Imported transactions stay Unmarked for review. The result is a compact summary indexed by input position, not the transactions; list them with list_transactions `["tag:float-import=<import_batch_id>"]`. To undo an import, pass the returned fids to bulk_delete_transactions.
- Large imports (hundreds of transactions) when you can run shell commands: keep the data out of your context. Write a script that builds the request JSON from the source file and POSTs it straight to the API on the same host as this MCP endpoint, then print only the counts:
  `curl -sS -X POST "$BASE/float.v1.LedgerService/ImportStructuredTransactions" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d @request.json | jq -c '{importBatchId, importedCount, skippedDuplicateCount}'`
  `$BASE` is this endpoint's URL without `/mcp` and `$TOKEN` is the same bearer token. The body has the same fields as import_structured_transactions. The API response uses camelCase keys and contains every transaction, so always filter it with jq; for a `"dry_run": true` request, print only the flagged indices with `[.candidates | to_entries[] | select(.value.isDuplicate or .value.matchedRuleId) | .key]`. At most 5000 transactions per request.
- Categorizing: find candidates with list_transactions (e.g. query `["status:"]` or `["acct:unknown"]`). For recurring payees, add a rule (add_rule), run preview_apply_rules, then apply_rules. For one-off changes use bulk_edit_transactions (tags, payee, reviewed status, replacing an `unknown` account) or update_transaction (replaces the postings).

Every write is also committed as a snapshot the user can restore from the web UI. Confirm with the user before deleting or overwriting transactions (bulk_delete_transactions, update_transaction).
