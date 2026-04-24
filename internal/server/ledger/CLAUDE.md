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

## Adding a New RPC

1. Add the method to `proto/float/v1/ledger.proto` and run `mise run proto-gen`
2. Implement the method on `*Handler` in a new or existing file in this package
3. For query RPCs: add a cache key helper and a `cached*` function
4. For mutation RPCs: wrap the file writes in `lock.Do(ctx, "description", fn)` and call `snap.Commit` if needed (txlock does this automatically when `SetSnap` is configured)
