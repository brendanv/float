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
		pi := PostingInput{
			Account: p.Account,
			Comment: strings.TrimSpace(p.Comment),
		}
		if len(p.Amounts) > 0 {
			a := p.Amounts[0]
			pi.Commodity = a.Commodity
			pi.Quantity = fmt.Sprintf("%.*f", a.Quantity.DecimalPlaces, a.Quantity.FloatingPoint)
			cost, _ := a.ParseCost()
			if cost != nil {
				pi.Cost = &CostInput{
					Commodity: cost.Contents.Commodity,
					Quantity:  fmt.Sprintf("%.*f", cost.Contents.Quantity.DecimalPlaces, math.Abs(cost.Contents.Quantity.FloatingPoint)),
					IsTotal:   cost.Tag == "TotalCost",
				}
			}
		}
		pi.BalanceAssertion = balanceAssertionInputFromHledger(p.BalanceAssertion)
		postings = append(postings, pi)
	}

	// Normalize hledger's "Unmarked" to "" to match TransactionInput convention.
	status := t.Status
	if status == "Unmarked" {
		status = ""
	}

	return TransactionInput{
		Date:        date,
		Description: t.Description,
		Comment:     strings.TrimSpace(t.Comment),
		Status:      status,
		Postings:    postings,
	}, nil
}
