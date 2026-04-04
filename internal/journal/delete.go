package journal

import (
	"context"
	"fmt"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/slogctx"
)

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
		return fmt.Errorf("journal: delete: no transaction found with fid %q", fid)
	case 1:
		// expected
	default:
		return fmt.Errorf("journal: delete: fid %q matched %d transactions (corrupt journal — run audit)", fid, len(txns))
	}

	txn := txns[0]
	sourceFile := txn.SourcePos[0].File
	sourceLine := txn.SourcePos[0].Line // 1-indexed header line

	if err := removeTransactionAtLine(sourceFile, sourceLine, fid); err != nil {
		return err
	}
	slogctx.FromContext(ctx).Info("journal: transaction deleted", "fid", fid, "file", sourceFile, "line", sourceLine)
	return nil
}
