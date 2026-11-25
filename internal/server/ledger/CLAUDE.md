# internal/server/ledger

ConnectRPC handler implementing `LedgerService`. This is the main application service used by the web UI.

## Handler

`Handler` holds references to the hledger client, txlock, data directory, config path, generation cache, a recently-used-accounts LRU, gitsnap repo, in-memory config, and log broadcaster. Construct it with `NewHandler(...)` from `cmd/floatd`.

RPCs are grouped by feature area, one file each: `handler.go` (the `Handler` type, constructor, config load/save, `RunHledgerQuery`), `handler_cache.go` (cache keys and `cached*`/`filtered*` helpers), `handler_convert.go` (shared `toProto*` conversions), and `handler_<area>.go` for `queries`, `transactions` (incl. bulk edit/delete), `timeseries`, `portfolio`, `accounts`, `prices`, `snapshots`, `bank_profiles`, `imports`, `structured_import`, `rules`, `templates`, `transfers`, `ai`, `general_config` and `logs`; Stripe lives in `stripe*.go`. Tests mirror the same split (`handler_<area>_test.go`), with shared fixtures and handler constructors in `handler_test.go`.

All accounting queries delegate to `internal/hledger`. All journal/config/rules/import mutations must run in `lock.Do(ctx, "message", fn)` unless they are purely read-only.

**Publish in-memory state only after the write commits.** `h.cfg` (and any other in-memory mirror of data-dir files) must change only once the txlock call has succeeded. `Do`/`DoWith` run `hledger check` *after* `fn` returns and revert the files (config.toml included) on failure, but cannot revert memory, so never call `h.cfg.Store` inside a `Do`/`DoWith` callback. Use `lock.DoWithPublish` / `lock.DoConfigPublish` and end `fn` with `return h.saveCfg(&newCfg)`: `saveCfg` writes config.toml and returns a publish func that txlock runs after validation succeeds, still under the mutex (storing after the lock call returns would let a concurrent writer read the stale config as its base and clobber the update). Read the base config with `h.loadCfg()` inside `fn`. Existing `DoConfig` callbacks that `Store` as their last step are safe only because `DoConfig` has no post-`fn` failure path.

## Cache Keys

Cached query helpers produce deterministic namespaced keys. Query tokens are sorted before joining, so `[]string{"b", "a"}` and `[]string{"a", "b"}` share entries.

Current cached families include:

- `transactions:<sorted-query>`
- `balances:<depth>:<sorted-query>` (`balancestree:…` for `--tree` reports)
- `balancesvalued:<valueSpec>:<depth>:<sorted-query>` (`balancesvaluedtree:…` for `--tree` reports)
- `lots:<sorted-query>` (not warmed)
- `aregister:<account>:<sorted-query>`
- `accounts`, `closedaccounts`, `tags`, `payees`
- `networth:<begin>:<end>`
- `incomestmt:<begin>:<end>`
- `portfolio:<accounts>:<begin>` and `portfoliocost:<accounts>:<begin>`

Pagination (`limit`/`offset`) is applied after loading/caching the full hledger result.

## Proactive Warming (`warm.go`)

`Handler.WarmEntries()` returns the fixed, priority-ordered `[]warm.Entry` that `cmd/floatd` feeds to an `internal/warm.Warmer`: `transactions:(nil)`, `accounts`/`closedaccounts`/`tags`/`payees`, `balances:0:(nil)`, `balancesvalued:now,USD` at depth 0 and 1 (plus depth 1 with `openAssetLiabilityQuery`, the web UI's net-worth summary on Overview and Trends, and the `--tree` report with the same query, the home page account list), full-history `networth`/`incomestmt`, plus one `aregister:<account>:(nil)` entry per account in `recentAccounts` — a small in-memory LRU (`recentAccountsCap` = 20) updated by every `GetAccountRegister` call. `WarmEntries` is called fresh at the start of each warm pass (not cached), so the LRU-derived entries reflect the latest accounts touched. Returns `nil` when `h.cache == nil`.

## Filtering in Go (`internal/txfilter`)

`ListTransactions`, `GetImportedTransactions`, `ListImports`, and `GetAccountRegister` prefer serving a query from a single canonical unfiltered fetch — `transactions:(nil)` / `aregister:<account>:(nil)` — filtered in Go, instead of a query-keyed hledger invocation. `filteredTransactions`/`filteredAregister` call `txfilter.Parse(query)` first: if every token is within its supported vocabulary (`date`, `acct`, `tag`, `payee`, `desc`, `status`, `code`, and `not:` negations), they load the unfiltered fetch and filter in memory; otherwise they fall back to a dedicated cached hledger query exactly as before. `internal/txfilter` has table-driven tests that run real hledger over fixtures and assert equivalence — extend those fixtures/cases rather than hand-verifying new token behavior.

`hledger areg`'s `date:` filtering matches on ANY posting's effective date (including posting-date overrides on legs other than the focused account), which differs from `print`'s transaction-date-only semantics that `txfilter`'s date predicate assumes. Rather than reimplement areg's exact rule, `filteredAregister` falls back to `cachedAregister` (a real hledger `areg` query) whenever the query contains a `date:`/`not:date:` token, keeping the Go path for the tag/status/payee/desc filters it was built for.

`GetPortfolioHoldings` and `GetPortfolioTimeseries` fetch the unfiltered `balances:0:(nil)`/`transactions:(nil)` datasets and filter by `AccountPrefix` in Go (`accountMatchesPrefix`/`transactionTouchesPrefix`) — this is a plain account-name-prefix match, not hledger's general regex matching, and is fine because `AccountPrefix` is always a literal prefix from the UI, not accounting logic.

`GetNetWorthTimeseries` and `GetIncomeStatementTimeseries` serve a sliceable range (`canSliceTimeseries`: each bound `""` or a valid first-of-month date, and `begin < end` when both are set) by fetching the full-history timeseries once and reshaping it in Go (`sliceBalanceSheetTimeseries`/`sliceIncomeStatementTimeseries`). Anything else goes straight to hledger: a non-month-aligned bound makes `bs`/`is --monthly` emit a partial period anchored to that exact date, and an empty/inverted range yields a dateless period. The slice reproduces hledger's ranged output exactly (`TestTimeseriesSliceMatchesHledger` runs real hledger for every begin/end shape and compares; extend it rather than hand-verifying):

- Periods (`slicedMonths`): a missing begin defaults to the first full-history period and a missing end to the month after the last (the journal span, which for `bs --value` includes price directives; both fetches use identical flags). If that span is empty — begin-only at/after the last period, end-only at/before the first, or an empty journal — hledger emits a single period at the given bound.
- `bs --historical` values are cumulative regardless of begin, so each month takes the latest full-history period at or before it: zero before the ledger's first period, carried forward after its last (`periodSources` with `carry`).
- `is` months absent from the full history are empty (no flow). Row totals (`ISRow.TotalAmounts`) are recomputed over the sliced periods (`sumAmounts`) and rows with no activity in range are dropped. hledger's `--tree` elides a parent with no own postings and a single active child branch, which the full-history rows can't decide; when any sliced parent row has exactly one active child branch (`ambiguousTreeElision`), `sliceIncomeStatementTimeseries` returns `ok=false` and the handler queries hledger for that range directly.

## RPC Categories

**Cached queries:** `ListTransactions`, `GetBalances`, `GetAccountRegister`, `ListLots`, `ListAccounts`, `ListTags`, `ListPayees`, `GetNetWorthTimeseries`, `GetIncomeStatementTimeseries`, portfolio timeseries helpers.

**Read/query helpers with limited or no cache:** `GetPortfolioHoldings` (uses balances plus `prices.journal`), `GetBalanceAssertionStatus`, `FindUnmatchedTransfers`, `ListPrices`, `ListAccountDeclarations`, `ListSnapshots`, `GetSnapshotDiff`, `ListBankProfiles`, `GetBankProfileContent`, `PreviewImport`, `ImportStructuredTransactions` with `dry_run`, `GetImportedTransactions`, `ListImports`, `GetImportFile`, `ListRules`, `PreviewApplyRules`, settings reads, `RunHledgerQuery`, `ExportJournal`, and `StreamLogs`.

**Mutations through txlock:** transaction add/update/delete/status/date/tag changes, bulk edit/delete, price add/delete/backfill, account declaration CRUD/rename, snapshot restore, bank profile CRUD, CSV import, structured import, rule CRUD/apply, template CRUD, Alpha Vantage/AI/Stripe/timezone settings, and Stripe link/unlink/import metadata writes.

**Streaming RPCs:** `ImportTransactions`, `BulkDeleteTransactions`, `ApplyRules`, `ImportStripeTransactions`, `ImportAllStripeTransactions`, `RefreshStripeAccount`, `RefreshAllStripeAccounts`, and `StreamLogs`.

## Feature Areas

### Transactions / Bulk Editing

Transaction writes convert proto input into `journal.TransactionInput`, call journal text helpers under txlock, then re-read via hledger before returning. `UpdateTransaction` keeps the existing comment/tags when `comment`/`tags` are empty (proto3 can't distinguish unset from empty) unless `replace_comment`/`replace_tags` is set; postings are replaced wholesale, including their comments. `AddTransaction` writes `Cleared` unless the optional `status` is set. Bulk edit supports mark reviewed/unreviewed, add/remove tag, set/clear payee, and delete. Bulk delete streams progress.

### Prices / Portfolio / Alpha Vantage

`ListPrices` reads `prices.journal`. `BackfillPrices` uses `internal/alphavantage`, skips already-present commodity/date prices, and writes new price directives inside txlock. Portfolio holdings aggregate raw commodity balances by account/symbol, use latest price directives for value/allocation, and derive cost basis from hledger cost annotations.

### Accounts and Assertions

Account declarations are managed in `accounts.journal`. `RenameAccount` renames the account and its sub-accounts (exact name or `old:` prefix, via `journal.RenameAccountName`) in declarations, posting lines, and float's own references to it — rule `account`/`match_account` in `rules.json`, template posting accounts in `templates.json`, and Stripe linked accounts' `hledger_account`, `portfolio.excluded_account_prefixes` and the effective `transfers.account` (so renaming the default `equity:transfers` writes the new name) in `config.toml` — all in one `DoWith` declaring those three files, so a failed `hledger check` reverts everything. The new in-memory config is published only after the lock succeeds (`CompareAndSwap` against the config it was derived from; if another config writer slipped in between, the config rename is re-applied under `DoConfig`). `GetBalanceAssertionStatus` ranks asset/liability accounts by transactions since their last simple balance assertion, skipping closed accounts.

`FindUnmatchedTransfers` (`handler_transfers.go`) is read-only: it runs `internal/transfers.FindUnmatched` over the cached `transactions:(nil)` fetch with the configured clearing account (`transfers.account`, default `equity:transfers`), the default 5-day window and today in the config timezone. Matching is scoped to routes (the clearing account's direct children, e.g. `equity:transfers:Schwab-Amex`); each unmatched side carries its route and the accounts its other side was expected on. `from_date` only filters the results; matching always uses all history. Results are newest first.

Closed accounts: `SetAccountClosed` toggles the `float-closed` tag on the account directive (float-only metadata; hledger reports are unaffected). `ListAccounts` sets `Account.closed` from `cachedClosedAccounts` (hledger `accounts tag:float-closed`, so sub-accounts inherit), and `ListAccountDeclarations` reports the directly-set flag. The web UI hides closed accounts from account pickers (`filterAndSortAccountNames`, opt back in with `includeClosed`) and the home account list, and computes home totals with `not:tag:float-closed`.

### Imports and Bank Profiles

Bank profiles live in config and point to hledger CSV rules files under the data directory. CSV preview runs `hledger print --rules-file`, applies float categorization rules, marks duplicates by fingerprint, and returns import candidates. Imports write selected candidates, assign an import batch ID, archive the source CSV, and stream progress.

`ImportStructuredTransactions` (`handler_structured_import.go`) is the unary, CSV-free equivalent for scripts and agents. It validates each `StructuredTransaction` in Go, optionally applies float rules (matched on the bare description; an explicit payee/tag/status on the input wins over the rule's), then parses the whole batch through hledger with `journal.ParseInputs` — validation plus the canonical form used for fingerprints and the returned candidates. `dry_run` stops there (duplicates flagged from the cache). Otherwise, inside one `lock.Do`, it re-fingerprints against the on-disk journal, skips duplicates unless `include_duplicates`, and appends the rest with hidden `float-import` (batch ID `<source>/<date>-<fid>`) and `float-import-source` metadata. Default status is Unmarked. `ListImports` reports `float-import-source` as `ImportSummary.source` when present, else `stripe`/`csv` by batch ID. No source file is archived, so `GetImportFile` returns NotFound for these batches.

### Categorization Rules

Rules are stored in `rules.json` and handled through `internal/rules`. Add/update/delete mutate `rules.json` under txlock. Preview/apply can target rule IDs and optional hledger query tokens; apply streams progress.

### Stripe Financial Connections

Stripe handlers live in `stripe.go` and read credentials from environment variables (`STRIPE_SECRET_KEY`, `STRIPE_PUBLISHABLE_KEY`). Customer ID, linked accounts, daily auto-import toggle, and timestamps are stored in config.

Fetch RPCs only list already-available settled transactions and hide duplicates by hidden `float-stripe-txn` metadata. Each linked account may have an `import_from` cutoff (YYYY-MM-DD, set via `CompleteStripeLinking` or `SetStripeAccountImportFrom`); `stripeTxnImportable` drops transactions dated before it (transacted_at in the config timezone) from fetch, both import RPCs and daily auto-import. It exists for relinked accounts: Stripe gives a relinked account new transaction IDs, so ID dedup alone would offer the whole history again. `CompleteStripeLinking` with an empty `import_from` keeps an existing cutoff. Refresh RPCs kick off/poll Stripe refreshes and stream progress/results, honoring Stripe's per-account throttle. Import RPCs re-read existing Stripe IDs inside txlock to prevent double writes during races with auto-import.

### Daily Stripe Auto-Import

`StartDailyStripeImport` runs in the server context. When enabled in config, it refreshes or lists accounts according to Stripe throttles, imports new settled transactions, and updates `LastDailyImportAt`.

### AI

AI handlers use `internal/ai` and OpenRouter via `OPENROUTER_API_KEY`. They can suggest rules for transactions, flag existing rules that are duplicates/contradictions/combinable (`FindRuleIssues`, read-only — it does not modify rules), translate natural language into hledger query tokens, and answer questions by running hledger and summarizing the output. Model/prompt config is persisted in `config.toml`.

Account suggestions (`handler_ai_accounts.go`) are computed once, in the background, when transactions are written, never on read. `cmd/floatd` registers `WakeAccountSuggester` as a `lock.OnCommit` hook and runs `StartAccountSuggester`, which debounces (3s) and runs `suggestPendingAccounts`: unreviewed transactions with a posting account containing "unknown" (the `update_unknown_account` test) and no stored result are sent to the decision model in batches of 50 — `ai.decision_model` in `config.toml` (`SetAIDecisionModel`), else the main model (`OPENROUTER_MODEL` → `ai.model` → default). The prompt gets the open, non-unknown account names and, per transaction, up to 5 past categorized transactions matched by payee or the description's first distinctive word, all from the cached `transactions:(nil)` fetch; suggestions naming an unlisted account are dropped. Results go to `account-suggestions.json` (`internal/suggestions`, gitignored by gitsnap) via `DoConfigPublish` (no generation bump, so no re-trigger), with an empty account recording "no suggestion" so nothing is sent twice; a failed AI batch is not recorded and retries on the next write; entries whose transaction no longer has an unknown posting are pruned. `h.suggestions` mirrors the file; `ListTransactions`/`GetAccountRegister` attach `suggested_account` only while the transaction still has an unknown posting. Nothing touches postings: the client applies a suggestion with `BulkEditTransactions`. Without `OPENROUTER_API_KEY` a pass does nothing.

### Snapshots

`RestoreSnapshot` defaults to a forward restore: the data directory is set back to the snapshot's tracked files and txlock commits that as a new snapshot (`restore snapshot <hash8> ("<message>")`), so later snapshots stay in history and the restore can itself be restored away. `hard_reset` instead discards the later snapshots, but only after `hledger check` has passed on the restored files. See `internal/gitsnap/CLAUDE.md` for the mechanics.

### Undo (`handler_undo.go`)

`NewUndoInterceptor` (registered by `cmd/floatd`) wraps the procedures in `undoableProcedures` — single-transaction delete/update, bulk edit/delete, apply rules, CSV and structured import, rule/template/price/account-declaration delete, close/reopen, forward restore — with a `txlock.Receipt` and, when the call made one undoable write, sets the `Float-Snapshot` response trailer (trailers, so server-streaming RPCs work too). Keep the list in sync with the `UndoSnapshotRequest` proto comment and the web's `undoLabel`. Writes that also touch `config.toml` (rename, Stripe, settings) never get the trailer because git can't restore that file.

`UndoSnapshot(hash)` plans with `gitsnap.PlanUndo` and applies under `lock.DoWith` (plan paths declared, `hledger check`, file-for-file revert on failure). It commits `undo: <message>` (undoing an undo commits `redo: …`) and returns that snapshot, so the client can offer Redo. If `hash` is no longer the newest snapshot — any other write landed, undoable or not — it returns `FailedPrecondition` and changes nothing; older history is for `RestoreSnapshot`.

### Settings / Logs / Debug

Settings handlers manage Alpha Vantage API key, AI model/decision model/prompt, Stripe customer/daily import config, and timezone. `RunHledgerQuery` runs a constrained raw hledger command with `-f <journal>` automatically prepended. `ExportJournal` runs `hledger print -f <journal>` and returns the flattened journal (all includes inlined) as bytes plus a suggested filename, for a one-file download of the entire ledger. `StreamLogs` streams broadcast slog entries filtered by minimum level.

## Adding a New RPC

1. Add the method/messages to `proto/float/v1/ledger.proto` and run `mise run proto-gen`.
2. Implement the method on `*Handler` in a focused file in this package.
3. For query RPCs, add deterministic cache keys and `cached*` helpers when useful.
4. For mutation RPCs, wrap writes in `lock.Do(ctx, "description", fn)` and re-check race-prone state inside the lock.
5. Update the web client and this documentation when adding user-visible behavior.
