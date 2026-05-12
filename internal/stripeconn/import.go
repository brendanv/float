package stripeconn

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/rules"
	"github.com/brendanv/float/internal/slogctx"
	"github.com/brendanv/float/internal/txlock"
	"github.com/stripe/stripe-go/v82"
)

// Tag keys stamped on every Stripe-imported transaction.
const (
	TagStripeTxnID     = "stripe-txn-id"
	TagSource          = "source"
	TagStripeConnID    = "stripe-connection"
	TagSourceValue     = "stripe"
	HiddenSyncMetaKey  = "float-stripe-sync"
	HiddenLastSyncedAt = "last_synced_at"
)

// MinSyncInterval is the minimum time that must pass between two syncs of
// the same connection. Stripe FC refresh requests cost real money in
// production, so float server-enforces a one-hour gap.
const MinSyncInterval = time.Hour

// SyncResult summarises the outcome of a single Sync call.
type SyncResult struct {
	Imported int
	Skipped  int
}

// Sync pulls posted transactions from Stripe for conn, dedups them, and
// appends new ones to the journal as fully-formed hledger transactions.
//
// Network I/O against Stripe happens outside the txlock. The journal write
// + connections.json update happen inside a single lock.Do call so that the
// gitsnap commit captures both. On any error the connections.json file is
// restored to its pre-sync bytes.
//
// Requires conn.HledgerAccount to be set; returns an error otherwise.
func Sync(
	ctx context.Context,
	hl *hledger.Client,
	lock *txlock.TxLock,
	dataDir string,
	connID string,
	rulesList []rules.Rule,
	api Stripe,
) (SyncResult, error) {
	logger := slogctx.FromContext(ctx)

	store, err := Load(dataDir)
	if err != nil {
		return SyncResult{}, err
	}
	conn := store.Find(connID)
	if conn == nil {
		return SyncResult{}, fmt.Errorf("stripeconn: connection %s not found", connID)
	}
	if conn.HledgerAccount == "" {
		return SyncResult{}, errors.New("stripeconn: connection is not mapped to an hledger account; configure hledger_account first")
	}
	if conn.DefaultInflowAccount == "" || conn.DefaultOutflowAccount == "" {
		return SyncResult{}, errors.New("stripeconn: connection is missing default_inflow_account or default_outflow_account")
	}
	if !conn.LastSyncedAt.IsZero() && time.Since(conn.LastSyncedAt) < MinSyncInterval {
		return SyncResult{}, fmt.Errorf("stripeconn: minimum %s between syncs; last synced %s ago", MinSyncInterval, time.Since(conn.LastSyncedAt).Truncate(time.Second))
	}

	// Network step (outside the lock).
	stripeTxns, err := api.ListPostedTransactions(ctx, conn.StripeAccountID)
	if err != nil {
		return SyncResult{}, err
	}
	logger.InfoContext(ctx, "stripeconn: fetched stripe transactions",
		"connection_id", connID,
		"stripe_account_id", conn.StripeAccountID,
		"count", len(stripeTxns))

	// Snapshot connections.json so we can restore on failure (txlock only
	// snapshots .journal files).
	storePath := storePath(dataDir)
	origStoreBytes, readErr := os.ReadFile(storePath)
	hadStoreFile := readErr == nil

	batchID := journal.MintFID()
	var result SyncResult
	err = lock.Do(ctx, "stripe sync "+conn.DisplayName, func() error {
		// Dedup set: union of tag-based scan and conn.ImportedIDs.
		seen, scanErr := scanImportedIDs(ctx, hl)
		if scanErr != nil {
			return scanErr
		}
		for _, id := range conn.ImportedIDs {
			seen[id] = struct{}{}
		}

		for _, tx := range stripeTxns {
			if _, ok := seen[tx.ID]; ok {
				result.Skipped++
				continue
			}
			txInput, buildErr := buildTransactionInput(*conn, tx, rulesList, batchID)
			if buildErr != nil {
				return fmt.Errorf("build transaction for %s: %w", tx.ID, buildErr)
			}
			if _, writeErr := journal.AppendTransaction(ctx, hl, dataDir, txInput); writeErr != nil {
				return fmt.Errorf("append transaction for %s: %w", tx.ID, writeErr)
			}
			conn.MarkImported(tx.ID)
			seen[tx.ID] = struct{}{}
			result.Imported++
		}

		conn.LastSyncedAt = time.Now().UTC()
		if len(stripeTxns) > 0 {
			// Use the most recent Stripe transaction id as the cursor.
			// (We re-scan everything anyway; the cursor is informational.)
			conn.LastTransactionCursor = stripeTxns[len(stripeTxns)-1].ID
		}
		if saveErr := Save(dataDir, store); saveErr != nil {
			return saveErr
		}
		return nil
	})
	if err != nil {
		// Restore connections.json to its pre-sync state.
		if hadStoreFile {
			_ = os.WriteFile(storePath, origStoreBytes, 0o644)
		} else {
			_ = os.Remove(storePath)
		}
		return SyncResult{}, err
	}

	logger.InfoContext(ctx, "stripeconn: sync complete",
		"connection_id", connID,
		"imported", result.Imported,
		"skipped", result.Skipped)
	return result, nil
}

// scanImportedIDs returns the set of stripe-txn-id tag values currently
// present in the journal. Used as the primary dedup source.
func scanImportedIDs(ctx context.Context, hl *hledger.Client) (map[string]struct{}, error) {
	txns, err := hl.Transactions(ctx, "tag:"+TagStripeTxnID)
	if err != nil {
		return nil, fmt.Errorf("scan stripe-txn-id tags: %w", err)
	}
	out := make(map[string]struct{}, len(txns))
	for _, t := range txns {
		for _, tag := range t.Tags {
			if tag[0] == TagStripeTxnID && tag[1] != "" {
				out[tag[1]] = struct{}{}
			}
		}
	}
	return out, nil
}

// buildTransactionInput converts a single Stripe transaction into the
// journal.TransactionInput form. Applies float categorization rules to
// override the default "other side" account, payee and tags when a rule
// matches the Stripe description.
func buildTransactionInput(
	conn Connection,
	tx *stripe.FinancialConnectionsTransaction,
	rulesList []rules.Rule,
	batchID string,
) (journal.TransactionInput, error) {
	date := time.Unix(tx.TransactedAt, 0).UTC()
	commodity := strings.ToUpper(conn.Currency)
	if commodity == "" {
		commodity = strings.ToUpper(string(tx.Currency))
	}

	linkedQty, otherQty, err := formatAmount(tx.Amount, commodity)
	if err != nil {
		return journal.TransactionInput{}, err
	}

	otherAccount := conn.DefaultInflowAccount
	if tx.Amount < 0 {
		otherAccount = conn.DefaultOutflowAccount
	}

	description := tx.Description
	tags := map[string]string{
		TagStripeTxnID:  tx.ID,
		TagSource:       TagSourceValue,
		TagStripeConnID: conn.ID,
	}
	status := ""

	if r := rules.Match(rulesList, description); r != nil {
		if r.Payee != "" {
			description = r.Payee + " | " + description
		}
		if r.Account != "" {
			otherAccount = r.Account
		}
		for k, v := range r.Tags {
			tags[k] = v
		}
		if r.AutoReviewed {
			status = "Cleared"
		}
	}

	return journal.TransactionInput{
		Date:        date,
		Description: description,
		Tags:        tags,
		Status:      status,
		FloatMeta: map[string]string{
			hledger.HiddenMetaPrefix + "stripe-sync": batchID,
		},
		Postings: []journal.PostingInput{
			{
				Account:   conn.HledgerAccount,
				Commodity: commodity,
				Quantity:  linkedQty,
			},
			{
				Account:   otherAccount,
				Commodity: commodity,
				Quantity:  otherQty,
			},
		},
	}, nil
}

// formatAmount renders a Stripe minor-units amount as two decimal strings:
// the linked-account posting quantity (sign preserved from Stripe) and the
// "other side" posting quantity (negated). Returns an error for currencies
// whose minor-unit count is unknown.
//
// Sign rule (commented in detail because it's load-bearing):
//
//	Stripe FC amounts are signed from the account holder's perspective:
//	  +X = money into the account holder
//	  -X = money out of the account holder
//
//	float follows hledger's mainstream convention where liabilities carry a
//	negative balance, so the Stripe sign flows through unchanged on the
//	linked-account posting and the other side is the arithmetic negation.
//
//	Examples:
//	  Checking, $50 ATM withdrawal:
//	    stripe = -5000 → assets:checking -50 / expenses:cash 50
//	  Checking, $1200 paycheck:
//	    stripe = +120000 → assets:checking 1200 / income:salary -1200
//	  Credit card, $80 purchase:
//	    stripe = -8000 → liabilities:cc -80 / expenses:foo 80
//	  Credit card, $80 refund:
//	    stripe = +8000 → liabilities:cc 80 / income:refunds -80
func formatAmount(minor int64, currency string) (linked, other string, err error) {
	decimals, ok := currencyDecimals(currency)
	if !ok {
		return "", "", fmt.Errorf("unknown currency %q (no decimal-places mapping)", currency)
	}
	linked = renderMinor(minor, decimals)
	other = renderMinor(-minor, decimals)
	return linked, other, nil
}

// renderMinor renders a signed minor-units integer as a decimal string with
// the given fractional digits. Uses math/big to avoid float precision loss.
func renderMinor(minor int64, decimals int) string {
	neg := minor < 0
	abs := minor
	if neg {
		abs = -minor
	}
	bigAbs := new(big.Int).SetInt64(abs)
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	whole := new(big.Int).Quo(bigAbs, divisor)
	frac := new(big.Int).Rem(bigAbs, divisor)

	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	b.WriteString(whole.String())
	if decimals > 0 {
		b.WriteByte('.')
		fracStr := frac.String()
		// Left-pad fractional part with zeros.
		for i := 0; i < decimals-len(fracStr); i++ {
			b.WriteByte('0')
		}
		b.WriteString(fracStr)
	}
	return b.String()
}

// currencyDecimals returns the standard ISO-4217 fractional-digit count
// for currency. Returns false for unknown currencies. The zero-decimal list
// matches Stripe's documented set.
func currencyDecimals(currency string) (int, bool) {
	switch strings.ToUpper(currency) {
	case "BIF", "CLP", "DJF", "GNF", "JPY", "KMF", "KRW",
		"MGA", "PYG", "RWF", "UGX", "VND", "VUV", "XAF", "XOF", "XPF":
		return 0, true
	case "":
		return 0, false
	default:
		// Default for every other currency is 2 minor digits.
		return 2, true
	}
}
