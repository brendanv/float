# internal/hledger

Typed Go wrapper around the `hledger` CLI. All accounting is delegated here — other packages should call these methods rather than parsing journal files or reimplementing hledger reports.

## Key Types

- `Client` — wraps the hledger binary, the main journal path, and a command runner. `New(bin, journal)` resolves the binary and validates the pinned version (`1.99.6`). `NewWithRunner` injects a stub runner for tests.

## Concurrency

`Client.run` bounds concurrent hledger processes with a `golang.org/x/sync/semaphore.Weighted`, default 2 slots. Each invocation parses the whole journal, so unbounded concurrency thrashes memory/CPU on small hardware under bursty concurrent RPCs.

- `SetConcurrency(n)` — change the slot count (n<=0 is a no-op). `cmd/floatd` calls this once at startup from `config.Server.HledgerConcurrency`. `acquire` re-reads `c.sem` under `semMu` and returns the specific `*semaphore.Weighted` instance it acquired from, so a concurrent `SetConcurrency` swap mid-invocation can't cause `run` to release a slot on the wrong (new) semaphore.
- `WaitUncontended(ctx)` — block until the semaphore is momentarily idle, without taking a slot or joining its FIFO queue (polls `TryAcquire`+`Release` every 25ms). `internal/warm` calls this before starting each warm load, so a load only *starts* once nothing interactive is waiting; once started, the load itself calls `acquire` at normal priority, so an interactive request that joins it via singleflight is never stuck behind other queued low-priority work.
- `Invocations()` — cumulative count of hledger processes run by this client, used by `cmd/floatd` to log per-generation hledger invocation counts.
- `Transaction` — parsed hledger transaction. Derived fields include `FID` from the transaction code, `Payee`/`Note` split on the first `|`, and `FloatMeta` for hidden `float-*` tags.
- `Posting`, `Amount`, `CostJSON`, `BalanceAssertion` — typed forms of hledger JSON posting/amount data, including cost annotations and simple balance assertions.
- `CostBasis` — lot cost basis (`cbDate`, `cbLabel`, `cbCost`), exposed as `Amount.CostBasis` (JSON `acostbasis`, nil when the amount has no lot annotation). On acquires `cbDate` is nil because the date comes from the transaction; on disposals it holds the selector as written, or the full inferred basis for a lotful commodity.
- `LotRow` — one lot subaccount from `Lots()`: base `Account`, `LotName`, `Date`, `Label`, net `Units` (with `CostBasis`), `UnitCost`, and `TotalCost` amounts.
- `Posting.LotRole()` / `Posting.Generated()` — read the hidden `TagLotRole` (`_ptype`) and `TagGenerated` (`_generated-posting`) posting tags.
- `BalanceReport`, `BalanceRow` — `hledger bal -O json` output.
- `RegisterRow` — `hledger reg -O json`, one row per posting.
- `AregisterRow` — `hledger areg -O json`, one row per transaction touching the focused account, with change and running balance computed by hledger.
- `AccountNode` — account tree/flat node from `hledger accounts --types`.
- `BalanceSheetTimeseries` — monthly balance sheet/net worth data from `hledger bs`.
- `IncomeStatementTimeseries`, `ISSubreport`, `ISRow` — monthly income statement data from `hledger is`.
- `CheckError` — returned by `Check()` when `hledger check` exits non-zero; `.Output` contains stderr.

## Client Methods

| Method | hledger command | Notes |
|--------|----------------|-------|
| `Balances(ctx, depth, query...)` | `bal -O json` | depth=0 omits `--depth` |
| `BalancesValued(ctx, valueSpec, depth, query...)` | `bal --infer-market-prices --value=<spec> -O json` | market-valued balances, e.g. `now,USD` |
| `BalancesCost(ctx, depth, query...)` | `bal -B -O json` | cost-basis balances |
| `BalanceSheetTimeseries(ctx, begin, end)` | `bs --monthly --historical --layout=bare --infer-market-prices --value=end,USD -O json` | net worth timeseries |
| `IncomeStatementTimeseries(ctx, begin, end)` | `is --monthly --tree -O json` | revenue/expense monthly dashboard |
| `PortfolioTimeseries(ctx, accounts, begin)` | `bs ... --value=end,USD` | portfolio value for explicit holding accounts |
| `PortfolioCostBasisTimeseries(ctx, accounts, begin)` | `bs ... --cost` | portfolio cost-basis timeseries |
| `Register(ctx, query...)` | `reg -O json` | flat posting rows |
| `Lots(ctx, query...)` | `bal --lots -N -O json` + `bal --lots -B -N -O json` | two invocations; see Lots below |
| `Aregister(ctx, account, query...)` | `areg -O json` | account-focused transaction rows |
| `Accounts(ctx, tree)` | `accounts --types [--tree]` | tree=true populates children |
| `UnusedAccounts(ctx)` | `accounts --unused` | declared but unused accounts |
| `UndeclaredAccounts(ctx)` | `accounts --undeclared` | used but not declared accounts |
| `ClosedAccounts(ctx)` | `accounts tag:float-closed` | closed accounts, including sub-accounts inheriting the tag |
| `Tags(ctx)` | `tags` | excludes hledger-internal `_`-prefixed tags (`_ptype`, `_generated-posting` on lot postings) |
| `Payees(ctx)` | `payees desc:.*[|].*` | payees only for descriptions containing `|` |
| `Transactions(ctx, query...)` | `print -O json` | full transaction objects with source positions |
| `PrintCSV(ctx, csvFile, rulesFile)` | `print -O json --rules-file <rules> -f <csv>` | CSV import preview; no journal file is written |
| `ParseJournalFile(ctx, file)` | `print --infer-costs -I -O json -f <file>` | parse/validate a standalone draft journal (structured import); inferred amounts filled in |
| `PrintJournal(ctx)` | `print -f <journal>` | flattened plain-text journal with all includes inlined, for single-file export |
| `PrintText(ctx, journalFile)` | `print -f <temp> -I` | canonical text formatting; ignores assertions until full `hledger check` |
| `RunQuery(ctx, argsStr)` | `-f <journal> <shell-like args>` | debug/query UI; restricted to read-only subcommands, rejects `-f`/`-o`/`--rules-file` (`ErrUnsafeQuery`) |
| `RunRaw(ctx, args...)` | arbitrary | escape hatch for CLI/debug only |
| `Check(ctx)` | `check -f <journal>` | validation gate for txlock |
| `Version(ctx)` | `--version` | returns parsed version |

## Parsing / Semantics

Parsing helpers translate hledger's heterogeneous JSON arrays into typed structs. They also enrich transactions with FID/payee/note/hidden-meta fields. Cost annotations are kept as raw JSON on `Amount` and parsed lazily by `Amount.ParseCost()`.

`RunQuery` uses a deliberately small shell splitter that supports single and double quotes but not shell expansion or escapes. It automatically prepends `-f <journal>`. The first token must be an allowlisted read-only subcommand, and arguments that redirect hledger input/output (`-f`, `-o`, `--file`, `--output-file`, `--rules-file`) are rejected with `ErrUnsafeQuery`.

All typed query methods (`Balances`, `Register`, `Transactions`, `Aregister`, ...) reject query terms starting with `-` with `ErrUnsafeQuery` — hledger parses flags anywhere in argv, so a flag-shaped query token could otherwise redirect output and bypass txlock.

## Lots

`Lots` joins the unit report and the `-B` cost report by full account name, keeping only rows whose account passes `parseLotAccount` (a final `{DATE[, "LABEL"][, COST]}` segment). Date and label come from the account name, because `cbDate`/`cbLabel` in `bal --lots` amounts are null/unreliable; the cost text in the name is ignored and the structured `cbCost` is used for `UnitCost`. The JSON does not merge acquire and disposal amounts within a lot row, so net units are summed per commodity with exact mantissa arithmetic (`sumAmounts`), never floats. Lots with zero net units are skipped.

Hidden posting tags on `print -O json`:

- `_ptype` — `acquire`, `dispose`, `transfer-from`, `transfer-to`, or `gain`.
- `_generated-posting` — marks an inferred capital-gain posting that `print` shows as a real posting but is not in the journal text. Writers must skip these so hledger re-infers them.

`holdings` is in the `RunQuery` allowlist for the debug UI. Do not use `holdings -O json` in typed code: its amounts are display strings (e.g. `"505,00 EUR"`), and parsing them would reimplement hledger's amount parsing.

Limitations:

- With the AVERAGE method, `bal --lots` shows each lot's original cost rather than the pool average.
- hledger query terms are regexes, so a lot account name containing `{` can't be used directly as a query term.
- hledger applies `cur:` after `-B` cost conversion, so a commodity query (e.g. `cur:AAPL`) leaves `TotalCost` empty. The web UI only sends `acct:` filters.
- Total-basis `{{...}}` is not modelled.

## Testing

Integration tests in `hledger_test.go` run the real hledger binary against fixture files in `testdata/`. Use `NewWithRunner` for unit tests of callers that should not execute hledger.

Fixtures (also used by `internal/journal`, `internal/server/ledger` and `cmd/floatctl` tests — update their expectations when changing one):

- `simple.journal`, `empty.journal`, `hiddenmeta.journal`, `closed.journal`, `assertions.journal` — basic, hidden-meta, closed-account and assertion-variant cases.
- `investments.journal` — `@` costs plus P directives for valued/cost balances and portfolio timeseries.
- `lots.journal` — one block per lot behaviour: specific-ID, labelled, label-only and selectorless disposals, a FIFO disposal spanning lots (empty `{}` basis, fully-consumed lot), a lot transfer between accounts, fractional units, gains and losses.
- `includes/` — multi-file layout like a real data dir (`main.journal` of includes, typed account declarations, prices, month files).
- `invalid.journal`, `badassertion.journal`, `badlot.journal` — must fail `Check()`.
- `import.csv` / `import_edge.csv` + `import.rules` — CSV import, including quoted commas, thousands separators and unmatched rows.

## Constants / Tags

- `supportedVersion = "1.99.6"` — startup fails if the installed hledger differs.
- `FIDLen = 8` — length of a float transaction code.
- `HiddenMetaPrefix = "float-"` — internal metadata tag prefix filtered from user-facing APIs.
- `ClosedAccountTag = "float-closed"` — account-directive tag marking an account closed. hledger inherits account tags to sub-accounts, so `tag:float-closed` / `not:tag:float-closed` match a closed account and everything beneath it. It does not affect accounting.
- `AccountType*` constants mirror hledger account type letters (`A`, `L`, `E`, `R`, `X`, `C`, `V`).
