# cmd/floatd

The main float server binary. It serves ConnectRPC/gRPC/gRPC-Web on an h2c HTTP server and serves the embedded web UI at `/`. It can also host the TUI over SSH and starts a daily Stripe auto-import background loop.

## Startup Sequence

1. Parse flags (`--data-dir`, `--addr`, `--verbose`).
2. Configure structured logging. Logs go to stderr and through `internal/logstream` for the `StreamLogs` RPC.
3. Load `<data-dir>/config.toml`.
4. Choose the listen address from `--addr`, `config.server.port`, or `:8080`.
5. Ensure `main.journal` exists, creating an empty file if missing.
6. Initialize `hledger.Client` for `main.journal` (validates the pinned hledger version).
7. Initialize `txlock.TxLock` and `gitsnap.Repo`; call `RecoverUncommitted` for crash leftovers and register the repo with `lock.SetSnap`.
8. Run startup migrations inside txlock:
   - `journal.MigrateFIDs` to assign transaction code fields.
   - `journal.MigrateStripeTxnTag` to rename old `stripe-txn` tags to `float-stripe-txn`.
   - `journal.EnsureCommodityDirective(..., "USD")`.
   - account declaration bootstrap: ensure `accounts.journal`, ensure its include, and append currently undeclared accounts.
9. Create the generation-aware cache, `LedgerService` handler, logging interceptor, h2c mux, and embedded web UI handler.
10. Start optional Wish SSH server if `config.server.ssh_port` is set.
11. Start `handler.StartDailyStripeImport(ctx)`.
12. Serve HTTP until interrupted, then shut down gracefully.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data-dir` | required | Path to float data directory |
| `--addr` | config port or `:8080` | Listen address override |
| `--verbose` | false | Enable debug-level logging, including hledger command args/durations |

Environment variable shortcuts such as `FLOAT_DATA_DIR` and `FLOAT_ADDR` are defined by root `mise.toml` tasks, not read directly by `main.go`.

## HTTP / API

`floatd` registers the generated `LedgerService` handler at `/float.v1.LedgerService/*` with `middleware.NewLoggingInterceptor`. The same mux then falls through to `webui.Handler()` for static web assets, allowing the embedded app and API to share one origin.

The server uses `h2c.NewHandler(..., &http2.Server{})`, so local clients can use HTTP/2 without TLS. There is currently no auth middleware.

## SSH TUI (`ssh.go`)

When `server.ssh_port` is set, `startSSHServer` launches a Wish SSH server. Each SSH session creates a local h2c `LedgerServiceClient` pointed at the running `floatd` address and runs `ui.New(client)` with Bubble Tea. The host key is `$FLOAT_DATA_DIR/ssh_host_key` and is generated on first start.

## Background Work

`StartDailyStripeImport` runs in the server context. It checks config state and Stripe refresh throttles, fetches/imports new settled transactions for linked accounts, updates `LastDailyImportAt`, and logs per-account errors without crashing the server.

## Web UI

The built web UI is embedded by `internal/webui` from `internal/webui/dist/` when present. Development uses the Vite dev server in `web/`, which proxies API requests to `floatd`.
