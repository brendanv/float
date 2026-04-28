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

	lines := strings.Split(string(existing), "\n")
	for _, line := range lines {
		if hasCommodityDirective(line, code) {
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

// hasCommodityDirective checks whether line is a commodity directive for code.
// It strips trailing comments and matches the last token exactly, so
// "commodity 1,000.00 USD ; added by float" matches "USD" but
// "commodity 1,000.00 TUSD" does not.
func hasCommodityDirective(line, code string) bool {
	line = strings.TrimSpace(line)
	// Strip inline comment.
	if idx := strings.Index(line, ";"); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	fields := strings.Fields(line)
	return len(fields) >= 2 && fields[0] == "commodity" && fields[len(fields)-1] == code
}
