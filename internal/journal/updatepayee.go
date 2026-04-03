package journal

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/slogctx"
)

// UpdateTransactionPayee sets or clears the payee on the transaction identified by fid.
// newPayee == "" clears the payee, making the description just the note portion.
// newPayee != "" sets the payee, making the description "newPayee | note".
// Callers must wrap in txlock.Do().
func UpdateTransactionPayee(ctx context.Context, client *hledger.Client, dataDir, fid, newPayee string) (hledger.Transaction, error) {
	txns, err := client.Transactions(ctx, "code:"+fid)
	if err != nil {
		return hledger.Transaction{}, fmt.Errorf("journal: update-payee: lookup fid %q: %w", fid, err)
	}
	switch len(txns) {
	case 0:
		return hledger.Transaction{}, fmt.Errorf("journal: update-payee: no transaction found with fid %q", fid)
	case 1:
		// expected
	default:
		return hledger.Transaction{}, fmt.Errorf("journal: update-payee: fid %q matched %d transactions (corrupt journal — run audit)", fid, len(txns))
	}

	txn := txns[0]
	sourceFile := txn.SourcePos[0].File
	headerLine := txn.SourcePos[0].Line // 1-indexed

	data, err := os.ReadFile(sourceFile)
	if err != nil {
		return hledger.Transaction{}, fmt.Errorf("journal: update-payee: read %s: %w", sourceFile, err)
	}

	lines := strings.Split(string(data), "\n")
	headerIdx := headerLine - 1 // 0-indexed

	if headerIdx < 0 || headerIdx >= len(lines) {
		return hledger.Transaction{}, fmt.Errorf("journal: update-payee: source line %d out of range in %s", headerLine, sourceFile)
	}

	if !txnHeaderRe.MatchString(lines[headerIdx]) || !strings.Contains(lines[headerIdx], "("+fid+")") {
		return hledger.Transaction{}, fmt.Errorf("journal: update-payee: line %d in %s does not match expected transaction header for fid %q", headerLine, sourceFile, fid)
	}

	m := headerStatusRe.FindStringSubmatch(lines[headerIdx])
	if m == nil {
		return hledger.Transaction{}, fmt.Errorf("journal: update-payee: cannot parse header line %d in %s", headerLine, sourceFile)
	}
	datePart := m[1]
	marker := ""
	if m[2] != "" {
		marker = m[2] + " "
	}
	codePart := m[3]
	rest := m[4] // description + optional inline comment

	// Separate description from inline comment (semicolon starts a comment in hledger).
	descPart := rest
	inlinePart := ""
	if idx := strings.Index(rest, " ;"); idx >= 0 {
		descPart = rest[:idx]
		inlinePart = rest[idx:]
	}
	_ = descPart // replaced below

	// Compute note: right side of "|" in current description, or full description if no "|".
	note := txn.Description
	if i := strings.Index(txn.Description, "|"); i >= 0 {
		note = strings.TrimSpace(txn.Description[i+1:])
	}

	var newDesc string
	if newPayee == "" {
		newDesc = note
	} else {
		newDesc = newPayee + " | " + note
	}

	lines[headerIdx] = datePart + marker + codePart + newDesc + inlinePart

	newContent := strings.Join(lines, "\n")
	if err := os.WriteFile(sourceFile, []byte(newContent), 0644); err != nil {
		return hledger.Transaction{}, fmt.Errorf("journal: update-payee: write %s: %w", sourceFile, err)
	}

	slogctx.FromContext(ctx).Info("journal: transaction payee updated", "fid", fid, "payee", newPayee, "file", sourceFile)

	// Stamp the last-updated timestamp. Merge with existing FloatMeta so other keys are preserved.
	meta := make(map[string]string, len(txn.FloatMeta)+1)
	for k, v := range txn.FloatMeta {
		meta[k] = v
	}
	meta[hledger.HiddenMetaPrefix+"updated-at"] = time.Now().UTC().Format(time.RFC3339)
	if err := ModifyFloatMeta(ctx, client, dataDir, fid, meta); err != nil {
		return hledger.Transaction{}, fmt.Errorf("journal: update-payee: stamp timestamp: %w", err)
	}

	updated, err := client.Transactions(ctx, "code:"+fid)
	if err != nil || len(updated) == 0 {
		return hledger.Transaction{}, fmt.Errorf("journal: update-payee: re-fetch fid %q: %w", fid, err)
	}
	return updated[0], nil
}
