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

// BulkEditTransactions applies operations to all fids in a single pass.
// Operations applied per transaction (in order): reviewed status, tag add/remove, payee.
// Callers must wrap in txlock.Do().
func BulkEditTransactions(
	ctx context.Context,
	client *hledger.Client,
	dataDir string,
	fids []string,
	reviewed *bool,      // nil=no change; true=set Cleared; false=set Unmarked
	addTagKey string,    // ""=skip
	addTagValue string,
	removeTagKey string, // ""=skip
	newPayee *string,    // nil=no change; &""=clear payee; &"foo"=set payee
) error {
	for _, fid := range fids {
		txns, err := client.Transactions(ctx, "code:"+fid)
		if err != nil {
			return fmt.Errorf("journal: bulk-edit: lookup fid %q: %w", fid, err)
		}
		switch len(txns) {
		case 0:
			return fmt.Errorf("journal: bulk-edit: no transaction found with fid %q", fid)
		case 1:
			// expected
		default:
			return fmt.Errorf("journal: bulk-edit: fid %q matched %d transactions (corrupt journal — run audit)", fid, len(txns))
		}

		txn := txns[0]

		if reviewed != nil {
			status := ""
			if *reviewed {
				status = "Cleared"
			}
			if err := UpdateTransactionStatus(ctx, client, dataDir, fid, status); err != nil {
				return fmt.Errorf("journal: bulk-edit: fid %q: reviewed: %w", fid, err)
			}
			// Re-fetch so subsequent operations see updated file state.
			txns, err = client.Transactions(ctx, "code:"+fid)
			if err != nil {
				return fmt.Errorf("journal: bulk-edit: re-fetch fid %q after status update: %w", fid, err)
			}
			txn = txns[0]
		}

		if addTagKey != "" || removeTagKey != "" {
			tags := make(map[string]string)
			for _, kv := range txn.Tags {
				if !strings.HasPrefix(kv[0], hledger.HiddenMetaPrefix) {
					tags[kv[0]] = kv[1]
				}
			}
			if addTagKey != "" {
				tags[addTagKey] = addTagValue
			}
			if removeTagKey != "" {
				delete(tags, removeTagKey)
			}
			if err := ModifyTags(ctx, client, dataDir, fid, tags); err != nil {
				return fmt.Errorf("journal: bulk-edit: fid %q: tags: %w", fid, err)
			}
			// Re-fetch so payee operation sees updated file state.
			if newPayee != nil {
				txns, err = client.Transactions(ctx, "code:"+fid)
				if err != nil {
					return fmt.Errorf("journal: bulk-edit: re-fetch fid %q after tag update: %w", fid, err)
				}
				txn = txns[0]
			}
		}

		if newPayee != nil {
			if err := setPayeeOnTransaction(ctx, client, dataDir, fid, txn, *newPayee); err != nil {
				return fmt.Errorf("journal: bulk-edit: fid %q: payee: %w", fid, err)
			}
		}

		slogctx.FromContext(ctx).Info("journal: bulk-edit: transaction updated", "fid", fid)
	}
	return nil
}

// setPayeeOnTransaction rewrites the payee portion of the transaction header line.
// newPayee == "" clears the payee (description becomes just the note/description part).
// Callers must wrap in txlock.Do().
func setPayeeOnTransaction(ctx context.Context, client *hledger.Client, dataDir, fid string, txn hledger.Transaction, newPayee string) error {
	sourceFile := txn.SourcePos[0].File
	headerLine := txn.SourcePos[0].Line // 1-indexed

	data, err := os.ReadFile(sourceFile)
	if err != nil {
		return fmt.Errorf("journal: set-payee: read %s: %w", sourceFile, err)
	}

	lines := strings.Split(string(data), "\n")
	headerIdx := headerLine - 1 // 0-indexed

	if headerIdx < 0 || headerIdx >= len(lines) {
		return fmt.Errorf("journal: set-payee: source line %d out of range in %s", headerLine, sourceFile)
	}

	if !txnHeaderRe.MatchString(lines[headerIdx]) || !strings.Contains(lines[headerIdx], "("+fid+")") {
		return fmt.Errorf("journal: set-payee: line %d in %s does not match expected transaction header for fid %q", headerLine, sourceFile, fid)
	}

	m := headerStatusRe.FindStringSubmatch(lines[headerIdx])
	if m == nil {
		return fmt.Errorf("journal: set-payee: cannot parse header line %d in %s", headerLine, sourceFile)
	}
	datePart := m[1] // e.g. "2026-01-05 "
	marker := ""
	if m[2] != "" {
		marker = m[2] + " " // e.g. "! " or "* "
	}
	codePart := m[3] // e.g. "(a1b2c3d4) " or ""
	rest := m[4]     // description + optional inline comment

	// Separate the description from any inline comment (hledger: `;` starts a comment).
	descPart := rest
	inlinePart := ""
	if idx := strings.Index(rest, " ;"); idx >= 0 {
		descPart = rest[:idx]
		inlinePart = rest[idx:]
	}
	_ = descPart // we replace it entirely

	// Compute note: the part after "|" in the current description, or full description if no "|".
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
		return fmt.Errorf("journal: set-payee: write %s: %w", sourceFile, err)
	}

	slogctx.FromContext(ctx).Info("journal: transaction payee updated", "fid", fid, "payee", newPayee, "file", sourceFile)

	// Stamp float-updated-at, preserving existing FloatMeta.
	meta := make(map[string]string, len(txn.FloatMeta)+1)
	for k, v := range txn.FloatMeta {
		meta[k] = v
	}
	meta[hledger.HiddenMetaPrefix+"updated-at"] = time.Now().UTC().Format(time.RFC3339)
	if err := ModifyFloatMeta(ctx, client, dataDir, fid, meta); err != nil {
		return fmt.Errorf("journal: set-payee: stamp timestamp: %w", err)
	}
	return nil
}
