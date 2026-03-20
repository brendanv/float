package journal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/slogctx"
)

// UpdateTransactionDate changes the date of the transaction identified by fid to newDate
// ("YYYY-MM-DD"). It deletes the transaction from its current file and re-appends it to
// the correct month file for the new date, preserving all postings, comment, non-fid tags,
// and the original fid.
// Callers must wrap in txlock.Do().
func UpdateTransactionDate(ctx context.Context, client *hledger.Client, dataDir, fid, newDate string) (hledger.Transaction, error) {
	parsedDate, err := time.Parse("2006-01-02", newDate)
	if err != nil {
		return hledger.Transaction{}, fmt.Errorf("journal: update-date: invalid date %q: must be YYYY-MM-DD", newDate)
	}

	txns, err := client.Transactions(ctx, "tag:fid="+fid)
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

	// Build the TransactionInput, stripping the fid tag from the comment
	// so draftFormat doesn't duplicate it when re-rendering.
	comment := fidTagRe.ReplaceAllString(strings.TrimSpace(t.Comment), "")
	comment = strings.TrimSpace(comment)

	var postings []PostingInput
	for _, p := range t.Postings {
		var amtStr string
		if len(p.Amounts) > 0 {
			a := p.Amounts[0]
			amtStr = fmt.Sprintf("%s%.2f", a.Commodity, a.Quantity.FloatingPoint)
		}
		postings = append(postings, PostingInput{
			Account: p.Account,
			Amount:  amtStr,
			Comment: strings.TrimSpace(p.Comment),
		})
	}

	input := TransactionInput{
		Date:        parsedDate,
		Description: t.Description,
		Comment:     comment,
		Postings:    postings,
		FID:         fid,
	}

	if err := DeleteTransaction(ctx, client, dataDir, fid); err != nil {
		return hledger.Transaction{}, fmt.Errorf("journal: update-date: delete: %w", err)
	}

	if _, err := AppendTransaction(ctx, client, dataDir, input); err != nil {
		return hledger.Transaction{}, fmt.Errorf("journal: update-date: append: %w", err)
	}

	updated, err := client.Transactions(ctx, "tag:fid="+fid)
	if err != nil {
		return hledger.Transaction{}, fmt.Errorf("journal: update-date: re-fetch fid %q: %w", fid, err)
	}
	if len(updated) != 1 {
		return hledger.Transaction{}, fmt.Errorf("journal: update-date: re-fetch fid %q returned %d transactions", fid, len(updated))
	}

	slogctx.FromContext(ctx).Info("journal: transaction date updated", "fid", fid, "old_date", t.Date, "new_date", newDate)
	return updated[0], nil
}
