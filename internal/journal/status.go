package journal

import (
	"context"
	"fmt"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/slogctx"
)

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

	txns, err := client.Transactions(ctx, "code:"+fid)
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

	t := txns[0]
	src := &SourceLocation{File: t.SourcePos[0].File, Line: t.SourcePos[0].Line}

	input, err := InputFromTransaction(t)
	if err != nil {
		return fmt.Errorf("journal: update-status: %w", err)
	}
	input.Status = newStatus

	if _, err := WriteTransaction(ctx, client, dataDir, input, src); err != nil {
		return fmt.Errorf("journal: update-status: write: %w", err)
	}

	slogctx.FromContext(ctx).Info("journal: transaction status updated", "fid", fid, "status", newStatus)
	return nil
}
