# internal/journal

Text-level journal file manipulation: creating month files, appending/deleting transactions, updating include directives, migrating legacy transactions to add `fid` tags. Does **not** understand accounting semantics — all validation is delegated to `internal/hledger`.

## Key Functions

- `MintFID()` — generates a random 8-char lowercase hex FID (first 8 chars of UUID v4)
- `AppendTransaction(ctx, hledgerClient, dataDir, tx)` — mints FID, formats via hledger, ensures month file exists, updates `main.journal` if needed, appends text. Does **not** acquire txlock — callers must wrap in `txlock.Do()`.
- `DeleteTransaction(ctx, hledgerClient, dataDir, fid)` — uses `hledger print -O json tag:fid=<fid>` to find source location, removes the block directly. Does **not** acquire txlock.
- `FormatViaHledger(ctx, client, tx, fid)` — writes a temp file, shells out to `hledger print` for canonical formatting.
- `EnsureMonthFile(dataDir, year, month)` — creates `YYYY/MM.journal` if missing; returns `(relPath, created, err)`.
- `UpdateMainIncludes(mainPath, relPath)` — adds an include directive to `main.journal` (idempotent).
- `MigrateFIDs(dataDir)` — back-fills `fid` tags on untagged transactions; safe to re-run.

Month files are named `YYYY/MM.journal` and written with `O_APPEND` — transaction order matches write order.
