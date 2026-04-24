# internal/gitsnap

Maintains a git repository inside the float data directory for transaction snapshots. Uses go-git (pure Go) — no git binary dependency.

## API

- `New(dir)` — opens an existing git repo in `dir`, or initialises a new one (creates `.gitignore` and an initial empty commit). Safe to call on every startup.
- `Commit(ctx, msg)` — stages all changes (`git add -A`) and commits. No-ops if the working tree is clean.
- `List(ctx, limit)` — returns up to `limit` snapshots (commit hash, message, timestamp) in reverse chronological order. Default limit is 50.
- `Restore(ctx, hash)` — hard-resets the working tree to the given commit. Preserves `config.toml`, `float.key`, and `ssh_host_key` across the reset.
- `RecoverUncommitted(ctx)` — called at startup to commit any leftover dirty files (e.g. from a crash mid-write). No-ops if the tree is clean.

## Integration

`txlock.TxLock` calls `Commit` automatically after each successful write when a `*Repo` has been registered via `lock.SetSnap(snap)`. The gitsnap commit is non-fatal — a commit failure is logged but does not roll back the journal mutation.

The `.gitignore` written on init excludes `config.toml` and `float.key` (secrets). `Restore` re-writes those files from memory after the hard reset so they survive snapshot rollbacks.

All git commits use the author `float <float@localhost>`.
