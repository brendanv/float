# internal/journal

Text-level journal file manipulation: creating month files, appending transactions,
updating include directives, and migrating legacy transactions to add `fid` tags.

This package does **not** understand accounting semantics — it does not parse amounts,
validate balances, or interpret hledger directives. All accounting validation is
delegated to `internal/hledger`.

## Files

| File | Contents |
|------|----------|
| `fid.go` | `MintFID()` — generates random 8-char lowercase hex FID |
| `format.go` | `TransactionInput`, `PostingInput`, `FormatViaHledger()`, `draftFormat()` |
| `files.go` | `EnsureMonthFile()`, `UpdateMainIncludes()`, `AppendTransaction()` |
| `migrate.go` | `MigrateFIDs()` — back-fills `fid` tags on untagged transactions |

## Key Functions

### FID

```go
fid := journal.MintFID() // e.g. "a1b2c3d4"
```

Returns the first 8 hex characters of a UUID v4. Always lowercase. Used to tag
every transaction written by float: `; fid:a1b2c3d4`.

### Writing a Transaction

```go
tx := journal.TransactionInput{
    Date:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
    Description: "AMAZON MARKETPLACE",
    Postings: []journal.PostingInput{
        {Account: "expenses:shopping", Amount: "$45.00"},
        {Account: "assets:checking"}, // empty Amount = auto-balance
    },
}
fid, err := journal.AppendTransaction(ctx, hledgerClient, dataDir, tx)
```

`AppendTransaction` mints a FID, formats via hledger, ensures the month file
exists, updates `main.journal` if a new file was created, and appends the text.

### Low-level Helpers

```go
// Canonical hledger-formatted text for a transaction
text, err := journal.FormatViaHledger(ctx, client, tx, fid)

// Ensure YYYY/MM.journal exists; returns (relPath, created, err)
relPath, created, err := journal.EnsureMonthFile(dataDir, 2026, 1)

// Add "include 2026/01.journal" to main.journal (idempotent)
err := journal.UpdateMainIncludes(mainPath, "2026/01.journal")
```

### Migration

```go
n, err := journal.MigrateFIDs(dataDir)
// n = number of transactions that had a fid tag added
```

Scans every `.journal` file listed in `main.journal`, finds transaction header
lines that lack a `fid:` tag, and appends one. Safe to re-run (idempotent).

## Constraints

- `AppendTransaction` does **not** acquire `txlock` — callers (e.g. `LedgerService`)
  must wrap it in `txlock.Do()` before calling.
- `FormatViaHledger` writes a temp file and shells out to `hledger print` for
  canonical formatting; the temp file is cleaned up automatically.
- Month files are named `YYYY/MM.journal` and opened with `O_APPEND` — order of
  transactions within a file matches the order they were written.
