package journal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/slogctx"
)

// EnsureMonthFile ensures dataDir/YYYY/MM.journal exists.
// Creates the year directory and file if needed.
// Returns the relative path (e.g. "2026/01.journal") and whether newly created.
func EnsureMonthFile(dataDir string, year, month int) (relPath string, created bool, err error) {
	relPath = fmt.Sprintf("%04d/%02d.journal", year, month)
	absPath := filepath.Join(dataDir, relPath)

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return "", false, fmt.Errorf("journal: mkdir %s: %w", filepath.Dir(absPath), err)
	}

	if _, err := os.Stat(absPath); err == nil {
		return relPath, false, nil
	}

	header := fmt.Sprintf("; float: %04d/%02d\n", year, month)
	if err := os.WriteFile(absPath, []byte(header), 0644); err != nil {
		return "", false, fmt.Errorf("journal: create %s: %w", absPath, err)
	}
	return relPath, true, nil
}

// UpdateMainIncludes adds an include directive to mainJournalPath if not already present.
// Idempotent: if the exact directive already exists, does nothing.
func UpdateMainIncludes(mainJournalPath string, relPath string) error {
	directive := "include " + relPath

	existing, err := os.ReadFile(mainJournalPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("journal: read %s: %w", mainJournalPath, err)
	}

	// Check if directive already present as a whole line
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == directive {
			return nil
		}
	}

	f, err := os.OpenFile(mainJournalPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("journal: open %s: %w", mainJournalPath, err)
	}
	defer func() { _ = f.Close() }()
	_, err = fmt.Fprintln(f, directive)
	return err
}

// AppendTransaction writes a new transaction to the correct month file.
// It mints a FID, uses hledger print to canonically format the transaction,
// ensures the month file exists, updates main.journal if a new file was created,
// and appends the canonical text.
// Returns the assigned FID.
func AppendTransaction(ctx context.Context, client *hledger.Client, dataDir string, tx TransactionInput) (string, error) {
	fid := MintFID()

	text, err := FormatViaHledger(ctx, client, tx, fid)
	if err != nil {
		return "", err
	}

	year, month := tx.Date.Year(), int(tx.Date.Month())
	relPath, created, err := EnsureMonthFile(dataDir, year, month)
	if err != nil {
		return "", err
	}

	if created {
		mainPath := filepath.Join(dataDir, "main.journal")
		if err := UpdateMainIncludes(mainPath, relPath); err != nil {
			return "", err
		}
	}

	absPath := filepath.Join(dataDir, relPath)
	f, err := os.OpenFile(absPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return "", fmt.Errorf("journal: open %s: %w", absPath, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.WriteString(text); err != nil {
		return "", fmt.Errorf("journal: write %s: %w", absPath, err)
	}
	slogctx.FromContext(ctx).InfoContext(ctx, "journal: transaction appended", "fid", fid, "path", relPath)
	return fid, nil
}
