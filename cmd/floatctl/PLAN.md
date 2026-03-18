# floatctl — Admin/Debug CLI

`floatctl` is the admin and debug CLI for float. It is distinct from the end-user `float`
gRPC client: `floatctl` operates directly on the data directory and internal packages,
bypassing the gRPC API. It is the tool for operators, developers, and automated scripts.

---

## Command Shape

```
floatctl <group> <subcommand> [flags] [args...]
floatctl help
floatctl <group> help
```

Commands are organized into groups by functional area. Adding a new command requires only
a new `init()` registration in a per-group file — `main.go` and `registry.go` never need
to change.

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
```

| Subcommand  | Description |
|-------------|-------------|
| `balance`   | Run `hledger bal -O json`, print parsed `BalanceReport` as JSON |
| `accounts`  | Run `hledger accounts`, print parsed `AccountNode` tree as JSON |
| `register`  | Run `hledger reg -O json`, print parsed `RegisterRow` slice as JSON |
| `print-csv` | Run `hledger print` on a CSV+rules file, print parsed `Transaction` slice as JSON |
| `version`   | Print the hledger binary version string |
| `check`     | Run `hledger check` on a journal; exit 0 if valid, 1 with error message |

---

## Planned Future Commands

Commands are unlocked as the corresponding internal packages are built (see root `PLAN.md`).

### `journal` group — journal file management (Step 2)

```
floatctl journal verify       <data-dir>
floatctl journal migrate-fids <data-dir>
floatctl journal list-files   <data-dir>
```

| Subcommand      | Description |
|-----------------|-------------|
| `verify`        | Run `hledger check` on the full data directory; report all errors |
| `migrate-fids`  | Scan all transactions, add `fid` tags to any that lack them |
| `list-files`    | List all `.journal` files found under the data directory |

### `git` group — snapshot management (Step 3)

```
floatctl git log     <data-dir>
floatctl git restore <data-dir> <commit-hash>
floatctl git status  <data-dir>
```

| Subcommand | Description |
|------------|-------------|
| `log`      | List recent git snapshots (hash, message, timestamp) |
| `restore`  | Hard-reset data directory to a given commit hash |
| `status`   | Show uncommitted changes in the data directory |

### `config` group — configuration inspection (Step 2)

```
floatctl config show     <config.toml>
floatctl config validate <config.toml>
```

| Subcommand | Description |
|------------|-------------|
| `show`     | Print parsed `config.toml` as JSON |
| `validate` | Validate `config.toml`; exit 0 if valid, 1 with errors |

### `txn` group — transaction admin (Step 7)

```
floatctl txn lookup <data-dir> <fid>
floatctl txn add    <data-dir>
floatctl txn delete <data-dir> <fid>
```

| Subcommand | Description |
|------------|-------------|
| `lookup`   | Look up a transaction by `fid` tag, print as JSON |
| `add`      | Add a transaction directly via `txlock` (bypasses gRPC) |
| `delete`   | Delete a transaction by `fid` via `txlock` (bypasses gRPC) |

### `import` group — import pipeline debug (Step 9)

```
floatctl import preview    <data-dir> <csv> --profile <name>
floatctl import rules-test <csv> <rules>
```

| Subcommand    | Description |
|---------------|-------------|
| `preview`     | Preview a CSV import without committing; print candidates + duplicates |
| `rules-test`  | Test a rules file against a CSV; print parsed transactions as JSON |

### `rules` group — rules file inspection (Step 10)

```
floatctl rules list <data-dir>
floatctl rules show <data-dir> <profile>
```

| Subcommand | Description |
|------------|-------------|
| `list`     | List all rules files in `data/rules/` |
| `show`     | Print the raw contents of a rules file |

### `cache` group — query cache admin (Step 11)

```
floatctl cache stats
floatctl cache warm <data-dir>
```

| Subcommand | Description |
|------------|-------------|
| `stats`    | Show cache hit/miss counters (requires running floatd) |
| `warm`     | Pre-warm cache for common queries |

---

## Architecture

See `registry.go` for the `Command` type and registration API.

```go
// To add a new command, register it in an init() in any .go file in this package:
func init() {
    register(
        &Command{
            Group:    "mygroup",
            Name:     "mycommand",
            Synopsis: "Does something useful",
            Run: func(args []string) error {
                fs := flag.NewFlagSet("mygroup mycommand", flag.ExitOnError)
                // ... define flags ...
                fs.Parse(args)
                // ... logic ...
                return nil
            },
        },
    )
}
```

No changes to `main.go` or `registry.go` are required.
