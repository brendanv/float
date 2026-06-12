# internal/gitsnap

Maintains a git repository inside a float data directory for journal/config snapshots. It uses go-git (pure Go); production code must not depend on the `git` binary.

## API

- `New(dir)` — opens an existing repo or initializes a new one, writes `.gitignore`, and creates an initial empty commit if needed.
- `Commit(ctx, msg)` — stages all changes and commits. It no-ops when the worktree is clean and caps long commit messages.
- `List(ctx, limit)` — returns snapshots (hash, message, timestamp) in reverse chronological order; limit 0 means the default of 50.
- `Restore(ctx, hash)` — hard-resets the worktree to the given commit while preserving secret/local files.
- `Diff(ctx, hash)` — returns per-file diffs for the target commit compared to its parent, including added/deleted/modified/renamed and binary-file markers.
- `RecoverUncommitted(ctx)` — commits dirty files left behind by a prior crash; no-ops on a clean tree.

## Integration

`txlock.TxLock` calls `Commit` automatically after each successful `Do()` when `lock.SetSnap(repo)` has been configured. Snapshot commit failures are non-fatal to the journal mutation.

`RestoreSnapshot` in the API runs `Repo.Restore` inside `txlock.DoWith`, so the restore is serialized against concurrent writes, validated by `hledger check` (reverted on failure), and bumps the generation so caches are invalidated.

## Ignored / Preserved Files

The repo-managed `.gitignore` excludes local secrets/state: `config.toml`, `float.key`, `ssh_host_key`, and `ssh_known_hosts`. `New` self-heals missing entries in existing repos (preserving user-added lines) so newly ignored files take effect without re-init. `Restore` reads preserved files before reset and writes them back afterward so snapshot rollbacks do not destroy local configuration/secrets.

All commits use author `float <float@localhost>`.
