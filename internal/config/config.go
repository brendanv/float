package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

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
	SkipRules bool   `toml:"skip_rules"`
}

type AlphaVantageConfig struct {
	APIKey string `toml:"api_key"`
}

type AIConfig struct {
	Model  string `toml:"model"`  // OpenRouter model ID; empty = use default
	Prompt string `toml:"prompt"` // User-defined guidelines prepended to AI system prompts
}

type StripeLinkedAccount struct {
	StripeAccountID string `toml:"stripe_account_id"`
	HledgerAccount  string `toml:"hledger_account"`
	DisplayName     string `toml:"display_name"`
	LastFetchedAt   string `toml:"last_fetched_at"` // RFC3339; last import time, empty if never imported
}

type StripeConfig struct {
	CustomerID         string                `toml:"customer_id"`
	DailyImportEnabled bool                  `toml:"daily_import_enabled"`
	LastDailyImportAt  string                `toml:"last_daily_import_at"` // RFC3339; empty if never run
	LinkedAccounts     []StripeLinkedAccount `toml:"linked_accounts"`
}

type Config struct {
	Server       ServerConfig       `toml:"server"`
	Users        []User             `toml:"users"`
	BankProfiles []BankProfile      `toml:"bank_profiles"`
	AlphaVantage AlphaVantageConfig `toml:"alpha_vantage"`
	AI           AIConfig           `toml:"ai"`
	Stripe       StripeConfig       `toml:"stripe"`
	// Timezone is an IANA timezone name (e.g. "America/New_York") used when
	// converting Stripe transaction timestamps to calendar dates. Defaults to
	// UTC when empty. Set this to your local timezone to avoid off-by-one-day
	// errors for transactions that occur in the evening in your local time.
	Timezone string `toml:"timezone"`
}

// Location returns the configured timezone as a *time.Location.
// Returns time.UTC if Timezone is empty or unrecognized.
func (c *Config) Location() *time.Location {
	if c == nil || c.Timezone == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
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
