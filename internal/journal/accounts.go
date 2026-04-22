package journal

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const accountsRelPath = "accounts.journal"

// AccountDeclaration is a parsed `account` directive from accounts.journal.
type AccountDeclaration struct {
	AID  string // 8-char hex from inline comment; empty if absent
	Name string // full account name, e.g. "assets:checking"
}

var (
	accountDeclRe = regexp.MustCompile(`^account\s+(\S+)`)
	aidTagRe      = regexp.MustCompile(`aid:([0-9a-f]{8})`)
)

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
		d := AccountDeclaration{Name: m[1]}
		if am := aidTagRe.FindStringSubmatch(line); am != nil {
			d.AID = am[1]
		}
		decls = append(decls, d)
	}
	return decls, nil
}

// AppendAccountDeclaration writes a new account directive to accounts.journal, minting an AID.
// Ensures accounts.journal exists and is included in main.journal (prepended before any other
// includes so account declarations are in scope for all transactions and prices).
// Returns the minted AID.
// Does NOT acquire txlock — callers must wrap in txlock.Do().
func AppendAccountDeclaration(dataDir, name string) (string, error) {
	aid := MintFID()
	line := fmt.Sprintf("account %s  ; aid:%s\n", name, aid)

	if err := EnsureAccountsFile(dataDir); err != nil {
		return "", err
	}
	if err := EnsureAccountsInclude(dataDir); err != nil {
		return "", err
	}

	path := filepath.Join(dataDir, accountsRelPath)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return "", fmt.Errorf("journal: open %s: %w", accountsRelPath, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(line); err != nil {
		return "", fmt.Errorf("journal: write %s: %w", accountsRelPath, err)
	}
	return aid, nil
}

// DeleteAccountDeclaration removes the account directive with the given AID from accounts.journal.
// Returns an error if accounts.journal does not exist or the AID is not found.
// Does NOT acquire txlock — callers must wrap in txlock.Do().
func DeleteAccountDeclaration(dataDir, aid string) error {
	path := filepath.Join(dataDir, accountsRelPath)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("journal: accounts.journal not found")
	}
	if err != nil {
		return fmt.Errorf("journal: read %s: %w", accountsRelPath, err)
	}

	needle := "aid:" + aid
	lines := strings.Split(string(data), "\n")
	found := false
	newLines := lines[:0:0]
	for _, line := range lines {
		if strings.Contains(line, needle) {
			found = true
			continue
		}
		newLines = append(newLines, line)
	}
	if !found {
		return fmt.Errorf("journal: account declaration with aid %q not found", aid)
	}

	return os.WriteFile(path, []byte(strings.Join(newLines, "\n")), 0644)
}

// EnsureAccountsFile creates accounts.journal with a header comment if it doesn't exist.
// Exported so startup can call it before any AppendAccountDeclaration calls.
func EnsureAccountsFile(dataDir string) error {
	path := filepath.Join(dataDir, accountsRelPath)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	header := "; float: account declarations\n"
	if err := os.WriteFile(path, []byte(header), 0644); err != nil {
		return fmt.Errorf("journal: create %s: %w", accountsRelPath, err)
	}
	return nil
}

// EnsureAccountsInclude ensures "include accounts.journal" appears in main.journal
// before any other include directives, so account declarations are in scope for
// all prices and transactions.
// Exported so startup can call it directly.
func EnsureAccountsInclude(dataDir string) error {
	mainPath := filepath.Join(dataDir, "main.journal")
	directive := "include " + accountsRelPath

	existing, err := os.ReadFile(mainPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("journal: read main.journal: %w", err)
	}

	lines := strings.Split(string(existing), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == directive {
			return nil // already present
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

	return os.WriteFile(mainPath, []byte(strings.Join(newLines, "\n")), 0644)
}
