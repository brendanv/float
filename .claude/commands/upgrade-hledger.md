Upgrade the pinned hledger version to `$ARGUMENTS` and verify that the JSON output from all hledger commands is still compatible with float's parsers.

## Step 1: Resolve versions

Identify the current pinned version from `mise.toml` (line with `hledger = { version = ...`). The target version is `$ARGUMENTS`. If no argument was provided, ask the user which version to upgrade to before proceeding.

## Step 2: Check release notes for breaking changes

Fetch https://hledger.org/relnotes.html and read the release notes for the target version. Pay specific attention to any changes in:

- JSON output format (`-O json`) for `bal`, `print`, or `reg` commands
- `accounts --types` text output format
- `hledger check` behavior or new checks that might affect the test journal
- `--version` output format (the string shape `hledger X.Y, arch` should stay the same)

Note any breaking changes — they will need corresponding updates to `internal/hledger/parse.go`.

## Step 3: Update all version strings

Replace every occurrence of the old version string with the new one. These are the exact locations:

1. **`mise.toml`** — `version = "OLD"` inside the hledger tool entry
2. **`internal/hledger/client.go`** — `const supportedVersion = "OLD"` and the doc comment on `parseVersion` that shows an example version string
3. **`internal/hledger/hledger_test.go`** — all mock `CommandRunner` stubs that return `"hledger OLD, linux-x86_64\n"` (there are two)
4. **`internal/server/ledger/handler_test.go`** — all mock stubs returning the same version string (there are two)
5. **`internal/hledger/PLAN.md`** — all prose and code snippet references to the old version

Use `replace_all: true` when editing test files since the same string appears multiple times.

## Step 4: Install the new binary

Run `mise install` from the repo root to download and activate the new hledger version. Then verify the correct version is on PATH:

```bash
hledger --version
```

The output should start with `hledger NEW`. If it still shows the old version, mise shims may not be on PATH — run `mise activate` or add `~/.local/share/mise/shims` to PATH.

## Step 5: Probe actual JSON output against test fixtures

Run each hledger command that float parses and capture raw output. This is the ground truth — compare it against what `internal/hledger/parse.go` expects.

```bash
# Balance report: parseBalanceReport() expects [rows[], totals[]]
# where each row is [displayName, fullName, indent, amounts[]]
hledger bal -f internal/hledger/testdata/simple.journal -O json 2>&1

# Transactions: parseTransactions() expects standard JSON objects with
# fields: tindex, tdate, tdate2, tdescription, tcode, tcomment,
#         ttags ([][2]string), tpostings, tstatus, tprecedingcomment
hledger print -f internal/hledger/testdata/simple.journal -O json 2>&1

# Register: parseRegisterRows() expects rows of [date, date2, description, posting, balance[]]
hledger reg -f internal/hledger/testdata/simple.journal -O json 2>&1

# Accounts: parseAccountsFlat/Tree() expects indented text lines,
# each optionally suffixed with "; type: X"
hledger accounts -f internal/hledger/testdata/simple.journal --types 2>&1
hledger accounts -f internal/hledger/testdata/simple.journal --types --tree 2>&1
```

For each command, check:
- **Balance report**: Is the outer structure still a 2-element array `[rows, totals]`? Is each row still a 4-element array `[string, string, int, amounts[]]`? Have any `Amount` field names changed (`acommodity`, `aquantity`, `acost`)?
- **Transactions**: Are all expected fields present? Have field names changed? Is `ttags` still `[][2]string`? Are `tpostings` still objects with `paccount`, `pamount`, `pcomment`, `ptags`, `pstatus`, `ptype`?
- **Register**: Is each row still a 5-element array? Are date/description elements nullable (null for continuation rows)?
- **Accounts**: Do lines still follow `  shortName ; type: X` indentation (2 spaces per level)?

If the JSON structure has changed, update `internal/hledger/parse.go` and/or `internal/hledger/types.go` accordingly before running tests.

## Step 6: Run the integration tests

```bash
go test ./internal/hledger/ -v
```

These tests run real hledger against `testdata/` fixture files. They exercise the full parse pipeline end-to-end. Read any failures carefully — a "unmarshal" error means the JSON structure changed and parse.go needs updating; an unexpected value means the fixture or test expectation needs adjustment for a behavioral change in hledger.

If `TestNew` fails with `unsupported hledger version`, the version strings from Step 3 weren't all updated — re-check.

## Step 7: Run the full test suite

```bash
go test ./...
```

All packages must pass. Fix any failures before committing.

## Step 8: Commit

Stage and commit all modified files with a message like:

```
Update hledger dependency from OLD to NEW
```

Files to commit (include `parse.go` and `types.go` only if they were changed):
- `mise.toml`
- `internal/hledger/client.go`
- `internal/hledger/hledger_test.go`
- `internal/server/ledger/handler_test.go`
- `internal/hledger/PLAN.md`
- `internal/hledger/parse.go` (if updated)
- `internal/hledger/types.go` (if updated)

Then push to the current branch.
