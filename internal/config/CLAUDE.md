# internal/config

Loads and saves `config.toml`, the single configuration file for a float data directory. This package stores config data only; it does not perform auth, encryption, or passphrase hashing.

## API

- `Load(path string) (*Config, error)` — reads and decodes TOML; returns an error if the file is missing or invalid.
- `Save(path string, cfg *Config) error` — writes TOML atomically using a temp file in the same directory plus `os.Rename`. Callers must hold `txlock` or another appropriate lock when saving shared config.
- `(*Config).Location() *time.Location` — returns the configured IANA timezone or `time.UTC` when empty/invalid.

## Config Shape

Top-level `Config` fields:

- `Server` — `port`.
- `BankProfiles` — name plus hledger CSV `rules_file` path relative to the data dir.
- `AlphaVantage` — `api_key` for price backfills.
- `AI` — OpenRouter `model` override and user prompt guidelines.
- `Stripe` — customer ID, daily import toggle/timestamp, and linked Financial Connections accounts.
- `Timezone` — IANA timezone used when converting external timestamps (notably Stripe) to journal dates; defaults to UTC.

## Stripe Config Notes

`StripeLinkedAccount` stores the Stripe Financial Connections account ID, target hledger account, display name, and `last_fetched_at` (informational last import time). Institution names are fetched from Stripe at runtime rather than persisted here.

## Boundaries

- Passphrase hashing/verification is outside this package.
- API handlers are responsible for validating user-supplied timezone/model/API-key values before saving.
- Config mutations should normally happen inside `txlock.Do()` so cache generation and snapshots remain coherent with file changes.
