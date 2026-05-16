# txlock

Serializes all journal mutations. Every write to `.journal` files must go through `txlock.Do()`.

## Write Protocol

1. Acquire `sync.Mutex`
2. Snapshot all `*.journal` files AND known data files (`config.toml`, `rules.json`) under `dataDir` into memory
3. Execute caller-provided `fn` (does the actual file writes)
4. If `fn` fails or `hledger check` fails: revert all snapshotted files, delete any files `fn` created that weren't in the snapshot, return error
5. On success: bump atomic generation counter (triggers full cache flush in `internal/cache/`), then commit to git via `gitsnap` (non-fatal on error)

If `SetSnap()` has been called with a `*gitsnap.Repo`, `Do()` automatically commits after each successful write. `BumpGeneration()` can be called externally to invalidate the cache without a write (e.g. after a snapshot restore).

## Non-journal file coverage

`config.toml` and `rules.json` are listed in `knownDataFiles` and are snapshotted on every `Do()` call. This means callers that write these files inside `fn()` (e.g. saving `LastFetchedAt` during a Stripe import, or updating bank profiles) automatically get the same all-or-nothing guarantee as journal files: if `hledger check` fails, the on-disk state of these files is rolled back to what it was before `fn()` ran.

Note: `h.cfg` (the in-memory config) is not rolled back automatically. Callers that mutate it inside `fn()` should revert the in-memory change if `fn()` itself returns an error (which they currently do), but a `hledger check` failure after a successful `fn()` may leave `h.cfg` temporarily stale. This resolves on the next server restart or config reload.
