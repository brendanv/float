# internal/journal

Text-level journal file manipulation: month files, transaction append/update/delete, include directives, account declarations, commodity/price directives, tag/meta rewrites, migrations, and account renames. This package manipulates text and source locations; accounting validation belongs to hledger and txlock.

## Invariants

- Callers must wrap mutations in `txlock.Do()`.
- Transactions written by float use an 8-character transaction code (`(a1b2c3d4)`) minted by `MintFID()`.
- Hidden float metadata uses `float-*` tags; user-facing tag operations preserve these hidden tags.
- Formatting goes through hledger (`FormatViaHledger` / `BatchFormatViaHledger`) whenever new transaction text is produced.

## Transactions

- `MintFID()` — random 8-character lowercase hex code from UUID v4.
- `WriteTransaction(ctx, client, dataDir, input, src)` — unified writer. `src == nil` appends as new; non-nil replaces the block at `SourceLocation` and moves it if the date changes month. Stamps `float-updated-at`.
- `AppendTransaction(ctx, client, dataDir, tx)` — append-only path: ensure month file, update include, format via hledger, append.
- `DeleteTransaction(ctx, client, dataDir, fid)` — find by `code:<fid>` using hledger source positions and remove the text block.
- `BatchDeleteTransactions(ctx, client, fids, onProgress)` — grouped deletion for bulk delete; emits progress.
- `UpdateTransaction(ctx, client, dataDir, fid, description, newDate, comment, tags, postings, newStatus)` — replace editable transaction fields while preserving FID and hidden meta.
- `UpdateTransactionDate(ctx, client, dataDir, fid, newDate)` — date-only update, including cross-month moves.
- `UpdateTransactionStatus(ctx, client, dataDir, fid, newStatus)` — status marker update (`""`, `"Pending"`, `"Cleared"`).
- `ModifyTags(ctx, client, dataDir, fid, tags)` — replace user-visible tags while preserving hidden metadata and comments.
- `ModifyFloatMeta(ctx, client, dataDir, fid, meta)` — replace hidden metadata while preserving user tags and comments.
- `InputFromTransaction` / `HledgerTxnToInput` — convert parsed hledger transactions back into writable `TransactionInput`.
- `TxnFingerprint` — duplicate-detection fingerprint for imports.

## Formatting Types

- `TransactionInput` — date, status, payee/description/comment, tags, postings, import batch, and Stripe transaction ID.
- `PostingInput` — account, commodity/quantity, comment, optional cost, optional simple balance assertion.
- `CostInput` — unit or total cost annotations.
- `BalanceAssertionInput` — simple `=` balance assertion only.

Balance assertions that are more complex than the simple `=` form are preserved by line-oriented rewrites but not represented in the public input type.

## Month Files / Includes

- `EnsureMonthFile(dataDir, year, month)` — creates `YYYY/MM.journal`; returns relative path and whether it was created.
- `UpdateMainIncludes(mainJournalPath, relPath)` — idempotently add an `include <relPath>` directive.

## Accounts / Commodities / Prices

- `ListAccountDeclarations`, `AppendAccountDeclaration`, `DeleteAccountDeclaration`, `RenameAccountDeclaration` — manage `accounts.journal` declaration lines.
- `EnsureAccountsFile`, `EnsureAccountsInclude` — startup bootstrap for account declarations.
- `RenameAccountInJournalFiles(dataDir, oldName, newName)` — line-oriented posting account rename across journal files.
- `EnsureCommodityDirective(dataDir, code)` — ensure a `commodity` directive exists.
- `ListPrices`, `AppendPrice`, `DeletePrice` — manage `prices.journal` `P` directives with generated PIDs and an include in `main.journal`.

## Migrations

- `MigrateFIDs(dataDir)` — converts legacy `; fid:` tags to code fields and mints codes for untagged transactions; safe to rerun.
- `MigrateStripeTxnTag(ctx, client, dataDir)` — moves legacy `stripe-txn` tags into hidden `float-stripe-txn` metadata.

## Low-Level Replacement Helpers

`BatchReplaceTransactions` and internal delete/replace helpers operate on line numbers from hledger source positions. Use them only after hledger has identified source locations; do not infer transaction boundaries manually for accounting decisions.
