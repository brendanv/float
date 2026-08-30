# internal/hledger

Typed Go wrapper around the `hledger` CLI. All accounting is delegated here — other packages should call these methods rather than parsing journal files or reimplementing hledger reports.

## Key Types

- `Client` — wraps the hledger binary, the main journal path, and a command runner. `New(bin, journal)` resolves the binary and validates the pinned version (`1.52`). `NewWithRunner` injects a stub runner for tests.

## Concurrency

`Client.run` bounds concurrent hledger processes with a `golang.org/x/sync/semaphore.Weighted`, default 2 slots. Each invocation parses the whole journal, so unbounded concurrency thrashes memory/CPU on small hardware under bursty concurrent RPCs.

- `SetConcurrency(n)` — change the slot count (n<=0 is a no-op). `cmd/floatd` calls this once at startup from `config.Server.HledgerConcurrency`.
- `WithLowPriority(ctx)` — mark a caller's context as low-priority. Normal calls block on the semaphore in FIFO order; low-priority calls (`internal/warm`'s background warming passes) instead poll with `TryAcquire` every 25ms, so they never queue ahead of an interactive request that starts waiting after them.
- `Invocations()` — cumulative count of hledger processes run by this client, used by `cmd/floatd` to log per-generation hledger invocation counts.
- `Transaction` — parsed hledger transaction. Derived fields include `FID` from the transaction code, `Payee`/`Note` split on the first `|`, and `FloatMeta` for hidden `float-*` tags.
- `Posting`, `Amount`, `CostJSON`, `BalanceAssertion` — typed forms of hledger JSON posting/amount data, including cost annotations and simple balance assertions.
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
| `Aregister(ctx, account, query...)` | `areg -O json` | account-focused transaction rows |
| `Accounts(ctx, tree)` | `accounts --types [--tree]` | tree=true populates children |
| `UnusedAccounts(ctx)` | `accounts --unused` | declared but unused accounts |
| `UndeclaredAccounts(ctx)` | `accounts --undeclared` | used but not declared accounts |
| `Tags(ctx)` | `tags` | excludes the legacy/internal `fid` tag |
| `Payees(ctx)` | `payees desc:.*[|].*` | payees only for descriptions containing `|` |
| `Transactions(ctx, query...)` | `print -O json` | full transaction objects with source positions |
| `PrintCSV(ctx, csvFile, rulesFile)` | `print -O json --rules-file <rules> -f <csv>` | CSV import preview; no journal file is written |
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

## Testing

Integration tests in `hledger_test.go` run the real hledger binary against fixture files in `testdata/`. Use `NewWithRunner` for unit tests of callers that should not execute hledger.

## Constants / Tags

- `supportedVersion = "1.52"` — startup fails if the installed hledger differs.
- `FIDLen = 8` — length of a float transaction code.
- `HiddenMetaPrefix = "float-"` — internal metadata tag prefix filtered from user-facing APIs.
- `AccountType*` constants mirror hledger account type letters (`A`, `L`, `E`, `R`, `X`, `C`, `V`).
