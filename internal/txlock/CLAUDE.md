# txlock

Serializes all journal mutations. Every write to `.journal` files must go through `txlock.Do()`.

## Write Protocol

1. Acquire `sync.Mutex`
2. Snapshot all `*.journal` files under `dataDir` into memory
3. Execute caller-provided `fn` (does the actual file writes)
4. If `fn` fails or `hledger check` fails: revert all journal files from snapshot, return error
5. On success: bump atomic generation counter (triggers full cache flush in `internal/cache/`), then commit to git via `gitsnap` (non-fatal on error)

If `SetSnap()` has been called with a `*gitsnap.Repo`, `Do()` automatically commits after each successful write. `BumpGeneration()` can be called externally to invalidate the cache without a write (e.g. after a snapshot restore). Files created by `fn` that weren't in the snapshot are deleted on revert.
