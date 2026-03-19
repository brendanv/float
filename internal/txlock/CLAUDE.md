# txlock

Serializes all journal mutations and enforces the write protocol for every change to `.journal` files.

## Write Protocol (`TxLock.Do`)

1. Acquire `sync.Mutex` (one writer at a time)
2. Snapshot all `*.journal` files under `dataDir` into memory
3. Execute caller-provided `fn` (does the actual file writes)
4. If `fn` fails: revert all journal files from snapshot, return error
5. Run `hledger check` to validate the resulting journal
6. If check fails: revert all journal files from snapshot, return error
7. On success: bump the atomic generation counter (triggers cache invalidation)

## Key Rules

- **Every mutation** in the codebase must go through `txlock.Do()`. Never write to journal files outside of it.
- The snapshot covers all `*.journal` files at the time `Do` is called. Files created by `fn` that were not in the snapshot are deleted on revert.
- The generation counter is read by `internal/cache/` to detect stale entries. Any bump causes a full cache flush.
- `TxLock` does not commit to git — the caller (`LedgerService`) is responsible for calling `internal/gitsnap/` after a successful `Do`.
