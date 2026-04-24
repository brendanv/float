# cmd/float

The float TUI client. Connects to a running `floatd` over HTTP/2 (h2c) and renders a terminal UI using Bubbletea.

## Usage

```bash
float --server localhost:8080   # default address
```

The `--server` flag specifies the `host:port` of `floatd`. The client uses plain HTTP/2 without TLS (h2c).

## Architecture

`main.go` creates a ConnectRPC `LedgerServiceClient` and passes it to `ui.New(client)`, which returns a Bubbletea `Model`. All gRPC calls are made inside the `ui/` package.

## ui/ Package

The TUI is a tabbed application (`ui/tabbar.go`, `ui/app.go`). Each tab is a Bubbletea component with its own update/view logic:

| File | Tab |
|------|-----|
| `hometab.go` | Dashboard with balance summary |
| `transactions.go` | Transaction list with search/filter |
| `accounttree.go`, `accountregister.go` | Account tree and drill-down register |
| `trendstab.go`, `networth.go` | Net worth chart |
| `importstab.go` | CSV import |
| `pricestab.go` | Commodity prices |
| `rulestab.go` | Auto-categorization rules |
| `snapshotstab.go` | Journal snapshots |
| `settingstab.go` | TUI settings |
| `managetab.go`, `managertab.go` | Bulk transaction management |
| `tagstab.go` | Tags overview |

Supporting files: `fetch.go` (gRPC call helpers), `filter.go` (search/filter state), `layout.go` (responsive sizing), `period.go` (date range selection), `modal.go` (overlay dialogs), `addtx.go` (add transaction form), `style.go` (Lipgloss styles), `spinner.go`, `helpbar.go`, `panel.go`, `summary.go`.

## SSH Access

The TUI can also be accessed via SSH when `floatd` has `ssh_port` configured. The `float` binary itself only does direct TCP connections — the SSH server is in `cmd/floatd/ssh.go`.
