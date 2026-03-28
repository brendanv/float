package journal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/slogctx"
)

// UpdateTransaction replaces the description, date, comment, and postings of the
// transaction identified by fid, preserving the original fid.
// If newDate is empty, the existing transaction date is kept.
// Callers must wrap in txlock.Do().
func UpdateTransaction(ctx context.Context, client *hledger.Client, dataDir, fid, description, newDate, comment string, postings []PostingInput) (hledger.Transaction, error) {
	txns, err := client.Transactions(ctx, "tag:fid="+fid)
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

	// Determine the date to use.
	var parsedDate time.Time
	if newDate == "" {
		// Keep the existing date.
		var parseErr error
		parsedDate, parseErr = time.Parse("2006-01-02", t.Date)
		if parseErr != nil {
			return hledger.Transaction{}, fmt.Errorf("journal: update: parse existing date %q: %w", t.Date, parseErr)
		}
	} else {
		var parseErr error
		parsedDate, parseErr = time.Parse("2006-01-02", newDate)
		if parseErr != nil {
			return hledger.Transaction{}, fmt.Errorf("journal: update: invalid date %q: must be YYYY-MM-DD", newDate)
		}
	}

	// Strip any fid tag from the caller-supplied comment so AppendTransaction
	// doesn't produce a duplicate when it re-embeds the fid.
	cleanComment := fidTagRe.ReplaceAllString(strings.TrimSpace(comment), "")
	cleanComment = strings.TrimSpace(cleanComment)

	input := TransactionInput{
		Date:        parsedDate,
		Description: description,
		Comment:     cleanComment,
		Postings:    postings,
		FID:         fid,
	}

	if err := DeleteTransaction(ctx, client, dataDir, fid); err != nil {
		return hledger.Transaction{}, fmt.Errorf("journal: update: delete: %w", err)
	}

	if _, err := AppendTransaction(ctx, client, dataDir, input); err != nil {
		return hledger.Transaction{}, fmt.Errorf("journal: update: append: %w", err)
	}

	updated, err := client.Transactions(ctx, "tag:fid="+fid)
	if err != nil {
		return hledger.Transaction{}, fmt.Errorf("journal: update: re-fetch fid %q: %w", fid, err)
	}
	if len(updated) != 1 {
		return hledger.Transaction{}, fmt.Errorf("journal: update: re-fetch fid %q returned %d transactions", fid, len(updated))
	}

	slogctx.FromContext(ctx).Info("journal: transaction updated", "fid", fid)
	return updated[0], nil
}
