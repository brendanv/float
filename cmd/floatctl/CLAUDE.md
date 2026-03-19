# cmd/floatctl

`floatctl` is the admin and debug CLI for float. It operates directly on internal packages
and the data directory — it bypasses the gRPC API entirely. It is distinct from `float`
(the end-user gRPC client).

## Adding a New Command

1. Create a new file named after the group, e.g. `journal.go`
2. Register commands via `init()` — **do not edit `main.go` or `registry.go`**

```go
func init() {
    register(
        &Command{
            Group:    "journal",
            Name:     "verify",
            Synopsis: "Run hledger check on the full data directory",
            Run: func(args []string) error {
                fs := flag.NewFlagSet("journal verify", flag.ExitOnError)
                // define flags, parse args, implement logic
                fs.Parse(args)
                return nil
            },
        },
    )
}
```

Help output, group listing, and dispatch are all automatic.

## Command Shape

```
floatctl <group> <subcommand> [flags] [args...]
floatctl help
floatctl <group> help
```

Each command creates its own `flag.NewFlagSet` with `flag.ExitOnError`.

**Two positional-arg conventions exist depending on the command:**

1. **Flags-first** (read-only commands with no mandatory data-dir): flags precede positional args, which are then read via `fset.Arg(0)`, `fset.Arg(1)`, etc. Standard Go `flag` package behavior — parsing stops at the first non-flag argument.
   ```
   floatctl hledger balance [--depth N] <journal> [query...]
   ```

2. **Data-dir-first** (write commands that take `<data-dir>` and optional flags): `<data-dir>` and other positional args are extracted from `args[0]`, `args[1]`, etc. before calling `fset.Parse(args[N:])`. This lets flags follow the data-dir naturally.
   ```
   floatctl journal add <data-dir> --description "..." --posting "..."
   floatctl journal import <data-dir> <csv> --profile <name>
   ```
   When implementing a new write command using this pattern, extract positional args from `args` before calling `fset.Parse`.

## File Layout

```
cmd/floatctl/
├── CLAUDE.md      ← you are here
├── main.go        ← entry point (arg parsing + dispatch)
├── registry.go    ← Command type, register(), dispatch(), help rendering
├── hledger.go     ← "hledger" group commands
├── journal.go     ← "journal" group commands
└── config.go      ← "config" group commands
```

Each group lives in its own file (`hledger.go`, `journal.go`, etc.). The pattern is:
one file per group, one `init()` per file, one `Command` struct per subcommand.

---

## Current Commands

### `hledger` group — hledger wrapper debug/inspection

```
floatctl hledger balance   [--depth N] <journal> [query...]
floatctl hledger accounts  [--tree]    <journal>
floatctl hledger register             <journal> [query...]
floatctl hledger print-csv            <csv> <rules>
floatctl hledger version
floatctl hledger check                <journal>
floatctl hledger raw                  <journal> <subcmd> [args...]
```

| Subcommand  | Description |
|-------------|-------------|
| `balance`   | Run `hledger bal -O json`, print parsed `BalanceReport` as JSON |
| `accounts`  | Run `hledger accounts`, print parsed `AccountNode` tree as JSON |
| `register`  | Run `hledger reg -O json`, print parsed `RegisterRow` slice as JSON |
| `print-csv` | Run `hledger print` on a CSV+rules file, print parsed `Transaction` slice as JSON |
| `version`   | Print the hledger binary version string |
| `check`     | Run `hledger check` on a journal; exit 0 if valid, 1 with error message |
| `raw`       | Run any hledger subcommand with arbitrary args; print raw stdout (escape hatch for debugging). The exact command is printed to stderr as `# command: ...` |

### `journal` group — journal file management

```
floatctl journal add          <data-dir> --description <text> --posting "account  [amount]" [--posting ...] [--date YYYY-MM-DD] [--comment <text>]
floatctl journal delete       <data-dir> <fid>
floatctl journal import       <data-dir> <csv> --profile <name> [--yes]
floatctl journal verify       <data-dir>
floatctl journal migrate-fids <data-dir>
floatctl journal list-files   <data-dir>
floatctl journal lookup       <data-dir> <fid>
floatctl journal stats        <data-dir>
floatctl journal audit        <data-dir>
```

| Subcommand      | Description |
|-----------------|-------------|
| `add`           | Add a new transaction via `txlock.Do()`. `--posting` is repeatable (min 2); format is `"account  amount"` (2+ spaces) or `"account"` for auto-balance. |
| `delete`        | Remove the transaction with the given `fid` from its journal file via `txlock.Do()`. Exits non-zero if not found. |
| `import`        | Parse `<csv>` using the rules file from the named bank profile in `config.toml`. Prints a preview with `[NEW]`/`[DUP]` status for each transaction (duplicate = same date+description+amounts as an existing transaction). Prompts for confirmation unless `--yes`. Writes new transactions via `txlock.Do()`. |
| `verify`        | Run `hledger check` on `main.journal` in the data directory; print `ok` or error |
| `migrate-fids`  | Scan all included journal files, add `fid` tags to any untagged transactions |
| `list-files`    | Walk the data directory and print all `.journal` file paths |
| `lookup`        | Look up a transaction by `fid` tag using `hledger reg tag:fid=<fid>`; print as JSON. Exits non-zero if not found |
| `stats`         | Print journal statistics as JSON: file count, transaction count, date range, account count, total size |
| `audit`         | Audit journal integrity: checks include directives exist, FIDs are unique, no orphaned journal files. Prints JSON report; exits non-zero if any issues found |

### `config` group — configuration inspection

```
floatctl config show     <config.toml>
floatctl config validate <config.toml>
```

| Subcommand | Description |
|------------|-------------|
| `show`     | Print parsed `config.toml` as JSON |
| `validate` | Validate `config.toml`; exit 0 if valid, 1 with error message |

---

## Planned Future Commands

Commands are unlocked as the corresponding internal packages are built.

### `journal` group additions — git snapshots (Step 12)

```
floatctl journal snapshots <data-dir>
floatctl journal restore   <data-dir> <commit-hash>
```

| Subcommand    | Description |
|---------------|-------------|
| `snapshots`   | List recent git snapshots (hash, message, timestamp) |
| `restore`     | Hard-reset data directory to a given commit hash |

### `rules` group — rules file inspection (Step 5)

```
floatctl rules list <data-dir>
floatctl rules show <data-dir> <profile>
```

| Subcommand | Description |
|------------|-------------|
| `list`     | List all rules files in `data/rules/` |
| `show`     | Print the raw contents of a rules file |

### `cache` group — query cache admin (Step 10)

```
floatctl cache stats
floatctl cache warm <data-dir>
```

| Subcommand | Description |
|------------|-------------|
| `stats`    | Show cache hit/miss counters (requires running floatd) |
| `warm`     | Pre-warm cache for common queries |
