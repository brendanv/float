# internal/hledger

Typed Go wrapper around the `hledger` CLI. All accounting is delegated here — never reimplement hledger logic in other packages.

## Key Types

- `Client` — wraps the hledger binary and a journal file path. Constructed with `New(bin, journal)` (validates binary exists and version matches `1.52`) or `NewWithRunner(bin, journal, runner)` for testing with a stub.
- `Transaction` — parsed hledger transaction with `FID` (code field), `Payee`/`Note` (split from description on `|`), and `FloatMeta` (hidden `float-*` tags) as derived fields.
- `BalanceReport`, `BalanceRow` — result of `hledger bal -O json`.
- `RegisterRow` — one posting row from `hledger reg -O json`.
- `AregisterRow` — one transaction row from `hledger areg -O json` with `Change` and `Balance`.
- `AccountNode` — account tree node; `Children` populated only when `Accounts(tree=true)`.
- `BalanceSheetTimeseries` — monthly net worth data from `hledger bs`.
- `CheckError` — returned by `Check()` when `hledger check` exits non-zero; `.Output` contains stderr.

## Client Methods

| Method | hledger command | Notes |
|--------|----------------|-------|
| `Balances(ctx, depth, query...)` | `hledger bal -O json` | depth=0 omits `--depth` |
| `BalanceSheetTimeseries(ctx, begin, end)` | `hledger bs --monthly --historical -O json` | for net worth chart |
| `Register(ctx, query...)` | `hledger reg -O json` | one row per posting |
| `Aregister(ctx, account, query...)` | `hledger areg -O json` | one row per transaction |
| `Accounts(ctx, tree)` | `hledger accounts --types` | tree=true populates Children |
| `UnusedAccounts(ctx)` | `hledger accounts --unused` | declared but no postings |
| `UndeclaredAccounts(ctx)` | `hledger accounts --undeclared` | used but not declared |
| `Tags(ctx)` | `hledger tags` | excludes internal `fid` tag |
| `Payees(ctx)` | `hledger payees` | unique payee list |
| `Transactions(ctx, query...)` | `hledger print -O json` | full transaction objects |
| `PrintCSV(ctx, csvFile, rulesFile)` | `hledger print -O json --rules-file` | import preview; no journal needed |
| `PrintText(ctx, journalFile)` | `hledger print -f` | canonicalize transaction text |
| `Check(ctx)` | `hledger check` | nil on success, `*CheckError` on failure |
| `Version(ctx)` | `hledger --version` | returns version string |
| `RunRaw(ctx, args...)` | arbitrary | escape hatch for debugging only |

## Testing

Integration tests in `hledger_test.go` run the real hledger binary against fixture files in `testdata/`. Use `NewWithRunner` to inject a stub runner when testing callers of this package.

## Constants / Tags

- `FIDLen = 8` — length of a float transaction ID
- `HiddenMetaPrefix = "float-"` — prefix for internal metadata tags (filtered from gRPC API)
- `AccountType*` constants — hledger account type letters (A, L, E, R, X, C, V)
