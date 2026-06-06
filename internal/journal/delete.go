package journal

import (
	"context"
	"fmt"
	"strings"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/slogctx"
)

// BatchDeleteTransactions deletes multiple transactions efficiently.
// It fetches all targets in a single hledger query, groups by source file,
// and removes all transactions in each file in one read+write pass (reverse
// line order so earlier removals don't shift subsequent positions).
// onProgress is called after each file group with cumulative counts.
// Callers must wrap this in txlock.Do().
func BatchDeleteTransactions(ctx context.Context, client *hledger.Client, fids []string, onProgress func(deleted, total int32)) error {
	if len(fids) == 0 {
		return nil
	}
	total := int32(len(fids))

	txns, err := client.Transactions(ctx, BuildFIDQuery(fids))
	if err != nil {
		return fmt.Errorf("journal: batch delete: lookup: %w", err)
	}

	byFID := make(map[string]hledger.Transaction, len(txns))
	for _, txn := range txns {
		if txn.FID != "" {
			byFID[txn.FID] = txn
		}
	}

	for _, fid := range fids {
		if _, ok := byFID[fid]; !ok {
			return fmt.Errorf("journal: batch delete: no transaction found with fid %q: %w", fid, ErrNotFound)
		}
	}

	type entry struct {
		fid  string
		line int
		path string
	}
	byFile := make(map[string][]entry)
	for _, fid := range fids {
		txn := byFID[fid]
		if len(txn.SourcePos) == 0 || txn.SourcePos[0].File == "" {
			return fmt.Errorf("journal: batch delete: fid %q has no source position", fid)
		}
		path := txn.SourcePos[0].File
		byFile[path] = append(byFile[path], entry{fid: fid, line: txn.SourcePos[0].Line, path: path})
	}

	log := slogctx.FromContext(ctx)
	var deleted int32
	for path, entries := range byFile {
		specs := make([]DeleteSpec, len(entries))
		for i, e := range entries {
			specs[i] = DeleteSpec{FID: e.fid, HeaderLine: e.line}
		}
		if err := batchRemoveFromFile(path, specs); err != nil {
			return err
		}
		deleted += int32(len(entries))
		for _, e := range entries {
			log.Info("journal: transaction deleted", "fid", e.fid, "file", path, "line", e.line)
		}
		if onProgress != nil {
			onProgress(deleted, total)
		}
	}
	return nil
}

// BuildFIDQuery constructs an hledger query matching any of the given FIDs.
// For multiple FIDs, uses a regex alternation: code:^(fid1|fid2|...)$
func BuildFIDQuery(fids []string) string {
	if len(fids) == 1 {
		return "code:" + fids[0]
	}
	return "code:^(" + strings.Join(fids, "|") + ")$"
}

// DeleteTransaction removes the transaction tagged with fid from its journal file.
// It uses hledger to look up the transaction's exact source location, then removes
// the transaction block at that line.
// Returns an error if the fid is not found or if file I/O fails.
// Callers must wrap this in txlock.Do().
func DeleteTransaction(ctx context.Context, client *hledger.Client, dataDir, fid string) error {
	txns, err := client.Transactions(ctx, "code:"+fid)
	if err != nil {
		return fmt.Errorf("journal: delete: lookup fid %q: %w", fid, err)
	}
	switch len(txns) {
	case 0:
		return fmt.Errorf("journal: delete: no transaction found with fid %q: %w", fid, ErrNotFound)
	case 1:
		// expected
	default:
		return fmt.Errorf("journal: delete: fid %q matched %d transactions (corrupt journal — run audit)", fid, len(txns))
	}

	txn := txns[0]
	if len(txn.SourcePos) == 0 || txn.SourcePos[0].File == "" {
		return fmt.Errorf("journal: delete: fid %q has no source position", fid)
	}
	sourceFile := txn.SourcePos[0].File
	sourceLine := txn.SourcePos[0].Line // 1-indexed header line

	if err := removeTransactionAtLine(sourceFile, sourceLine, fid); err != nil {
		return err
	}
	slogctx.FromContext(ctx).Info("journal: transaction deleted", "fid", fid, "file", sourceFile, "line", sourceLine)
	return nil
}
