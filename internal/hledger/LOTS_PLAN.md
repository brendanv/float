# Plan: Basic lot support

## Context

float now pins hledger 1.99.5 (hledger 2 preview), whose headline feature is lot tracking: cost-basis annotations (`10 AAPL {$50}`, `-5 AAPL {2026-01-15, "label", $50} @ $70`), lotful commodities (`commodity AAPL ; lots:`), inferred lot subaccounts, and inferred capital-gain postings. float can't show any of this yet. Worse, the round-trip write paths would corrupt lot transactions. Goal: read journals that use lots, expose lot info over gRPC, and keep float's own writes from breaking lot entries (Phase 1, backend). Then add basic lot editing to the web UI's transaction add/edit flows and a read-only Lots page (Phase 2). Portfolio and other complex pages are not part of this plan.

## What hledger 1.99.5 emits (verified in a scratch journal)

- **Amount JSON** has a new `acostbasis` field: `{"cbDate": "YYYY-MM-DD"|null, "cbLabel": string|null, "cbCost": Amount|null}`, or null. On acquisitions `cbDate` is null because the date comes from the transaction. On disposals it holds the selector as written, or the full inferred basis when the commodity is lotful. Auto labels (`"0001"`) do **not** appear in `print` JSON.
- **Posting `ptags`** gain hidden tags. `_ptype` is one of `acquire|dispose|transfer-from|transfer-to|gain`. `_generated-posting` marks an inferred gain posting, which `print` includes as a real posting (e.g. `revenues:gain $-100`). A commodity's `lots:` tag is also inherited onto its postings.
- **Default reports hide lots.** `print`, `bal` and `accounts` show base accounts only. With `--lots`, `bal` shows one row per lot subaccount named `base:{DATE[, "LABEL"][, COST]}`, with structured units and `acostbasis.cbCost`. In these rows `cbDate`/`cbLabel` are null, so the date and label exist only in the account name. `bal --lots -B` gives each lot's total cost.
- **`holdings -O json`** exists but formats amounts as display strings (`"505,00 EUR"`). Don't use it, because parsing those strings would reimplement hledger's amount parsing.
- **`print -I`** (used by `FormatViaHledger`/`PrintText`) keeps `{...}` selectors verbatim, and `txlock`'s `hledger check` validates lots.

### Write-path bug this must fix
`journal.InputFromTransaction` → `postingsFromTransaction`, and `HledgerTxnToInput`, rebuild postings from JSON. They drop `acostbasis` and keep generated gain postings. Example: a `ModifyTags` on `-5 AAPL {2026-02-01} @ $70` rewrites it as `-5 AAPL @ $70`, so FIFO now picks a different lot. It also writes the old inferred gain `$-50` as an explicit posting, so `hledger check` fails ("realised gain amount is wrong"). The result is that tag, status, date, rules-apply and bulk edits all fail on lot disposals. Callers: `internal/journal/{modifytags,status,updatedate,update,migrate}.go`, `internal/rules/apply.go`, `internal/server/ledger/handler.go` (bulk edit ~L2131, import ~L2758).

## Phase 1: Backend (done)

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

## Phase 2: Web UI

Today a full edit from the UI drops `{...}` selectors and writes the generated gain posting back explicitly, so editing any lot disposal fails `hledger check` or picks a different lot. Phase 2 fixes this and adds a read-only view of open lots.

### 7. Lot fields in transaction editing (`web/src/components/posting-fields.jsx`, `transaction-table.jsx`, `add-transaction-modal.jsx`)
- **Shared posting↔form mapping.** Add `fieldsFromPosting(p)` to `posting-fields.jsx`. It reads `amounts[0]` (account, commodity, quantity, `cost`, `costBasis` → `lot: {date, label, costQuantity, costCommodity}`) and `balanceAssertion`. Add `editablePostings(postings)`, which filters out `p.generated`. Replace the four hand-rolled copies with these: `EditableDetailRow.toFields`, `EditableDescriptionCell` (it currently also drops balance assertions), `EditableAccountSideCell`, and `EditableOtherAccountCell`.
- **`toPostingInput`.** When `p.lot` has any non-empty part, emit `costBasis: {date, label, cost?: {commodity, quantity}}`. Omit `cost` when there is no quantity, and omit `costBasis` entirely when every part is empty. This follows the existing `cost` trimming pattern.
- **`LotFields` row.** Mirror `CostFields`/`AssertionFields`: a `{ }` chip, a date `Input type="date"`, a label text input, a cost quantity input, a cost commodity input, and a remove button. Add a toolbar toggle button (lucide `Layers`, tooltip "Add lot / cost basis") next to the price and assertion buttons. The row is pre-shown when the posting already has a `costBasis`. Tooltip/help text: on a sale, date or label selects the lot; on a buy, cost sets the basis.
- **Generated gain postings.** They are left out of the editable list, so hledger re-infers them on save. Under the postings in `EditableDetailRow`, render them read-only as muted text, e.g. `revenues:gain  -100 USD · inferred by hledger`. This needs no form state.
- **Inline cells.** In `EditableAccountSideCell`, compute `matching` from non-generated postings only. Otherwise a gain posting could be the "single" from/to side, and its rename would be silently discarded.
- **Duplicate.** `AddTransactionModal`'s `defaultPostings` keeps `lot` (it already keeps `cost`). Duplicating a disposal keeps its selector, and the generated posting is already gone because the input comes from `fieldsFromPosting`/`editablePostings`.
- **Display.** Add `formatCostBasis(cb)` to `web/src/format.js`. It renders ` {2026-01-15, "label", 50 USD}` with only the parts present. Append it in `formatAmounts` before `formatCost`, so table amounts show lots.
- Accepted consequence (as in section 5): for a lotful commodity, saving a disposal from the UI pins the selector that hledger inferred, because it arrives pre-filled.

### 8. Lots page (`web/src/pages/lots.jsx`)
- Route `/lots` in `router.jsx` (lazy `LotsPage`). Add a nav item in `app-shell.jsx` next to Portfolio (icon `Layers`). Add `queryKeys.lots(query)` → `["lots", query]`.
- `ledgerClient.listLots({ query })`. `ListLots` already drops zero-unit lots, so this lists only open (non-sold) lots. Add an optional account filter with the existing `FilterInput`, sent as a query term. Follow the structure of `prices.jsx`: `Page`/`PageCard`/`PageHeader`, `Loading`, `ErrorBanner`, `EmptyState`, and a shadcn `Table` with `TableSortHeader`.
- Columns: Account, Commodity, Acquired (`formatDate`), Label, Units, Unit cost, Total cost (`formatAmounts`). The default sort is account, then date. Clicking an account goes to `/transactions?account=…`. On mobile, show a stacked card list in the same way existing pages handle narrow widths.
- Cache freshness: `staleTime` is 30s, so invalidate `["lots"]` in `AddTransactionForm` (alongside `["balances"]`) and in the `onSaved`/`onDeleted` refresh path used by the transactions table.
- Update `web/CLAUDE.md` (routes table).

### 9. Mocks and screenshots
- `web/tests/mock-api.js`: add a `ListLots` case with 3–4 lots (one labelled, two commodities). Add mock transactions for an acquire with `costBasis.cost`, and for a disposal with a `costBasis.date` selector, `lotRole: "dispose"` and a `generated: true` gain posting.
- `web/tests/new-features-screenshots.spec.js`: add the lots page (desktop and mobile), plus an expanded disposal transaction showing lot fields and the inferred-gain line.

## Out of scope (follow-ups)
- Portfolio page lot awareness, lot-picker UX (e.g. choosing from open lots when selling), and editing generated gain postings.
- Switching `GetPortfolioHoldings` to lot-aware cost basis, market value per lot (`--value`), unrealised/realised gain, and XIRR.

## Verification

### Phase 1
- New fixture `internal/hledger/testdata/lots.journal` covering: a lotful commodity, an explicit `{$60}` acquire, a SPECID disposal `{2026-02-01}` with an inferred gain, a labelled lot `{2026-01-16, "mine", $40}`, a `$1,234.567` cost, and a `P` price.
- `hledger_test.go` (real hledger): `Transactions` parses `CostBasis`, `LotRole` and `Generated`. `Lots` returns the expected base account, date, label, units, unit cost and total cost. Add a table-driven test for `parseLotAccount`, including a label and a cost containing a comma.
- `journal` tests: run `ModifyTags`, `UpdateTransactionStatus` and `UpdateTransactionDate` on the SPECID disposal. Assert that `hledger check` passes, the `{2026-02-01}` selector is still in the file, and no explicit gain posting was written. Add a `postingAmountString` table test for the cost-basis rendering.
- Handler test: `ListLots` and the `ListTransactions` proto fields work against the fixture.
- `mise run check-all`. Then run a manual smoke test with `mise run floatd` on a data dir that includes the fixture, using `buf curl ... /float.v1.LedgerService/ListLots --data '{}'` and `ListTransactions`.

### Phase 2
- `cd web && bun run build` and `bun run screenshots`, then review the new screenshots.
- Manual check with `mise run floatd` + `mise run web-dev` on a data dir that includes `internal/hledger/testdata/lots.journal`:
  - Edit the SPECID disposal's account or tags from the expanded row and save. `hledger check` passes, `{2026-02-01}` is still in the file, and there is no explicit gain posting.
  - Add a new buy with a lot cost `{$60}`, and a sale with a date selector. Both appear correctly on `/lots`, and sold-out lots disappear.
  - Inline from/to edits on a disposal row don't target the gain posting.
