package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/brendanv/float/internal/config"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		path    func(t *testing.T) string
		wantErr bool
		check   func(t *testing.T, cfg *config.Config)
	}{
		{
			name: "valid config parses all fields",
			path: func(t *testing.T) string { return "testdata/config.toml" },
			check: func(t *testing.T, cfg *config.Config) {
				if cfg.Server.Port != 9090 {
					t.Errorf("port: got %d, want 9090", cfg.Server.Port)
				}
				if len(cfg.Users) != 2 {
					t.Fatalf("users: got %d, want 2", len(cfg.Users))
				}
				if cfg.Users[0].Name != "alice" || cfg.Users[0].Role != "admin" {
					t.Errorf("users[0]: got %+v", cfg.Users[0])
				}
				if len(cfg.BankProfiles) != 2 {
					t.Fatalf("bank_profiles: got %d, want 2", len(cfg.BankProfiles))
				}
				if cfg.BankProfiles[0].Name != "Chase Checking" {
					t.Errorf("bank_profiles[0].name: got %q", cfg.BankProfiles[0].Name)
				}
			},
		},
		{
			name:    "missing file returns error",
			path:    func(t *testing.T) string { return "testdata/nonexistent.toml" },
			wantErr: true,
		},
		{
			name: "invalid TOML returns error",
			path: func(t *testing.T) string {
				p := filepath.Join(t.TempDir(), "bad.toml")
				if err := os.WriteFile(p, []byte("not = [valid toml"), 0644); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantErr: true,
		},
		{
			name: "empty file returns zero-value config",
			path: func(t *testing.T) string {
				p := filepath.Join(t.TempDir(), "empty.toml")
				if err := os.WriteFile(p, []byte{}, 0644); err != nil {
					t.Fatal(err)
				}
				return p
			},
			check: func(t *testing.T, cfg *config.Config) {
				if cfg.Server.Port != 0 {
					t.Errorf("port: got %d, want 0", cfg.Server.Port)
				}
				if len(cfg.Users) != 0 {
					t.Errorf("users: got %d, want 0", len(cfg.Users))
				}
				if len(cfg.BankProfiles) != 0 {
					t.Errorf("bank_profiles: got %d, want 0", len(cfg.BankProfiles))
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.Load(tt.path(t))
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

func TestSave_RoundTrip(t *testing.T) {
	original := &config.Config{
		Server: config.ServerConfig{Port: 7777},
		Users: []config.User{
			{Name: "carol", Role: "admin", PassphraseHash: "hash1"},
		},
		BankProfiles: []config.BankProfile{
			{Name: "My Bank", RulesFile: "rules/mybank.rules"},
		},
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := config.Save(path, original); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Server.Port != original.Server.Port {
		t.Errorf("port: got %d, want %d", loaded.Server.Port, original.Server.Port)
	}
	if len(loaded.Users) != 1 || loaded.Users[0].Name != "carol" {
		t.Errorf("users mismatch: got %+v", loaded.Users)
	}
	if len(loaded.BankProfiles) != 1 || loaded.BankProfiles[0].Name != "My Bank" {
		t.Errorf("bank_profiles mismatch: got %+v", loaded.BankProfiles)
	}
}
