package config

import (
	"fmt"
	"os"

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

type Config struct {
	Server       ServerConfig  `toml:"server"`
	Users        []User        `toml:"users"`
	BankProfiles []BankProfile `toml:"bank_profiles"`
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

// Save encodes cfg as TOML and writes it to path (creates or overwrites).
func Save(path string, cfg *Config) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("config: create %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	if err := toml.NewEncoder(f).Encode(cfg); err != nil {
		return fmt.Errorf("config: encode %s: %w", path, err)
	}
	return nil
}
