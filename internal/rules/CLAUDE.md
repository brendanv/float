# internal/rules

Float's transaction auto-categorization rules. These are distinct from hledger CSV rules files (`data/rules/*.rules`): hledger rules parse source files, while float rules modify parsed/imported transactions or existing journal entries after matching.

Rules are stored as JSON in `<data-dir>/rules.json`.

## Rule Fields

- `id` — 8-character hex ID minted with `journal.MintFID`.
- `pattern` — case-insensitive regular expression matched against the transaction description.
- `payee` — set payee on match; empty means no payee change.
- `account` — set the category posting account; empty means no account change.
- `tags` — user-visible tags to add.
- `priority` — lower number is matched first; `Load` sorts ascending.
- `auto_reviewed` — if true, mark matched transactions `Cleared`.
- `match_account` — optional source account prefix filter; empty means any account.

## API

- `Load(dataDir)` — read `rules.json`; missing file returns an empty slice. Results are sorted by priority.
- `Save(dataDir, rules)` — write `rules.json`; call inside `txlock.Do()`.
- `Match(rules, description, account)` — return the first matching rule after regex and optional source-account checks.
- `Preview(rules, transactions)` — produce `RuleMatch` entries without writing. Skips transactions without a FID or with no effective changes.
- `Apply(ctx, client, dataDir, matches)` — apply all matches and return count. Must be called inside `txlock.Do()`.
- `ApplyBatch(ctx, client, dataDir, matches, onProgress)` — batched variant that emits progress callbacks and writes grouped replacements per source file.

## ChangeSet / Apply Semantics

`buildChangeSet` computes payee/account/tag/status changes. Account changes only apply when the transaction has an unambiguous category posting: currently a non-asset/non-liability posting in a 2-posting transaction. Existing user tags are preserved and rule tags are merged. Hidden `float-*` metadata and free-text comments are preserved.

`ApplyBatch` re-fetches source transactions, builds new `journal.TransactionInput` values, formats via hledger in batches, groups replacements by source file, and uses `journal.BatchReplaceTransactions` to minimize file I/O.
