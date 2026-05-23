# cmd/floatd

The main float server binary. Serves gRPC, gRPC-Web, and Connect protocols on the same HTTP/2 port, with the web UI embedded at `/`.

## Startup Sequence

1. Parse flags (`--data-dir`, `--addr`, `--verbose`)
2. Load `config.toml` from the data directory
3. Initialise `hledger.Client` (validates hledger binary and version)
4. Initialise `txlock.TxLock` and `gitsnap.Repo`; call `RecoverUncommitted` to snapshot any dirty files left from a crash
5. Run FID backfill (`journal.MigrateFIDs`) to assign codes to any legacy transactions
6. Declare any undeclared accounts in `accounts.journal`
7. Start the ConnectRPC HTTP/2 server (h2c — plain HTTP/2 without TLS)
8. Optionally start the SSH TUI server (`ssh.go`) if `ssh_port` is configured
9. Optionally start the embedded Tailscale node + webhook listener (`internal/tsnetsrv`) if `[tailscale]` is enabled. The webhook listener has its own dedicated `http.ServeMux` — only `/webhooks/stripe` and `/healthz` are exposed; the main API on `:8080` is never reachable from this listener.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data-dir` | (required) | Path to float data directory |
| `--addr` | from config or `:8080` | Listen address |
| `--verbose` | false | Enable debug logging (hledger commands, durations) |

Environment variable shortcuts: `FLOAT_DATA_DIR`, `FLOAT_ADDR` (set in `mise.toml` tasks).

## Stripe Webhooks (Tailscale Funnel)

When `[tailscale] enabled = true` in `config.toml` and a Stripe webhook signing secret is configured (via `[webhooks] stripe_signing_secret` or `STRIPE_WEBHOOK_SECRET` env var), floatd joins the user's tailnet via `tsnet` and serves a dedicated listener on `:443` containing only:

- `POST /webhooks/stripe` — verifies the `Stripe-Signature` header (5-minute replay tolerance from the Stripe SDK), parses the event, and dispatches `financial_connections.account.refreshed_transactions` and `financial_connections.account.disconnected` to `serverledger.Handler.WebhookImportStripeAccount` / `WebhookMarkStripeAccountDisconnected` asynchronously. Always 200s within Stripe's 10 s window.
- `GET /healthz` — for Stripe's webhook dashboard health check.

The webhook handler does **not** trigger a refresh — it treats the webhook itself as the refresh-completion signal and goes straight to `ListTransactions` + import (mirroring the daily auto-import path).

Configuration:

```toml
[tailscale]
enabled  = true
hostname = "floatd"
funnel   = true       # publicly expose via Tailscale Funnel; require Funnel approval in tailnet ACLs

[webhooks]
# Prefer env var STRIPE_WEBHOOK_SECRET when secrets shouldn't live in config.toml.
# stripe_signing_secret = "whsec_..."
```

Required env vars (auth keys must not be committed):

- `TS_AUTHKEY` — one-time tailnet auth key (overrides `tailscale.auth_key` if set)
- `STRIPE_SECRET_KEY` — Stripe API key used by the import flow that the webhook triggers
- `STRIPE_WEBHOOK_SECRET` — webhook signing secret from the Stripe dashboard

State directory: tsnet stores its node state at `<data_dir>/tsnet/` by default (override with `[tailscale] state_dir`).

## SSH TUI (`ssh.go`)

When `server.ssh_port` is set in `config.toml`, `startSSHServer` launches a Wish-based SSH server that runs the `float` TUI for each connection. The host key is stored at `$FLOAT_DATA_DIR/ssh_host_key` (generated on first start, gitignored).

## Web UI

The built web UI (`internal/webui/dist/`) is embedded via `internal/webui` and served at `/`. API requests to `/float.v1.LedgerService/*` are handled by the ConnectRPC mux before falling through to the static file handler.
