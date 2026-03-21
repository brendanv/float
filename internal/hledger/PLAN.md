# Plan: internal/hledger — Step 1 Implementation

## Context

`internal/hledger/` is the foundational package that everything else depends on. It wraps the hledger CLI: builds commands, shells out, parses JSON/text output, and returns typed Go structs. No external dependencies — pure stdlib. The supported hledger version is **1.52** (pinned in `mise.toml`).

---

## File Structure

```
internal/hledger/
├── client.go          # Client struct, New(), run(), version check, all 6 public methods
├── types.go           # All type definitions
├── parse.go           # All JSON/text parser functions
├── hledger_test.go    # Integration tests (package hledger_test)
└── testdata/
    ├── simple.journal
    ├── empty.journal
    ├── invalid.journal
    ├── import.csv
    └── import.rules
```

---

## Sub-Step 1.1 — Types (`types.go`)

No logic, no I/O. Pure type declarations that the rest of the project will depend on.

```go
package hledger

import "encoding/json"

type AmountQuantity struct {
    DecimalMantissa int64   `json:"decimalMantissa"`
    DecimalPlaces   int     `json:"decimalPlaces"`
    FloatingPoint   float64 `json:"floatingPoint"`
}

type Amount struct {
    Commodity string           `json:"acommodity"`
    Quantity  AmountQuantity   `json:"aquantity"`
    Cost      *json.RawMessage `json:"acost"`
}

type Posting struct {
    Account          string      `json:"paccount"`
    Amounts          []Amount    `json:"pamount"`
    Comment          string      `json:"pcomment"`
    Tags             [][2]string `json:"ptags"`
    Status           string      `json:"pstatus"`
    Type             string      `json:"ptype"`
    TransactionIndex string      `json:"ptransaction_"`
    Date             *string     `json:"pdate"`
    Date2            *string     `json:"pdate2"`
}

type Transaction struct {
    Index            int         `json:"tindex"`
    Date             string      `json:"tdate"`
    Date2            *string     `json:"tdate2"`
    Description      string      `json:"tdescription"`
    Code             string      `json:"tcode"`
    Comment          string      `json:"tcomment"`
    Tags             [][2]string `json:"ttags"`
    Postings         []Posting   `json:"tpostings"`
    Status           string      `json:"tstatus"`
    PrecedingComment string      `json:"tprecedingcomment"`
}

// RegisterRow is one row from `hledger reg -O json`.
// Each row is a heterogeneous 5-element JSON array — see parseRegisterRows.
// Date and Description are non-nil only for the first posting of each transaction.
type RegisterRow struct {
    Date        *string
    Date2       *string
    Description *string
    Posting     Posting
    Balance     []Amount
}

// BalanceRow is one account entry from `hledger bal -O json`.
// The JSON encodes each row as a heterogeneous 4-element array — see parseBalanceReport.
type BalanceRow struct {
    DisplayName string
    FullName    string
    Indent      int
    Amounts     []Amount
}

type BalanceReport struct {
    Rows  []BalanceRow
    Total []Amount
}

// AccountNode is a node in the account tree.
// Returned by Accounts(tree=true) with children populated,
// or Accounts(tree=false) as a flat list with no children.
type AccountNode struct {
    Name     string         // short segment (e.g. "checking")
    FullName string         // full colon path (e.g. "assets:checking")
    Children []*AccountNode
}

type CheckError struct {
    Output string
}

func (e *CheckError) Error() string { return e.Output }
```

---

## Sub-Step 1.2 — Client Core (`client.go`)

```go
package hledger

import (
    "bytes"
    "fmt"
    "os/exec"
    "strings"
)

const supportedVersion = "1.52"

type Client struct {
    bin     string
    journal string
}

// New validates the binary exists and the version matches supportedVersion.
func New(bin, journal string) (*Client, error)

// parseVersion extracts version from "hledger 1.52, linux-x86_64\n":
//   split on " " → take index 1 → strip trailing comma
func parseVersion(output string) (string, error)

// run executes hledger with args, capturing stdout and stderr separately.
// Returns non-nil err when exit code != 0.
func (c *Client) run(args ...string) (stdout []byte, stderr []byte, err error)
```

**`New` logic:**
1. `exec.LookPath(bin)` — return `fmt.Errorf("hledger binary not found at %q: %w", bin, err)` on failure.
2. Run `hledger --version`, parse with `parseVersion`.
3. Compare to `supportedVersion` (exact string). Return `fmt.Errorf("unsupported hledger version %q, need %q", got, supportedVersion)` on mismatch.

**`run` logic:** `exec.Command(c.bin, args...)` with `cmd.Stdout = &stdoutBuf` and `cmd.Stderr = &stderrBuf`. Return both buffers and `cmd.Run()` error.

---

## Sub-Step 1.3 — Public Methods (add to `client.go`)

```go
func (c *Client) Version() (string, error)

// Check runs `hledger check -f <journal>`.
// Returns nil on exit 0. Returns *CheckError (with full stderr) on exit non-0.
func (c *Client) Check() error

// Balances runs `hledger bal -O json -f <journal> [--depth N] [query...]`.
// depth 0 = no --depth flag.
func (c *Client) Balances(depth int, query ...string) (*BalanceReport, error)

// Register runs `hledger reg -O json -f <journal> [query...]`.
// Returns flat RegisterRows (one per posting). Caller groups into transactions if needed.
func (c *Client) Register(query ...string) ([]RegisterRow, error)

// Accounts runs `hledger accounts [--tree] -f <journal>`.
// tree=true: returns populated tree. tree=false: flat list with no children.
func (c *Client) Accounts(tree bool) ([]*AccountNode, error)

// PrintCSV runs `hledger print -O json --rules-file <rulesFile> -f <csvFile>`.
// Note: CSV is passed via -f, not as a positional arg.
// Used for import preview — no journal file is needed/written.
func (c *Client) PrintCSV(csvFile, rulesFile string) ([]Transaction, error)
```

---

## Sub-Step 1.4 — Parsers (`parse.go`)

### `parseBalanceReport(data []byte) (*BalanceReport, error)`

JSON structure of `hledger bal -O json`:
```json
[
  [                                    <- element [0]: rows
    ["display", "full", 0, [amounts]], <- each row is [string, string, int, []Amount]
    ...
  ],
  [amounts]                            <- element [1]: totals
]
```

Unmarshal into `[2]json.RawMessage`. Unmarshal element [0] into `[]json.RawMessage`. For each row, unmarshal into `[4]json.RawMessage`, then decode each field individually. Unmarshal element [1] into `[]Amount` for totals.

### `parseRegisterRows(data []byte) ([]RegisterRow, error)`

JSON structure of `hledger reg -O json`:
```json
[
  ["2026-01-05", null, "PAYROLL...", {posting_obj}, [balance_amounts]],
  [null, null, null, {posting_obj}, [balance_amounts]],   <- 2nd posting, same txn
  ...
]
```

Unmarshal outer into `[]json.RawMessage`. For each, unmarshal into `[5]json.RawMessage`. Elements [0],[1],[2] → `*string`, [3] → `Posting`, [4] → `[]Amount`.

**Key**: date/description are null for the 2nd+ posting of each transaction.

### `parseTransactions(data []byte) ([]Transaction, error)`

Straightforward: `json.Unmarshal(data, &[]Transaction{})`. Used by `Register` and `PrintCSV`.

**For PrintCSV**: `acommodity` may be `""` when the CSV has no currency symbol — this is normal.

**Tags**: use `ttags` field (`[][2]string`). Do not parse `tcomment`.

### `parseAccountsTree(text string) ([]*AccountNode, error)`

`hledger accounts --tree` output — **text, not JSON**:
```
assets          <- depth 0 (no leading spaces)
  checking      <- depth 1 (2 spaces per level)
expenses
  food
  shopping
```

Algorithm:
1. Split on newlines, skip empty lines.
2. For each line: `depth = len(leading spaces) / 2`, `shortName = strings.TrimSpace(line)`.
3. Maintain `stack []struct{ depth int; node *AccountNode }`.
4. depth=0: `fullName = shortName`, pop stack to empty, append to roots, push.
5. depth>0: pop until `stack[top].depth == depth-1`, `fullName = parent.FullName + ":" + shortName`, add as child of stack top, push.

### `parseAccountsFlat(text string) ([]*AccountNode, error)`

`hledger accounts` (no `--tree`) output: one full colon-path per line.
- Split on `\n`, skip empty lines.
- For each line: `AccountNode{Name: last segment, FullName: line, Children: nil}`.
- Last segment = after the last `:`.

---

## Sub-Step 1.5 — Testdata Fixtures

### `testdata/simple.journal`
```journal
account assets:checking
account expenses:shopping
account expenses:food
account income:salary

2026/01/05 PAYROLL DIRECT DEPOSIT  ; fid:aa001100
    assets:checking     $3500.00
    income:salary

2026/01/15 AMAZON MARKETPLACE  ; fid:bb002200
    expenses:shopping   $45.00
    assets:checking

2026/01/20 WHOLE FOODS  ; fid:cc003300
    expenses:food       $120.00
    assets:checking

2026/02/01 AMAZON PRIME  ; fid:dd004400
    expenses:shopping   $15.00
    assets:checking

2026/02/05 PAYROLL DIRECT DEPOSIT  ; fid:ee005500
    assets:checking     $3500.00
    income:salary
```

### `testdata/empty.journal`
```journal
account assets:checking
```

### `testdata/invalid.journal`
```journal
2026/01/05 UNBALANCED TRANSACTION
    assets:checking     $100.00
    expenses:shopping   $50.00
```

### `testdata/import.csv`
```csv
date,description,amount
2026-01-15,AMAZON MARKETPLACE,-45.00
2026-01-05,PAYROLL DIRECT DEPOSIT,3500.00
2026-01-20,WHOLE FOODS,-120.00
```

### `testdata/import.rules`
```
skip 1
fields date, description, amount
account1 assets:checking

if AMAZON
  account2 expenses:shopping

if PAYROLL
  account2 income:salary

if WHOLE FOODS
  account2 expenses:food
```

---

## Sub-Step 1.6 — Integration Tests (`hledger_test.go`)

Package: `hledger_test`
Imports: `errors`, `path/filepath`, `testing`, `github.com/brendanv/float/internal/hledger`

**Helper:**
```go
func mustClient(t *testing.T, journal string) *hledger.Client {
    t.Helper()
    c, err := hledger.New("hledger", filepath.Join("testdata", journal))
    if err != nil {
        t.Fatal(err)
    }
    return c
}
```

| Test | Setup | Assertions |
|---|---|---|
| `TestNew_ValidVersion` | `New("hledger", "testdata/simple.journal")` | no error |
| `TestNew_BadBinary` | `New("/nonexistent/hledger", ...)` | error contains "not found" |
| `TestCheck_Valid` | `mustClient("simple.journal").Check()` | nil |
| `TestCheck_Invalid` | `mustClient("invalid.journal").Check()` | `errors.As(*CheckError)` succeeds; `Error()` non-empty |
| `TestCheck_Empty` | `mustClient("empty.journal").Check()` | nil |
| `TestBalances_All` | `Balances(0)` on simple.journal | 4+ rows; `assets:checking` ≈ 6820; `expenses:shopping` ≈ 60; `income:salary` < 0 |
| `TestBalances_Depth1` | `Balances(1)` | rows have no `:` in FullName; `expenses` Amounts[0].FloatingPoint ≈ 180 |
| `TestBalances_Query` | `Balances(0, "expenses")` | all rows have FullName starting with `expenses`; no `assets` row |
| `TestBalances_Empty` | `Balances(0)` on empty.journal | no error; Rows empty or nil |
| `TestRegister_All` | `Register()` on simple.journal | 10 rows (5 txns × 2 postings); first row Date non-nil, second row Date nil |
| `TestRegister_FidQuery` | `Register("tag:fid=bb002200")` | 2 rows; Description=="AMAZON MARKETPLACE"; Posting.Account=="expenses:shopping"; floatingPoint==45 |
| `TestRegister_DateFilter` | `Register("date:2026-01")` | 6 rows (3 January txns × 2 postings) |
| `TestAccounts_Flat` | `Accounts(false)` on simple.journal | 4 nodes; all have nil Children; FullNames include `assets:checking` |
| `TestAccounts_Tree` | `Accounts(true)` on simple.journal | 3 root nodes (assets, expenses, income); `expenses` has 2 children; child FullNames correct |
| `TestPrintCSV` | `PrintCSV("testdata/import.csv", "testdata/import.rules")` | 3 transactions; AMAZON txn has posting Account=="expenses:shopping" floatingPoint==45; PAYROLL txn has posting Account=="income:salary" |
| `TestPrintCSV_BadRules` | `PrintCSV("testdata/import.csv", "testdata/nonexistent.rules")` | error non-nil |

---

## Verification

```bash
go test ./internal/hledger/ -v
```

Prereq: `mise install` must be run so hledger 1.52 is on PATH.

---

## Notes for Implementor

- `go.mod` stays pure stdlib for this step — no `go get` needed.
- `Transaction.Tags` is `[][2]string`, not `map[string]string` — multiple same-name tags are valid.
- `hledger check` errors go to **stderr**, not stdout. Capture both in `run()`.
- `PrintCSV` passes the CSV via `-f`, not as a positional arg: `hledger print -O json --rules-file <rules> -f <csv>`.
- `hledger accounts --tree` uses 2 spaces per indent level (verify by running locally if needed).
- The `floatingPoint` field in `AmountQuantity` is the pre-computed decimal — use it directly; avoid recomputing from mantissa/places.
