# float — Personal Finance Manager

A self-hostable personal finance manager built on top of hledger's plain-text accounting format. float acts as a management and UX layer, delegating all accounting math and validation to hledger.

## Core Principles

- **Don't reinvent the wheel.** Use hledger for all accounting logic, parsing, validation, and reporting.
- **Plain text is the source of truth.** `.journal` files on disk, version-controlled with git.
- **Single binary.** `floatd` (server) and `float` (CLI) are the only artifacts. The server embeds the web UI.

## Architecture

### Tech Stack

- **Language:** Go
- **API:** gRPC with ConnectRPC (supports gRPC, gRPC-Web, and Connect protocols — no Envoy proxy needed)
- **Protobuf management:** Buf
- **Web frontend:** SvelteKit or React (TBD), using grpc-web via ConnectRPC
- **Accounting engine:** hledger (shelled out to, never reimplemented). A specific version is pinned in the Dockerfile and in `mise.toml` for local dev. On startup, `floatd` runs `hledger --version`, parses the result, and exits with a clear error if the version is unsupported.
- **Git integration:** go-git (pure Go, no git binary dependency)

### Project Structure

```
float/
├── proto/                          # Protobuf definitions (shared types)
│   └── float/v1/
│       ├── ledger.proto            # Transaction, Account, Balance messages & service
│       ├── import.proto            # Import service
│       ├── rules.proto             # Rules service
│       └── auth.proto              # Auth service (Login, ChangePassphrase)
├── cmd/
│   ├── floatd/                     # Server binary (gRPC + embedded web)
│   │   └── main.go
│   └── float/                      # CLI binary (gRPC client)
│       └── main.go
├── internal/
│   ├── server/                     # gRPC/Connect service implementations
│   │   ├── auth/                   # AuthService handler
│   │   ├── ledger/                 # LedgerService handler
│   │   ├── import/                 # ImportService handler
│   │   └── rules/                  # RulesService handler
│   ├── auth/                       # Passphrase hashing, session token issuing/validation
│   ├── cache/                      # Generation-counter query cache + LRU + pre-warm
│   ├── config/                     # config.toml parsing, bank profiles, user model
│   ├── gitsnap/                    # Git commit/list/restore via go-git
│   ├── hledger/                    # hledger CLI wrapper (shell out + parse JSON)
│   ├── journal/                    # Journal file read/write (text manipulation, not accounting)
│   ├── rules/                      # Rule management + retroactive application
│   ├── importer/                   # Import orchestration (delegates to hledger import)
│   └── txlock/                     # Write-flow mutex + validate + revert
├── web/                            # Frontend SPA
│   └── src/
├── buf.yaml
├── go.mod
└── Dockerfile
```

### Data Directory

```
data/
├── .git/                           # Auto-managed by floatd
├── main.journal                    # Top-level file with include directives
├── accounts.journal                # Account declarations
├── rules/                          # hledger CSV rules files
│   ├── chase-checking.rules
│   └── amex-gold.rules
├── 2026/
│   ├── 01.journal                  # Transactions by month
│   ├── 02.journal
│   └── ...
├── float.key                       # Server-generated HMAC signing secret (256-bit)
└── config.toml                     # float config (bank profiles, users, etc.)
```

`main.journal` is simply:
```
include accounts.journal
include 2026/*.journal
```

## Journal File Management

### Transaction IDs

Every transaction managed by float carries a transaction code — a short unique identifier minted at creation time, stored in hledger's native code field:

```journal
2026/02/15 (a1b2c3d4) AMAZON MARKETPLACE
    expenses:shopping    $25.00
    assets:checking
```

- IDs are generated for every write path: import, manual entry, split, etc.
- Format: first 8 characters of a UUID4, keeping journal files human-readable.
- hledger natively supports code queries (`hledger reg code:a1b2c3d4`), and exposes the code as `tcode` in JSON output.
- On first startup against a pre-existing journal, float runs a one-time migration pass to add codes to all untagged transactions and convert any legacy `; fid:` tags to code fields.

### Write Flow

Every mutation follows the same pattern:

1. Acquire an in-process `sync.Mutex`
2. Write changes to the journal file(s), minting codes for any new transactions
3. Run `hledger check -f main.journal` to validate
4. If invalid: revert file changes, return error
5. Git commit with a descriptive message (see Git Snapshots below)
6. Increment the query cache generation counter
7. Release lock

### Git Snapshots

Git commits are **per API operation**, not periodic. Every successful write through the mutex produces exactly one commit:

- `AddTransaction` → `"add: AMAZON MARKETPLACE 2026-02-15"`
- `ImportCSV` → `"import: 47 transactions from Chase Checking"`
- `ApplyRuleRetroactively` → `"rule: recategorize 12 txns to expenses:shopping"`
- `DeleteTransaction` → `"delete: AMAZON MARKETPLACE 2026-02-15 (a1b2c3d4)"`

This gives every mutation a clean, instantly-reversible snapshot. Bulk operations (imports, retroactive rules) are atomic — one commit for the entire batch.

On server startup: check for uncommitted changes and commit them as a recovery snapshot.

Snapshots can be listed and restored via the gRPC API (`ListSnapshots`, `RestoreSnapshot`). Users can also manually `cd data/ && git log` since it's a standard git repo.

`RestoreSnapshot` performs a hard reset to the target commit, intentionally discarding all subsequent history. This is the intended behavior — the feature is exposed to users by timestamp and description ("Revert to March 15 at 2:04 PM"), not by git hash. Users interact with it as a point-in-time restore rather than a git operation; the destructive semantics are a feature.

Performance: `git add + commit` adds ~50-100ms per write, which is negligible for a personal finance app where writes are infrequent. Plain-text journals compress extremely well in git packfiles, so repo size stays small even with years of per-edit history.

## Import Pipeline

float uses hledger for CSV parsing (column mapping, date format conversion, categorization via rules files), but handles deduplication, file routing, and code-field tagging itself.

### hledger Rules Files

Each bank/source gets a `.rules` file that defines column mappings and categorization rules:

```
# rules/chase-checking.rules
skip 1
fields date, description, amount
date-format %m/%d/%Y
account1 assets:checking

if Amazon
  account2 expenses:shopping

if PAYROLL
  account2 income:salary
```

### Import Flow

1. User profides a CSV file and selects a bank profile (or float auto-detects based on CSV headers)
2. float runs `hledger print --rules-file <profile>.rules input.csv` — this parses the CSV and prints categorized journal entries to stdout without touching any journal file or writing `.latest` tracking files
3. float parses the stdout output into individual transactions
4. **Dedup pass**: compare candidates against existing transactions (see Duplicate Detection below)
5. User reviews the preview in the UI/CLI — net-new transactions are shown, potential duplicates are flagged separately for confirmation
6. On confirmation, float groups transactions by month and appends to the correct `YYYY/MM.journal` files, minting transaction codes and `import` tags (see below)
7. If the year directory or month file doesn't exist yet, float creates it and adds the necessary `include` directive to `main.journal`
8. Validate with `hledger check`, git commit (`"import: 47 transactions from Chase Checking"`)

### Duplicate Detection

hledger's built-in `.latest` file mechanism only tracks the latest imported date per source, which breaks for out-of-order imports, corrected CSVs, and overlapping bank exports. float replaces this with its own dedup strategy:

- **Content fingerprint**: For each candidate transaction, compute a fingerprint from `(date, normalized_description, amount, account)`. Check if any existing transaction has a matching fingerprint.
- **Import source tag**: Each imported transaction gets an `import` tag recording its origin: `; import:chase-checking-2026-03-15.csv:42` (source filename + row number). Re-importing the same CSV detects already-imported rows precisely.
- **Legitimate duplicates**: Two identical $5.00 Amazon charges on the same day are real transactions. Rather than silently deduplicating, float flags these as *potential* duplicates in the preview and lets the user confirm or skip. The `fid` tags on existing transactions make this unambiguous — if a transaction already has a `fid`, it was already imported through float.

Example of a fully-tagged imported transaction:

```journal
2026/02/15 AMAZON MARKETPLACE  ; fid:a1b2c3d4, import:chase-checking-2026-03-15.csv:42
    expenses:shopping    $25.00
    assets:checking
```

### Bank Profiles

A bank profile is a mapping of a friendly name to a `.rules` file:

```toml
# config.toml
[[bank_profiles]]
name = "Chase Checking"
rules_file = "rules/chase-checking.rules"
account = "assets:checking"
```

## Rules Engine

The rules engine has two responsibilities: managing hledger `.rules` files for future imports, and retroactively applying new rules to existing transactions.

### Future Imports

When a user adds a new categorization rule, it's written into the appropriate `.rules` file so that `hledger import` picks it up automatically.

### Retroactive Application

This is the main piece of custom logic in float. When a user adds a rule and wants to apply it to past transactions:

1. Use `hledger reg -O json` (or similar) to find existing transactions matching the rule's pattern
2. Show a preview: "14 transactions would be re-categorized from `expenses:unknown` → `expenses:shopping`"
3. On confirmation, perform text-level modifications to the journal files:
   - Look up each matching transaction by its `fid` tag (unique and unambiguous)
   - Modify the relevant posting lines (change account, add tags, add postings)
4. Validate the result with `hledger check`

Example CLI flow:
```
$ float rules add --match "Amazon" --account "expenses:shopping"
Rule saved to chase-checking.rules
14 existing transactions match. Apply retroactively? [y/n] y
  2026/02/15 AMAZON MARKETPLACE  expenses:unknown → expenses:shopping
  2026/02/18 AMAZON PRIME        expenses:unknown → expenses:shopping
  ...
Applied. Validation passed.
```

## Querying & Reporting

All queries delegate to hledger:

- **Balances:** `hledger bal -O json`
- **Transaction list:** `hledger reg -O json`
- **Account tree:** `hledger accounts --tree`
- **Income/expense reports:** `hledger is -O json`, `hledger bs -O json`

float parses the JSON output and serves it through the gRPC API. The CLI and web UI consume the same API.

### Query Cache

Shelling out to hledger on every read re-parses all journal files from disk, which gets slow as history grows. float uses a **generation-counter cache** to avoid redundant work.

#### Generation Counter

An atomic `generation uint64` counter is the sole invalidation mechanism. It increments after every successful write (step 6 of the write flow). Since all writes go through the `sync.Mutex`, there's no need for file-watching or partial invalidation — every write bumps the generation, and the entire cache is treated as stale.

#### Cache Keys & Storage

Each cache entry stores the parsed, protobuf-ready Go structs (not raw JSON bytes), so a cache hit skips both the hledger subprocess and the JSON-to-protobuf conversion.

The cache key is a hash of the normalized hledger command, including all flags and filters. Examples:

| gRPC call | hledger command | Cache key (conceptual) |
|---|---|---|
| `GetBalances()` | `hledger bal -O json` | `hash("bal -O json")` |
| `GetBalances(depth=2)` | `hledger bal -O json --depth 2` | `hash("bal -O json --depth 2")` |
| `ListTransactions(account="expenses")` | `hledger reg -O json expenses` | `hash("reg -O json expenses")` |
| `ListTransactions(date="2026-03")` | `hledger reg -O json date:2026-03` | `hash("reg -O json date:2026-03")` |

Each unique parameter combination produces a distinct cache entry. A cache lookup checks: does an entry exist for this key at the current generation? If yes, return it. If no, shell out to hledger, store the result at the current generation, and return it.

#### Concurrency

Multiple gRPC handlers may read concurrently. The cache uses a `sync.RWMutex`:

- **Cache hit:** acquire read lock, check generation + key, return cached struct. No contention between concurrent readers.
- **Cache miss:** use `golang.org/x/sync/singleflight` to deduplicate concurrent misses on the same key — only one goroutine shells out to hledger while others wait and share the result. Once the result is returned, acquire a brief write lock only to store it. No readers are blocked during the hledger subprocess itself.

The generation counter itself is `atomic.Uint64`, so readers can do a cheap check before acquiring any lock — if the generation matches, it's worth checking the cache; if not, the cache is known-stale and can be skipped entirely.

#### Pre-Warming

After a write invalidates the cache, a background goroutine pre-warms the queries most likely to be needed next:

1. **Account tree** (`hledger accounts --tree`) — used by navigation in the web UI
2. **Top-level balances** (`hledger bal -O json --depth 1`) — used by the dashboard
3. **Current month transactions** (`hledger reg -O json date:thismonth`) — used by the default transaction list view

Pre-warming runs asynchronously and does not block the write path. If a read arrives before pre-warming completes, it simply triggers a normal cache miss and shells out to hledger itself. The pre-warm goroutine checks the generation before storing its result — if another write happened in the meantime, it discards the now-stale result rather than caching it.

#### Memory Limits

For a personal finance app, total cached data is small (a few MB of parsed structs for years of history). As a safety net, the cache enforces a max entry count (default: 128). If the limit is reached, the least-recently-used entry is evicted. In practice this is rarely triggered — most users will hit the same handful of queries repeatedly.

#### Design Rationale

This is deliberately simple. Full cache invalidation on every write is the right trade-off because:

- Writes are infrequent (a handful per day, plus occasional bulk imports).
- Partial invalidation (e.g., "this write only affects March 2026 transactions") would require understanding hledger's accounting semantics, which violates the core principle of delegating all accounting logic to hledger.
- The pre-warming step ensures the most common queries are always fast despite full invalidation.

## Authentication

float is **single-tenant**: one data directory, one journal, all authenticated users see everything. This matches the reality of a personal finance app — one person (or household) per instance.

### Dependencies

Only two dependencies are needed, both well-established:

- **`golang.org/x/crypto/argon2`** — argon2id passphrase hashing (part of the official Go sub-repos)
- **`github.com/golang-jwt/jwt/v5`** — HMAC-SHA256 session tokens with expiry and claims

Everything else (interceptor, cookie handling, role checking) is vanilla Go.

### User Model

Two roles, configured in `config.toml`:

```toml
[[users]]
name = "alice"
passphrase_hash = "..."   # argon2id hash, set via `float setup` or `float user add`
role = "admin"

[[users]]
name = "bob"
passphrase_hash = "..."
role = "viewer"
```

| Role | Can query | Can import/edit | Can manage rules | Can change settings |
|---|---|---|---|---|
| **admin** | ✓ | ✓ | ✓ | ✓ |
| **viewer** | ✓ | ✗ | ✗ | ✗ |

### Setup Flow

1. On first startup, `floatd` generates a random 256-bit secret and writes it to `data/float.key`. This key is used to sign all session tokens.
2. The user creates the initial admin account via the CLI: `float setup --user alice --passphrase`. The passphrase is hashed with argon2id and stored in `config.toml`.

### Login Flow

1. Client sends `Login(username, passphrase)` via the `AuthService` RPC
2. Server verifies the passphrase against the stored argon2id hash
3. Server issues an HMAC-SHA256 JWT signed with `float.key`, containing claims: `sub` (username), `role`, `exp` (expiry)
4. Token is returned as:
   - A `Set-Cookie` header (`httponly`, `secure`, `samesite=strict`) for the web UI
   - A plain token in the response body for the CLI

Token expiry defaults to 30 days, configurable in `config.toml`.

### Request Validation

A ConnectRPC `UnaryInterceptorFunc` runs on every request except `AuthService/Login`:

1. Extract the token from the `Authorization: Bearer <token>` header (CLI/API) or the session cookie (web UI)
2. Verify the HMAC signature and check expiry
3. Inject the username and role into the request context
4. For mutating RPCs (`AddTransaction`, `ImportCSV`, `ApplyRule`, etc.), reject if role is `viewer`

### CLI Authentication

`float login` prompts for username and passphrase, stores the returned token in `~/.config/float/token`. Subsequent commands send it as a `Bearer` header automatically. If a request returns `Unauthenticated`, the CLI prompts to re-login.

### Proto Definition

```protobuf
// proto/float/v1/auth.proto
service AuthService {
  rpc Login(LoginRequest) returns (LoginResponse);
  rpc ChangePassphrase(ChangePassphraseRequest) returns (ChangePassphraseResponse);
}
```

## Nice-to-Haves (Future)

- **Encryption at rest:** Age-encrypt journal files on disk, decrypt in-memory at runtime
- **Split transaction UI:** Interactive UI for splitting a single transaction into multiple postings
- **Profile auto-detection:** Sniff CSV headers to guess which bank profile to use
- **Recurring transaction templates**
- **Budget tracking**

## Deployment

- Single `Dockerfile` that bundles the Go binary, embedded web assets, and a pinned hledger binary
- `docker-compose.yml` with a volume mount for the data directory
- Config via environment variables or `config.toml`
