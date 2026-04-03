package journal

import (
	"context"
	"fmt"
	"time"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/slogctx"
)

// UpdateTransactionDate changes the date of the transaction identified by fid to newDate
// ("YYYY-MM-DD"). It replaces the transaction in the journal, moving it to a new month
// file if the date crosses a month boundary. All other fields are preserved.
// Callers must wrap in txlock.Do().
func UpdateTransactionDate(ctx context.Context, client *hledger.Client, dataDir, fid, newDate string) (hledger.Transaction, error) {
	parsedDate, err := time.Parse("2006-01-02", newDate)
	if err != nil {
		return hledger.Transaction{}, fmt.Errorf("journal: update-date: invalid date %q: must be YYYY-MM-DD", newDate)
	}

	txns, err := client.Transactions(ctx, "code:"+fid)
	if err != nil {
		return hledger.Transaction{}, fmt.Errorf("journal: update-date: lookup fid %q: %w", fid, err)
	}
	switch len(txns) {
	case 0:
		return hledger.Transaction{}, fmt.Errorf("journal: update-date: no transaction found with fid %q", fid)
	case 1:
		// expected
	default:
		return hledger.Transaction{}, fmt.Errorf("journal: update-date: fid %q matched %d transactions (corrupt journal — run audit)", fid, len(txns))
	}

	t := txns[0]
	src := &SourceLocation{File: t.SourcePos[0].File, Line: t.SourcePos[0].Line}

	input, err := inputFromTransaction(t)
	if err != nil {
		return hledger.Transaction{}, fmt.Errorf("journal: update-date: %w", err)
	}
	input.Date = parsedDate

	if _, err := WriteTransaction(ctx, client, dataDir, input, src); err != nil {
		return hledger.Transaction{}, fmt.Errorf("journal: update-date: write: %w", err)
	}

	updated, err := client.Transactions(ctx, "code:"+fid)
	if err != nil {
		return hledger.Transaction{}, fmt.Errorf("journal: update-date: re-fetch fid %q: %w", fid, err)
	}
	if len(updated) != 1 {
		return hledger.Transaction{}, fmt.Errorf("journal: update-date: re-fetch fid %q returned %d transactions", fid, len(updated))
	}

	slogctx.FromContext(ctx).Info("journal: transaction date updated", "fid", fid, "old_date", t.Date, "new_date", newDate)
	return updated[0], nil
}
