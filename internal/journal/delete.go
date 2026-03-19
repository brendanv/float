package journal

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/brendanv/float/internal/hledger"
)

// DeleteTransaction removes the transaction tagged with fid from its journal file.
// It uses hledger to look up the transaction's exact source location, then removes
// the transaction block at that line.
// Returns an error if the fid is not found or if file I/O fails.
// Callers must wrap this in txlock.Do().
func DeleteTransaction(ctx context.Context, client *hledger.Client, dataDir, fid string) error {
	txns, err := client.Transactions(ctx, "tag:fid="+fid)
	if err != nil {
		return fmt.Errorf("journal: delete: lookup fid %q: %w", fid, err)
	}
	if len(txns) == 0 {
		return fmt.Errorf("journal: delete: no transaction found with fid %q", fid)
	}

	txn := txns[0]
	sourceFile := txn.SourcePos[0].File
	sourceLine := txn.SourcePos[0].Line // 1-indexed header line

	return removeTransactionAtLine(sourceFile, sourceLine, fid)
}

// removeTransactionAtLine removes the transaction block starting at headerLine
// (1-indexed) from path. The fid is used only as a sanity check on the header.
func removeTransactionAtLine(path string, headerLine int, fid string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("journal: delete: read %s: %w", path, err)
	}

	lines := strings.Split(string(data), "\n")
	headerIdx := headerLine - 1 // convert to 0-indexed

	if headerIdx < 0 || headerIdx >= len(lines) {
		return fmt.Errorf("journal: delete: source line %d out of range in %s", headerLine, path)
	}

	// Sanity check: the line should be a transaction header containing the fid.
	if !txnHeaderRe.MatchString(lines[headerIdx]) || !strings.Contains(lines[headerIdx], "fid:"+fid) {
		return fmt.Errorf("journal: delete: line %d in %s does not match expected transaction header for fid %q", headerLine, path, fid)
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
		return fmt.Errorf("journal: delete: write %s: %w", path, err)
	}
	return nil
}
