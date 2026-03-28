# cmd/floatctl

Admin and debug CLI for float. Operates directly on internal packages and the data directory — bypasses the gRPC API entirely. Distinct from `float` (the end-user gRPC client).

## Adding a New Command

1. Create a file named after the group (e.g. `journal.go`)
2. Register via `init()` — **do not edit `main.go` or `registry.go`**

```go
func init() {
    register(&Command{
        Group:    "journal",
        Name:     "verify",
        Synopsis: "Run hledger check on the full data directory",
        Run: func(args []string) error {
            fs := flag.NewFlagSet("journal verify", flag.ExitOnError)
            fs.Parse(args)
            return nil
        },
    })
}
```

## Command Shape

```
floatctl <group> <subcommand> [flags] [args...]
floatctl help
floatctl <group> help
```

**Two arg conventions:**

1. **Flags-first** (read-only, no mandatory data-dir): flags precede positional args, read via `fset.Arg(0)`, etc.
   ```
   floatctl hledger balance [--depth N] <journal> [query...]
   ```

2. **Data-dir-first** (write commands): extract `<data-dir>` from `args[0]` before calling `fset.Parse(args[1:])`.
   ```
   floatctl journal add <data-dir> --description "..." --posting "..."
   ```

## File Layout

```
cmd/floatctl/
├── main.go        ← entry point
├── registry.go    ← Command type, register(), dispatch(), help
├── hledger.go     ← "hledger" group
├── journal.go     ← "journal" group
└── config.go      ← "config" group
```

## Current Commands

### `hledger` group

| Subcommand  | Description |
|-------------|-------------|
| `balance [--depth N] <journal> [query...]` | Run `hledger bal -O json` |
| `accounts [--tree] <journal>` | Run `hledger accounts`, print as JSON tree |
| `register <journal> [query...]` | Run `hledger reg -O json` |
| `print-csv <csv> <rules>` | Parse CSV via rules, print transactions as JSON |
| `version` | Print hledger binary version |
| `check <journal>` | Validate journal; exit 0 if valid |
| `raw <journal> <subcmd> [args...]` | Run any hledger subcommand, print raw stdout |

### `journal` group

| Subcommand  | Description |
|-------------|-------------|
| `add <data-dir> --description <text> --posting "account  amount" [--posting ...] [--date YYYY-MM-DD]` | Add transaction via txlock |
| `delete <data-dir> <fid>` | Remove transaction by fid via txlock |
| `import <data-dir> <csv> --profile <name> [--yes]` | Preview + import CSV using bank profile rules |
| `verify <data-dir>` | Run `hledger check`; print `ok` or error |
| `lookup <data-dir> <fid>` | Look up transaction by fid, print as JSON |
| `stats <data-dir>` | Print journal statistics as JSON |
| `audit <data-dir>` | Check include integrity, FID uniqueness, orphaned files |
| `migrate-fids <data-dir>` | Add fid tags to untagged transactions |
| `list-files <data-dir>` | List all `.journal` files under the data directory |

### `config` group

| Subcommand | Description |
|------------|-------------|
| `show <config.toml>` | Print parsed config as JSON |
| `validate <config.toml>` | Validate config; exit 0 if valid |
