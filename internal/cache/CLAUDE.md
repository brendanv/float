# internal/cache

Generation-aware in-memory cache for hledger query results. It sits between `internal/server/ledger` handlers and the hledger wrapper.

## How It Works

`Cache[T]` stores entries under the current generation returned by a caller-provided function, normally `txlock.TxLock.Generation`. A successful write bumps the generation, so old entries are ignored and pruned on the next store. There is no partial invalidation.

Concurrent calls for the same `key@generation` share one `singleflight` load. Calls at different generations do not share results.

## API

```go
c := cache.New[any](lock.Generation)
val, err := c.Get(ctx, key, loadFn)
```

`Get` returns a cached value when present; otherwise it calls `loadFn(ctx)`, stores the result for the generation observed before the load, and returns it. Load errors are not cached.

## Integration

Cache keys are built by `internal/server/ledger` helpers (`transactionsKey`, `balancesKey`, `balancesValuedKey`, `accountRegisterKey`, etc.). Query-token keys are sorted for determinism, so equivalent query slices share cache entries.

Pass `nil` as the handler cache to bypass caching in tests or specialized callers.

## Concurrency

A `sync.RWMutex` guards the generation-tiered map. `singleflight.Group` deduplicates loads. Cache values are not deep-copied; callers should treat cached values as read-only.

## Stats

`Stats()` returns cumulative `(hits, misses, loads)` counters: `hits` for `Get` calls served from the tiered map, `misses` for calls that fell through to singleflight, `loads` for load funcs actually executed (lower than `misses` when concurrent callers dedupe onto one load). `cmd/floatd` diffs these against the previous generation's snapshot and logs the delta on every `txlock.OnCommit`.
