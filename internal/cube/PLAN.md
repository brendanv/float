# Plan: internal/cube — Precomputed Read Model for Responsive Dashboards

## Status

Phases 1-4 are implemented. Phase 5 (new dashboard types) is not, and remains a
list of things the cube makes possible rather than committed work.

Six things changed once the code met real hledger output. Each is a measurement
or a correctness finding, not a preference:

1. **`bal`, not `bs`, for period balances.** `bs` applies the balance-sheet
   display convention that renders liabilities positive, which would have to be
   undone per subreport to recover signed balances. `bal --historical type:AL`
   reports raw signed balances, emits month labels directly, and is faster and
   ~65x smaller.
2. **`print -O csv`, not `reg -O csv`, for the fact table.** It splits amount
   and commodity into separate columns instead of emitting `"35.21 USD"`.
3. **The transaction-code dimension is dropped.** It cost 265KB of a 354KB
   gzipped payload — 35% — for a per-transaction lookup the aggregate surface
   never performs. Drilldown builds a query from the cell's date/account/payee
   coordinates instead. Final payload: 253KB gzipped for 48,000 postings.
4. **An account-type dimension was added.** Not in the original plan, but
   without it the migration would silently redefine `type:X` as "accounts named
   expenses". Implementing it surfaced a second trap: hledger types a
   plainly-named `assets:checking` as `C` (Cash), and `type:A` matches `C`
   because Cash is a subtype of Asset. Exact matching would have reported zero
   assets on an ordinary ledger.
5. **IndexedDB persistence is skipped.** `Cache-Control: immutable` on a
   content-addressed URL already makes a repeat cold start a browser disk-cache
   hit; a second copy in IndexedDB would duplicate that for no gain.
6. **The build is ~3.5s, not ~10s.** The source reports are independent hledger
   processes and run concurrently, so a build costs the slowest of them.

Measured on the live server against the 24k-transaction journal: cube built in
3.5s at startup, served in 1.4ms, 249KB gzipped. The Trends page issues two
requests on load (`/api/generation`, `/api/cube/{gen}.bin`) and **zero** on any
subsequent range change; twenty range switches made no network requests at all.
Its net worth and expense-category totals match hledger to the cent.

---

## Context

Every float dashboard query costs one hledger process invocation. hledger's cost is
dominated by journal parsing, not by the query, so **every** call pays the same floor.
Measured on a 24k-transaction / 96k-line / 2.4 MB journal (~10 years of realistic
personal finance — see [Appendix A](#appendix-a--measurements) to reproduce):

| Call | Time | Output |
|---|---|---|
| `bal type:X --depth 2 -O json` (trivial query) | **2366 ms** | 5 KB |
| `bs --monthly --historical --layout=bare -O json` | **3295 ms** | 739 KB |
| `is --monthly --tree -O json` | **3770 ms** | 2.6 MB |
| `print -O json` | 9404 ms | 56 MB |

Consequences today:

- `TrendsPage` (`web/src/pages/trends.jsx:224-242`) fires three such calls on mount.
- `MonthlyDashboardPage` pays ~3.8 s on **every** date-range change.
- `internal/cache` hides this only within a generation. `txlock.DoWith`
  (`internal/txlock/txlock.go:94`) bumps the generation on every successful write, which
  drops the whole cache — so adding one transaction makes every dashboard cold again.
  A server restart does the same.

The fix is a **derived read model**: precompute once per generation, ship it whole to the
browser, and answer every filter/drilldown interaction client-side with zero round trips.

### Why not parquet + hyparquet

This plan is inspired by [Fast drilldown dashboards with R2 and
hyparquet](https://www.hamiltonulmer.com/customer-dashboards-r2-hyparquet/), but adopts
only its *idea*, not its machinery. That article needs `GROUP BY GROUPING SETS`, parquet
row groups, and HTTP range requests because it serves billions of rows to thousands of
tenants from object storage with no backend. float has tens of thousands of postings, one
user, and a server. Measured encoded sizes on the same fixture:

| Candidate cube | rows | raw | gzipped |
|---|---:|---:|---:|
| month x account -> sum, count | 2,926 | 53 KB | 15 KB |
| month x account x payee (income/expense) | 23,686 | 516 KB | 114 KB |
| **every posting** (date, account, payee, amount) | 48,000 | 849 KB | **253 KB** |

The full posting table is only ~2x the size of a single pre-aggregated grouping set, and
**every grouping set is derivable from it client-side**. Client-side full-scan group-by
over 48,000 rows in a `Float64Array`: **0.137 ms**.

So: ship ~253 KB once, then answer interactions in ~0.1 ms instead of 2400 ms.

Revisit parquet/hyparquet only if the encoded cube exceeds ~5 MB (multi-entity ledgers,
per-lot investment journals, 30-year histories) or if we want DuckDB-WASM for ad-hoc SQL
on the `HledgerQuery` page. Note the size asymmetry: hyparquet is 18 KB, DuckDB-WASM is
~3 MB. Range requests buy nothing when floatd serves the bytes over localhost or LAN.

---

## Design Rules

### Rule 1: Flows sum over time. Stocks do not.

This is the load-bearing constraint of the whole design.

- **Flow measures** (expense/income sums, transaction counts) are distributive. The client
  may slice and roll them up freely along both the date and account-hierarchy axes.
- **Stock measures** (market-valued balances, cost basis) are **not** derivable from a sum
  of posting deltas. `--infer-market-prices --value=end,USD` depends on the price series;
  `--cost` depends on hledger's lot matching. These must be materialized per period-end.

Rolling a stock measure *up the account tree at a fixed date* is legal. Summing one
*across dates* is not, and doing so silently produces wrong net-worth numbers. The binary
format encodes this per-column (`"summable": "both" | "account-only"`) and the client query
API refuses the illegal reduction.

### Rule 2: hledger stays the accounting engine.

The cube is built by aggregating hledger's own CSV output. hledger still does all parsing,
valuation, lot matching, and account-type classification. We only sum numbers it already
computed. This respects the top-level constraint in `CLAUDE.md`.

### Rule 3: The cube is a cache, never a source of truth.

It must always be safe to delete and rebuild. No mutation path ever reads it. If the cube
and hledger disagree, hledger is right and the cube is a bug.

---

## File Structure

```
internal/cube/
├── PLAN.md            # this document
├── CLAUDE.md          # package guidance (written in Phase 1)
├── cube.go            # Cube type, Build(), measure semantics
├── build_flows.go     # `hledger print -O csv` -> posting fact table
├── build_stocks.go    # `hledger bs --monthly ...` -> valued/cost tables
├── encode.go          # Cube -> binary wire format
├── dict.go            # string dictionary + account hierarchy metadata
├── cube_test.go       # integration tests vs real hledger (package cube_test)
└── testdata/
    └── multi.journal  # multi-commodity + priced fixture

internal/server/
└── cube_http.go       # GET /api/cube/{gen}.bin, behind auth

web/src/
├── lib/cube.js        # decode + client query engine
├── lib/cube-query.js  # flows()/balance() reducers
└── hooks/use-cube.js  # React Query integration
```

---

## Phase 1 — `internal/cube` builder

### 1.1 Source commands

Use `-O csv`, **not** `-O json`. Measured: `print -O csv` is 3761 ms / 5.1 MB versus
`print -O json` at 9404 ms / 56 MB. JSON serialization dominates hledger's cost on large
outputs; CSV is a ~2.5x build-time win and ~11x less to parse.

```go
// Fact table (flows). Columns:
//   txnidx,date,date2,status,code,description,comment,
//   account,amount,commodity,credit,debit,posting-status,posting-comment
hledger print -O csv -f <journal>

// Stock measures, one column per period end:
//   row 1: report title (ignored)
//   row 2: "Account","Commodity",<period-end dates...>
//   rows: <account>,<commodity>,<value per period...>
//   section rows ("Assets"/"Liabilities") have empty commodity and empty values
hledger bs --monthly --historical --layout=bare --infer-market-prices --value=end,USD -O csv
hledger bs --monthly --historical --layout=bare --cost -O csv
```

`print -O csv` is preferred over `reg -O csv` because it splits `amount` and `commodity`
into separate columns (no `"35.21 USD"` string parsing) and carries `code`, which float
uses for its 8-character transaction code — needed for the drilldown escape hatch.

Total build: ~10 s on the fixture. Paid once per generation, off the write path.

### 1.2 Types (`cube.go`)

```go
package cube

// Summability encodes Rule 1. The wire format carries it per measure column and
// the client query engine refuses illegal reductions.
type Summability uint8

const (
    // SumBoth: distributive over both date and account-hierarchy axes (flows).
    SumBoth Summability = iota
    // SumAccountOnly: may be rolled up the account tree at a fixed instant,
    // never summed across periods (market value, cost basis).
    SumAccountOnly
)

// Cube is the complete derived read model for one generation.
type Cube struct {
    Generation uint64
    BuiltAt    time.Time
    ConfigHash string // covers timezone + reporting currency; see 2.3

    Accounts    *Dict // full account paths, with parent/depth metadata
    Payees      *Dict
    Commodities []Commodity // code + decimal scale
    Periods     []string    // "2016-01" .. , month grain
    Codes       *Dict       // float 8-char transaction codes

    EpochDate time.Time // Postings.Date is days since this
    Postings  PostingTable
    Valued    BalanceTable // market value at each period end
    Cost      BalanceTable // cost basis at each period end
}

// Commodity carries the decimal scale so amounts round-trip exactly.
// USD is 2; shares and crypto are commonly 8.
type Commodity struct {
    Code  string
    Scale int32
}

// PostingTable is column-oriented and sorted ascending by Date, so a date-range
// filter is a binary search yielding one contiguous slice.
type PostingTable struct {
    Date      []uint16  // days since Cube.EpochDate
    Account   []uint32  // index into Accounts
    Payee     []uint32  // index into Payees
    Commodity []uint16  // index into Commodities
    Amount    []int64   // minor units, scaled per commodity
    Code      []uint32  // index into Codes
}

// BalanceTable holds a stock measure at period ends. Amount is SumAccountOnly.
type BalanceTable struct {
    Period    []uint16 // index into Periods
    Account   []uint32
    Commodity []uint16
    Amount    []int64 // minor units
}
```

### 1.3 Precision

hledger emits decimal strings. Parse to `int64` **minor units** using the per-commodity
scale, never `float64`, and never a fixed scale of 2 — shares and crypto need 8 decimals.
Carry the scale in the dictionary so the client can format correctly.

On the JS side these become `Float64Array`, not `BigInt64Array`. Integers are exact below
2^53 (i.e. $90 trillion in cents), and the measured difference is large: the same
group-by benchmark ran at **2.721 ms** with `BigInt64Array` versus **0.137 ms** with
`Float64Array` — a 20x penalty for no correctness gain at these magnitudes.

### 1.4 Multi-commodity

Commodity is a dimension, not a collapsed value. The client must not sum across
commodities. Cross-commodity totals come only from the pre-valued `Valued` table, which
hledger produced in the reporting currency.

### 1.5 Build entrypoint

```go
// Build runs hledger and assembles the read model. It performs no writes and
// must never be called from inside txlock.Do — see 2.2.
func Build(ctx context.Context, hl *hledger.Client, cfg Config) (*Cube, error)
```

---

## Phase 2 — Serving

### 2.1 Wire format (`encode.go`)

A small JSON header plus fixed-width columns, so the browser can build zero-copy typed
array views (`new Float64Array(buf, offset, n)`) with no decode pass.

```
offset 0   : magic "FLTCUBE1"        (8 bytes)
offset 8   : uint32 headerLen        (little-endian)
offset 12  : JSON header             (headerLen bytes)
offset ... : padding to 8-byte alignment
offset ... : column data, each column 8-byte aligned
```

Eight-byte alignment for every column is required, not cosmetic: `Float64Array` construction
throws on a misaligned byte offset.

```jsonc
{
  "generation": 42,
  "builtAt": "2026-08-29T12:00:00Z",
  "configHash": "sha256:...",
  "reportingCurrency": "USD",
  "epochDate": "2016-01-01",
  "dicts": {
    "account": [{"path": "assets:checking", "parent": -1, "depth": 1}],
    "payee": ["Acme Co"],
    "commodity": [{"code": "USD", "scale": 2}],
    "code": ["a1b2c3d4"]
  },
  "periods": ["2016-01"],
  "tables": {
    "postings": {
      "rows": 48000,
      "sortedBy": "date",
      "columns": {
        "date":      {"type": "u16", "offset": 0},
        "account":   {"type": "u32", "offset": 96000},
        "payee":     {"type": "u32", "offset": 288000},
        "commodity": {"type": "u16", "offset": 480000},
        "amount":    {"type": "f64", "offset": 576000,
                      "units": "minor", "summable": "both"},
        "code":      {"type": "u32", "offset": 960000}
      }
    },
    "valued": {
      "rows": 2926,
      "columns": {
        "amount": {"type": "f64", "units": "minor", "summable": "account-only"}
      }
    }
  }
}
```

Serve `Content-Encoding: gzip`; the fixture compressed 849 KB -> 253 KB.

Protobuf via the existing buf toolchain was considered and rejected for this payload:
packed varints defeat zero-copy typed-array views, which is the entire point of the format.
The cube is bulk binary data, not an API message.

### 2.2 Build trigger and invalidation

**Do not build inside `txlock.DoWith`.** It would add ~10 s to every write, and a cube
build failure must never be able to fail or revert a journal write.

Instead:

1. `txlock` bumps the generation exactly as today — no change to the write protocol.
2. The cube is built lazily on first request through the existing `internal/cache`, whose
   generation tiering and `singleflight` already give correct invalidation and
   thundering-herd protection for free.
3. Optionally, warm it in a background goroutine after commit. Failure there is logged and
   otherwise ignored; the lazy path is the guarantee.

Price updates are already covered: `prices.journal` is a journal file, so Alpha Vantage
backfills go through `txlock` and bump the generation, correctly invalidating valued
measures.

### 2.3 Config hash

Timezone affects month bucketing and reporting currency affects valuation, and neither
bumps the txlock generation. Hash the relevant `config.toml` fields into `ConfigHash` and
include it in the cache key alongside the generation.

### 2.4 HTTP endpoint (`internal/server/cube_http.go`)

```
GET /api/cube/{gen}.bin
Cache-Control: public, max-age=31536000, immutable
```

Generation in the **path** makes the URL content-addressed. The browser caches it forever
and a new generation is simply a new URL, so the stale-cache class of bug cannot occur.
This is the article's immutable-object trick, translated.

**The cube must be served behind auth.** Static assets from `internal/webui` are
deliberately served unauthenticated so the SPA shell can render the login page
(`cmd/floatd/main.go:172`), but the cube contains the user's entire ledger. Register it
next to the authenticated `/api/*` handlers at `cmd/floatd/main.go:169-171` and route it
through `internal/auth`'s credential check (bearer header or `float_session` cookie).
A request for a generation other than the current one returns 409 with the current
generation, rather than serving a stale cube.

### 2.5 Generation discovery

The client needs to know which generation to request.

- Add a Connect interceptor that stamps `X-Float-Generation` on every RPC response, so any
  ordinary call keeps the client's view fresh.
- Add `GET /api/generation` for the initial page load, before any RPC has run.

On mismatch the client refetches the cube. Because the URL is immutable and content-addressed,
that is a plain cache-miss fetch with no invalidation logic.

---

## Phase 3 — Client query engine

### 3.1 Load and decode (`web/src/lib/cube.js`)

```js
// Zero-copy: each column is a typed-array view over the same ArrayBuffer.
export async function loadCube(gen) {
  const buf = await (await fetch(`/api/cube/${gen}.bin`)).arrayBuffer();
  const headerLen = new DataView(buf).getUint32(8, true);
  const header = JSON.parse(new TextDecoder().decode(new Uint8Array(buf, 12, headerLen)));
  return { header, tables: viewTables(buf, header) };
}
```

Persist the raw bytes in IndexedDB keyed by generation so cold starts are instant.

### 3.2 Query API (`web/src/lib/cube-query.js`)

```js
// Flows: distributive, may group by any period grain.
cube.flows({ from, to, accountPrefix, payees, commodity, groupBy: "month" })

// Stocks: a single instant only. Throws if callers pass a range, which is how
// Rule 1 is enforced at runtime rather than by convention.
cube.balance({ at, accountPrefix, measure: "value" | "cost" })
```

Implementation notes:

- Postings are sorted by date, so a range filter is a binary search yielding one
  contiguous slice — the same "sort rows by the filter column" optimization the article
  uses to minimize byte ranges, here minimizing scanned rows and cache misses.
- Account-hierarchy filters compile to a `Uint8Array` mask over account ids, built once
  per prefix by string matching at load time (a few hundred accounts — trivial).
- Inverted indexes (account -> `Uint32Array` of row ids) are built lazily on first use.
  At 0.137 ms per full scan they are an optimization, not a requirement.

### 3.3 React integration (`web/src/hooks/use-cube.js`)

```js
const { data: cube } = useQuery({
  queryKey: queryKeys.cube(generation),
  queryFn: () => loadCube(generation),
  staleTime: Infinity,        // immutable per generation
  gcTime: Infinity,
});
```

Everything downstream is `useMemo` over `(cube, filters)`. Add the key to
`web/src/query-keys.js` per the existing convention.

---

## Phase 4 — Migrate existing pages

Order chosen so the first slice proves the number end-to-end and the rest is incremental.

1. **Trends** (`web/src/pages/trends.jsx`) — 3 RPCs -> 0. The 1Y/2Y/5Y/All buttons stop
   refetching. **This is the proof slice**: land Phases 1-3 plus this page and measure.
2. **Monthly dashboard** (`web/src/pages/monthly-dashboard.jsx`) — ~3.8 s per date-range
   change -> instant. Depth expand/collapse on the account tree becomes free, since the
   hierarchy is already in the dictionary.
3. **Home**, **Payees**, **Accounts** summaries.
4. **Portfolio** — allocation, market value, cost basis, and gain all read from the
   `Valued` and `Cost` tables.

**`TransactionsPage` stays server-side.** It needs full posting detail, pagination, and
free-text search. Its facet counts could be cube-driven later, but the list itself
should not be.

### The drilldown escape hatch

The cube holds aggregates, not transactions. Clicking a cell to see the transactions behind
it remains a `GetTransactions` RPC, with the query built from the cell's coordinates
(`date:`, `account`, `payee`, or `code:` from the `Code` column). Cube for the interactive
surface, server for the leaf. This boundary is what keeps the cube small.

---

## Phase 5 — Dashboards that were not previously feasible

At 2.4 s per query these are unbuildable; at 0.1 ms they are straightforward.

- **Crossfilter explorer** — date x account tree x payee x commodity, all cross-highlighting
  at 60 fps. Brushing the net-worth chart live-filters the expense donut.
- **Spending drilldown** — `type:` -> depth-1 -> depth-2 -> payee, each level instant.
- **Year-over-year small multiples** — one chart per year, all from one slice.
- **Month-over-month "what changed"** — ranked deltas by account and payee.
- **Cashflow sankey** — revenue accounts -> asset accounts -> expense accounts.

---

## Testing

Conventional unit tests for the encoder and dictionary. The one that matters:

**Golden cross-check against hledger.** An integration test in `cube_test.go` (package
`cube_test`, real hledger against `testdata/`, `t.Context()`, per the `internal/hledger`
convention) that:

1. Builds a cube from a fixture journal.
2. Enumerates a table of slices — date ranges x account prefixes x depths x commodities,
   including empty ranges, single-day ranges, and multi-commodity accounts.
3. For each slice, computes the answer from the cube and asserts it equals what
   `hledger bal` / `hledger is` reports for the equivalent query.

This is cheap to write and is the only thing standing between us and a dashboard that is
fast and wrong. Include at least one priced multi-commodity fixture so the Rule 1 boundary
is actually exercised — a cube that only ever sees single-currency USD flows will pass a
test suite that would not catch the valuation bug.

Add a JS test asserting `cube.balance()` throws when handed a date range.

---

## Risks and Non-Goals

| Risk | Mitigation |
|---|---|
| Valued measures summed across time | `Summability` in the format; client API throws; multi-commodity priced test fixture |
| Cross-commodity sums | Commodity is a dimension; cross-currency totals only from the pre-valued table |
| Rounding drift | `int64` minor units with per-commodity scale; `Float64Array` client-side (exact below 2^53) |
| Cube staleness | Generation in the immutable URL; 409 on mismatch |
| Timezone/currency config changes | `ConfigHash` in the cache key |
| Build time grows with journal | Off the write path, lazy + background warm; revisit grouping sets past ~5 MB |
| Cube becomes a second source of truth | No mutation path reads it; always safe to delete |

**Non-goals for this plan:** parquet/hyparquet, DuckDB-WASM, grouping sets, R2 or any
external object storage, and TUI adoption. The TUI (`cmd/float/ui`) talks to the same
server and could consume the cube later, but it does not share the browser's caching model
and is out of scope here.

**Deferred — the tag dimension.** float's tags live in transaction comments, and
`print -O csv` exposes them only as raw `comment` / `posting-comment` text rather than as
a structured column. Adding a tag dimension means either parsing comments at build time or
a second hledger pass, so it is deliberately not in Phase 1. Revisit once the flow and
stock tables are proven.

---

## Appendix A — Measurements

Fixture: 24,000 transactions / 48,000 postings / 96k lines / 2.4 MB, 23 accounts,
400 payees, single-commodity USD, dates spanning 2016-01-01 to 2026-08-01.
hledger 1.52 as pinned in `mise.toml`.

**hledger latency (`-f fixture.journal`)**

| Command | Time | Output |
|---|---:|---:|
| `bal type:X --depth 2 -O json` | 2366 ms | 5 KB |
| `bs --monthly --historical --layout=bare -O json` | 3295 ms | 739 KB |
| `is --monthly --tree -O json` | 3770 ms | 2.6 MB |
| `bal --monthly --layout=bare --depth 99 -O csv` | 2242 ms | 33 KB |
| `reg -O csv` | 3445 ms | 4.2 MB |
| `print -O csv` | 3761 ms | 5.1 MB |
| `reg -O json` | 10317 ms | 62 MB |
| `print -O json` | 9404 ms | 56 MB |

**Encoded cube size (dictionary-encoded fixed-width columns)**

| Table | rows | raw | gzip -6 |
|---|---:|---:|---:|
| month x account | 2,926 | 53.4 KB | 15.2 KB |
| month x account x payee | 23,686 | 515.9 KB | 114.4 KB |
| full posting table | 48,000 | 849.4 KB | 253.0 KB |

**Client-side query (Node, full-scan group-by over 48,000 rows, 200 iterations)**

| Amount column type | Time per query |
|---|---:|
| `BigInt64Array` | 2.721 ms |
| `Float64Array` | **0.137 ms** |

Net: ~2400-3800 ms per dashboard interaction today, versus a ~10 s build once per
generation plus ~0.1 ms per interaction. Roughly a 20,000x improvement on the interactive
path.
