package journal

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/brendanv/float/internal/hledger"
)

const accountsRelPath = "accounts.journal"

// AccountDeclaration is a parsed `account` directive from accounts.journal.
type AccountDeclaration struct {
	Name   string // full account name, e.g. "assets:checking"
	Closed bool   // directive itself carries the float-closed tag (not inherited from a parent)
}

var accountDeclRe = regexp.MustCompile(`^account\s+(\S+)`)

// closedTagRe matches the float-closed tag in a same-line directive comment.
// Tags are comma-separated, so the tag name starts the comment or follows
// whitespace or a comma.
var closedTagRe = regexp.MustCompile(`(?:^|[\s,])` + regexp.QuoteMeta(hledger.ClosedAccountTag) + `:`)

// splitDirectiveComment splits a directive line into the part before its
// same-line comment (right-trimmed) and the comment text after ";" ("" if none).
func splitDirectiveComment(line string) (head, comment string) {
	i := strings.IndexByte(line, ';')
	if i < 0 {
		return line, ""
	}
	return strings.TrimRight(line[:i], " \t"), line[i+1:]
}

// setClosedTag returns the directive line with the float-closed tag added to
// or removed from its same-line comment, preserving any other comment text and
// tags (e.g. "type: A").
func setClosedTag(line string, closed bool) string {
	head, comment := splitDirectiveComment(line)
	var kept []string
	for _, seg := range strings.Split(comment, ",") {
		if !closedTagRe.MatchString(seg) {
			kept = append(kept, seg)
		}
	}
	rest := strings.TrimRight(strings.Join(kept, ","), " \t")
	if closed {
		tag := hledger.ClosedAccountTag + ":"
		if strings.TrimSpace(rest) == "" {
			return head + "  ; " + tag
		}
		return head + "  ;" + rest + ", " + tag
	}
	if strings.TrimSpace(rest) == "" {
		return head
	}
	return head + "  ;" + rest
}

// ListAccountDeclarations reads accounts.journal and returns all parsed account directives.
// Returns an empty slice (not an error) if accounts.journal does not yet exist.
// Does NOT acquire txlock — safe to call concurrently with reads.
func ListAccountDeclarations(dataDir string) ([]AccountDeclaration, error) {
	path := filepath.Join(dataDir, accountsRelPath)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("journal: read %s: %w", accountsRelPath, err)
	}

	var decls []AccountDeclaration
	for _, line := range strings.Split(string(data), "\n") {
		m := accountDeclRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		_, comment := splitDirectiveComment(line)
		decls = append(decls, AccountDeclaration{Name: m[1], Closed: closedTagRe.MatchString(comment)})
	}
	return decls, nil
}

// AppendAccountDeclaration writes a new account directive to accounts.journal.
// Ensures accounts.journal exists and is included in main.journal (prepended before any other
// includes so account declarations are in scope for all transactions and prices).
// Does NOT acquire txlock — callers must wrap in txlock.Do().
func AppendAccountDeclaration(dataDir, name string) error {
	return appendAccountDirective(dataDir, "account "+name)
}

// appendAccountDirective appends one directive line to accounts.journal,
// creating the file and its main.journal include if needed. A missing final
// newline in the existing file is repaired so the new line stays separate.
func appendAccountDirective(dataDir, line string) error {
	if _, err := EnsureAccountsFile(dataDir); err != nil {
		return err
	}
	if _, err := EnsureAccountsInclude(dataDir); err != nil {
		return err
	}

	path := filepath.Join(dataDir, accountsRelPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("journal: read %s: %w", accountsRelPath, err)
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		line = "\n" + line
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("journal: open %s: %w", accountsRelPath, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(line + "\n"); err != nil {
		return fmt.Errorf("journal: write %s: %w", accountsRelPath, err)
	}
	return nil
}

// RenameAccountDeclaration renames account directives in accounts.journal. It renames any
// declaration that is exactly oldName or starts with oldName+":" (sub-accounts).
// Returns an error if accounts.journal does not exist or no matching declaration is found.
// Does NOT acquire txlock — callers must wrap in txlock.Do().
func RenameAccountDeclaration(dataDir, oldName, newName string) error {
	path := filepath.Join(dataDir, accountsRelPath)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("journal: accounts.journal not found: %w", ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("journal: read %s: %w", accountsRelPath, err)
	}

	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		m := accountDeclRe.FindStringSubmatchIndex(line)
		if m == nil {
			continue
		}
		name := line[m[2]:m[3]]
		// Replace only the name so trailing comments/tags (type:, float-closed:) survive.
		if name == oldName {
			lines[i] = line[:m[2]] + newName + line[m[3]:]
			found = true
		} else if strings.HasPrefix(name, oldName+":") {
			lines[i] = line[:m[2]] + newName + name[len(oldName):] + line[m[3]:]
			found = true
		}
	}
	if !found {
		return fmt.Errorf("journal: account declaration %q not found: %w", oldName, ErrNotFound)
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// SetAccountClosed adds or removes the float-closed tag on name's directive in
// accounts.journal. Closing an undeclared account appends a new declaration
// carrying the tag; reopening an undeclared or already-open account is a no-op.
// Does NOT acquire txlock — callers must wrap in txlock.Do().
func SetAccountClosed(dataDir, name string, closed bool) error {
	path := filepath.Join(dataDir, accountsRelPath)
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("journal: read %s: %w", accountsRelPath, err)
	}

	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		if m := accountDeclRe.FindStringSubmatch(line); m != nil && m[1] == name {
			lines[i] = setClosedTag(line, closed)
			found = true
		}
	}
	if !found {
		if !closed {
			return nil
		}
		return appendAccountDirective(dataDir, setClosedTag("account "+name, true))
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// DeleteAccountDeclaration removes the account directive with the given name from accounts.journal.
// Returns an error if accounts.journal does not exist or the name is not found.
// Does NOT acquire txlock — callers must wrap in txlock.Do().
func DeleteAccountDeclaration(dataDir, name string) error {
	path := filepath.Join(dataDir, accountsRelPath)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("journal: accounts.journal not found: %w", ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("journal: read %s: %w", accountsRelPath, err)
	}

	lines := strings.Split(string(data), "\n")
	found := false
	newLines := lines[:0:0]
	for _, line := range lines {
		if m := accountDeclRe.FindStringSubmatch(line); m != nil && m[1] == name {
			found = true
			continue
		}
		newLines = append(newLines, line)
	}
	if !found {
		return fmt.Errorf("journal: account declaration %q not found: %w", name, ErrNotFound)
	}

	return os.WriteFile(path, []byte(strings.Join(newLines, "\n")), 0644)
}

// EnsureAccountsFile creates accounts.journal with a header comment if it doesn't exist.
// Returns whether the file was created.
// Exported so startup can call it before any AppendAccountDeclaration calls.
func EnsureAccountsFile(dataDir string) (bool, error) {
	path := filepath.Join(dataDir, accountsRelPath)
	if _, err := os.Stat(path); err == nil {
		return false, nil
	}
	header := "; float: account declarations\n"
	if err := os.WriteFile(path, []byte(header), 0644); err != nil {
		return false, fmt.Errorf("journal: create %s: %w", accountsRelPath, err)
	}
	return true, nil
}

// EnsureAccountsInclude ensures "include accounts.journal" appears in main.journal
// before any other include directives, so account declarations are in scope for
// all prices and transactions. Returns whether main.journal was modified.
// Exported so startup can call it directly.
func EnsureAccountsInclude(dataDir string) (bool, error) {
	mainPath := filepath.Join(dataDir, "main.journal")
	directive := "include " + accountsRelPath

	existing, err := os.ReadFile(mainPath)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("journal: read main.journal: %w", err)
	}

	lines := strings.Split(string(existing), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == directive {
			return false, nil // already present
		}
	}

	// Prepend before the first include line (or at end if none).
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
	if err := os.WriteFile(mainPath, []byte(content), 0644); err != nil {
		return false, err
	}
	return true, nil
}
