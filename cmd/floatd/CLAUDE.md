# cmd/floatd

The main float server binary. It serves ConnectRPC/gRPC/gRPC-Web on an h2c HTTP server, serves the embedded web UI at `/`, and starts a daily Stripe auto-import background loop.

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
9. Read `FLOAT_AUTH_PASSPHRASE` into `internal/auth` (auth disabled when unset) and log the auth state.
10. Create the generation-aware cache, `LedgerService` handler, logging + auth interceptors, `/api/auth`, `/api/login`, `/api/logout` endpoints, h2c mux, and embedded web UI handler.
11. Start `handler.StartDailyStripeImport(ctx)`.
12. Serve HTTP until interrupted, then shut down gracefully.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data-dir` | required | Path to float data directory |
| `--addr` | config port or `:8080` | Listen address override |
| `--verbose` | false | Enable debug-level logging, including hledger command args/durations |

Environment variable shortcuts such as `FLOAT_DATA_DIR` and `FLOAT_ADDR` are defined by root `mise.toml` tasks, not read directly by `main.go`. `FLOAT_AUTH_PASSPHRASE` is the exception: `main.go` reads it directly to configure auth.

## HTTP / API

`floatd` registers the generated `LedgerService` handler at `/float.v1.LedgerService/*` with `middleware.NewLoggingInterceptor` and `auth.NewServerInterceptor`. The mux also serves the auth endpoints (`/api/auth`, `/api/login`, `/api/logout`) and falls through to `webui.Handler()` for static web assets, allowing the embedded app and API to share one origin.

The server uses `h2c.NewHandler(..., &http2.Server{})`, so local clients can use HTTP/2 without TLS.

When `FLOAT_AUTH_PASSPHRASE` is set, every RPC requires an `Authorization: Bearer` header (passphrase or session token) or the `float_session` cookie; static assets and the `/api/*` auth endpoints remain open. When unset, all access is open.

## Background Work

`StartDailyStripeImport` runs in the server context. It checks config state and Stripe refresh throttles, fetches/imports new settled transactions for linked accounts, updates `LastDailyImportAt`, and logs per-account errors without crashing the server.

## Web UI

The built web UI is embedded by `internal/webui` from `internal/webui/dist/` when present. Development uses the Vite dev server in `web/`, which proxies API requests to `floatd`.
