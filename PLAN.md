# Vault — Implementation Plan

Each step produces something you can run, test, or interact with. No step is pure scaffolding.

---

## Step 1: hledger Wrapper

Build `internal/hledger/` — the foundation everything else calls.

- Shell out to `hledger` with structured command building (subcommand, flags, filters)
- Parse JSON output from `hledger bal -O json`, `hledger reg -O json`, `hledger accounts --tree`
- Parse output from `hledger check` (success/failure + error messages)
- Return typed Go structs (not raw JSON)
- Integration tests that run real hledger against fixture `.journal` files

**Testable artifact:** `go test ./internal/hledger/` — runs hledger against fixture journals, asserts parsed balances, transactions, and account trees match expectations.

---

## Step 2: Config & Journal Management

Build `internal/config/` and `internal/journal/`.

- **Config:** Parse `config.toml` — bank profiles, user entries, server settings. Return typed Go structs.
- **Journal read:** Parse journal files to extract individual transactions with their metadata (date, description, postings, tags including `vid`).
- **Journal write:** Append transactions to the correct `YYYY/MM.journal` file, creating year directories and files as needed. Update `main.journal` include directives when new year/month files are created.
- **VID minting:** Generate 8-char UUID prefix tags, attach to new transactions.
- **VID migration:** Scan existing transactions, add `vid` tags to any that lack them.

**Testable artifact:** Unit tests against temp directories — write transactions, read them back, verify file organization and vid assignment. Config round-trip tests.

---

## Step 3: Git Snapshots

Build `internal/gitsnap/` using go-git.

- `Init(dir)` — initialize a git repo in the data directory if one doesn't exist
- `Commit(msg)` — stage all changes and commit with the given message
- `List()` — return recent commits (hash, message, timestamp)
- `Restore(hash)` — hard reset to a given commit
- `RecoverUncommitted()` — on startup, commit any dirty working tree as a recovery snapshot

**Testable artifact:** Integration tests — init a repo in a temp dir, make file changes, commit, list commits, restore to a previous one, verify file contents revert.

---

## Step 4: Write Protocol

Build `internal/txlock/`.

- `TxLock` struct holding the mutex, data dir path, references to hledger and gitsnap
- `Do(fn func() error, commitMsg string) error` — the full write protocol:
  1. Acquire mutex
  2. Snapshot current file state (for revert)
  3. Execute `fn` (caller writes files)
  4. Run `hledger check` via the hledger wrapper
  5. If check fails: revert files, return error
  6. Git commit via gitsnap
  7. Bump generation counter
  8. Release mutex
- Expose the `generation` counter as an `atomic.Uint64` for the cache to read

**Testable artifact:** Integration tests — write a valid transaction through `Do()`, verify commit exists. Write an invalid transaction, verify files are reverted and no commit is created.

---

## Step 5: Protobuf & gRPC Server (Read Path)

Define protobufs and stand up the server with read-only LedgerService.

- **Protobufs:** Define `ledger.proto` — messages for `Transaction`, `Posting`, `Account`, `Balance`. RPCs: `ListTransactions`, `GetBalances`, `ListAccounts`.
- **Buf setup:** `buf.yaml`, `buf.gen.yaml`, generate Go + ConnectRPC code.
- **LedgerService handler:** `internal/server/ledger/` — implements the read RPCs by calling the hledger wrapper and mapping results to protobuf.
- **Server binary:** `cmd/vaultd/main.go` — minimal main that wires up config, hledger wrapper, and the ConnectRPC handler on an HTTP server.

**Testable artifact:** Start `vaultd` pointing at a fixture data directory, query it with `buf curl` or `grpcurl`. See real balances and transactions come back.

---

## Step 6: CLI (Read Path)

Build `cmd/vault/` — a gRPC client CLI that talks to `vaultd`.

- Connect to the server using ConnectRPC's HTTP client
- Commands: `vault balances`, `vault transactions`, `vault accounts`
- Tabular output for terminal (no auth yet — open access)
- `--server` flag for the vaultd address (default `localhost:8080`)

**Testable artifact:** Start `vaultd`, run `vault balances` — see formatted account balances in the terminal. A complete read-path round trip from CLI → gRPC → hledger → journal files.

---

## Step 7: Write Path (Add & Delete Transactions)

Add mutating RPCs and wire them through the write protocol.

- **Protobufs:** Add `AddTransaction`, `UpdateTransaction`, `DeleteTransaction` RPCs to `ledger.proto`.
- **LedgerService handler (writes):** Use `txlock.Do()` to coordinate journal writes, hledger validation, and git commits.
- **CLI commands:** `vault add`, `vault delete`
- **Snapshot RPCs:** Add `ListSnapshots`, `RestoreSnapshot` to expose git history via the API. CLI: `vault snapshots`, `vault restore`.

**Testable artifact:** `vault add` a transaction, `vault transactions` to see it, `vault delete` to remove it, `vault snapshots` to see the history, `vault restore` to undo.

---

## Step 8: Authentication

Build `internal/auth/` and `AuthService`.

- **Passphrase hashing:** argon2id hash/verify functions
- **JWT tokens:** Issue and validate HMAC-SHA256 tokens with `sub`, `role`, `exp` claims. Sign with `vault.key` (generated on first startup).
- **Auth interceptor:** ConnectRPC interceptor that validates tokens on all RPCs except `Login`. Injects user/role into context. Rejects `viewer` role on mutating RPCs.
- **AuthService:** `Login`, `ChangePassphrase` RPCs. Proto definition + handler.
- **Setup flow:** `vault setup --user alice --passphrase` creates the initial admin and writes the hash to `config.toml`.
- **CLI auth:** `vault login` stores token to `~/.config/vault/token`. All commands attach it as `Bearer` header.
- **`--auth=none` flag** for reverse proxy deployments.

**Testable artifact:** `vault setup`, `vault login`, then all existing commands still work (now authenticated). Viewer role can query but not mutate.

---

## Step 9: Import Pipeline

Build `internal/importer/` and `ImportService`.

- **ImportService proto:** `PreviewImport` (returns candidates + duplicates), `ConfirmImport` (commits the import).
- **Preview flow:** Take CSV + bank profile → run `hledger import` to temp file → parse candidates → run dedup (content fingerprint + import tag matching) → return preview with net-new vs. potential duplicates.
- **Confirm flow:** Group confirmed transactions by month → append to journal files with `vid` + `import` tags → validate → commit through `txlock.Do()`.
- **CLI:** `vault import <file> --profile "Chase Checking"` — shows preview, prompts for confirmation.
- **Bank profile management:** `vault profiles list`, reading from config.

**Testable artifact:** Import a CSV, see the preview with duplicate detection, confirm, verify transactions appear in `vault transactions` and git log shows the import commit.

---

## Step 10: Rules Engine

Build `internal/rules/` and `RulesService`.

- **RulesService proto:** `AddRule`, `ListRules`, `DeleteRule`, `PreviewRetroactive`, `ApplyRetroactive`.
- **Rules file management:** Append/remove `if` blocks in `.rules` files so future `hledger import` picks them up.
- **Retroactive preview:** Use `hledger reg -O json` to find matching existing transactions. Return a preview of what would change.
- **Retroactive apply:** Look up transactions by `vid`, perform text-level edits to journal files (change account on posting lines), validate with `hledger check`, commit through `txlock.Do()`.
- **CLI:** `vault rules add --match "Amazon" --account "expenses:shopping"` with retroactive prompt.

**Testable artifact:** Add a rule, see it reflected in the `.rules` file. Import new transactions and see the rule applied. Apply retroactively to existing transactions and verify the changes.

---

## Step 11: Query Cache

Build `internal/cache/`.

- Generation-counter LRU cache storing parsed Go structs
- `sync.RWMutex` for concurrent reads, `atomic.Uint64` for generation checks
- Max 128 entries, LRU eviction
- Pre-warming goroutine: after generation bump, asynchronously warm account tree, top-level balances, and current month transactions
- Wire into `LedgerService` — cache sits between the handler and the hledger wrapper

**Testable artifact:** Benchmarks comparing cached vs. uncached query latency. Integration test: query, write, query again — verify cache invalidation works (results reflect the write).

---

## Step 12: Web UI

Build the frontend SPA in `web/`.

- Choose framework (SvelteKit or React)
- ConnectRPC client generation from the existing protobufs
- Pages: dashboard (balances), transaction list, import wizard, rules management, snapshots
- Embed built assets into the `vaultd` binary via `embed.FS`
- Serve the SPA from `vaultd` alongside the gRPC API

**Testable artifact:** Open `localhost:8080` in a browser, log in, see balances and transactions, import a CSV, add a rule.

---

## Step 13: Deployment

- **Dockerfile:** Multi-stage build — Go binary + embedded web assets + hledger binary
- **docker-compose.yml:** Volume mount for `data/`, port mapping, env var config
- **Startup checks:** Verify hledger is available, init git repo if needed, run recovery snapshot, run vid migration on first startup against existing journals

**Testable artifact:** `docker compose up`, access vault from the browser, all features work.
