package journal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RenameAccountInJournalFiles renames account names in posting lines across all
// YYYY/MM.journal transaction files. It renames postings that reference exactly
// oldName or any sub-account starting with oldName+":".
// Returns the total number of posting lines updated.
// Does NOT modify accounts.journal — use RenameAccountDeclaration for that.
// Does NOT acquire txlock — callers must wrap in txlock.Do().
func RenameAccountInJournalFiles(dataDir, oldName, newName string) (int, error) {
	pattern := filepath.Join(dataDir, "*", "*.journal")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return 0, fmt.Errorf("journal: glob transaction files: %w", err)
	}

	total := 0
	for _, path := range files {
		n, err := renameAccountInFile(path, oldName, newName)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func renameAccountInFile(path, oldName, newName string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("journal: read %s: %w", path, err)
	}

	lines := strings.Split(string(data), "\n")
	count := 0
	for i, line := range lines {
		newLine, changed := renameAccountInPosting(line, oldName, newName)
		if changed {
			lines[i] = newLine
			count++
		}
	}

	if count == 0 {
		return 0, nil
	}

	return count, os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// renameAccountInPosting renames the account in a single posting line.
// A posting line is identified by having 2+ leading spaces followed by a
// non-space, non-semicolon character. Returns the (possibly modified) line
// and whether a rename occurred.
func renameAccountInPosting(line, oldName, newName string) (string, bool) {
	// Count leading whitespace — must be at least 2 spaces (or a tab) to be a posting.
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i < 2 || i == len(line) {
		return line, false
	}
	// Skip comment lines that happen to be indented.
	if line[i] == ';' {
		return line, false
	}

	// Extract the account name: everything up to the first whitespace or end of line.
	rest := line[i:]
	j := strings.IndexAny(rest, " \t")
	var account, suffix string
	if j == -1 {
		account = rest
		suffix = ""
	} else {
		account = rest[:j]
		suffix = rest[j:]
	}

	if account != oldName && !strings.HasPrefix(account, oldName+":") {
		return line, false
	}

	newAccount := newName + account[len(oldName):]
	return line[:i] + newAccount + suffix, true
}
