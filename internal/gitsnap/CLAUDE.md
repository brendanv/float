# internal/gitsnap

Maintains a git repository inside a float data directory for journal/config snapshots. It uses go-git (pure Go); production code must not depend on the `git` binary.

## API

- `New(dir)` — opens an existing repo or initializes a new one, writes `.gitignore`, and creates an initial empty commit if needed.
- `Commit(ctx, msg)` — stages all changes and commits. It no-ops when the worktree is clean and caps long commit messages.
- `List(ctx, limit)` — returns snapshots (hash, message, timestamp) in reverse chronological order; limit 0 means the default of 50.
- `Restore(ctx, hash)` — non-destructive restore: writes the target commit's tree into the worktree (creating, overwriting, and deleting tracked files) **without moving HEAD or the index**, so the next `Commit` records the restore as a new snapshot on top of history and restoring a newer snapshot undoes it. Shorthand for `PlanRestore` + `ApplyRestore`.
- `PlanRestore(ctx, hash)` — computes a `*RestorePlan`; `plan.Paths()` returns the sorted absolute paths the restore may create/modify/delete (union of files tracked at HEAD and files in the target tree, minus preserved and gitignored paths) so callers can snapshot them first.
- `ApplyRestore(ctx, plan)` — applies a plan. Returns `ErrStaleRestorePlan` if HEAD moved since planning (the path set may then be incomplete).
- `Diff(ctx, hash)` — returns per-file diffs for the target commit compared to its parent, including added/deleted/modified/renamed and binary-file markers.
- `RecoverUncommitted(ctx)` — commits dirty files left behind by a prior crash; no-ops on a clean tree.

## Integration

`txlock.TxLock` calls `Commit` automatically after each successful `Do()` when `lock.SetSnap(repo)` has been configured. Snapshot commit failures are non-fatal to the journal mutation.

`RestoreSnapshot` in the API calls `PlanRestore`, passes the plan's non-journal `Paths()` as `txlock.DoWith` extra paths, and runs `ApplyRestore` inside the lock. The restore is therefore serialized against concurrent writes, validated by `hledger check` and fully reverted on failure (journals, rules files, uploads, etc., with HEAD untouched), bumps the generation so caches are invalidated, and on success is committed by txlock as `restore snapshot <hash>`. `floatctl journal restore` commits the same message itself.

## Ignored / Preserved Files

The repo-managed `.gitignore` excludes local secrets/state: `config.toml`, `float.key`, `ssh_host_key`, and `ssh_known_hosts`. `New` self-heals missing entries in existing repos (preserving user-added lines) so newly ignored files take effect without re-init. `Restore` never creates, overwrites, or deletes preserved files or paths matched by the current `.gitignore`, and leaves untracked files alone. It re-heals `.gitignore` afterward in case the target snapshot's copy predates newer entries.

All commits use author `float <float@localhost>`.
