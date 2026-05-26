package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type ServerConfig struct {
	Port    int `toml:"port"`     // default 8080 if zero
	SSHPort int `toml:"ssh_port"` // SSH TUI port; disabled if zero
}

type User struct {
	Name           string `toml:"name"`
	Role           string `toml:"role"` // "admin" or "viewer"
	PassphraseHash string `toml:"passphrase_hash"`
}

type BankProfile struct {
	Name      string `toml:"name"`
	RulesFile string `toml:"rules_file"` // relative to data dir
}

type AlphaVantageConfig struct {
	APIKey string `toml:"api_key"`
}

type AIConfig struct {
	Model  string `toml:"model"`  // OpenRouter model ID; empty = use default
	Prompt string `toml:"prompt"` // User-defined guidelines prepended to AI system prompts
}

type StripeLinkedAccount struct {
	StripeAccountID          string `toml:"stripe_account_id"`
	HledgerAccount           string `toml:"hledger_account"`
	DisplayName              string `toml:"display_name"`
	LastFetchedAt            string `toml:"last_fetched_at"`             // RFC3339; empty if never fetched
	LastTransactionRefreshID string `toml:"last_transaction_refresh_id"` // Stripe refresh ID used for next ListTransactions filter; empty if never fetched
}

type StripeConfig struct {
	CustomerID         string                `toml:"customer_id"`
	DailyImportEnabled bool                  `toml:"daily_import_enabled"`
	LastDailyImportAt  string                `toml:"last_daily_import_at"` // RFC3339; empty if never run
	LinkedAccounts     []StripeLinkedAccount `toml:"linked_accounts"`
}

type GitConfig struct {
	RemoteURL  string `toml:"remote_url"`   // empty = push disabled
	AuthToken  string `toml:"auth_token"`   // HTTPS personal access token
	SSHKeyPath string `toml:"ssh_key_path"` // path to SSH private key; empty = ~/.ssh/id_rsa
}

type Config struct {
	Server       ServerConfig       `toml:"server"`
	Users        []User             `toml:"users"`
	BankProfiles []BankProfile      `toml:"bank_profiles"`
	AlphaVantage AlphaVantageConfig `toml:"alpha_vantage"`
	AI           AIConfig           `toml:"ai"`
	Stripe       StripeConfig       `toml:"stripe"`
	Git          GitConfig          `toml:"git"`
}

// Load parses config.toml at path and returns a *Config.
// Returns error if the file doesn't exist or is not valid TOML.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	var cfg Config
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return &cfg, nil
}

// Save encodes cfg as TOML and atomically writes it to path (write to temp, then rename).
func Save(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".config-*.toml.tmp")
	if err != nil {
		return fmt.Errorf("config: create temp: %w", err)
	}
	tmpPath := f.Name()
	if err := toml.NewEncoder(f).Encode(cfg); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("config: encode %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("config: close temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("config: rename %s -> %s: %w", tmpPath, path, err)
	}
	return nil
}
