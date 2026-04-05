package journal

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/brendanv/float/internal/hledger"
)

// TxnFingerprint returns a deduplication fingerprint for a transaction:
// date | description | sorted(account:amount) for each posting.
func TxnFingerprint(t hledger.Transaction) string {
	parts := []string{t.Date, t.Description}
	var postings []string
	for _, p := range t.Postings {
		amtStr := ""
		if len(p.Amounts) > 0 {
			a := p.Amounts[0]
			amtStr = fmt.Sprintf("%s%.6f", a.Commodity, a.Quantity.FloatingPoint)
		}
		postings = append(postings, p.Account+":"+amtStr)
	}
	sort.Strings(postings)
	parts = append(parts, postings...)
	return strings.Join(parts, "|")
}

// HledgerTxnToInput converts a parsed hledger.Transaction to a TransactionInput
// suitable for AppendTransaction. All posting amounts are preserved explicitly.
func HledgerTxnToInput(t hledger.Transaction) (TransactionInput, error) {
	date, err := time.Parse("2006-01-02", t.Date)
	if err != nil {
		return TransactionInput{}, fmt.Errorf("parse date %q: %w", t.Date, err)
	}

	var postings []PostingInput
	for _, p := range t.Postings {
		var amtStr string
		if len(p.Amounts) > 0 {
			a := p.Amounts[0]
			val := float64(a.Quantity.DecimalMantissa) / math.Pow10(a.Quantity.DecimalPlaces)
			amtStr = fmt.Sprintf("%s%.2f", a.Commodity, val)
		}
		postings = append(postings, PostingInput{
			Account: p.Account,
			Amount:  amtStr,
			Comment: strings.TrimSpace(p.Comment),
		})
	}

	return TransactionInput{
		Date:        date,
		Description: t.Description,
		Comment:     strings.TrimSpace(t.Comment),
		Postings:    postings,
	}, nil
}
