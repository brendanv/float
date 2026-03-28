# txlock

Serializes all journal mutations. Every write to `.journal` files must go through `txlock.Do()`.

## Write Protocol

1. Acquire `sync.Mutex`
2. Snapshot all `*.journal` files under `dataDir` into memory
3. Execute caller-provided `fn` (does the actual file writes)
4. If `fn` fails or `hledger check` fails: revert all journal files from snapshot, return error
5. On success: bump atomic generation counter (triggers full cache flush in `internal/cache/`)

`TxLock` does not commit to git — the caller (`LedgerService`) is responsible for calling `internal/gitsnap/` after a successful `Do`. Files created by `fn` that weren't in the snapshot are deleted on revert.
