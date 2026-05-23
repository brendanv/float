# internal/config

Loads and saves `config.toml` — the single configuration file for a float data directory.

- `Load(path string) (*Config, error)` — reads and decodes `config.toml`; returns error if missing or invalid TOML
- `Save(path string, cfg *Config) error` — encodes and writes `config.toml` (not goroutine-safe; caller must hold a lock)

Key types: `Config` (top-level), `ServerConfig` (port, defaults to 8080), `User` (name, role `"admin"`/`"viewer"`, argon2id passphrase hash), `BankProfile` (name, rules_file path), `StripeConfig` (holds `[]StripeLinkedAccount`), `StripeLinkedAccount` (StripeAccountID, HledgerAccount, DisplayName, LastFetchedAt RFC3339), `TailscaleConfig` (Enabled, AuthKey, Hostname, StateDir, Funnel — drives `internal/tsnetsrv` which embeds a Tailscale node for the Stripe webhook listener), `WebhooksConfig` (StripeSigningSecret — env `STRIPE_WEBHOOK_SECRET` takes precedence).

`Save` writes atomically via a temp file + rename to prevent partial writes.

Passphrase hashing (argon2id) is handled outside this package — `config` stores only the already-hashed value.
