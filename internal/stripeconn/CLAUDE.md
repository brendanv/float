# internal/stripeconn

Stripe Financial Connections integration. Manages per-connection state on
disk and converts Stripe transactions into hledger journal entries.

## Storage

- `<data-dir>/stripe/connections.json` — full state. Schema in `Store` /
  `Connection` (`store.go`). Includes the Stripe FC account id, the
  hledger-side mapping (`HledgerAccount`, default in/outflow accounts), and
  the durable set of `ImportedIDs`.
- API key is **not** kept here — it lives in `config.toml` under
  `[stripe] api_key`.

## API

- `Load(dataDir)` / `Save(dataDir, *Store)` — JSON-on-disk persistence.
  Returns an empty Store (not an error) when the file doesn't exist.
- `Store.Upsert(c)`, `Find(id)`, `FindByStripeID(...)`, `Delete(id)` — basic
  CRUD.
- `Connection.MarkImported(id)` / `HasImported(id)` — manipulate the
  per-connection dedup set.
- `NewLiveStripe(apiKey)` — wraps stripe-go. Implements the `Stripe`
  interface used by `Sync` so tests can inject a fake.
- `Sync(ctx, hl, lock, dataDir, connID, rules, api)` — pulls posted
  transactions for one connection and writes new ones to the journal.

## Sign convention (load-bearing)

Stripe FC amounts are signed from the account holder's perspective
(`+` = into the account, `-` = out). float follows hledger's mainstream
convention of liabilities-as-negative, so the Stripe sign flows through
unchanged on the linked-account posting:

```
linked = stripe.amount (decimal, sign preserved)
other  = -linked
```

This is the entire rule — no branching on cash-vs-credit. See the
`formatAmount` docstring and `import_test.go` for worked examples.

## Dedup

Per sync, `scanImportedIDs` runs `hledger print -O json tag:stripe-txn-id`
to find every transaction that already carries a Stripe id tag. That set is
unioned with `connection.ImportedIDs` from `connections.json`. If a user
explicitly deletes the `stripe-txn-id:` tag from a transaction, the next
sync will re-import it — documented as the "forget" mechanism.

## Required tags on imported transactions

- `stripe-txn-id: <stripe id>` — dedup key, user-visible
- `source: stripe` — broad filter
- `stripe-connection: <Connection.ID>` — which connection imported it
- `float-stripe-sync: <batchID>` — hidden meta, traces a single sync run

## Sync protocol

1. Load store, validate the connection has `HledgerAccount` and both
   default accounts configured.
2. Enforce `MinSyncInterval` (1 hour) since the last successful sync.
3. Pull posted transactions from Stripe **outside** the txlock.
4. Take `txlock.Do()`:
   - Build the dedup `seen` set from hledger + `ImportedIDs`.
   - For each new transaction, build a `TransactionInput`, apply float
     rules via `rules.Match`, write via `journal.AppendTransaction`,
     `MarkImported`.
   - Update `LastSyncedAt` / `LastTransactionCursor`, save the store.
5. On any error, restore `connections.json` to its pre-sync bytes
   (txlock only snapshots `.journal` files, so the store needs explicit
   roll-back).

## Pricing caveat

Stripe FC refresh requests cost real money in production. `Sync` enforces a
minimum one-hour gap per connection (`MinSyncInterval`). Manual triggers
should respect this.

## Testing

`import_test.go` covers the sign convention exhaustively (cash + credit,
inflows + outflows, JPY zero-decimal) without touching Stripe or hledger.
Handler integration tests live in `internal/server/ledger/stripe_test.go`
and use a fake `Stripe` implementation alongside a real hledger client.
