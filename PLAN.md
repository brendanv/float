# float — Implementation Plan

Each step produces something you can run, test, or interact with. No step is pure scaffolding.

---

## Step 1: hledger Wrapper ✓

Build `internal/hledger/` — the foundation everything else calls.

- Shell out to `hledger` with structured command building (subcommand, flags, filters)
- Parse JSON output from `hledger bal -O json`, `hledger reg -O json`, `hledger accounts --tree`
- Parse output from `hledger check` (success/failure + error messages)
- Return typed Go structs (not raw JSON)
- On startup, run `hledger --version` and fail fast with a clear error if the version is unsupported. The supported version is pinned in the Dockerfile and in `mise.toml`.
- Integration tests that run real hledger against fixture `.journal` files

**Testable artifact:** `go test ./internal/hledger/` — runs hledger against fixture journals, asserts parsed balances, transactions, and account trees match expectations.

---

## Step 2: Config & Journal Management ✓

Build `internal/config/` and `internal/journal/`.

- **Config:** Parse `config.toml` — bank profiles, user entries, server settings. Return typed Go structs.
- **Journal read:** Parse journal files to extract individual transactions with their metadata (date, description, postings, tags including `fid`).
- **Journal write:** Append transactions to the correct `YYYY/MM.journal` file, creating year directories and files as needed. Update `main.journal` include directives when new year/month files are created.
- **fid minting:** Generate 8-char UUID prefix tags, attach to new transactions.
- **fid migration:** Scan existing transactions, add `fid` tags to any that lack them.

**Testable artifact:** Unit tests against temp directories — write transactions, read them back, verify file organization and fid assignment. Config round-trip tests.

---

## Step 3: Write Protocol

Build `internal/txlock/`.

- `TxLock` struct holding the mutex, data dir path, and a reference to the hledger wrapper
- `Do(fn func() error) error` — the write protocol:
  1. Acquire mutex
  2. Snapshot current file state (copy modified files to a temp location for revert)
  3. Execute `fn` (caller writes files)
  4. Run `hledger check` via the hledger wrapper
  5. If check fails: revert files from snapshot, return error
  6. Bump generation counter
  7. Release mutex
- Expose the `generation` counter as an `atomic.Uint64` for the cache to read later

No git in this step — safety comes from the revert-on-failure logic. Git is added later as a separate enhancement.

**Testable artifact:** Integration tests — write a valid transaction through `Do()`, verify it persists. Write an invalid transaction, verify files are reverted.

---

## Step 4: floatctl Write Commands (MVP)

Add write commands to `floatctl` — the first point where float is genuinely useful as a personal finance tool.

- **`floatctl journal import <csv> --profile <name>`** — preview CSV import (show parsed transactions + duplicate detection), prompt for confirmation, write via `txlock.Do()`
- **`floatctl journal add`** — interactive or flag-driven transaction entry, write via `txlock.Do()`
- **`floatctl journal delete <fid>`** — remove transaction by fid, write via `txlock.Do()`
- **Duplicate detection:** content fingerprint matching against existing transactions during import preview

**Testable artifact:** Import a real bank CSV, see the preview, confirm, run `floatctl hledger balance` and see updated balances. Add and delete transactions manually.

---

## Step 5: Protobuf & gRPC Server (Read Path)

Define protobufs and stand up the server with read-only LedgerService.

- **Protobufs:** Define `ledger.proto` — messages for `Transaction`, `Posting`, `Account`, `Balance`. RPCs: `ListTransactions`, `GetBalances`, `ListAccounts`.
- **Buf setup:** `buf.yaml`, `buf.gen.yaml`, generate Go + ConnectRPC code.
- **LedgerService handler:** `internal/server/ledger/` — implements the read RPCs by calling the hledger wrapper and mapping results to protobuf.
- **Server binary:** `cmd/floatd/main.go` — minimal main that wires up config, hledger wrapper, and the ConnectRPC handler on an HTTP server.

**Testable artifact:** Start `floatd` pointing at a data directory, query it with `buf curl` or `grpcurl`. See real balances and transactions come back.

---

## Step 6: CLI (Read Path)

Build `cmd/float/` — a gRPC client CLI that talks to `floatd`.

- Connect to the server using ConnectRPC's HTTP client
- Commands: `float balances`, `float transactions`, `float accounts`
- Tabular output for terminal (no auth yet — open access)
- `--server` flag for the floatd address (default `localhost:8080`)

**Testable artifact:** Start `floatd`, run `float balances` — see formatted account balances in the terminal. A complete read-path round trip from CLI → gRPC → hledger → journal files.

---

## Step 7: Write Path via gRPC

Add mutating RPCs to `floatd` and corresponding `float` CLI commands.

- **Protobufs:** Add `AddTransaction`, `DeleteTransaction`, `PreviewImport`, `ConfirmImport` RPCs.
- **Handlers:** Use `txlock.Do()` to coordinate journal writes, hledger validation, and generation bumps.
- **CLI commands:** `float add`, `float delete`, `float import`

**Testable artifact:** Full round trip through the gRPC API for all write operations that already work via `floatctl`.

---

## Step 8: Authentication

Build `internal/auth/` and `AuthService`.

- **Passphrase hashing:** argon2id hash/verify functions
- **JWT tokens:** Issue and validate HMAC-SHA256 tokens with `sub`, `role`, `exp` claims. Sign with `float.key` (generated on first startup).
- **Auth interceptor:** ConnectRPC interceptor that validates tokens on all RPCs except `Login`. Injects user/role into context. Rejects `viewer` role on mutating RPCs.
- **AuthService:** `Login`, `ChangePassphrase` RPCs. Proto definition + handler.
- **Setup flow:** `float setup --user alice --passphrase` creates the initial admin and writes the hash to `config.toml`.
- **CLI auth:** `float login` stores token to `~/.config/float/token`. All commands attach it as `Bearer` header.

**Testable artifact:** `float setup`, `float login`, then all existing commands still work (now authenticated). Viewer role can query but not mutate.

---

## Step 9: Query Cache

Build `internal/cache/`.

- Generation-counter LRU cache storing parsed Go structs
- `sync.RWMutex` for concurrent reads, `atomic.Uint64` for generation checks
- `golang.org/x/sync/singleflight` to deduplicate concurrent cache misses — only one hledger subprocess fires per unique key, others wait and share the result
- Max 128 entries, LRU eviction
- Pre-warming goroutine: after generation bump, asynchronously warm account tree, top-level balances, and current month transactions
- Wire into `LedgerService` — cache sits between the handler and the hledger wrapper

**Testable artifact:** Benchmarks comparing cached vs. uncached query latency. Integration test: query, write, query again — verify cache invalidation works (results reflect the write).

---

## Step 10: Web UI

Build the frontend SPA in `web/`.

- Choose framework (SvelteKit or React)
- ConnectRPC client generation from the existing protobufs
- Pages: dashboard (balances), transaction list, import wizard, snapshots
- Embed built assets into the `floatd` binary via `embed.FS`
- Serve the SPA from `floatd` alongside the gRPC API

**Testable artifact:** Open `localhost:8080` in a browser, log in, see balances and transactions, import a CSV, add a rule.

---

## Step 11: Git Snapshots

Build `internal/gitsnap/` using go-git and wire it into `txlock` and `floatctl`.

- `Init(dir)` — initialize a git repo in the data directory if one doesn't exist
- `Commit(msg)` — stage all changes and commit with the given message
- `List()` — return recent commits (hash, message, timestamp)
- `Restore(hash)` — hard reset to a given commit, intentionally discarding all subsequent commits
- `RecoverUncommitted()` — on startup, commit any dirty working tree as a recovery snapshot
- Update `txlock.Do()` to commit after each successful write
- **`floatctl journal snapshots`** and **`floatctl journal restore <hash>`**
- **`float snapshots`** and **`float restore`** via gRPC

**Testable artifact:** Make changes, list snapshots, restore to a previous one, verify file contents revert.

---

## Step 12: Rules Engine

Build `internal/rules/` and wire it into `floatctl` and the gRPC API.

- **Rules file management:** Append/remove `if` blocks in `.rules` files so future imports pick them up
- **`floatctl rules add --match <pattern> --account <account>`** — add a rule, optionally apply retroactively
- **`floatctl rules list`** — show current rules
- **`floatctl rules delete <n>`** — remove a rule
- **Retroactive preview:** use `hledger reg -O json` to find matching existing transactions, show what would change
- **Retroactive apply:** look up transactions by `fid`, edit posting lines in journal files, validate with `hledger check`, write via `txlock.Do()`
- **Protobufs:** Add `AddRule`, `ListRules`, `DeleteRule` RPCs to `floatd`; wire `float rules` CLI commands
- **Web UI:** Add rules management page

**Testable artifact:** Add a rule, import new transactions and see it applied automatically. Apply retroactively to existing transactions. Manage rules via CLI, gRPC, and web UI.

---

## Step 13: Deployment

- **Dockerfile:** Multi-stage build — Go binary + embedded web assets + pinned hledger binary
- **docker-compose.yml:** Volume mount for `data/`, port mapping, env var config
- **Startup checks:** Verify hledger is available, init git repo if needed, run recovery snapshot, run fid migration on first startup against existing journals

**Testable artifact:** `docker compose up`, access float from the browser, all features work.
