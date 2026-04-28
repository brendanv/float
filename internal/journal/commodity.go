package journal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnsureCommodityDirective ensures a `commodity 1,000.00 <code>` directive
// exists in main.journal. If absent it is inserted before any include lines.
// This tells hledger the canonical display format for the given currency so it
// normalises output consistently.
// Does NOT acquire txlock — callers must wrap in txlock.Do().
func EnsureCommodityDirective(dataDir, code string) error {
	mainPath := filepath.Join(dataDir, "main.journal")
	existing, err := os.ReadFile(mainPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("journal: read main.journal: %w", err)
	}

	needle := "commodity " // any commodity directive line
	lines := strings.Split(string(existing), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, needle) && strings.HasSuffix(trimmed, code) {
			return nil // already present
		}
	}

	directive := fmt.Sprintf("commodity 1,000.00 %s", code)

	// Insert before the first include line.
	insertAt := 0
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "include ") {
			insertAt = i
			break
		}
		insertAt = i + 1
	}

	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:insertAt]...)
	newLines = append(newLines, directive)
	newLines = append(newLines, lines[insertAt:]...)

	content := strings.Join(newLines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(mainPath, []byte(content), 0644)
}
