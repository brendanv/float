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
- `CompilePattern(pattern)` — compile a pattern as a cached case-insensitive `*regexp.Regexp`. Use this everywhere a rule pattern is compiled (Match, AI suggestion validation, RPC input validation) so the compile cache is shared and the `(?i)` convention stays consistent. Go's regexp is RE2 (linear-time).
- `Match(rules, description, account)` — return the first matching rule after regex and optional source-account checks. Uses `CompilePattern`, so patterns are compiled once and reused across the per-transaction calls made during imports.
- `Preview(rules, transactions)` — produce `RuleMatch` entries without writing. Skips transactions without a FID or with no effective changes.
- `Apply(ctx, client, dataDir, matches)` — apply all matches and return count. Must be called inside `txlock.Do()`.
- `ApplyBatch(ctx, client, dataDir, matches, onProgress)` — batched variant that emits progress callbacks and writes grouped replacements per source file.

### Import-time helpers (`categorize.go`)

Every import path (CSV, Stripe fetch/import/auto-import, structured import) applies rules through these; don't re-implement rule effects in handlers.

- `IsAssetOrLiabilityAccount(account)` — true when the top-level segment is `assets`/`asset`/`liabilities`/`liability` (case-insensitive). Asset/liability postings are the source (bank) side; the rest are categories.
- `SourceAccount(postings)` / `InputSourceAccount(postings)` — first asset/liability account, the `account` argument to `Match`.
- `CategoryPostingIndex(txn)` — the single posting Preview/Apply would recategorize, or -1.
- `ApplyToCandidate(txn, rule)` — copy with the fingerprinted effects (payee prefix `Payee | desc`, category account) applied. Fingerprint this form for duplicate detection.
- `ApplyTagsAndStatus(input, rule)` — the non-fingerprinted effects: add tags (overwriting), mark `Cleared` if `auto_reviewed`.
- `ApplyToInput(input, rule)` — all of the above on a `journal.TransactionInput`.
- `SetCategoryAccount(postings, account)` — on a 2-posting input, set every non-asset/liability posting to `account`. Structured import uses this directly because explicit payee/tags/status on its input win over the rule's.

All of them accept a nil rule as a no-op.

## ChangeSet / Apply Semantics

`buildChangeSet` computes payee/account/tag/status changes. Account changes only apply when the transaction has an unambiguous category posting: currently a non-asset/non-liability posting in a 2-posting transaction. Existing user tags are preserved and rule tags are merged. Hidden `float-*` metadata and free-text comments are preserved.

`ApplyBatch` re-fetches source transactions, builds new `journal.TransactionInput` values, formats via hledger in batches, groups replacements by source file, and uses `journal.BatchReplaceTransactions` to minimize file I/O.
