# internal/server/ledger

ConnectRPC handler implementing `LedgerService`. This is the single service that powers both the web UI and the `float` TUI client.

## Handler

`Handler` holds references to the hledger client, txlock, gitsnap repo, cache, config, and data directory. Construct with `NewHandler(...)`.

All query RPCs go through the cache helpers (`cachedTransactions`, `cachedBalances`, etc.) which produce deterministic, sorted cache keys and call hledger on a miss.

All mutation RPCs (add, delete, update, bulk-tag, import, restore snapshot, etc.) wrap their file writes in `txlock.Do()`, which handles rollback on failure and bumps the generation counter to invalidate the cache.

## Cache Keys

Cache keys are namespaced by RPC type. Query args are sorted before joining so `["b","a"]` and `["a","b"]` are identical:
- `transactions:<sorted-query>`
- `balances:<depth>:<sorted-query>`
- `aregister:<account>:<sorted-query>`
- `accounts`, `tags`, `payees`
- `networth:<begin>:<end>`

## RPC Categories

**Queries** (cached): `GetTransactions`, `GetBalances`, `GetAccounts`, `GetAccountRegister`, `GetTags`, `GetPayees`, `GetNetWorth`

**Mutations** (go through txlock): `AddTransaction`, `DeleteTransaction`, `UpdateTransaction`, `ModifyTags`, `BulkModifyTags`, `ImportTransactions`, `RestoreSnapshot`, `AddPrice`, `DeletePrice`, `SaveRules`, `AddAccountDeclaration`

**Pass-through queries** (no cache): `GetRules`, `GetSnapshots`, `ListPrices`, `GetImportBatches`, `PreviewImport`, `GetConfig`

**Stripe Financial Connections** (pass-through, implemented in `stripe.go`): `GetStripeConfig`, `CreateStripeLinkSession`, `CompleteStripeLinking`, `ListStripeLinkedAccounts`, `UnlinkStripeAccount`, `FetchStripeTransactions`, `ImportStripeTransactions`, `FetchAllStripeTransactions`, `ImportAllStripeTransactions`, `RefreshStripeAccount`, `RefreshAllStripeAccounts`, `UpdateStripeAccountLastFetchedAt`

Stripe RPCs read credentials from environment variables (`STRIPE_SECRET_KEY`, `STRIPE_PUBLISHABLE_KEY`, `STRIPE_ACCOUNT_ID`) and degrade gracefully when unset. `CompleteStripeLinking`, `UnlinkStripeAccount`, and `ImportStripeTransactions` are mutations that go through `txlock.Do()` to persist account mappings / `LastFetchedAt` to `config.toml`. `FetchStripeTransactions` and `ImportStripeTransactions` apply categorization rules via `internal/rules` before returning or writing candidates.

**Refresh vs fetch (split)**: `FetchStripeTransactions` and `FetchAllStripeTransactions` only list transactions — they are fast and never trigger a Stripe refresh. `RefreshStripeAccount` and `RefreshAllStripeAccounts` are server-streaming RPCs that kick off a Stripe refresh and emit `RefreshStripeAccountProgress` events while polling, then a terminal `RefreshStripeAccountResult`. Streaming keeps the HTTP/2 connection alive past upstream proxy timeouts during the polling loop. **Invariant**: refresh RPCs never mutate `LastTransactionRefreshID` or `LastFetchedAt` in config. That high-water mark is advanced only by successful imports (`ImportStripeTransactions`, `ImportAllStripeTransactions`, daily auto-import), which guarantees a refresh-without-import doesn't drop transactions on the next refresh — the next list with `after=oldID` still captures everything from any intervening refreshes.

## Adding a New RPC

1. Add the method to `proto/float/v1/ledger.proto` and run `mise run proto-gen`
2. Implement the method on `*Handler` in a new or existing file in this package
3. For query RPCs: add a cache key helper and a `cached*` function
4. For mutation RPCs: wrap the file writes in `lock.Do(ctx, "description", fn)` and call `snap.Commit` if needed (txlock does this automatically when `SetSnap` is configured)
