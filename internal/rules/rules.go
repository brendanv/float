package rules

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Rule is a categorization rule that matches transactions by description and
// applies payee, account, and/or tag changes.
type Rule struct {
	ID           string            `json:"id"`             // 8-char hex (MintFID)
	Pattern      string            `json:"pattern"`        // regex matched against description (case-insensitive)
	Payee        string            `json:"payee"`          // set payee (empty = no change)
	Account      string            `json:"account"`        // set category account (empty = no change)
	Tags         map[string]string `json:"tags"`           // tags to add (empty = no change)
	Priority     int               `json:"priority"`       // lower = higher priority, matched first
	AutoReviewed bool              `json:"auto_reviewed"`  // if true, mark transaction Cleared on import
	MatchAccount string            `json:"match_account"`  // if non-empty, only apply to txns whose source account has this prefix (empty = all accounts)
}

const rulesFile = "rules.json"

// Load reads data/rules.json from dataDir. Returns empty slice if the file
// doesn't exist (not an error). Rules are returned sorted by priority (ascending).
func Load(dataDir string) ([]Rule, error) {
	path := filepath.Join(dataDir, rulesFile)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("rules: read %s: %w", path, err)
	}
	var rules []Rule
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("rules: parse %s: %w", path, err)
	}
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Priority < rules[j].Priority
	})
	return rules, nil
}

// Save writes rules to data/rules.json in dataDir. Must be called within
// txlock.Do() since it modifies the data directory.
func Save(dataDir string, rules []Rule) error {
	path := filepath.Join(dataDir, rulesFile)
	data, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return fmt.Errorf("rules: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("rules: write %s: %w", path, err)
	}
	return nil
}

// Match iterates rules in priority order and returns the first rule whose
// pattern matches description (case-insensitive) and whose MatchAccount (if
// set) is a prefix of account. Returns nil if no rule matches.
// account is the source account of the transaction (the asset/liability posting).
func Match(rules []Rule, description, account string) *Rule {
	for i := range rules {
		r := &rules[i]
		if r.Pattern == "" {
			continue
		}
		if r.MatchAccount != "" && !strings.HasPrefix(account, r.MatchAccount) {
			continue
		}
		re, err := regexp.Compile("(?i)" + r.Pattern)
		if err != nil {
			continue
		}
		if re.MatchString(description) {
			return r
		}
	}
	return nil
}
