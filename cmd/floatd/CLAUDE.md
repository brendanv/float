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

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data-dir` | (required) | Path to float data directory |
| `--addr` | from config or `:8080` | Listen address |
| `--verbose` | false | Enable debug logging (hledger commands, durations) |

Environment variable shortcuts: `FLOAT_DATA_DIR`, `FLOAT_ADDR` (set in `mise.toml` tasks).

## SSH TUI (`ssh.go`)

When `server.ssh_port` is set in `config.toml`, `startSSHServer` launches a Wish-based SSH server that runs the `float` TUI for each connection. The host key is stored at `$FLOAT_DATA_DIR/ssh_host_key` (generated on first start, gitignored).

## Web UI

The built web UI (`internal/webui/dist/`) is embedded via `internal/webui` and served at `/`. API requests to `/float.v1.LedgerService/*` are handled by the ConnectRPC mux before falling through to the static file handler.

## Webhooks

floatd accepts Stripe Financial Connections webhooks at `POST /webhooks/stripe`. The receiver verifies the `Stripe-Signature` header against `STRIPE_WEBHOOK_SECRET` and, on a `financial_connections.account.refreshed_transactions` event, kicks off an async per-account import that reuses the same dedup/rules/`txlock` flow as the daily auto-import.

| Env var | Required | Purpose |
|---------|----------|---------|
| `STRIPE_WEBHOOK_SECRET` | yes (for the endpoint) | Signing secret (`whsec_...`) from the Stripe dashboard. Endpoint returns 503 if unset. |
| `STRIPE_SECRET_KEY` | yes (to import) | API key used to refresh + list transactions. |

The endpoint accepts only `POST`, caps body size at 1 MiB, and dedupes recently-seen `event.id`s for an hour. Daily polling remains enabled in parallel as a safety net for missed deliveries.

### Exposing floatd to Stripe via Tailscale Funnel

When floatd runs on a private network (e.g. a NAS on a tailnet), use [Tailscale Funnel](https://tailscale.com/kb/1223/funnel) to expose just the webhook path to the public internet. TLS terminates at Tailscale's edge; the container needs no certificates and no router port-forward.

```bash
# Enable Funnel on port 8080 (or 443/8443/10000 — Funnel's allowed ports).
tailscale funnel --bg 8080
tailscale funnel status  # confirm the public URL
```

Then register the webhook in the Stripe dashboard at:

```
https://<machine>.<tailnet>.ts.net/webhooks/stripe
```

Subscribe to event `financial_connections.account.refreshed_transactions`. Copy the signing secret into the floatd container as `STRIPE_WEBHOOK_SECRET`.

Because Funnel exposes the endpoint publicly, signature verification is the only authentication — keep `STRIPE_WEBHOOK_SECRET` secret and rotate it through the Stripe dashboard if it leaks.
