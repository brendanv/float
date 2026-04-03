package journal

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/slogctx"
)

// BulkEditTransactions applies operations to all fids in a single pass per transaction.
// For each FID: one hledger lookup, one file read, all changes applied in memory, one file
// write. Operations: reviewed status, tag add/remove, payee set/clear.
// Callers must wrap in txlock.Do().
func BulkEditTransactions(
	ctx context.Context,
	client *hledger.Client,
	dataDir string,
	fids []string,
	reviewed *bool,      // nil=no change; true=set Cleared (*); false=set Pending (!)
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

		if err := applyBulkEdits(ctx, fid, txns[0], reviewed, addTagKey, addTagValue, removeTagKey, newPayee); err != nil {
			return err
		}
		slogctx.FromContext(ctx).Info("journal: bulk-edit: transaction updated", "fid", fid)
	}
	return nil
}

// applyBulkEdits applies all requested changes to a single transaction in one file read/write.
func applyBulkEdits(
	ctx context.Context,
	fid string,
	txn hledger.Transaction,
	reviewed *bool,
	addTagKey, addTagValue, removeTagKey string,
	newPayee *string,
) error {
	sourceFile := txn.SourcePos[0].File
	headerLine := txn.SourcePos[0].Line // 1-indexed

	data, err := os.ReadFile(sourceFile)
	if err != nil {
		return fmt.Errorf("journal: bulk-edit: read %s: %w", sourceFile, err)
	}

	lines := strings.Split(string(data), "\n")
	headerIdx := headerLine - 1 // 0-indexed

	if headerIdx < 0 || headerIdx >= len(lines) {
		return fmt.Errorf("journal: bulk-edit: source line %d out of range in %s", headerLine, sourceFile)
	}

	if !txnHeaderRe.MatchString(lines[headerIdx]) || !strings.Contains(lines[headerIdx], "("+fid+")") {
		return fmt.Errorf("journal: bulk-edit: line %d in %s does not match expected transaction header for fid %q", headerLine, sourceFile, fid)
	}

	// --- Step 1: Rewrite header line (status marker and/or payee) ---
	m := headerStatusRe.FindStringSubmatch(lines[headerIdx])
	if m == nil {
		return fmt.Errorf("journal: bulk-edit: cannot parse header line %d in %s", headerLine, sourceFile)
	}
	datePart := m[1]     // e.g. "2026-01-05 "
	currentMarker := m[2] // "" | "!" | "*"
	codePart := m[3]     // e.g. "(a1b2c3d4) " or ""
	rest := m[4]         // description + optional inline comment

	// Determine new status marker.
	newMarker := currentMarker
	if reviewed != nil {
		if *reviewed {
			newMarker = "*"
		} else {
			newMarker = "!"
		}
	}
	markerStr := ""
	if newMarker != "" {
		markerStr = newMarker + " "
	}

	// Separate description from any inline comment (hledger: `;` starts a comment).
	descPart := rest
	inlinePart := ""
	if idx := strings.Index(rest, " ;"); idx >= 0 {
		descPart = rest[:idx]
		inlinePart = rest[idx:]
	}

	// Replace description if payee is being changed.
	if newPayee != nil {
		// Compute note: right side of "|" in current description, or full description if no "|".
		note := txn.Description
		if i := strings.Index(txn.Description, "|"); i >= 0 {
			note = strings.TrimSpace(txn.Description[i+1:])
		}
		if *newPayee == "" {
			descPart = note
		} else {
			descPart = *newPayee + " | " + note
		}
	}

	lines[headerIdx] = datePart + markerStr + codePart + descPart + inlinePart

	// --- Step 2: Rebuild comment block (user tags + float-meta) ---
	// Find where header comments end and postings begin.
	headerEnd := headerIdx + 1
	for headerEnd < len(lines) && !postingLineRe.MatchString(lines[headerEnd]) {
		headerEnd++
	}

	// Compute new user tag set: existing + add - remove.
	newTags := make(map[string]string)
	for _, kv := range txn.Tags {
		if !strings.HasPrefix(kv[0], hledger.HiddenMetaPrefix) {
			newTags[kv[0]] = kv[1]
		}
	}
	if addTagKey != "" {
		newTags[addTagKey] = addTagValue
	}
	if removeTagKey != "" {
		delete(newTags, removeTagKey)
	}

	// Compute new float-meta: preserve existing + stamp updated-at.
	newMeta := make(map[string]string, len(txn.FloatMeta)+1)
	for k, v := range txn.FloatMeta {
		newMeta[k] = v
	}
	newMeta[hledger.HiddenMetaPrefix+"updated-at"] = time.Now().UTC().Format(time.RFC3339)

	// Collect free-text comment lines from the existing comment block.
	// Float-meta lines are dropped (replaced by newMeta above).
	// User-tag-only lines are dropped (replaced by newTags above).
	var freeTextLines []string
	for i := headerIdx + 1; i < headerEnd; i++ {
		if !commentLineRe.MatchString(lines[i]) {
			continue // blank lines in header block — drop
		}
		if isFloatMetaLine(lines[i]) {
			continue // dropped; newMeta replaces them
		}
		stripped := stripTagsFromCommentLine(lines[i])
		if stripped != "" {
			freeTextLines = append(freeTextLines, stripped)
		}
		// else: line was all user-tags — drop it (replaced by newTags)
	}

	// Reconstruct: header → free-text → user-tags (sorted) → float-meta (sorted) → postings.
	newLines := make([]string, 0, len(lines))
	newLines = append(newLines, lines[:headerIdx+1]...)
	newLines = append(newLines, freeTextLines...)

	tagKeys := make([]string, 0, len(newTags))
	for k := range newTags {
		tagKeys = append(tagKeys, k)
	}
	sort.Strings(tagKeys)
	for _, k := range tagKeys {
		newLines = append(newLines, "    ; "+k+":"+newTags[k])
	}

	metaKeys := make([]string, 0, len(newMeta))
	for k := range newMeta {
		metaKeys = append(metaKeys, k)
	}
	sort.Strings(metaKeys)
	for _, k := range metaKeys {
		newLines = append(newLines, "    ; "+k+":"+newMeta[k])
	}

	newLines = append(newLines, lines[headerEnd:]...)

	if err := os.WriteFile(sourceFile, []byte(strings.Join(newLines, "\n")), 0644); err != nil {
		return fmt.Errorf("journal: bulk-edit: write %s: %w", sourceFile, err)
	}
	return nil
}
