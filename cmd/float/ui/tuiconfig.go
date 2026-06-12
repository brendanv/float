package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type tuiConfig struct {
	Theme string `json:"theme"`
	// AuthTokens maps a floatd server address (the --server value) to the
	// session token obtained from POST /api/login.
	AuthTokens map[string]string `json:"auth_tokens,omitempty"`
}

// String returns the canonical string name for a Theme.
func (t Theme) String() string {
	switch t {
	case ThemeDracula:
		return "dracula"
	case ThemeCatppuccin:
		return "catppuccin"
	case ThemeNord:
		return "nord"
	case ThemeEverforest:
		return "everforest"
	default:
		return "default"
	}
}

func themeFromString(s string) Theme {
	switch s {
	case "dracula":
		return ThemeDracula
	case "catppuccin":
		return ThemeCatppuccin
	case "nord":
		return ThemeNord
	case "everforest":
		return ThemeEverforest
	default:
		return ThemeDefault
	}
}

func tuiConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "float", "tui.json")
}

// loadTUIConfig reads the TUI config file, returning a zero-value config if
// the file does not exist or cannot be parsed.
func loadTUIConfig() tuiConfig {
	var cfg tuiConfig
	p := tuiConfigPath()
	if p == "" {
		return cfg
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

// saveTUIConfig writes the config file, creating its directory if needed.
func saveTUIConfig(cfg tuiConfig) {
	p := tuiConfigPath()
	if p == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return
	}
	_ = os.WriteFile(p, data, 0o600)
}

// LoadTUITheme reads the saved theme from the TUI config file.
// Returns ThemeDefault if the file does not exist or cannot be read.
func LoadTUITheme() Theme {
	return themeFromString(loadTUIConfig().Theme)
}

// saveTUITheme persists the given theme, preserving other config fields.
func saveTUITheme(theme Theme) {
	cfg := loadTUIConfig()
	cfg.Theme = theme.String()
	saveTUIConfig(cfg)
}

// LoadAuthToken returns the saved session token for the given server address,
// or "" if none is saved.
func LoadAuthToken(server string) string {
	return loadTUIConfig().AuthTokens[server]
}

// SaveAuthToken persists the session token for the given server address,
// preserving other config fields.
func SaveAuthToken(server, token string) {
	cfg := loadTUIConfig()
	if cfg.AuthTokens == nil {
		cfg.AuthTokens = make(map[string]string)
	}
	cfg.AuthTokens[server] = token
	saveTUIConfig(cfg)
}
