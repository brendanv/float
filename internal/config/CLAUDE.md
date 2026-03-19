# internal/config

Loads and saves `config.toml` — the single configuration file for a float data directory.

## Types

| Type | Description |
|------|-------------|
| `Config` | Top-level struct; contains `Server`, `Users`, `BankProfiles` |
| `ServerConfig` | `port` (int); defaults to 8080 if zero |
| `User` | `name`, `role` (`"admin"` or `"viewer"`), `passphrase_hash` (argon2id) |
| `BankProfile` | `name`, `rules_file` (relative path to the hledger rules file) |

## Functions

| Function | Description |
|----------|-------------|
| `Load(path string) (*Config, error)` | Read and decode `config.toml`; error if missing or invalid TOML |
| `Save(path string, cfg *Config) error` | Encode `cfg` as TOML and write to `path` (creates or overwrites) |

## Usage Notes

- `Load` returns an error on missing file — callers that need a default config should check `os.IsNotExist`.
- `Save` creates the file if it doesn't exist; no atomic write (do not call from concurrent goroutines without a lock).
- Passphrase hashing (argon2id) is handled elsewhere — `config` stores only the already-hashed value.
