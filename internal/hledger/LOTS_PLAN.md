# Plan: Basic lot support (backend only)

## Context

float now pins hledger 1.99.5 (hledger 2 preview), whose headline feature is lot tracking: cost-basis annotations (`10 AAPL {$50}`, `-5 AAPL {2026-01-15, "label", $50} @ $70`), lotful commodities (`commodity AAPL ; lots:`), inferred lot subaccounts, and inferred capital-gain postings. float can't show any of this yet. Worse, the round-trip write paths would corrupt lot transactions. Goal: read journals that use lots, expose lot info over gRPC, and keep float's own writes from breaking lot entries. No frontend work.

## What hledger 1.99.5 emits (verified in a scratch journal)

- **Amount JSON** has a new `acostbasis` field: `{"cbDate": "YYYY-MM-DD"|null, "cbLabel": string|null, "cbCost": Amount|null}`, or null. On acquisitions `cbDate` is null because the date comes from the transaction. On disposals it holds the selector as written, or the full inferred basis when the commodity is lotful. Auto labels (`"0001"`) do **not** appear in `print` JSON.
- **Posting `ptags`** gain hidden tags. `_ptype` is one of `acquire|dispose|transfer-from|transfer-to|gain`. `_generated-posting` marks an inferred gain posting, which `print` includes as a real posting (e.g. `revenues:gain $-100`). A commodity's `lots:` tag is also inherited onto its postings.
- **Default reports hide lots.** `print`, `bal` and `accounts` show base accounts only. With `--lots`, `bal` shows one row per lot subaccount named `base:{DATE[, "LABEL"][, COST]}`, with structured units and `acostbasis.cbCost`. In these rows `cbDate`/`cbLabel` are null, so the date and label exist only in the account name. `bal --lots -B` gives each lot's total cost.
- **`holdings -O json`** exists but formats amounts as display strings (`"505,00 EUR"`). Don't use it, because parsing those strings would reimplement hledger's amount parsing.
- **`print -I`** (used by `FormatViaHledger`/`PrintText`) keeps `{...}` selectors verbatim, and `txlock`'s `hledger check` validates lots.

### Write-path bug this must fix
`journal.InputFromTransaction` → `postingsFromTransaction`, and `HledgerTxnToInput`, rebuild postings from JSON. They drop `acostbasis` and keep generated gain postings. Example: a `ModifyTags` on `-5 AAPL {2026-02-01} @ $70` rewrites it as `-5 AAPL @ $70`, so FIFO now picks a different lot. It also writes the old inferred gain `$-50` as an explicit posting, so `hledger check` fails ("realised gain amount is wrong"). The result is that tag, status, date, rules-apply and bulk edits all fail on lot disposals. Callers: `internal/journal/{modifytags,status,updatedate,update,migrate}.go`, `internal/rules/apply.go`, `internal/server/ledger/handler.go` (bulk edit ~L2131, import ~L2758).

## Changes

### 1. `internal/hledger` — types and parsing
- `types.go`: add `CostBasis{Date *string "cbDate"; Label *string "cbLabel"; Cost *Amount "cbCost"}` and `Amount.CostBasis *CostBasis \`json:"acostbasis"\``.
- `types.go`: add `Posting` helpers `LotRole() string` (the `_ptype` value, "" if none) and `Generated() bool` (whether `_generated-posting` is present). Use constants for the tag names.
- `types.go`: add `LotRow{Account, LotName string; Date, Label string; Units Amount; UnitCost *Amount; TotalCost []Amount}`.
- `parse.go`: add `parseLotAccount(full string) (base, date, label string, ok bool)`. It splits off a final `{...}` segment and reads only the leading `YYYY-MM-DD` and an optional `"quoted label"`. Ignore the cost text in the name, because it can contain `,` and display formatting; the structured `cbCost` is used instead.

### 2. `internal/hledger/client.go` — `Lots(ctx, query...)`
- Run `bal --lots -N -O json -f <journal> [query]` and `bal --lots -B -N -O json -f <journal> [query]`. Reuse `parseBalanceReport` for both, keep only rows where `parseLotAccount` succeeds, and join them by full account name into `[]LotRow`. `UnitCost` comes from `Units.CostBasis.Cost`; `TotalCost` comes from the `-B` row.
- Apply `validateQueryTerms` as the other typed methods do.
- Add `holdings` to `runQueryAllowedCommands`, since it is read-only and useful from the debug query UI.
- Known limitation, noted in docs: with the `AVERAGE` method, `bal --lots` shows each lot's original cost rather than the pool average.

### 3. `proto/float/v1/ledger.proto` (then `mise run proto-gen`)
- `message CostBasis { string date = 1; string label = 2; Amount cost = 3; }`. Empty strings mean unspecified or inferred.
- `Amount`: add `CostBasis cost_basis = 4;`.
- `Posting`: add `string lot_role = 5;` (acquire/dispose/transfer-from/transfer-to/gain, or "") and `bool generated = 6;` (an inferred gain posting that is not in the journal text).
- `PostingInput`: add `CostBasis cost_basis = 7;` so API writers can send lot selectors. No UI uses it yet.
- `message Lot { string account = 1; string lot_name = 2; string date = 3; string label = 4; Amount units = 5; Amount unit_cost = 6; repeated Amount total_cost = 7; }`.
- `ListLotsRequest { repeated string query = 1; }`, `ListLotsResponse { repeated Lot lots = 1; }`, and `rpc ListLots`.

### 4. `internal/server/ledger/handler.go`
- `toProtoAmount`: map `CostBasis`. `toProtoPosting`: set `LotRole`/`Generated`.
- `ListLots`: add a cached `cachedGet(ctx, h.cache, "lots:<sorted-query>", ...)` key, following the existing `balances:` key helpers, and convert with `toProtoLot`. Don't add it to `WarmEntries`.
- `protoToJournalCost`-style mapping from proto `CostBasis` to `journal.CostBasisInput` for Add/UpdateTransaction.
- Update `internal/server/ledger/CLAUDE.md` (cache key list, RPC list).

### 5. `internal/journal` — keep lot entries intact when round-tripping
- `format.go`: add `CostBasisInput{Date, Label, Commodity, Quantity string}` and `PostingInput.CostBasis *CostBasisInput`. In `postingAmountString`, render ` {DATE, "LABEL", COST}` with only the parts that are present, placed before the `@` cost.
- `write.go` `postingsFromTransaction` and `convert.go` `HledgerTxnToInput`: copy `acostbasis` into `CostBasis`, and **skip postings where `p.Generated()`** so hledger re-infers the gain.
- Accepted consequence: for a lotful commodity, a round-tripped disposal is written with the selector hledger inferred (e.g. `{2026-01-15, $50}`), which pins that lot. The manual itself recommends making past disposals explicit this way. Note this in `internal/journal/CLAUDE.md`.

### 6. Docs
Update `internal/hledger/CLAUDE.md`: new types, the `Lots` row in the method table, `acostbasis`, the `_ptype`/`_generated-posting` hidden tags, and the AVERAGE limitation.

## Out of scope (follow-ups)
- Frontend display/editing. Until the UI ignores `generated` postings and sends `cost_basis`, a full edit from the UI pins the gain and drops the selectors. `hledger check` will catch the inconsistencies.
- Switching `GetPortfolioHoldings` to lot-aware cost basis, market value per lot (`--value`), unrealised/realised gain, and XIRR.

## Verification
- New fixture `internal/hledger/testdata/lots.journal` covering: a lotful commodity, an explicit `{$60}` acquire, a SPECID disposal `{2026-02-01}` with an inferred gain, a labelled lot `{2026-01-16, "mine", $40}`, a `$1,234.567` cost, and a `P` price.
- `hledger_test.go` (real hledger): `Transactions` parses `CostBasis`, `LotRole` and `Generated`. `Lots` returns the expected base account, date, label, units, unit cost and total cost. Add a table-driven test for `parseLotAccount`, including a label and a cost containing a comma.
- `journal` tests: run `ModifyTags`, `UpdateTransactionStatus` and `UpdateTransactionDate` on the SPECID disposal. Assert that `hledger check` passes, the `{2026-02-01}` selector is still in the file, and no explicit gain posting was written. Add a `postingAmountString` table test for the cost-basis rendering.
- Handler test: `ListLots` and the `ListTransactions` proto fields work against the fixture.
- `mise run check-all`. Then run a manual smoke test with `mise run floatd` on a data dir that includes the fixture, using `buf curl ... /float.v1.LedgerService/ListLots --data '{}'` and `ListTransactions`.
