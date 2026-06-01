# CLAUDE.md

`float` is a self-hostable personal finance manager wrapping [hledger](https://hledger.org/). float provides the UX layer (Connect/gRPC API, CLI/TUI, web UI, imports, snapshots, and integrations) and delegates all accounting math, parsing, validation, reports, and CSV rule parsing to hledger. **Never reimplement accounting logic that hledger already handles.**

Three binaries:

- `floatd` — server: ConnectRPC over h2c, embedded web UI, optional SSH-hosted TUI, background Stripe auto-import.
- `float` — Bubble Tea terminal UI client that connects to `floatd`.
- `floatctl` — admin/debug CLI that bypasses the API and operates on the data directory directly.

## Commands

```bash
mise run test        # go test ./...
mise run lint        # golangci-lint run ./...
mise run check       # lint + test
mise run check-all   # lint + vet + test (full pre-commit gate)
mise run proto-gen   # buf generate for Go + JS (after editing .proto files)
mise run web-gen     # generate JS protobuf code only
mise run web-build   # build web UI → internal/webui/dist/
mise run build       # web UI + compile floatd
mise run web-dev     # Vite dev server on :5173 (proxies API to floatd on :8080)
mise run ssh         # SSH into the floatd TUI (requires ssh_port in config.toml)
```

Tool versions are managed by `mise`. Run `mise install` to get pinned Go, buf, golangci-lint, hledger, and related tools.

## Running floatd

```bash
mise run floatd
FLOAT_DATA_DIR=/path/to/data mise run floatd
FLOAT_ADDR=:9090 mise run floatd
```

A data directory must contain `config.toml`; `floatd` creates an empty `main.journal` if it is missing. For web UI development run `mise run floatd` and `mise run web-dev` concurrently.

### SSH TUI

Enable the SSH server by adding `ssh_port` to `config.toml`:

```toml
[server]
port = 8080
ssh_port = 2222
```

Then connect:

```bash
mise run ssh                                                    # localhost:2222
FLOAT_SSH_PORT=2222 mise run ssh                               # explicit port
FLOAT_SSH_HOST=myserver.example.com FLOAT_SSH_PORT=2222 mise run ssh
```

The host key is generated at `$FLOAT_DATA_DIR/ssh_host_key` on first start. The task stores known hosts in `$FLOAT_DATA_DIR/ssh_known_hosts` (separate from `~/.ssh/known_hosts`) using `StrictHostKeyChecking=accept-new`.

## Querying floatd

`floatd` supports gRPC, gRPC-Web, and Connect protocols on the same h2c endpoint. Use `buf curl` from the repo root:

```bash
buf curl --schema . --protocol grpc --http2-prior-knowledge \
  http://localhost:8080/float.v1.LedgerService/GetBalances --data '{}'
```

Mise shortcuts include `mise run grpc-balances`, `mise run grpc-transactions`, `mise run grpc-accounts`, `mise run grpc-delete`, and `mise run grpc-modify-tags`. They support `FLOAT_DEPTH`, `FLOAT_QUERY`, and `FLOAT_ADDR` where relevant.

## Architecture

### Core Constraint: hledger Is the Accounting Engine

float shells out to hledger for accounting: parsing journals, computing balances/registers/net worth/income reports, validating correctness, importing CSVs, applying market values, and account discovery. `internal/hledger/` wraps these CLI calls and returns typed Go structs. Do not parse `.journal` files for accounting logic in Go.

### Write Flow

All writes go through `internal/txlock/` — see `internal/txlock/CLAUDE.md`. `txlock.Do()` serializes mutations, snapshots journal files in memory, runs `hledger check`, reverts on failure, bumps the generation counter, and (when configured as in `floatd`) asks `internal/gitsnap/` to commit a snapshot.

### Query Cache (`internal/cache/`)

The cache sits between `internal/server/ledger` handlers and `internal/hledger`. Entries are grouped by `txlock.TxLock.Generation`; any successful write bumps the generation and effectively invalidates the whole cache. `sync.RWMutex` plus `singleflight` prevents duplicate concurrent hledger invocations for the same key and generation.

### Journal File Organization

```text
data/
├── main.journal          # include directives
├── accounts.journal      # account declarations
├── prices.journal        # P directives for commodity prices (auto-created)
├── rules/                # hledger CSV rules files per bank profile
├── rules.json            # float categorization rules (post-import / bulk apply)
├── 2026/01.journal       # transactions grouped by month
└── config.toml           # server, bank profiles, Stripe, Alpha Vantage, AI, timezone, users
```

`internal/journal/` handles text-level file manipulation only. Every float-written transaction gets an 8-character code field, e.g. `(a1b2c3d4)`, and lookups use `code:a1b2c3d4`. Hidden float metadata is stored in tags with the `float-` prefix and is filtered from user-facing APIs.

### API Layer

ConnectRPC powers all clients. Protobufs live in `proto/float/v1/` and generated code is produced with Buf. `LedgerService` currently includes:

- Core queries: transactions (with pagination), balances (raw or valued), account register, accounts, tags, payees, net worth, income statement, portfolio holdings/timeseries.
- Mutations: add/update/delete transactions, status/date changes, bulk edit/delete, account declarations, account rename, prices, Alpha Vantage price backfill, snapshots.
- Imports/rules: bank profile CRUD, CSV preview/import with import batches and original file storage, categorization rule CRUD, preview/apply rules.
- Integrations/settings/debug: Stripe Financial Connections, daily Stripe auto-import toggles, AI helper RPCs via OpenRouter, Alpha Vantage API key, timezone, raw hledger query, server log stream.

### Authentication

**Not yet implemented** — the server currently runs with open access. The config schema still has users with `admin`/`viewer` roles and passphrase hashes, but there is no active JWT interceptor or `AuthService` yet.

### Web UI and TUI

The web UI is React + Vite + TanStack + shadcn/ui under `web/` and is embedded in `floatd` through `internal/webui`. The TUI under `cmd/float/ui` is a Bubble Tea app with tabs for home, accounts, trends, portfolio, monthly income/expense, balance assertions, bulk management, and settings.

## Go Practices

- Return typed structs from `internal/hledger/`, never raw JSON or `[]byte` from production APIs.
- Integration tests in `internal/hledger/` run real hledger against `testdata/` fixtures.
- Use `internal/txlock/` for every mutation — never write journal files outside `txlock.Do()`.
- `internal/gitsnap/` uses go-git (pure Go); do not shell out to `git` in production code.
- Use `t.Context()` in tests, not `context.Background()`.
- Prefer table-driven tests for functions with multiple input/output variants.

## Web UI Screenshots

Playwright in `web/` captures screenshots with mocked API data, so no live `floatd` is required. Use the `web-screenshots` skill or `cd web && bun run screenshots`. Config: `web/playwright.config.js`, `web/tests/*.spec.js`, `web/tests/mock-api.js`. `@playwright/test` is pinned to **1.56.1** — do not upgrade without matching the system Chromium.
