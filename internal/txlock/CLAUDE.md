# internal/txlock

Serializes all mutations to the float data directory. Every write to journal files, config, rules, bank profiles, prices, account declarations, snapshots, or integration metadata should happen through `TxLock.Do()` unless a caller has an explicit higher-level lock and knows why cache/snapshot behavior is not needed.

## Write Protocol

`Do(ctx, msg, fn)`:

1. Acquires an internal `sync.Mutex`.
2. Snapshots all `*.journal` files under `dataDir` into memory.
3. Runs caller-provided `fn` to perform the actual writes.
4. Runs `hledger check` against the configured main journal.
5. If `fn` or `hledger check` fails, restores the journal snapshot and deletes journal files created during the failed write.
6. On success, bumps the atomic generation counter to invalidate caches.
7. If a `gitsnap.Repo` was registered with `SetSnap`, commits the successful mutation with `msg`; commit errors are logged and are not returned.

## API

- `New(dataDir, client)` — create a lock using the data directory and hledger client.
- `Do(ctx, msg, fn)` — run a mutation under the write protocol.
- `Generation()` — current cache generation.
- `BumpGeneration()` — manually invalidate caches, e.g. after a snapshot restore.
- `SetSnap(repo)` — enable automatic git snapshots after successful writes.

## Important Boundaries

- The rollback snapshot covers `*.journal` files only. Callers that write other files (for example `config.toml`, rules files, or import archives) still rely on txlock for serialization and git snapshots, but those non-journal files are not reverted by the in-memory journal rollback.
- `hledger check` is the validation gate; do not duplicate hledger accounting validation in callers.
- Keep `fn` small and deterministic. Do network calls before acquiring txlock when possible, then re-check deduplication/state inside the lock before writing.
