# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

`float` is a self-hostable personal finance manager that wraps [hledger](https://hledger.org/) — a plain-text accounting tool. float provides the UX layer (gRPC API, CLI, web UI) and delegates all accounting math, parsing, and validation to hledger. Never reimplement accounting logic that hledger already handles.

Three binaries:
- `floatd` — gRPC server (embeds web UI)
- `float` — CLI gRPC client
- `floatctl` — admin/debug CLI that bypasses gRPC and operates directly on internal packages and the data directory

## Commands

```bash
# Run all tests
mise run test
# or: go test ./...

# Run a single package's tests
go test ./internal/hledger/

# Lint
mise run lint
# or: golangci-lint run ./...

# Lint + test
mise run check

# Generate protobuf code (after editing .proto files)
mise run proto-gen
# or: buf generate

# Generate JS protobuf client (after editing .proto files)
mise run web-gen
# or: cd web && npx buf generate --template buf.gen.yaml ../proto

# Build web UI for production (outputs to internal/webui/dist/)
mise run web-build
# or: cd web && npm run build

# Build web UI + compile floatd
mise run build

# Start Vite dev server for web UI (HMR, proxies API to floatd)
mise run web-dev
```

Tool versions are managed by `mise`. Run `mise install` to get pinned versions of Go, buf, golangci-lint, and hledger.

## Running floatd

```bash
# Start the server (uses $VAULT_DATA_DIR by default)
mise run floatd

# Custom data dir or address
FLOAT_DATA_DIR=/path/to/data mise run floatd
FLOAT_ADDR=:9090 mise run floatd
```

`floatd` requires a `config.toml` in the data directory and a `main.journal` file.

### Web UI Development

The web UI is a Preact SPA in `web/` that gets embedded into the `floatd` binary via `go:embed`. For development with hot reload:

1. Terminal 1: `mise run floatd` (Go server on :8080)
2. Terminal 2: `mise run web-dev` (Vite dev server on :5173, proxies API calls to :8080)

For production: `mise run build` builds the web UI into `internal/webui/dist/` and compiles `floatd` with the embedded files. The web UI uses the Connect protocol (JSON over HTTP POST) to call the same gRPC endpoints.

### Web UI Screenshots (Playwright)

Playwright is set up in `web/` to capture screenshots without a running `floatd`. All `LedgerService` API calls are intercepted with mock data.

```bash
# Capture screenshots of all pages
cd web && npm run screenshots

# Output files:
#   web/test-results/home.png
#   web/test-results/transactions.png
#   web/test-results/add-transaction.png
```

Use the `web-screenshots` skill to run tests and display screenshots inline in a Claude Code session. Key files:

| File | Purpose |
|------|---------|
| `web/playwright.config.js` | Playwright config; starts Vite on port 5174 |
| `web/tests/screenshots.spec.js` | Screenshot tests for each page |
| `web/tests/mock-api.js` | Mock data + Connect RPC interception |

`@playwright/test` is pinned to **1.56.1** to match the system-installed Chromium at `/root/.cache/ms-playwright/chromium-1194`. Do not upgrade it without also installing the matching browser.

## Querying floatd with buf curl

`floatd` supports three protocols: **gRPC** (HTTP/2), **gRPC-Web**, and **Connect** (HTTP/1.1 or HTTP/2). The `buf curl` tool is the primary way to send test requests.

All `buf curl` commands must be run from the repo root (so `--schema .` can find `proto/`). The `--http2-prior-knowledge` flag is required for gRPC over plain HTTP (no TLS).

```bash
# Get balances (all accounts)
buf curl --schema . --protocol grpc --http2-prior-knowledge \
  http://localhost:8080/float.v1.LedgerService/GetBalances \
  --data '{}'

# Get balances at depth 1 (top-level accounts only)
buf curl --schema . --protocol grpc --http2-prior-knowledge \
  http://localhost:8080/float.v1.LedgerService/GetBalances \
  --data '{"depth": 1}'

# Get balances filtered to a specific account subtree
buf curl --schema . --protocol grpc --http2-prior-knowledge \
  http://localhost:8080/float.v1.LedgerService/GetBalances \
  --data '{"query": ["expenses"]}'

# List transactions (all)
buf curl --schema . --protocol grpc --http2-prior-knowledge \
  http://localhost:8080/float.v1.LedgerService/ListTransactions \
  --data '{}'

# List transactions with hledger query filters
buf curl --schema . --protocol grpc --http2-prior-knowledge \
  http://localhost:8080/float.v1.LedgerService/ListTransactions \
  --data '{"query": ["date:2026-01", "expenses"]}'

# List accounts
buf curl --schema . --protocol grpc --http2-prior-knowledge \
  http://localhost:8080/float.v1.LedgerService/ListAccounts \
  --data '{}'
```

**mise shortcuts** (wrap the above with `FLOAT_ADDR` and `FLOAT_QUERY` env vars):

```bash
mise run grpc-balances
mise run grpc-transactions
mise run grpc-accounts

# With options
FLOAT_DEPTH=1 mise run grpc-balances
FLOAT_QUERY=expenses mise run grpc-balances
FLOAT_QUERY="date:2026-01" mise run grpc-transactions
FLOAT_ADDR=localhost:9090 mise run grpc-accounts
```

**Using the Connect protocol** (works with HTTP/1.1, no `--http2-prior-knowledge` needed):

```bash
buf curl --schema . \
  http://localhost:8080/float.v1.LedgerService/GetBalances \
  --data '{}'
```

## Architecture

### Core Constraint: hledger Is the Accounting Engine

float shells out to hledger for everything accounting-related: parsing journal files, computing balances, validating journal correctness, generating reports, and importing CSVs. The `internal/hledger/` package wraps these CLI calls. Never parse `.journal` files for accounting logic in Go — that's hledger's job.

### Write Flow (Every Mutation)

All writes go through `internal/txlock/` which enforces:
1. Acquire `sync.Mutex`
2. Execute the file write (caller-provided fn)
3. Run `hledger check` — if invalid, revert files and return error
4. Git commit via `internal/gitsnap/`
5. Bump atomic generation counter (cache invalidation)
6. Release mutex

### Query Cache (`internal/cache/`)

Cache sits between `LedgerService` handlers and the hledger wrapper. Cache key = hash of the normalized hledger command + flags. Invalidation is full (entire cache) on every generation bump — no partial invalidation. `sync.RWMutex` for concurrent reads; `singleflight` to deduplicate concurrent misses on the same key. 128-entry LRU as a safety net.

### Journal File Organization

```
data/
├── main.journal          # include directives only
├── accounts.journal      # account declarations
├── rules/                # hledger CSV rules files per bank
├── 2026/01.journal       # transactions grouped by month
└── config.toml           # bank profiles, users
```

`internal/journal/` handles text-level journal file manipulation (appending transactions, creating year/month files, updating include directives). It does not understand accounting semantics.

Every transaction written by float gets a `fid` tag — an 8-char UUID prefix — for unambiguous lookup: `; fid:a1b2c3d4`. hledger's native tag query (`tag:fid=a1b2c3d4`) handles lookups.

### API Layer

gRPC via ConnectRPC (supports gRPC, gRPC-Web, and Connect protocols — no Envoy needed). Protobufs defined in `proto/float/v1/`, generated with Buf. Services:
- `LedgerService` — read/write transactions, balances, accounts, snapshots
- `ImportService` — CSV import preview + confirm
- `RulesService` — manage hledger rules files + retroactive application
- `AuthService` — login, change passphrase

### Authentication

Single-tenant. Two roles: `admin` (full access) and `viewer` (read-only). JWT (HMAC-SHA256) signed with `data/float.key`. ConnectRPC interceptor validates tokens on every RPC except `AuthService/Login`. Passphrases hashed with argon2id.

### `floatctl` — Admin/Debug CLI

`floatctl` is distinct from `float` (the end-user gRPC client). It bypasses the API entirely and is used for admin tasks, data migrations, and debugging. See `cmd/floatctl/CLAUDE.md` for the full command reference.

```
floatctl <group> <subcommand> [flags] [args...]
floatctl help
floatctl <group> help
```

**Current commands (`hledger` group):**

| Command | Description |
|---------|-------------|
| `floatctl hledger balance <journal> [query...]` | Run `hledger bal`, print as JSON |
| `floatctl hledger accounts <journal>` | Run `hledger accounts`, print tree as JSON |
| `floatctl hledger register <journal> [query...]` | Run `hledger reg`, print as JSON |
| `floatctl hledger print-csv <csv> <rules>` | Parse CSV via rules, print transactions as JSON |
| `floatctl hledger version` | Print hledger binary version |
| `floatctl hledger check <journal>` | Validate journal; exit 0 if valid |

**Current commands (`journal` group):**

| Command | Description |
|---------|-------------|
| `floatctl journal add <data-dir> --description <text> --posting "account  amount" [--posting ...]` | Add a transaction via txlock |
| `floatctl journal delete <data-dir> <fid>` | Delete a transaction by fid via txlock |
| `floatctl journal import <data-dir> <csv> --profile <name> [--yes]` | Preview and import a CSV using a bank profile's rules |
| `floatctl journal verify <data-dir>` | Run `hledger check`; print `ok` or error |
| `floatctl journal lookup <data-dir> <fid>` | Look up a transaction by fid, print as JSON |
| `floatctl journal stats <data-dir>` | Print journal statistics as JSON |
| `floatctl journal audit <data-dir>` | Check include integrity, FID uniqueness, orphaned files |
| `floatctl journal migrate-fids <data-dir>` | Add fid tags to any untagged transactions |
| `floatctl journal list-files <data-dir>` | List all `.journal` files under the data directory |

**Adding a new command:** create a new file per group (e.g. `journal.go`), register via `init()` — do not edit `main.go` or `registry.go`.

## Go Practices for This Repo

- Return typed structs from `internal/hledger/`, never raw JSON or `[]byte`
- Integration tests in `internal/hledger/` run real hledger against fixture `.journal` files in `testdata/`
- Use `internal/txlock/` for every mutation — never write to journal files outside of `txlock.Do()`
- `internal/gitsnap/` uses go-git (pure Go); no dependency on a git binary
- `atomic.Uint64` for the generation counter; `sync.RWMutex` + `singleflight` for cache concurrency
- `floatd` validates hledger version on startup via `hledger --version` and exits with a clear error if unsupported
- In tests, always use `t.Context()` instead of `context.Background()`
- Prefer table-driven tests (`tests := []struct{...}{...}` + `for _, tc := range tests { t.Run(tc.name, ...) }`) for any function with multiple input/output variants; keep standalone test functions only for cases that require unique setup or fundamentally different structure
