# internal/warm

Proactively populates `internal/cache` so steady-state browsing hits warm entries instead of paying for a full-journal hledger parse on the first request after startup or after a write.

## How It Works

`Warmer` holds a generation function (`txlock.TxLock.Generation`) and an `entriesFn func() []Entry` called fresh at the start of every pass — this lets a dynamic entry set (the recently-used-accounts LRU in `internal/server/ledger`) reflect current state rather than whatever it was at construction time.

`run()` executes entries one at a time, in order, checking `gen() != startGen` before each one and aborting the rest of the pass if the generation moved — a newer write landed mid-pass, and its own triggered pass supersedes this one. Passes are serialized with a mutex (concurrency 1), so a startup `Start` racing a debounced `Trigger` never runs two passes at once.

Each `Entry.Load` should call the same `cached*` helper the RPC handler uses (e.g. `cachedTransactions`), so a warm load and a racing user request share one hledger invocation via the cache's singleflight, and strong consistency holds — everything is generation-keyed.

`run()` marks its context with `hledger.WithLowPriority` before calling any entry, so warm loads poll for an hledger concurrency slot instead of queuing ahead of interactive requests (see `internal/hledger`'s concurrency semaphore).

## API

- `New(gen, entriesFn, debounce)` — construct a Warmer.
- `Start(ctx)` — run one pass immediately in the background. Call this only after the server has started listening, so warming never delays boot.
- `Trigger(gen)` — schedule a debounced pass; register this as a `txlock.TxLock.OnCommit` hook. Resets the debounce timer on each call, so a burst of writes (imports, apply-rules) collapses into one pass after the burst settles.

## Integration

`cmd/floatd/main.go` wires `warm.New(lock.Generation, handler.WarmEntries, warmDebounce)`, registers `lock.OnCommit(warmer.Trigger)`, and calls `warmer.Start(ctx)` right after the listener binds. `internal/server/ledger`'s `Handler.WarmEntries()` (in `warm.go`) defines the fixed dataset set plus per-account `areg` entries for a small in-memory LRU of recently-touched accounts (`recentAccounts`, updated in `GetAccountRegister`). It returns `nil` when the handler's cache is disabled (`h.cache == nil`), since warming would just repeat uncached hledger work for nothing.
