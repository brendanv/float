# cmd/float

The end-user terminal UI client. It connects to a running `floatd` over plain HTTP/2 (h2c) using the generated ConnectRPC `LedgerServiceClient`, then renders a Bubble Tea UI.

## Usage

```bash
float --server localhost:8080   # default address
```

The `--server` flag is a `host:port` for `floatd`. The client builds an h2c HTTP client and uses `http://<server>`; TLS is not used by this binary.

## Architecture

`main.go` creates the ConnectRPC client and passes it to `ui.New(client)`. All RPC calls, state, rendering, key handling, and forms live in `cmd/float/ui/`.

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

The TUI detects terminal background color and applies a saved theme loaded by `LoadTUITheme()`. Theme changes are persisted by `tuiconfig.go`.

## SSH Access

The TUI can also be accessed over SSH when `floatd` has `server.ssh_port` configured. The SSH server lives in `cmd/floatd/ssh.go`; this binary only performs direct h2c client connections.
