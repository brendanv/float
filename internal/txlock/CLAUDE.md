# internal/txlock

Serializes all mutations to the float data directory. Every write to journal files, config, rules, bank profiles, prices, account declarations, snapshots, or integration metadata should happen through `TxLock.Do()` or `TxLock.DoWith()` unless a caller has an explicit higher-level lock and knows why cache/snapshot behavior is not needed.

## Write Protocol

`Do(ctx, msg, fn)` / `DoWith(ctx, msg, extraPaths, fn)`:

1. Acquires an internal `sync.Mutex`.
2. Snapshots all `*.journal` files under `dataDir` into memory, plus any extra paths declared by the caller.
3. Runs caller-provided `fn` to perform the actual writes.
4. Runs `hledger check` against the configured main journal.
5. If `fn` or `hledger check` fails, restores all snapshotted files (journals and declared extras) and deletes journal files or declared-absent extra files that `fn` created during the failed write. The generation counter is bumped on failure too, since a concurrent read may have cached results computed from the intermediate (reverted) file state.
6. On success, bumps the atomic generation counter to invalidate caches.
7. If a `gitsnap.Repo` was registered with `SetSnap`, commits the successful mutation with `msg`; commit errors are logged and are not returned.

## API

- `New(dataDir, client)` — create a lock using the data directory and hledger client.
- `Do(ctx, msg, fn)` — run a journal-only mutation under the write protocol. Equivalent to `DoWith` with no extra paths.
- `DoWith(ctx, msg, extraPaths, fn)` — run a mutation that also touches non-journal files. `extraPaths` is a list of absolute paths to files that `fn` may create, modify, or delete. These are snapshotted before `fn` runs and reverted on failure alongside the journal files.
- `DoConfig(ctx, msg, paths, fn)` — run a mutation confined to non-journal files that cannot change any hledger query result (config.toml, rules.json, templates.json, bank profile rules files). Snapshots/reverts `paths` like `DoWith`, but skips `hledger check` and does not bump the generation counter — only `fn` failing (not `ErrNoChanges`) triggers a revert. Gitsnap still commits on success. Refuses any path ending in `.journal`. Use `DoWith` instead whenever a mutation might affect a query result — when in doubt, default to `DoWith`.
- `Generation()` — current cache generation.
- `BumpGeneration()` — manually invalidate caches, e.g. after a snapshot restore.
- `SetSnap(repo)` — enable automatic git snapshots after successful writes.
- `ErrNoChanges` — sentinel `fn` can return to signal it made no modifications. `DoWith`/`Do` treat this as success but skip `hledger check`, the generation bump, and the gitsnap commit. Useful for combining multiple idempotent startup migrations into one locked section without paying for a check when none of them had anything to do.

## Declaring Extra Paths

Use `DoWith` whenever `fn` writes files other than `*.journal` files. Pass the absolute paths of all non-journal files that `fn` may touch:

- `config.toml` writes: `h.configPath`
- `rules.json` writes: `rules.FilePath(h.dataDir)`
- `templates.json` writes: `templates.FilePath(h.dataDir)`
- `rules/*.rules` writes: `filepath.Join(h.dataDir, profile.RulesFile)`
- `uploads/*.csv` writes: the upload file path computed before the lock

Paths that do not exist before `fn` are tracked as absent: if `fn` creates them, they are removed on revert. Paths that exist before `fn` have their content recorded and are restored on revert.

## Important Boundaries

- `hledger check` is the validation gate; do not duplicate hledger accounting validation in callers.
- Keep `fn` small and deterministic. Do network calls before acquiring txlock when possible, then re-check deduplication/state inside the lock before writing.
- Callers that write both journal files and non-journal files (e.g. CSV imports that also save uploaded files) must use `DoWith` so all touched files are covered by the snapshot.
