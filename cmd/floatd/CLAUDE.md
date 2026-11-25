# cmd/floatd

The main float server binary. It serves ConnectRPC/gRPC/gRPC-Web on an h2c HTTP server, serves the embedded web UI at `/`, and starts a daily Stripe auto-import background loop.

## Startup Sequence

`main` parses flags, reads `FLOAT_AUTH_PASSPHRASE`, sets up SIGINT/SIGTERM cancellation and calls `run(ctx, options)`, which does everything below and returns an error instead of exiting (so `main_test.go` can start the real server on `127.0.0.1:0`; `options.ready` reports the bound address).

1. Parse flags (`--data-dir`, `--addr`, `--verbose`).
2. Configure structured logging. Logs go to stderr and through `internal/logstream` for the `StreamLogs` RPC.
3. Load `<data-dir>/config.toml`.
4. Choose the listen address from `--addr`, `config.server.port`, or `:8080`.
5. Ensure `main.journal` exists, creating an empty file if missing.
6. Initialize `hledger.Client` for `main.journal` (validates the pinned hledger version), then apply `cfg.Server.HledgerConcurrency` via `hl.SetConcurrency`.
7. Initialize `txlock.TxLock` and `gitsnap.Repo`; call `RecoverUncommitted` for crash leftovers and register the repo with `lock.SetSnap`.
8. Run startup migrations inside txlock:
   - `journal.MigrateFIDs` to assign transaction code fields.
   - `journal.MigrateStripeTxnTag` to rename old `stripe-txn` tags to `float-stripe-txn`.
   - `journal.EnsureCommodityDirective(..., "USD")`.
   - account declaration bootstrap: ensure `accounts.journal`, ensure its include, and append currently undeclared accounts.
9. Read `FLOAT_AUTH_PASSPHRASE` into `internal/auth` (auth disabled when unset) and log the auth state.
10. Create the generation-aware cache, `LedgerService` handler, logging + auth interceptors, gRPC reflection handlers, `/api/auth`, `/api/login`, `/api/logout` endpoints, h2c mux, and embedded web UI handler.
11. Create an `internal/warm.Warmer` over `handler.WarmEntries` and register it as a `lock.OnCommit` hook, so every successful write schedules a debounced (500ms) cache-warming pass. Also register a second `lock.OnCommit` hook (`newStatsLogger`) that diffs `cache.Cache.Stats()` and `hledger.Client.Invocations()` against the previous generation and logs the delta at Info level (reaches `StreamLogs` via the broadcast handler) — the target steady state is zero hledger invocations between writes.
12. Bind the listener (`net.Listen`), then start `handler.StartDailyStripeImport(ctx)` and kick the startup warm pass (`warmer.Start(ctx)`) — only once the listener is bound, so warming never delays accepting connections — before serving HTTP.
13. Serve HTTP until `ctx` is cancelled, then shut down gracefully and return.

## Tests

`main_test.go` is the whole-server smoke test: it runs `run()` against a temp data dir with real hledger and checks the public `/api/auth`, unauthenticated RPC rejection, login cookie and bearer auth, gRPC reflection (h2c) behind auth, a write that lands on disk / in balances / as a git snapshot, the `/mcp` endpoint (401 without a bearer, one tool call with it), rejection of an unbalanced transaction, and startup failure on an invalid journal. Keep it fast and wiring-focused; feature behavior belongs in package tests and `web/e2e`.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data-dir` | required | Path to float data directory |
| `--addr` | config port or `:8080` | Listen address override |
| `--verbose` | false | Enable debug-level logging, including hledger command args/durations |

Environment variable shortcuts such as `FLOAT_DATA_DIR` and `FLOAT_ADDR` are defined by root `mise.toml` tasks, not read directly by `main.go`. `FLOAT_AUTH_PASSPHRASE` is the exception: `main.go` reads it directly to configure auth.

## HTTP / API

`floatd` registers the generated `LedgerService` handler at `/float.v1.LedgerService/*` with `middleware.NewLoggingInterceptor`, `auth.NewServerInterceptor` and `serverledger.NewUndoInterceptor` (sets the `Float-Snapshot` trailer on undoable mutations). It also registers gRPC server reflection handlers (`grpcreflect` v1 and v1alpha) for `LedgerService` with the same interceptors, so reflection requires auth when it is enabled. It mounts the MCP endpoint at `/mcp` (`internal/server/mcpserver`) behind `auth.RequireBearer`: tool calls use a Connect client over an `mcpserver.InProcess` server wrapping the same interceptor-wrapped service handler, which is shut down only when `run` returns, after the public server has drained. The mux also serves the auth endpoints (`/api/auth`, `/api/login`, `/api/logout`) and falls through to `webui.Handler()` for static web assets, allowing the embedded app and API to share one origin.

The server uses `h2c.NewHandler(..., &http2.Server{})`, so local clients can use HTTP/2 without TLS.

When `FLOAT_AUTH_PASSPHRASE` is set, `/mcp` requires the bearer header, and every RPC requires an `Authorization: Bearer` header (passphrase or session token) or the `float_session` cookie; static assets and the `/api/*` auth endpoints remain open. When unset, all access is open.

## Background Work

`StartDailyStripeImport` runs in the server context. It checks config state and Stripe refresh throttles, fetches/imports new settled transactions for linked accounts, updates `LastDailyImportAt`, and logs per-account errors without crashing the server.

## Web UI

The built web UI is embedded by `internal/webui` from `internal/webui/dist/` when present. Development uses the Vite dev server in `web/`, which proxies API requests to `floatd`.
