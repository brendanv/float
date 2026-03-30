package journal

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/brendanv/float/internal/hledger"
)

// PostingInput represents one leg of a transaction.
type PostingInput struct {
	Account string // e.g. "expenses:shopping"
	Amount  string // e.g. "$45.00"; empty string means auto-balance posting
	Comment string // optional inline comment text (without "; " prefix)
}

// TransactionInput represents a transaction to be written.
type TransactionInput struct {
	Date        time.Time
	Description string
	Comment     string // optional transaction-level comment (without "; " prefix)
	Postings    []PostingInput
	FID         string // optional; if empty, AppendTransaction mints a new fid
	Status      string // "", "Pending" (!), or "Cleared" (*); empty means Unmarked
}

// draftFormat renders a TransactionInput + fid as minimal hledger journal text.
// Output is valid for hledger to parse but not canonically formatted.
// Used internally as input to FormatViaHledger.
func draftFormat(tx TransactionInput, fid string) string {
	var b strings.Builder
	statusPart := ""
	switch tx.Status {
	case "Pending":
		statusPart = "! "
	case "Cleared":
		statusPart = "* "
	}
	fmt.Fprintf(&b, "%s %s(%s) %s\n", tx.Date.Format("2006-01-02"), statusPart, fid, tx.Description)
	if tx.Comment != "" {
		fmt.Fprintf(&b, "    ; %s\n", tx.Comment)
	}
	for _, p := range tx.Postings {
		if p.Amount == "" {
			fmt.Fprintf(&b, "    %s\n", p.Account)
		} else if p.Comment != "" {
			fmt.Fprintf(&b, "    %s  %s  ; %s\n", p.Account, p.Amount, p.Comment)
		} else {
			fmt.Fprintf(&b, "    %s  %s\n", p.Account, p.Amount)
		}
	}
	return b.String()
}

// FormatViaHledger writes tx to a temp file, runs `hledger print -f <tmpfile>`,
// and returns the canonical hledger-formatted output.
func FormatViaHledger(ctx context.Context, client *hledger.Client, tx TransactionInput, fid string) (string, error) {
	draft := draftFormat(tx, fid)

	f, err := os.CreateTemp("", "float-txn-*.journal")
	if err != nil {
		return "", fmt.Errorf("journal: create temp file: %w", err)
	}
	tmpPath := f.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := f.WriteString(draft); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("journal: write temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("journal: close temp file: %w", err)
	}

	out, err := client.PrintText(ctx, tmpPath)
	if err != nil {
		return "", fmt.Errorf("journal: format via hledger: %w", err)
	}
	return out, nil
}
