package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type tuiConfig struct {
	Theme string `json:"theme"`
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

// LoadTUITheme reads the saved theme from the TUI config file.
// Returns ThemeDefault if the file does not exist or cannot be read.
func LoadTUITheme() Theme {
	p := tuiConfigPath()
	if p == "" {
		return ThemeDefault
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return ThemeDefault
	}
	var cfg tuiConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ThemeDefault
	}
	return themeFromString(cfg.Theme)
}

// saveTUITheme persists the given theme to the TUI config file.
func saveTUITheme(theme Theme) {
	p := tuiConfigPath()
	if p == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(tuiConfig{Theme: theme.String()})
	if err != nil {
		return
	}
	_ = os.WriteFile(p, data, 0o644)
}
