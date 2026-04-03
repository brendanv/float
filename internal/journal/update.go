package journal

import (
	"context"
	"fmt"
	"time"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/slogctx"
)

// UpdateTransaction replaces the description, date, comment, and postings of the
// transaction identified by fid, preserving the original fid, status, and hidden meta.
// If newDate is empty, the existing transaction date is kept.
// Callers must wrap in txlock.Do().
func UpdateTransaction(ctx context.Context, client *hledger.Client, dataDir, fid, description, newDate, comment string, postings []PostingInput) (hledger.Transaction, error) {
	txns, err := client.Transactions(ctx, "code:"+fid)
	if err != nil {
		return hledger.Transaction{}, fmt.Errorf("journal: update: lookup fid %q: %w", fid, err)
	}
	switch len(txns) {
	case 0:
		return hledger.Transaction{}, fmt.Errorf("journal: update: no transaction found with fid %q", fid)
	case 1:
		// expected
	default:
		return hledger.Transaction{}, fmt.Errorf("journal: update: fid %q matched %d transactions (corrupt journal — run audit)", fid, len(txns))
	}

	t := txns[0]
	src := &SourceLocation{File: t.SourcePos[0].File, Line: t.SourcePos[0].Line}

	input, err := inputFromTransaction(t)
	if err != nil {
		return hledger.Transaction{}, fmt.Errorf("journal: update: %w", err)
	}

	// Apply requested changes.
	input.Description = description
	input.Comment = comment
	input.Postings = postings
	if newDate != "" {
		parsedDate, parseErr := time.Parse("2006-01-02", newDate)
		if parseErr != nil {
			return hledger.Transaction{}, fmt.Errorf("journal: update: invalid date %q: must be YYYY-MM-DD", newDate)
		}
		input.Date = parsedDate
	}

	if _, err := WriteTransaction(ctx, client, dataDir, input, src); err != nil {
		return hledger.Transaction{}, fmt.Errorf("journal: update: write: %w", err)
	}

	updated, err := client.Transactions(ctx, "code:"+fid)
	if err != nil {
		return hledger.Transaction{}, fmt.Errorf("journal: update: re-fetch fid %q: %w", fid, err)
	}
	if len(updated) != 1 {
		return hledger.Transaction{}, fmt.Errorf("journal: update: re-fetch fid %q returned %d transactions", fid, len(updated))
	}

	slogctx.FromContext(ctx).Info("journal: transaction updated", "fid", fid)
	return updated[0], nil
}
