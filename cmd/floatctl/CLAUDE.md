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

Flags must precede positional arguments (standard Go `flag` package behavior).
Each command creates its own `flag.NewFlagSet` with `flag.ExitOnError`.

## Current Groups and Commands

| Group     | Subcommands                                          | Unlocked by      |
|-----------|------------------------------------------------------|-----------------|
| `hledger` | balance, accounts, register, print-csv, version, check | now (Step 1)  |
| `journal` | verify, migrate-fids, list-files                     | Step 2          |
| `git`     | log, restore, status                                 | Step 3          |
| `config`  | show, validate                                       | Step 2          |
| `txn`     | lookup, add, delete                                  | Step 7          |
| `import`  | preview, rules-test                                  | Step 9          |
| `rules`   | list, show                                           | Step 10         |
| `cache`   | stats, warm                                          | Step 11         |

See `PLAN.md` for full command descriptions and argument shapes.

## File Layout

```
cmd/floatctl/
├── CLAUDE.md      ← you are here
├── PLAN.md        ← full command scope + future roadmap
├── main.go        ← entry point (arg parsing + dispatch)
├── registry.go    ← Command type, register(), dispatch(), help rendering
└── hledger.go     ← "hledger" group commands
```

Each group lives in its own file (`hledger.go`, `journal.go`, etc.). The pattern is:
one file per group, one `init()` per file, one `Command` struct per subcommand.
