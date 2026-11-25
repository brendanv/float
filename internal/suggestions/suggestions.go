// Package suggestions stores AI account suggestions for transactions whose
// posting account contains "unknown". Suggestions are advisory metadata kept
// outside the journal (account-suggestions.json, gitignored by gitsnap): they
// never change postings, and an entry is only meaningful while its
// transaction still has an unknown posting.
package suggestions

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// FileName is the store's file name in the data directory.
const FileName = "account-suggestions.json"

// Suggestion is the stored AI result for one transaction. An empty Account
// records that the transaction was processed but the AI had no suggestion,
// so it is not sent again.
type Suggestion struct {
	Account    string `json:"account,omitempty"`
	Confidence string `json:"confidence,omitempty"` // "high" | "medium" | "low"
	Reasoning  string `json:"reasoning,omitempty"`
	Model      string `json:"model,omitempty"`
	CreatedAt  string `json:"created_at"` // RFC3339
}

// Store maps transaction FID to its suggestion.
type Store map[string]Suggestion

// FilePath returns the absolute path to the store in dataDir.
func FilePath(dataDir string) string {
	return filepath.Join(dataDir, FileName)
}

// Load reads the store from dataDir. A missing file is an empty store.
func Load(dataDir string) (Store, error) {
	path := FilePath(dataDir)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Store{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("suggestions: read %s: %w", path, err)
	}
	s := Store{}
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("suggestions: parse %s: %w", path, err)
	}
	return s, nil
}

// Save writes s to dataDir atomically (temp file + rename). Must be called
// within a txlock callback that declares FilePath(dataDir).
func Save(dataDir string, s Store) error {
	path := FilePath(dataDir)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("suggestions: marshal: %w", err)
	}
	f, err := os.CreateTemp(dataDir, ".account-suggestions-*.json.tmp")
	if err != nil {
		return fmt.Errorf("suggestions: create temp: %w", err)
	}
	tmpPath := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("suggestions: write %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("suggestions: close temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("suggestions: rename %s -> %s: %w", tmpPath, path, err)
	}
	return nil
}
