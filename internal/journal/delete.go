package journal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DeleteTransaction removes the transaction tagged with fid from the journal files.
// It scans all files included from main.journal, finds the transaction block
// containing fid:<fid>, and removes it.
// Returns an error if the fid is not found or if file I/O fails.
// Callers must wrap this in txlock.Do().
func DeleteTransaction(dataDir, fid string) error {
	mainPath := filepath.Join(dataDir, "main.journal")
	mainData, err := os.ReadFile(mainPath)
	if err != nil {
		return fmt.Errorf("journal: delete: read main.journal: %w", err)
	}

	var includes []string
	for _, line := range strings.Split(string(mainData), "\n") {
		if m := includeRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			includes = append(includes, strings.TrimSpace(m[1]))
		}
	}

	for _, rel := range includes {
		abs := filepath.Join(dataDir, rel)
		removed, err := removeTransactionFromFile(abs, fid)
		if err != nil {
			return err
		}
		if removed {
			return nil
		}
	}
	return fmt.Errorf("journal: delete: no transaction found with fid %q", fid)
}

// removeTransactionFromFile removes the transaction containing fid from path.
// Returns true if found and removed, false if not found.
func removeTransactionFromFile(path, fid string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("journal: delete: read %s: %w", path, err)
	}

	lines := strings.Split(string(data), "\n")
	fidPat := "fid:" + fid

	// Find the transaction header line containing this fid.
	// fid is always on the transaction header line (see format.go).
	headerIdx := -1
	for i, line := range lines {
		if txnHeaderRe.MatchString(line) && strings.Contains(line, fidPat) {
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		return false, nil
	}

	// Walk forward to find the end of the transaction block (non-blank lines).
	endIdx := headerIdx + 1
	for endIdx < len(lines) && strings.TrimSpace(lines[endIdx]) != "" {
		endIdx++
	}
	// Include one trailing blank line if present.
	if endIdx < len(lines) && strings.TrimSpace(lines[endIdx]) == "" {
		endIdx++
	}

	// Reconstruct file without the removed block.
	newLines := append(lines[:headerIdx:headerIdx], lines[endIdx:]...)
	newContent := strings.Join(newLines, "\n")

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return false, fmt.Errorf("journal: delete: write %s: %w", path, err)
	}
	return true, nil
}
