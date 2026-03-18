package journal

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	txnHeaderRe = regexp.MustCompile(`^\d{4}[/\-]\d{2}[/\-]\d{2} `)
	fidTagRe    = regexp.MustCompile(`fid:[0-9a-f]{8}`)
	includeRe   = regexp.MustCompile(`^include\s+(.+)`)
)

// MigrateFIDs scans all journal files included from main.journal in dataDir,
// finds transaction header lines without fid tags, and adds fid tags to them.
// Returns the count of transactions modified.
// Safe to call on an already-migrated journal (count=0, no changes).
func MigrateFIDs(dataDir string) (int, error) {
	mainPath := filepath.Join(dataDir, "main.journal")
	mainData, err := os.ReadFile(mainPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	var includes []string
	for _, line := range strings.Split(string(mainData), "\n") {
		if m := includeRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			includes = append(includes, strings.TrimSpace(m[1]))
		}
	}

	total := 0
	for _, rel := range includes {
		abs := filepath.Join(dataDir, rel)
		n, err := migrateFile(abs)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func migrateFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	lines := strings.Split(string(data), "\n")
	count := 0
	for i, line := range lines {
		if txnHeaderRe.MatchString(line) && !fidTagRe.MatchString(line) {
			lines[i] = line + "  ; fid:" + MintFID()
			count++
		}
	}

	if count > 0 {
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644); err != nil {
			return 0, err
		}
	}
	return count, nil
}
