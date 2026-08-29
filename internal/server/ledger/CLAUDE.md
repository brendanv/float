# internal/server/ledger

ConnectRPC handler implementing `LedgerService`. This is the main application service used by the web UI.

## Handler

`Handler` holds references to the hledger client, txlock, data directory, config path, generation cache, gitsnap repo, in-memory config, and log broadcaster. Construct it with `NewHandler(...)` from `cmd/floatd`.

All accounting queries delegate to `internal/hledger`. All journal/config/rules/import mutations must run in `lock.Do(ctx, "message", fn)` unless they are purely read-only.

## Cache Keys

Cached query helpers produce deterministic namespaced keys. Query tokens are sorted before joining, so `[]string{"b", "a"}` and `[]string{"a", "b"}` share entries.

Current cached families include:

- `transactions:<sorted-query>`
- `balances:<depth>:<sorted-query>`
- `balancesvalued:<valueSpec>:<depth>:<sorted-query>`
- `aregister:<account>:<sorted-query>`
- `accounts`, `tags`, `payees`
- `networth:<begin>:<end>`
- `incomestmt:<begin>:<end>`
- `portfolio:<accounts>:<begin>` and `portfoliocost:<accounts>:<begin>`
- `cube:<config-hash>` — the encoded dashboard cube (see `handler_cube.go`)

Pagination (`limit`/`offset`) is applied after loading/caching the full hledger result.

## RPC Categories

**Cached queries:** `ListTransactions`, `GetBalances`, `GetAccountRegister`, `ListAccounts`, `ListTags`, `ListPayees`, `GetNetWorthTimeseries`, `GetIncomeStatementTimeseries`, portfolio timeseries helpers.

**Read/query helpers with limited or no cache:** `GetPortfolioHoldings` (uses balances plus `prices.journal`), `GetBalanceAssertionStatus`, `ListPrices`, `ListAccountDeclarations`, `ListSnapshots`, `GetSnapshotDiff`, `ListBankProfiles`, `GetBankProfileContent`, `PreviewImport`, `GetImportedTransactions`, `ListImports`, `GetImportFile`, `ListRules`, `PreviewApplyRules`, settings reads, `RunHledgerQuery`, `ExportJournal`, and `StreamLogs`.

**Mutations through txlock:** transaction add/update/delete/status/date/tag changes, bulk edit/delete, price add/delete/backfill, account declaration CRUD/rename, snapshot restore, bank profile CRUD, CSV import, rule CRUD/apply, template CRUD, Alpha Vantage/AI/Stripe/timezone settings, and Stripe link/unlink/import metadata writes.

**Streaming RPCs:** `ImportTransactions`, `BulkDeleteTransactions`, `ApplyRules`, `ImportStripeTransactions`, `ImportAllStripeTransactions`, `RefreshStripeAccount`, `RefreshAllStripeAccounts`, and `StreamLogs`.

## Feature Areas

### Transactions / Bulk Editing

Transaction writes convert proto input into `journal.TransactionInput`, call journal text helpers under txlock, then re-read via hledger before returning. Bulk edit supports mark reviewed/unreviewed, add/remove tag, set/clear payee, and delete. Bulk delete streams progress.

### Prices / Portfolio / Alpha Vantage

`ListPrices` reads `prices.journal`. `BackfillPrices` uses `internal/alphavantage`, skips already-present commodity/date prices, and writes new price directives inside txlock. Portfolio holdings aggregate raw commodity balances by account/symbol, use latest price directives for value/allocation, and derive cost basis from hledger cost annotations.

### Accounts and Assertions

Account declarations are managed in `accounts.journal`; `RenameAccount` updates both declarations and posting lines. `GetBalanceAssertionStatus` ranks asset/liability accounts by transactions since their last simple balance assertion.

### Imports and Bank Profiles

Bank profiles live in config and point to hledger CSV rules files under the data directory. CSV preview runs `hledger print --rules-file`, applies float categorization rules, marks duplicates by fingerprint, and returns import candidates. Imports write selected candidates, assign an import batch ID, archive the source CSV, and stream progress.

### Categorization Rules

Rules are stored in `rules.json` and handled through `internal/rules`. Add/update/delete mutate `rules.json` under txlock. Preview/apply can target rule IDs and optional hledger query tokens; apply streams progress.

### Stripe Financial Connections

Stripe handlers live in `stripe.go` and read credentials from environment variables (`STRIPE_SECRET_KEY`, `STRIPE_PUBLISHABLE_KEY`). Customer ID, linked accounts, daily auto-import toggle, and timestamps are stored in config.

Fetch RPCs only list already-available settled transactions and hide duplicates by hidden `float-stripe-txn` metadata. Refresh RPCs kick off/poll Stripe refreshes and stream progress/results, honoring Stripe's per-account throttle. Import RPCs re-read existing Stripe IDs inside txlock to prevent double writes during races with auto-import.

### Daily Stripe Auto-Import

`StartDailyStripeImport` runs in the server context. When enabled in config, it refreshes or lists accounts according to Stripe throttles, imports new settled transactions, and updates `LastDailyImportAt`.

### AI

AI handlers use `internal/ai` and OpenRouter via `OPENROUTER_API_KEY`. They can suggest rules for transactions, flag existing rules that are duplicates/contradictions/combinable (`FindRuleIssues`, read-only — it does not modify rules), translate natural language into hledger query tokens, and answer questions by running hledger and summarizing the output. Model/prompt config is persisted in `config.toml`.

### Settings / Logs / Debug

Settings handlers manage Alpha Vantage API key, AI model/prompt, Stripe customer/daily import config, and timezone. `RunHledgerQuery` runs a constrained raw hledger command with `-f <journal>` automatically prepended. `ExportJournal` runs `hledger print -f <journal>` and returns the flattened journal (all includes inlined) as bytes plus a suggested filename, for a one-file download of the entire ledger. `StreamLogs` streams broadcast slog entries filtered by minimum level.

## Plain HTTP endpoints

Not everything is an RPC. `handler_cube.go` serves the dashboard read model
(`internal/cube`) over plain HTTP, because the payload is bulk binary the client
decodes as zero-copy typed arrays rather than an API message.

- `GET /api/cube/{generation}.bin` — the encoded cube. The generation is in the
  path so the URL is content-addressed: the response is `immutable` with a
  one-year max-age, and a request for any generation other than the current one
  gets 409 plus the current generation rather than a stale payload. The
  generation is re-checked after the build in case a write landed while hledger
  was running.
- `GET /api/generation` — the current txlock generation, for the initial page
  load. Afterwards `middleware.NewGenerationInterceptor` stamps
  `X-Float-Generation` on every Connect response, so ordinary RPCs keep the
  client current with no polling.

**Both must be wrapped in `auth.RequireAuth` when registered.** The static web
assets are deliberately served without auth so the SPA shell can render the
login page; being served over HTTP by floatd is not by itself protection, and
the cube is the entire ledger.

The cube build runs hledger and takes seconds, so it must never be called from
inside `txlock.Do`. Writes bump the generation as usual and the next request
rebuilds lazily, with the cache's singleflight collapsing concurrent callers.
`WarmCube` pre-builds once at startup so a restart does not make the first
dashboard load pay for it.

## Adding a New RPC

1. Add the method/messages to `proto/float/v1/ledger.proto` and run `mise run proto-gen`.
2. Implement the method on `*Handler` in a focused file in this package.
3. For query RPCs, add deterministic cache keys and `cached*` helpers when useful.
4. For mutation RPCs, wrap writes in `lock.Do(ctx, "description", fn)` and re-check race-prone state inside the lock.
5. Update the web client and this documentation when adding user-visible behavior.
