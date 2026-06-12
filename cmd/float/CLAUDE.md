# cmd/float

The end-user terminal UI client. It connects to a running `floatd` over plain HTTP/2 (h2c) using the generated ConnectRPC `LedgerServiceClient`, then renders a Bubble Tea UI.

## Usage

```bash
float --server localhost:8080   # default address
```

The `--server` flag is a `host:port` for `floatd`. The client builds an h2c HTTP client and uses `http://<server>`; TLS is not used by this binary.

## Authentication

When floatd has `FLOAT_AUTH_PASSPHRASE` set, the TUI authenticates with a bearer credential attached by `auth.NewClientInterceptor`. The credential is seeded from the `FLOAT_AUTH_PASSPHRASE` environment variable or a session token previously saved in `tui.json`. On startup, `ui.AuthGate` (`ui/authgate.go`) probes the server with a cheap RPC; on `CodeUnauthenticated` it shows a passphrase prompt, exchanges the passphrase for a session token via `POST /api/login`, persists the token per server in `tui.json`, and only then initializes the main UI. Other probe failures (e.g. connection refused) fall through to the UI, which surfaces them per-tab as before.

## Architecture

`main.go` creates the ConnectRPC client (with the auth client interceptor) and wraps `ui.New(client)` in `ui.NewAuthGate(...)`. All RPC calls, state, rendering, key handling, and forms live in `cmd/float/ui/`.

The root `ui.Model` owns the tab state, help model, theme, layout, and the per-tab models. Global keys:

- `tab` / `shift+tab` switch tabs.
- `?` toggles expanded help.
- `q` / `ctrl+c` quit unless an active form or confirmation dialog is capturing keys.

## ui/ Package

Current top-level tabs:

| Tab | Primary files | Purpose |
|-----|---------------|---------|
| Home | `hometab.go`, `transactions.go`, `addtx.go` | Dashboard, recent/searchable transactions, add/edit/delete transaction modal flows |
| Accounts | `managertab.go`, `accounts.go`, `accounttree.go`, `accountregister.go` | Account tree plus focused account register |
| Trends | `trendstab.go`, `networth.go`, `chartpanel.go`, `insights.go` | Net worth and trend visualizations |
| Portfolio | `portfoliotab.go` | Investment holdings, prices, allocation, gain/loss |
| Monthly | `monthlytab.go` | Income statement / monthly revenue-expense dashboard |
| Assertions | `assertionstab.go` | Accounts ranked by balance-assertion drift; edit transaction to add assertions |
| Manage | `managetab.go`, plus `importstab.go`, `rulestab.go`, `pricestab.go`, `snapshotstab.go`, `tagstab.go`, `payeestab.go`, `stripetab.go` | Bulk management, imports, rules, prices, snapshots, tags, payees, Stripe connections |
| Settings | `settingstab.go`, `tuiconfig.go` | Theme and TUI settings |

Supporting files:

- `fetch.go` — shared RPC command helpers and message types.
- `filter.go`, `presets.go`, `period.go` — search/filter/date-range state.
- `layout.go`, `panel.go`, `summary.go`, `style.go`, `helpbar.go`, `spinner.go`, `modal.go` — shared layout and presentation utilities.

## Theme / Config

The TUI detects terminal background color and applies a saved theme loaded by `LoadTUITheme()`. Theme changes and per-server auth session tokens are persisted by `tuiconfig.go` in `~/.config/float/tui.json` using load-modify-write so the fields don't clobber each other.

## SSH Access

The TUI can also be accessed over SSH when `floatd` has `server.ssh_port` configured. The SSH server lives in `cmd/floatd/ssh.go`; this binary only performs direct h2c client connections.
