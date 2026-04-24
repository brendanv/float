# CLAUDE.md

`float` is a self-hostable personal finance manager wrapping [hledger](https://hledger.org/). float provides the UX layer (gRPC API, CLI, web UI) and delegates all accounting math, parsing, and validation to hledger. **Never reimplement accounting logic that hledger already handles.**

Three binaries: `floatd` (gRPC server + embedded web UI), `float` (TUI gRPC client), `floatctl` (admin/debug CLI, bypasses gRPC).

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

Tool versions managed by `mise`. Run `mise install` to get pinned Go, buf, golangci-lint, hledger.

## Running floatd

```bash
mise run floatd
FLOAT_DATA_DIR=/path/to/data mise run floatd
FLOAT_ADDR=:9090 mise run floatd
```

Requires `config.toml` and `main.journal` in the data directory. For web UI development run `mise run floatd` and `mise run web-dev` concurrently.

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

`floatd` supports gRPC, gRPC-Web, and Connect protocols. Use `buf curl` from the repo root:

```bash
buf curl --schema . --protocol grpc --http2-prior-knowledge \
  http://localhost:8080/float.v1.LedgerService/GetBalances --data '{}'
```

mise shortcuts: `mise run grpc-balances`, `mise run grpc-transactions`, `mise run grpc-accounts`, `mise run grpc-delete`, `mise run grpc-modify-tags`. Supports `FLOAT_DEPTH`, `FLOAT_QUERY`, `FLOAT_ADDR` env vars.

## Architecture

### Core Constraint: hledger Is the Accounting Engine

float shells out to hledger for all accounting: parsing journals, computing balances, validating correctness, generating reports, importing CSVs. `internal/hledger/` wraps these CLI calls. Never parse `.journal` files for accounting logic in Go.

### Write Flow

All writes go through `internal/txlock/` — see `internal/txlock/CLAUDE.md`. When `lock.SetSnap(snap)` is configured (as in `floatd`), txlock automatically commits to git after each successful write.

### Query Cache (`internal/cache/`)

Sits between `LedgerService` handlers and the hledger wrapper. Entries are grouped by generation (from `txlock.TxLock.Generation`); any write bumps the generation and effectively invalidates the entire cache — no partial invalidation. `sync.RWMutex` + `singleflight` prevent concurrent duplicate hledger invocations for the same key.

### Journal File Organization

```
data/
├── main.journal          # include directives only
├── accounts.journal      # account declarations
├── prices.journal        # P directives for commodity prices (auto-created)
├── rules/                # hledger CSV rules files per bank (for import parsing)
├── rules.json            # float categorization rules (auto-categorization after import)
├── 2026/01.journal       # transactions grouped by month
└── config.toml           # bank profiles, users
```

`internal/journal/` handles text-level file manipulation (no accounting semantics). See `internal/journal/CLAUDE.md`.

Every float-written transaction gets a code field (8-char UUID prefix): `(a1b2c3d4)`. Lookups use `code:a1b2c3d4`.

### API Layer

ConnectRPC (gRPC, gRPC-Web, Connect — no Envoy needed). Protobufs in `proto/float/v1/`, generated with Buf. One service: `LedgerService` (query, write, bulk edit, snapshots, prices).

### Authentication

**Not yet implemented** — the server currently runs with open access. The config schema supports users with `admin`/`viewer` roles and argon2id passphrase hashes, but the JWT interceptor and `AuthService` are planned for a future step (see `PLAN.md` Step 8).

### floatctl

Admin/debug CLI that bypasses the API. See `cmd/floatctl/CLAUDE.md` for commands and how to add new ones.

## Go Practices

- Return typed structs from `internal/hledger/`, never raw JSON or `[]byte`
- Integration tests in `internal/hledger/` run real hledger against `testdata/` fixture files
- Use `internal/txlock/` for every mutation — never write to journal files outside `txlock.Do()`
- `internal/gitsnap/` uses go-git (pure Go); no git binary dependency
- Use `t.Context()` in tests, not `context.Background()`
- Prefer table-driven tests for functions with multiple input/output variants

## Web UI Screenshots

Playwright in `web/` captures screenshots with mocked API data (no live floatd needed). Use the `web-screenshots` skill or `cd web && bun run screenshots`. Config: `web/playwright.config.js`, `web/tests/screenshots.spec.js`, `web/tests/mock-api.js`. `@playwright/test` is pinned to **1.56.1** — do not upgrade without matching the system Chromium.
