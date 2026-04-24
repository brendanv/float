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
