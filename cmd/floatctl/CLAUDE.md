# cmd/floatctl

Admin and debug CLI for float. It operates directly on internal packages and the data directory, bypassing the gRPC/Connect API entirely. This is distinct from the end-user web UI, which goes through the API.

## Command Shape

```text
floatctl <group> <subcommand> [flags] [args...]
floatctl help
floatctl <group> help
```

Two argument conventions are used:

1. **Flags-first read-only commands** parse flags and then inspect positional args with `fset.Arg(n)`.
   ```bash
   floatctl hledger balance [--depth N] <journal> [query...]
   ```
2. **Data-dir-first mutation commands** extract `<data-dir>` from `args[0]` before parsing `args[1:]`.
   ```bash
   floatctl journal add <data-dir> --description "..." --posting "..."
   ```

## Adding a New Command

1. Create or extend the file for the group, e.g. `journal.go` or `rules.go`.
2. Register commands in `init()` with `register(&Command{...})`.
3. Do **not** edit `main.go` or `registry.go` unless changing dispatch/help behavior itself.

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

## File Layout

```text
cmd/floatctl/
├── main.go        # entry point
├── registry.go    # Command type, register(), dispatch(), help
├── hledger.go     # "hledger" group
├── journal.go     # "journal" group
├── rules.go       # "rules" group
└── config.go      # "config" group
```

## Current Commands

### `hledger` group

| Subcommand | Description |
|------------|-------------|
| `balance [--depth N] <journal> [query...]` | Run `hledger bal -O json` through `internal/hledger` |
| `accounts [--tree] <journal>` | Run `hledger accounts --types`, optionally as a tree |
| `register <journal> [query...]` | Run `hledger reg -O json` |
| `print-csv <csv> <rules>` | Parse CSV with an hledger rules file and print transactions as JSON |
| `version` | Print hledger binary version |
| `check <journal>` | Validate a journal; exits 0 if valid |
| `raw <journal> <subcmd> [args...]` | Run any hledger subcommand and print raw stdout |

### `journal` group

| Subcommand | Description |
|------------|-------------|
| `add <data-dir> --description <text> --posting "account  amount" [--posting ...] [--date YYYY-MM-DD]` | Add a transaction through txlock |
| `delete <data-dir> <fid>` | Remove a transaction by code/FID through txlock |
| `import <data-dir> <csv> --profile <name> [--yes]` | Preview/import CSV using the named bank profile |
| `verify <data-dir>` | Run `hledger check`; print `ok` or the check error |
| `lookup <data-dir> <fid>` | Look up a transaction by FID and print JSON |
| `stats <data-dir>` | Print journal statistics as JSON |
| `audit <data-dir>` | Check include integrity, FID uniqueness, and orphaned files |
| `migrate-fids <data-dir>` | Add/migrate transaction code fields for legacy entries |
| `list-files <data-dir>` | List all `.journal` files under the data directory |
| `snapshots <data-dir> [--limit N]` | List git snapshots maintained by `internal/gitsnap` |
| `restore <data-dir> <hash>` | Restore the data directory to a snapshot |

### `rules` group

| Subcommand | Description |
|------------|-------------|
| `list <data-dir>` | Print `rules.json` rules as JSON |
| `add <data-dir> --pattern <regex> [--payee ...] [--account ...] [--tag k=v] [--priority N] [--auto-reviewed] [--match-account prefix]` | Add a float categorization rule |
| `delete <data-dir> <rule-id>` | Delete a rule from `rules.json` |
| `apply <data-dir> [--rule-id ID ...] [--query TOKEN ...] [--yes]` | Preview/apply matching rules to existing transactions |

### `config` group

| Subcommand | Description |
|------------|-------------|
| `show <config.toml>` | Print parsed config as JSON |
| `validate <config.toml>` | Validate config; exits 0 if valid |
