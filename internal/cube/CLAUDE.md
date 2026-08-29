# internal/cube

Builds a precomputed, column-oriented read model of the ledger so the web dashboards can answer filters and drilldowns in the browser instead of spawning an hledger process per interaction. See `PLAN.md` for the measurements and rationale.

## The rule everything rests on

**Flows sum over time. Stocks do not.**

- `PostingTable.Amount` is distributive — legal to sum over any date range and any account subtree.
- `Valued` and `Cost` are stock measures materialized per period end. They may be rolled up the account tree *at a fixed period* and never summed across periods. Market value depends on the price series and cost basis on hledger's lot matching; neither is recoverable from a sum of posting deltas.

`Summability` is encoded per measure column in the wire format so the client can enforce this. There is deliberately no range form of `BalanceAt` — offering the signature would invite the illegal reduction.

## Invariants

- The cube is a **cache, never a source of truth**. No mutation path reads it; it must always be safe to delete and rebuild.
- hledger stays the accounting engine. This package only aggregates numbers hledger already computed.
- `Build` must **never** run inside `txlock.Do`. It takes seconds, and a build failure must not be able to fail or revert a journal write.

## Key Types

- `Cube` — the complete read model for one generation: dictionaries, commodities with scales, periods, and the three tables.
- `PostingTable` — the flow fact table, sorted ascending by date so a range filter is a binary search over a contiguous run.
- `BalanceTable` — a sparse stock measure at period ends. No entry means a zero balance.
- `Dict` — string interning to dense `uint32` ids. `AccountHierarchy` derives parent/depth metadata; parents are not synthesized, so an account tier with no postings of its own has no id and the client reaches it by prefix matching.
- `FlowFilter` — selects postings. The date interval is **half-open, `[From, To)`**, matching hledger's `date:A..B`, whose upper bound is exclusive. `Account` matches the account and its descendants — a tree match, not hledger's substring regex.

## Sources

| Method | hledger command | Why |
|---|---|---|
| `hledger.PostingRows` | `print -O csv` | CSV not JSON: 3.8s/5.1MB vs 9.4s/56MB for the same data on a 24k-transaction journal. hledger's cost on large reports is JSON serialization. |
| `hledger.PeriodBalancesValued` | `bal --monthly --historical --layout=bare --infer-market-prices --value=end,USD type:AL -O csv` | `bal` not `bs`: `bs` renders liabilities positive by display convention, which would have to be undone per-section. `bal` reports raw signed balances and emits month labels directly. |
| `hledger.PeriodBalancesCost` | same, with `--cost` | cost basis |

The three run concurrently, so a build costs roughly the slowest of them (~4s on a 24k-transaction journal) rather than their sum.

## Precision

Amounts are `int64` **minor units** at a per-commodity scale — the widest precision seen for that commodity across every source, so nothing is rounded on the way in. `rescale` refuses to round down rather than silently losing digits. Shares and crypto need 8 places, not 2.

On the wire, amounts become `float64` so the client can use `Float64Array` (measured 20x faster than `BigInt64Array` for the same aggregation). `Encode` verifies every amount stays below 2^53, where float64 stops representing integers exactly.

## Wire format

```
"FLTCUBE1" | uint32 headerLen | JSON header | pad | columns
```

Column offsets are relative to the start of the data section, not the file, so the header's own length does not feed back into the offsets it contains. Every column is 8-byte aligned — `Float64Array` construction throws on a misaligned byte offset, which would break the client's zero-copy decode.

## Testing

`cube_test.go` runs real hledger against `testdata/`. The load-bearing test is `TestFlowSumsMatchHledger`: a table of slices whose cube-derived answers must equal what `hledger bal` reports for the equivalent query.

Two things that suite learned the hard way, both worth preserving:

- **Every date-sensitive case must name an account.** A balanced double-entry journal sums to zero over any slice with no account filter, so an unfiltered case discriminates nothing.
- **Some bounds must land exactly on a posting date.** Without them, treating hledger's exclusive upper bound as inclusive passes every other case.

`testdata/multi.journal` is priced and multi-commodity on purpose: a suite that only sees single-currency USD flows passes while the valuation path is broken.
