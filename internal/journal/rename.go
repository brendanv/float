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

// RenameAccountName applies a rename of oldName to newName to a single account
// name: name is renamed when it equals oldName exactly or is a sub-account of
// it (starts with oldName+":"). "assets:bankx" and "assets:bank checking" are
// different accounts from "assets:bank" and are left alone. Returns the
// (possibly renamed) name and whether it changed.
func RenameAccountName(name, oldName, newName string) (string, bool) {
	if name != oldName && !strings.HasPrefix(name, oldName+":") {
		return name, false
	}
	return newName + name[len(oldName):], true
}

// isBlank reports whether c is a space or tab (hledger's "spacenonewline").
func isBlank(c byte) bool { return c == ' ' || c == '\t' }

// accountFieldEnd returns the index in s, at or after start, where an hledger
// account name ends. Account names may contain single spaces; like hledger,
// the name ends at the first run of two or more blanks (spaces and tabs in any
// mix) or at end of line, and a single trailing blank is not part of the name.
// A lone tab between two words counts as a single space, exactly as hledger
// parses it, so it does NOT end the name.
func accountFieldEnd(s string, start int) int {
	for j := start; j < len(s); j++ {
		if isBlank(s[j]) && (j+1 == len(s) || isBlank(s[j+1])) {
			return j
		}
	}
	return len(s)
}

// renameAccountInPosting renames the account in a single posting line,
// preserving every other byte (indentation, status mark, virtual-posting
// brackets, spacing, amount, and comments). Returns the (possibly modified)
// line and whether a rename occurred.
//
// A posting line has the shape
//
//	INDENT [STATUS [BLANKS]] ACCOUNT-FIELD [2+ BLANKS AMOUNT/ASSERTION/COMMENT...]
//
// where INDENT is one or more spaces/tabs, STATUS is "*" or "!", and
// ACCOUNT-FIELD is the account name, optionally wrapped in "(...)" or "[...]"
// for virtual postings. Indented lines starting with ";" or "#" are comments
// and are never changed.
func renameAccountInPosting(line, oldName, newName string) (string, bool) {
	// INDENT: postings are indented by at least one space or tab.
	i := 0
	for i < len(line) && isBlank(line[i]) {
		i++
	}
	if i == 0 || i == len(line) || line[i] == ';' || line[i] == '#' {
		return line, false
	}

	// STATUS: optional "*" or "!", optionally followed by blanks.
	if line[i] == '*' || line[i] == '!' {
		i++
		for i < len(line) && isBlank(line[i]) {
			i++
		}
	}

	// ACCOUNT-FIELD, minus the brackets of a virtual posting.
	start, end := i, accountFieldEnd(line, i)
	if end-start >= 2 {
		first, last := line[start], line[end-1]
		if (first == '(' && last == ')') || (first == '[' && last == ']') {
			start++
			end--
		}
	}

	renamed, ok := RenameAccountName(line[start:end], oldName, newName)
	if !ok {
		return line, false
	}
	return line[:start] + renamed + line[end:], true
}
