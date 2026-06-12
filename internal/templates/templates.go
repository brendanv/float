package templates

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Template is a saved transaction shape used for bulk entry.
// It captures accounts and optional default amounts so users
// can quickly enter recurring transactions without re-specifying
// the posting structure each time.
type Template struct {
	ID       string            `json:"id"`       // 8-char hex (MintFID)
	Name     string            `json:"name"`     // display name
	Payee    string            `json:"payee"`    // optional default payee
	Note     string            `json:"note"`     // optional default note (after "|" in description)
	Postings []TemplatePosting `json:"postings"` // ordered posting slots
	Tags     map[string]string `json:"tags"`     // default tags to apply to each transaction
}

// TemplatePosting is one posting slot in a template.
// If DefaultQuantity is empty, that posting is the auto-balance posting
// (hledger infers its amount to make the transaction balance).
type TemplatePosting struct {
	Account         string `json:"account"`          // required account path
	Commodity       string `json:"commodity"`        // e.g. "USD"
	DefaultQuantity string `json:"default_quantity"` // empty = auto-balance
	Comment         string `json:"comment"`          // optional inline posting comment
}

const templatesFile = "templates.json"

// FilePath returns the absolute path to templates.json in dataDir.
func FilePath(dataDir string) string {
	return filepath.Join(dataDir, templatesFile)
}

// Load reads templates.json from dataDir. Returns an empty slice if the file
// does not exist (not an error).
func Load(dataDir string) ([]Template, error) {
	path := filepath.Join(dataDir, templatesFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("templates: read %s: %w", path, err)
	}
	var ts []Template
	if err := json.Unmarshal(data, &ts); err != nil {
		return nil, fmt.Errorf("templates: parse %s: %w", path, err)
	}
	return ts, nil
}

// Save writes templates to templates.json in dataDir. Must be called within
// txlock.Do() since it modifies the data directory. The write is atomic
// (temp file + rename) so a crash mid-write cannot truncate the file, and
// readers outside the lock never observe a partial write.
func Save(dataDir string, ts []Template) error {
	path := filepath.Join(dataDir, templatesFile)
	data, err := json.MarshalIndent(ts, "", "  ")
	if err != nil {
		return fmt.Errorf("templates: marshal: %w", err)
	}
	f, err := os.CreateTemp(dataDir, ".templates-*.json.tmp")
	if err != nil {
		return fmt.Errorf("templates: create temp: %w", err)
	}
	tmpPath := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("templates: write %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("templates: close temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("templates: rename %s -> %s: %w", tmpPath, path, err)
	}
	return nil
}
