# Step 2: Config & Journal Management — Implementation Plan

## Context

Step 1 (`internal/hledger/`) is complete and provides the accounting engine wrapper. Step 2 builds the two supporting packages that everything else will depend on:

- **`internal/config/`** — Parse/save `data/config.toml`: bank profiles, user accounts (for auth later), server settings.
- **`internal/journal/`** — Text-level journal file manipulation: format and append transactions, create `YYYY/MM.journal` files, maintain `main.journal` include directives, mint `fid` tags, migrate existing transactions to add missing `fid` tags.

These packages are intentionally simple: config is pure TOML I/O, journal is mostly text manipulation (no accounting logic — hledger remains the accounting engine), with the exception that transaction formatting delegates to `hledger print` for canonical output.

**Testable artifact:** `go test ./internal/config/ ./internal/journal/` — unit tests using temp dirs and fixture files. Config tests require no binary. Journal formatting and append tests require a real hledger binary (integration tests, similar to Step 1).

---

## Files to Create

```
internal/
├── config/
│   ├── config.go           # Types + Load() + Save()
│   ├── config_test.go      # Unit tests
│   └── testdata/
│       └── config.toml     # Fixture for tests
└── journal/
    ├── fid.go              # MintFID()
    ├── format.go           # TransactionInput, PostingInput, Format()
    ├── files.go            # EnsureMonthFile(), UpdateMainIncludes(), AppendTransaction()
    ├── migrate.go          # MigrateFIDs()
    └── journal_test.go     # Unit tests (all journal sub-packages)
```

---

## Journal Path Convention

All journal file paths are relative to a `dataDir` runtime parameter. There is no compiled-in path. The caller (eventually `floatd`) passes `dataDir` as a `--data-dir` CLI flag (defaults to `./data`). `config.toml` lives at `filepath.Join(dataDir, "config.toml")`.

## Dependencies to Add (go.mod)

```
github.com/BurntSushi/toml v1.4.0   # TOML parsing/encoding
github.com/google/uuid v1.6.0       # UUID for FID generation
```

Run `go get github.com/BurntSushi/toml github.com/google/uuid && go mod tidy` after creating go.mod entries.

---

## Sub-Step 2.1 — Add Dependencies

```bash
cd /home/user/float
go get github.com/BurntSushi/toml@latest
go get github.com/google/uuid@latest
go mod tidy
```

---

## Sub-Step 2.2 — Config Package (`internal/config/config.go`)

### Types

```go
package config

type ServerConfig struct {
    Port int `toml:"port"` // default 8080 if zero
}

type User struct {
    Name           string `toml:"name"`
    Role           string `toml:"role"` // "admin" or "viewer"
    PassphraseHash string `toml:"passphrase_hash"`
}

type BankProfile struct {
    Name      string `toml:"name"`
    RulesFile string `toml:"rules_file"` // relative to data dir
}

type Config struct {
    Server       ServerConfig  `toml:"server"`
    Users        []User        `toml:"users"`
    BankProfiles []BankProfile `toml:"bank_profiles"`
}
```

### Functions

```go
// Load parses config.toml at path and returns a *Config.
// Returns error if the file doesn't exist or is not valid TOML.
func Load(path string) (*Config, error)

// Save encodes cfg as TOML and writes it to path (creates or overwrites).
func Save(path string, cfg *Config) error
```

**`Load` algorithm:**
1. `os.ReadFile(path)` — wrap error with path context.
2. `toml.Decode(string(data), &cfg)` — return decode error directly.
3. Return `&cfg, nil`.

**`Save` algorithm:**
1. Create or truncate file at path.
2. `toml.NewEncoder(f).Encode(cfg)`.
3. Return any error.

### Testdata (`internal/config/testdata/config.toml`)

```toml
[server]
port = 9090

[[users]]
name = "alice"
role = "admin"
passphrase_hash = "argon2id$abc123"

[[users]]
name = "bob"
role = "viewer"
passphrase_hash = "argon2id$def456"

[[bank_profiles]]
name = "Chase Checking"
rules_file = "rules/chase-checking.rules"

[[bank_profiles]]
name = "Amex"
rules_file = "rules/amex.rules"
```

### Tests (`internal/config/config_test.go`)

| Test | Setup | Assertions |
|------|-------|-----------|
| `TestLoad_Valid` | Load `testdata/config.toml` | `cfg.Server.Port == 9090`; `len(cfg.Users) == 2`; first user name "alice", role "admin"; `len(cfg.BankProfiles) == 2`; first profile name "Chase Checking" |
| `TestLoad_Missing` | Load `testdata/nonexistent.toml` | error non-nil |
| `TestLoad_InvalidTOML` | Write `"not = [valid toml"` to temp file, Load | error non-nil |
| `TestLoad_Empty` | Write `""` to temp file, Load | no error; `cfg.Users == nil`; `cfg.BankProfiles == nil`; `cfg.Server.Port == 0` |
| `TestSave_RoundTrip` | Build a Config, Save to temp file, Load back | loaded struct equals original (same port, same users, same profiles) |

---

## Sub-Step 2.2b — Add `PrintText` to hledger client (`internal/hledger/client.go`)

Add one new method to the existing hledger `Client`:

```go
// PrintText runs `hledger print -f <journalFile>` and returns the formatted
// plain-text output. Used to normalize/canonicalize transaction text before
// appending to real journal files.
func (c *Client) PrintText(ctx context.Context, journalFile string) (string, error) {
    stdout, _, err := c.run(ctx, "print", "-f", journalFile)
    if err != nil {
        return "", fmt.Errorf("hledger print: %w", err)
    }
    return string(stdout), nil
}
```

`hledger print` output characteristics (verified against testdata):
- Date format: `2026-01-05` (ISO, hyphen-separated)
- Amounts: right-aligned with padding
- FID tags preserved in transaction comment
- Blank line after each transaction

---

## Sub-Step 2.3 — FID Minting (`internal/journal/fid.go`)

```go
package journal

import "github.com/google/uuid"

// MintFID generates a random 8-character hex string using a UUID v4.
// It takes the first 8 characters of the UUID (excluding dashes).
// Example output: "a1b2c3d4"
func MintFID() string {
    id := uuid.New().String()          // "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
    clean := strings.ReplaceAll(id, "-", "")  // remove dashes
    return clean[:8]                   // first 8 hex chars
}
```

---

## Sub-Step 2.4 — Transaction Formatting (`internal/journal/format.go`)

Formatting delegates to `hledger print` rather than rolling custom alignment logic. `format.go` only provides:
1. The Go types for transaction input
2. A minimal "draft" serializer — just enough for hledger to parse (no padding/alignment)
3. A `FormatViaHledger` function that writes to a temp file and runs `hledger print`

### Types

```go
package journal

import "time"

// PostingInput represents one leg of a transaction.
type PostingInput struct {
    Account string // e.g. "expenses:shopping"
    Amount  string // e.g. "$45.00"; empty string means auto-balance posting
    Comment string // optional inline comment text (without "; " prefix)
}

// TransactionInput represents a transaction to be written.
type TransactionInput struct {
    Date        time.Time
    Description string
    Comment     string         // optional transaction-level comment (without "; " prefix)
    Postings    []PostingInput
}
```

### Functions

```go
// draftFormat renders a TransactionInput + fid as minimal hledger journal text.
// Output is valid for hledger to parse but not canonically formatted.
// Used internally as input to FormatViaHledger.
func draftFormat(tx TransactionInput, fid string) string

// FormatViaHledger writes tx to a temp file, runs `hledger print -f <tmpfile>`,
// and returns the canonical hledger-formatted output.
// The hledger client is used only to invoke `hledger print`.
func FormatViaHledger(ctx context.Context, client *hledger.Client, tx TransactionInput, fid string) (string, error)
```

**`draftFormat` output (minimal, not aligned):**
```
2026-01-15 AMAZON MARKETPLACE  ; fid:bb002200
    expenses:shopping  $45.00
    assets:checking
```

**`FormatViaHledger` algorithm:**
1. Call `draftFormat(tx, fid)` to get minimal text.
2. Write to `os.CreateTemp("", "float-txn-*.journal")`.
3. Call `client.PrintText(ctx, tmpFile)` to get canonical output.
4. Delete temp file, return canonical output.

**`hledger print` output characteristics:**
- ISO date format: `2026-01-05`
- Right-aligned amounts
- FID tags preserved
- Trailing blank line after each transaction

---

## Sub-Step 2.5 — File Management (`internal/journal/files.go`)

### Functions

```go
// EnsureMonthFile ensures data/YYYY/MM.journal exists.
// Creates the year directory and file if needed.
// Returns the relative path (e.g. "2026/01.journal") and whether newly created.
func EnsureMonthFile(dataDir string, year, month int) (relPath string, created bool, err error)

// UpdateMainIncludes adds an include directive to mainJournalPath if not already present.
// Directive format: "include RELPATH\n"
// Idempotent: if the exact directive already exists, does nothing.
func UpdateMainIncludes(mainJournalPath string, relPath string) error

// AppendTransaction writes a new transaction to the correct month file.
// It mints a FID, uses hledger print to canonically format the transaction,
// ensures the month file exists, updates main.journal if a new file was created,
// and appends the canonical text.
// Returns the assigned FID.
func AppendTransaction(ctx context.Context, client *hledger.Client, dataDir string, tx TransactionInput) (fid string, err error)
```

**`EnsureMonthFile` algorithm:**
1. `relPath = fmt.Sprintf("%04d/%02d.journal", year, month)`
2. `absPath = filepath.Join(dataDir, relPath)`
3. `os.MkdirAll(filepath.Dir(absPath), 0755)`
4. If file exists (`os.Stat`): return `relPath, false, nil`
5. Create file with header comment: `; float: YYYY/MM\n`
6. Return `relPath, true, nil`

**`UpdateMainIncludes` algorithm:**
1. `directive = "include " + relPath`
2. Read existing content with `os.ReadFile` (empty string if file not found)
3. If content already contains `directive` as a whole line: return nil
4. Append `directive + "\n"` to file with `os.WriteFile` (O_APPEND or read+rewrite)

**`AppendTransaction` algorithm:**
1. `fid = MintFID()`
2. `text, err = FormatViaHledger(ctx, client, tx, fid)` — canonical hledger output
3. `year, month = tx.Date.Year(), int(tx.Date.Month())`
4. `relPath, created, err = EnsureMonthFile(dataDir, year, month)`
5. If `created`: call `UpdateMainIncludes(filepath.Join(dataDir, "main.journal"), relPath)`
6. `os.OpenFile(absPath, O_APPEND|O_WRONLY, 0644)` and write `text`
   - Note: `hledger print` already ends with a blank line — no need to prepend one
7. Return `fid, nil`

---

## Sub-Step 2.6 — FID Migration (`internal/journal/migrate.go`)

### Function

```go
// MigrateFIDs scans all journal files included from main.journal in dataDir,
// finds transaction header lines without fid tags, and adds fid tags to them.
// Returns the count of transactions modified.
// Safe to call on an already-migrated journal (count=0, no changes).
func MigrateFIDs(dataDir string) (int, error)
```

**Algorithm:**
1. Read `data/main.journal`. Parse include directives: lines matching `^include (.+)`.
2. For each included file (relative to dataDir):
   a. Read all lines.
   b. For each line, check if it's a transaction header:
      - Pattern: `^\d{4}[/\-]\d{2}[/\-]\d{2} ` (date prefix followed by space)
      - NOT a comment line
   c. If it's a transaction header AND does not contain `fid:` in its content:
      - Append `  ; fid:XXXXXXXX` to the end of the line (mint a new FID)
      - Increment counter
   d. Write modified lines back to the file.
3. Return total count.

**FID detection:** A line contains a fid tag if it matches `fid:[0-9a-f]{8}`.

**Edge cases:**
- If main.journal doesn't exist: return 0, nil (no-op).
- If an included file doesn't exist: skip it (log-worthy but not fatal).
- Transactions with existing fid tags (any value): leave unchanged.

---

## Sub-Step 2.7 — Tests (`internal/journal/journal_test.go`)

All tests use `t.TempDir()` for isolation. Package: `journal_test`.

### FID Tests

| Test | Assertions |
|------|-----------|
| `TestMintFID_Length` | `len(MintFID()) == 8` |
| `TestMintFID_HexChars` | All chars in `[0-9a-f]` |
| `TestMintFID_Unique` | Two calls return different strings |

### Format Tests

`draftFormat` is unexported; test it via `FormatViaHledger` or test it through the exported path.

| Test | Setup | Assertions |
|------|-------|-----------|
| `TestFormatViaHledger_Basic` | tx with date 2026-01-15, desc "AMAZON", two postings with amounts, fid "bb002200"; requires hledger binary | output contains `"2026-01-15 AMAZON  ; fid:bb002200"`, posting with `expenses:shopping` and `$45.00` |
| `TestFormatViaHledger_AutoBalance` | one posting with amount, one with empty amount | no error; output has two postings; auto-balance posting has computed amount |
| `TestFormatViaHledger_FIDPreserved` | any tx with fid "aa001100" | output contains `"fid:aa001100"` |
| `TestFormatViaHledger_TrailingNewline` | any tx | output ends with `"\n"` |

### File Management Tests

| Test | Setup | Assertions |
|------|-------|-----------|
| `TestEnsureMonthFile_Creates` | dataDir=tempDir, year=2026, month=1 | returns `"2026/01.journal"`, created=true; file exists at `tempDir/2026/01.journal`; file starts with `"; float: 2026/01"` |
| `TestEnsureMonthFile_Idempotent` | call twice | second call: created=false, same relPath, file not modified |
| `TestEnsureMonthFile_CreatesDir` | month=3 | `tempDir/2026/` directory created |
| `TestUpdateMainIncludes_Adds` | empty main.journal, add `"2026/01.journal"` | file contains `"include 2026/01.journal\n"` |
| `TestUpdateMainIncludes_Idempotent` | call twice with same path | file contains the directive exactly once |
| `TestUpdateMainIncludes_Preserves` | main.journal has existing content | existing content preserved, new include appended |
| `TestAppendTransaction_Basic` | AppendTransaction with hledger client + Jan 2026 tx | returns 8-char hex fid; `2026/01.journal` created; file contains ISO date, description, fid tag |
| `TestAppendTransaction_UpdatesMain` | main.journal exists, new month | main.journal gets `include 2026/01.journal` |
| `TestAppendTransaction_MultipleMonths` | two transactions in Jan and Feb 2026 | two files created; main.journal has two includes; each file has correct transaction |
| `TestAppendTransaction_SameMonth` | two transactions in same month | single file; both transactions present |

*Note: `AppendTransaction` tests require a real hledger binary (integration tests). Mark with a helper that skips if hledger is unavailable, similar to the hledger package tests.*

### Migration Tests

| Test | Setup | Assertions |
|------|-------|-----------|
| `TestMigrateFIDs_NoMainJournal` | empty tempDir | returns 0, nil |
| `TestMigrateFIDs_AddsToUntagged` | main.journal includes a file with 2 txns lacking fid | returns 2; both txn header lines now contain `; fid:` |
| `TestMigrateFIDs_PreservesExisting` | journal file with all txns having fid | returns 0; files unchanged |
| `TestMigrateFIDs_Mixed` | 3 txns: 2 with fid, 1 without | returns 1; only missing fid is added |
| `TestMigrateFIDs_ValidFidFormat` | untagged txn | added fid value is 8 hex chars |
| `TestMigrateFIDs_PreservesPostings` | txn with postings | posting lines and blank lines not modified |

---

## Verification

```bash
# Add dependencies
cd /home/user/float
go get github.com/BurntSushi/toml@latest
go get github.com/google/uuid@latest
go mod tidy

# Run tests
go test ./internal/config/ -v
go test ./internal/journal/ -v

# Run all tests (regression check)
go test ./...

# Lint
golangci-lint run ./internal/config/ ./internal/journal/
```

---

## Implementation Order

1. Sub-Step 2.1: Add dependencies to go.mod
2. Sub-Step 2.2: `internal/config/` — types, Load, Save, testdata, tests
3. Sub-Step 2.3: `internal/journal/fid.go` — MintFID
4. Sub-Step 2.4: `internal/journal/format.go` — TransactionInput, PostingInput, Format
5. Sub-Step 2.5: `internal/journal/files.go` — EnsureMonthFile, UpdateMainIncludes, AppendTransaction
6. Sub-Step 2.6: `internal/journal/migrate.go` — MigrateFIDs
7. Sub-Step 2.7: `internal/journal/journal_test.go` — all journal tests

Each sub-step can be implemented independently after step 2.1 is complete. Sub-steps 2.3–2.6 can be parallelized (different files). Sub-step 2.7 requires all prior journal sub-steps.

---

## Notes for Implementors

- `internal/journal/` imports `internal/hledger/` only for `FormatViaHledger` and `AppendTransaction`. `EnsureMonthFile`, `UpdateMainIncludes`, `MigrateFIDs`, and `MintFID` are hledger-free.
- `draftFormat` date: use ISO format `2026-01-15` (hyphen-separated) for the temp file — this is what hledger expects and what `hledger print` will output.
- FID is exactly 8 lowercase hex characters from the start of a UUID v4.
- `UpdateMainIncludes` must be idempotent — check for exact directive string before appending.
- Migration uses text pattern matching, not hledger parsing. The regex `^\d{4}[/\-]\d{2}[/\-]\d{2} ` identifies transaction header lines.
- `hledger print` output already ends with a trailing blank line — `AppendTransaction` does not need to add one.
- The `accounts.journal` file is not managed by this step — it's edited manually or in a later step.
