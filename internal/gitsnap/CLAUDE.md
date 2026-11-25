# internal/gitsnap

Maintains a git repository inside a float data directory for journal/config snapshots. It uses go-git (pure Go); production code must not depend on the `git` binary.

## API

- `New(dir)` — opens an existing repo or initializes a new one, writes `.gitignore`, and creates an initial empty commit if needed.
- `Commit(ctx, msg)` — stages all changes and commits. It no-ops when the worktree is clean and caps long commit messages.
- `List(ctx, limit)` — returns snapshots (hash, message, timestamp) in reverse chronological order; limit 0 means the default of 50.
- `Head()` — hash of the current HEAD commit.
- `PlanRestore(ctx, hash)` — computes a forward restore: the tracked-file changes that turn HEAD's tree into `hash`'s tree, as a `RestorePlan` whose `Paths` (absolute) callers declare to txlock. Reads git objects only, so it can run outside the lock. `.gitignore` and the preserved files are never touched.
- `ApplyRestore(ctx, plan)` — writes the plan's files into the worktree without moving HEAD or the index, so the next `Commit` records the restore as a new snapshot on top of history. Refuses with `ErrHeadMoved` if HEAD changed since planning and `ErrDirty` if the worktree has uncommitted changes. Call inside txlock.
- `Restore(ctx, hash, msg)` — `PlanRestore` + `ApplyRestore` + `Commit`, for callers outside txlock (`floatctl journal restore`).
- `ResetHard(ctx, hash)` — hard-resets the branch and worktree to the given commit, discarding every later snapshot from history, while preserving secret/local files (`floatctl journal restore --hard`, `RestoreSnapshot` with `hard_reset`).
- `Diff(ctx, hash)` — returns per-file diffs for the target commit compared to its parent, including added/deleted/modified/renamed and binary-file markers.
- `RecoverUncommitted(ctx)` — commits dirty files left behind by a prior crash; no-ops on a clean tree.

## Integration

`txlock.TxLock` calls `Commit` automatically after each successful `Do()` when `lock.SetSnap(repo)` has been configured. Snapshot commit failures are non-fatal to the journal mutation.

`RestoreSnapshot` in the API plans the restore, then runs `ApplyRestore` inside `txlock.DoWithPublish` with the plan's paths declared, so the restore is serialized against concurrent writes, validated by `hledger check` (every changed file reverted on failure), bumps the generation, and is committed by txlock as a new snapshot. A stale plan (`ErrHeadMoved`) is re-planned, up to three attempts; a dirty worktree is first committed with `RecoverUncommitted`, then re-planned. With `hard_reset`, the same files are applied and checked, and the publish func then calls `ResetHard` to move the branch back, so txlock's commit is a no-op and a failed check never rewrites history.

## Ignored / Preserved Files

The repo-managed `.gitignore` excludes local secrets/state: `config.toml`, `float.key`, `ssh_host_key`, and `ssh_known_hosts`. `New` self-heals missing entries in existing repos (preserving user-added lines) so newly ignored files take effect without re-init. `Restore` reads preserved files before reset and writes them back afterward so snapshot rollbacks do not destroy local configuration/secrets.

All commits use author `float <float@localhost>`.
