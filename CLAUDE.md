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
buf generate
```

Tool versions are managed by `mise`. Run `mise install` to get pinned versions of Go, buf, golangci-lint, and hledger.

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

**Adding a new command:** create a new file per group (e.g. `journal.go`), register via `init()` — do not edit `main.go` or `registry.go`.

## Go Practices for This Repo

- Return typed structs from `internal/hledger/`, never raw JSON or `[]byte`
- Integration tests in `internal/hledger/` run real hledger against fixture `.journal` files in `testdata/`
- Use `internal/txlock/` for every mutation — never write to journal files outside of `txlock.Do()`
- `internal/gitsnap/` uses go-git (pure Go); no dependency on a git binary
- `atomic.Uint64` for the generation counter; `sync.RWMutex` + `singleflight` for cache concurrency
- `floatd` validates hledger version on startup via `hledger --version` and exits with a clear error if unsupported
- In tests, always use `t.Context()` instead of `context.Background()`
