package journal

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/slogctx"
)

// headerStatusRe matches a transaction header line, capturing:
//  1. The date prefix (e.g. "2026-01-05 ")
//  2. An optional status marker ("! " or "* ")
//  3. The rest of the line (description + inline comment)
var headerStatusRe = regexp.MustCompile(`^(\d{4}[/\-]\d{2}[/\-]\d{2} )(?:([!*]) )?(.*)$`)

// UpdateTransactionStatus changes the hledger status marker on the transaction
// identified by fid. newStatus must be "", "Pending", or "Cleared".
// Callers must wrap in txlock.Do().
func UpdateTransactionStatus(ctx context.Context, client *hledger.Client, dataDir, fid, newStatus string) error {
	switch newStatus {
	case "", "Pending", "Cleared":
		// valid
	default:
		return fmt.Errorf("journal: update-status: invalid status %q (must be \"\", \"Pending\", or \"Cleared\")", newStatus)
	}

	txns, err := client.Transactions(ctx, "tag:fid="+fid)
	if err != nil {
		return fmt.Errorf("journal: update-status: lookup fid %q: %w", fid, err)
	}
	switch len(txns) {
	case 0:
		return fmt.Errorf("journal: update-status: no transaction found with fid %q", fid)
	case 1:
		// expected
	default:
		return fmt.Errorf("journal: update-status: fid %q matched %d transactions (corrupt journal — run audit)", fid, len(txns))
	}

	txn := txns[0]
	sourceFile := txn.SourcePos[0].File
	headerLine := txn.SourcePos[0].Line // 1-indexed

	data, err := os.ReadFile(sourceFile)
	if err != nil {
		return fmt.Errorf("journal: update-status: read %s: %w", sourceFile, err)
	}

	lines := strings.Split(string(data), "\n")
	headerIdx := headerLine - 1 // 0-indexed

	if headerIdx < 0 || headerIdx >= len(lines) {
		return fmt.Errorf("journal: update-status: source line %d out of range in %s", headerLine, sourceFile)
	}

	// Sanity check: must be a transaction header containing the fid.
	if !txnHeaderRe.MatchString(lines[headerIdx]) || !strings.Contains(lines[headerIdx], "fid:"+fid) {
		return fmt.Errorf("journal: update-status: line %d in %s does not match expected transaction header for fid %q", headerLine, sourceFile, fid)
	}

	// Parse and rewrite the header line with the new status marker.
	m := headerStatusRe.FindStringSubmatch(lines[headerIdx])
	if m == nil {
		return fmt.Errorf("journal: update-status: cannot parse header line %d in %s", headerLine, sourceFile)
	}
	datePart := m[1]  // e.g. "2026-01-05 "
	rest := m[3]      // description + inline comment

	marker := ""
	switch newStatus {
	case "Pending":
		marker = "! "
	case "Cleared":
		marker = "* "
	}
	lines[headerIdx] = datePart + marker + rest

	newContent := strings.Join(lines, "\n")
	if err := os.WriteFile(sourceFile, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("journal: update-status: write %s: %w", sourceFile, err)
	}

	slogctx.FromContext(ctx).Info("journal: transaction status updated", "fid", fid, "status", newStatus, "file", sourceFile)
	return nil
}
