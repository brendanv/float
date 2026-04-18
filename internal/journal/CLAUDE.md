# internal/journal

Text-level journal file manipulation: creating month files, appending/deleting/updating transactions, updating include directives, managing commodity prices, migrating legacy transactions to use code fields. Does **not** understand accounting semantics — all validation is delegated to `internal/hledger`.

## Key Functions

### Transactions

- `MintFID()` — generates a random 8-char lowercase hex code (first 8 chars of UUID v4)
- `WriteTransaction(ctx, hledgerClient, dataDir, input, src)` — unified write function. If `src` is nil, appends as a new transaction (minting a FID if input.FID is empty). If `src` is non-nil, replaces the existing block at that location, moving to a new month file if the date changed. Always stamps `float-updated-at` hidden meta. Does **not** acquire txlock — callers must wrap in `txlock.Do()`.
- `AppendTransaction(ctx, hledgerClient, dataDir, tx)` — mints code, formats via hledger, ensures month file exists, updates `main.journal` if needed, appends text. Does **not** acquire txlock — callers must wrap in `txlock.Do()`.
- `DeleteTransaction(ctx, hledgerClient, dataDir, fid)` — uses `hledger print -O json code:<fid>` to find source location, removes the block directly. Does **not** acquire txlock.
- `UpdateTransaction(ctx, hledgerClient, dataDir, fid, description, newDate, comment, tags, postings)` — replaces description, date, comment, and postings while preserving fid, status, and hidden meta. If `newDate` is empty, keeps the existing date. If `tags` is non-nil, replaces all user-visible tags; if nil, existing tags are preserved.
- `ModifyTags(ctx, hledgerClient, dataDir, fid, tags)` — replaces all user-visible (non-`float-`) tags; preserves free-text comment, hidden meta, status, and postings.
- `ModifyFloatMeta(ctx, hledgerClient, dataDir, fid, meta)` — replaces all hidden-meta tags (keys must include `float-` prefix); preserves user-visible tags and free-text comment.
- `UpdateTransactionDate(ctx, hledgerClient, dataDir, fid, newDate)` — changes the date; moves to a different month file if the date crosses a month boundary. Returns the updated transaction.
- `UpdateTransactionStatus(ctx, hledgerClient, dataDir, fid, newStatus)` — changes the hledger status marker (`""`, `"Pending"`, or `"Cleared"`).
- `FormatViaHledger(ctx, client, tx, fid)` — writes a temp file, shells out to `hledger print` for canonical formatting.

### Month Files

- `EnsureMonthFile(dataDir, year, month)` — creates `YYYY/MM.journal` if missing; returns `(relPath, created, err)`.
- `UpdateMainIncludes(mainPath, relPath)` — adds an include directive to `main.journal` (idempotent).

### Prices

- `ListPrices(dataDir)` — reads `prices.journal` and returns all parsed P directives. Returns empty slice (not an error) if the file doesn't exist.
- `AppendPrice(dataDir, date, commodity, quantity, currency)` — writes a new P directive to `prices.journal`, minting a PID. Creates the file and prepends the include in `main.journal` if needed.
- `DeletePrice(dataDir, pid)` — removes the P directive with the given PID from `prices.journal`.

### Migration

- `MigrateFIDs(dataDir)` — converts old `; fid:` tags to code fields, and mints codes for untagged transactions; safe to re-run.

## Notes

Month files are named `YYYY/MM.journal` and written with `O_APPEND` — transaction order matches write order. All mutation functions require the caller to hold txlock.
