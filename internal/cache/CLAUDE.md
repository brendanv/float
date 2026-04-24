# internal/cache

Generation-aware in-memory cache for hledger query results. Sits between `internal/server/ledger` handlers and the hledger wrapper.

## How It Works

`Cache[T]` stores entries in generation-keyed tiers. Each call to `Get` checks the current generation (from `txlock.TxLock.Generation`). When the generation advances (after any write), all prior-generation tiers are pruned on the next store, effectively invalidating the entire cache. There is no partial invalidation.

Concurrent calls for the same key+generation share a single `singleflight` flight — only one hledger invocation fires regardless of how many goroutines race.

## API

```go
c := cache.New[any](lock.Generation)     // pass generation func from txlock
val, err := c.Get(ctx, key, loadFn)      // cache hit or call loadFn
```

Cache keys are produced by the handler's helper functions (`transactionsKey`, `balancesKey`, etc.) which sort query args for determinism.

Pass `nil` for the cache in `Handler` to bypass caching entirely (useful in tests).

## Concurrency

`sync.RWMutex` guards tier map reads/writes. `singleflight.Group` is scoped per `key@genN` so callers at different generations never share a load result.
