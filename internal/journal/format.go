package journal

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/brendanv/float/internal/hledger"
)

// CostInput represents a cost annotation on a posting (@ or @@).
type CostInput struct {
	Commodity string
	Quantity  string
	IsTotal   bool // false = per-unit (@), true = total (@@)
}

// BalanceAssertionInput represents an hledger balance assertion on a posting.
// Inclusive==true renders as =* (subaccount-inclusive); Total==true renders
// as == (sole-commodity total). Both false is the simple = form, which is
// the only variant exposed via the gRPC API. The full struct is preserved
// internally so that operations that round-trip postings (ModifyTags,
// UpdateTransactionStatus, etc.) don't destroy assertions written by hand.
type BalanceAssertionInput struct {
	Commodity string
	Quantity  string
	Inclusive bool
	Total     bool
}

// PostingInput represents one leg of a transaction.
type PostingInput struct {
	Account          string                 // e.g. "expenses:shopping"
	Commodity        string                 // e.g. "USD", "AAPL"; empty = auto-balance posting
	Quantity         string                 // e.g. "45.00"; empty = auto-balance posting
	Comment          string                 // optional inline comment text (without "; " prefix)
	Cost             *CostInput             // optional cost annotation
	BalanceAssertion *BalanceAssertionInput // optional balance assertion (= / =* / == / ==*)
}

// TransactionInput represents a transaction to be written.
type TransactionInput struct {
	Date        time.Time
	Description string
	Comment     string            // optional transaction-level free-text comment (without "; " prefix)
	Tags        map[string]string // optional user-visible tags (keys must NOT have hledger.HiddenMetaPrefix)
	Postings    []PostingInput
	FID         string            // optional; if empty, WriteTransaction mints a new fid
	Status      string            // "", "Pending" (!), or "Cleared" (*); empty means Unmarked
	FloatMeta   map[string]string // optional internal metadata; keys must have hledger.HiddenMetaPrefix
}

// postingAmountString builds the hledger amount string for a posting.
// Uses "QUANTITY COMMODITY" format which hledger accepts universally and
// FormatViaHledger will canonicalize to the commodity directive's placement.
// Returns "" for auto-balance postings (empty commodity and quantity).
func postingAmountString(p PostingInput) string {
	if p.Commodity == "" && p.Quantity == "" {
		return ""
	}
	s := p.Quantity + " " + p.Commodity
	if p.Cost != nil {
		op := "@"
		if p.Cost.IsTotal {
			op = "@@"
		}
		s += " " + op + " " + p.Cost.Quantity + " " + p.Cost.Commodity
	}
	return s
}

// postingAssertionString builds the hledger balance-assertion suffix
// (e.g. " = $100", " =* $100", " == $100", " ==* $100"). The leading space
// lets callers concatenate it directly after the amount or account.
// Returns "" when no assertion is set.
func postingAssertionString(p PostingInput) string {
	if p.BalanceAssertion == nil {
		return ""
	}
	op := "="
	if p.BalanceAssertion.Total {
		op = "=="
	}
	if p.BalanceAssertion.Inclusive {
		op += "*"
	}
	return " " + op + " " + p.BalanceAssertion.Quantity + " " + p.BalanceAssertion.Commodity
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
	for _, line := range strings.Split(tx.Comment, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			fmt.Fprintf(&b, "    ; %s\n", line)
		}
	}
	if len(tx.Tags) > 0 {
		keys := make([]string, 0, len(tx.Tags))
		for k := range tx.Tags {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "    ; %s:%s\n", k, tx.Tags[k])
		}
	}
	if len(tx.FloatMeta) > 0 {
		keys := make([]string, 0, len(tx.FloatMeta))
		for k := range tx.FloatMeta {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "    ; %s:%s\n", k, tx.FloatMeta[k])
		}
	}
	for _, p := range tx.Postings {
		amtStr := postingAmountString(p)
		// assertStr is "" or starts with a space (e.g. " = $100").
		// hledger requires the assertion to appear directly after the
		// amount and BEFORE any inline comment.
		assertStr := postingAssertionString(p)
		head := "    " + p.Account
		if amtStr != "" {
			head += "  " + amtStr
		}
		head += assertStr
		if p.Comment != "" {
			fmt.Fprintf(&b, "%s  ; %s\n", head, p.Comment)
		} else {
			fmt.Fprintf(&b, "%s\n", head)
		}
	}
	return b.String()
}

// BatchFormatViaHledger formats multiple transactions in a single hledger subprocess call.
// Returns canonical hledger-formatted text for each transaction in the same order as inputs.
// Each returned string ends with "\n\n" (same format as FormatViaHledger).
func BatchFormatViaHledger(ctx context.Context, client *hledger.Client, inputs []TransactionInput, fids []string) ([]string, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	if len(inputs) != len(fids) {
		return nil, fmt.Errorf("journal: BatchFormatViaHledger: inputs/fids length mismatch")
	}

	var all strings.Builder
	for i, tx := range inputs {
		all.WriteString(draftFormat(tx, fids[i]))
		all.WriteString("\n") // blank line between transactions
	}

	f, err := os.CreateTemp("", "float-batch-*.journal")
	if err != nil {
		return nil, fmt.Errorf("journal: create temp file: %w", err)
	}
	tmpPath := f.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := f.WriteString(all.String()); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("journal: write temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("journal: close temp file: %w", err)
	}

	out, err := client.PrintText(ctx, tmpPath)
	if err != nil {
		return nil, fmt.Errorf("journal: batch format via hledger: %w", err)
	}

	// Split output into per-transaction blocks. hledger print separates
	// transactions with exactly one blank line; transactions never contain
	// internal blank lines.
	var result []string
	for _, block := range strings.Split(strings.TrimRight(out, "\n"), "\n\n") {
		block = strings.TrimSpace(block)
		if block != "" {
			result = append(result, block+"\n\n")
		}
	}

	if len(result) != len(inputs) {
		return nil, fmt.Errorf("journal: batch format: expected %d transactions, got %d", len(inputs), len(result))
	}
	return result, nil
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
