// Package stripeconn implements Stripe Financial Connections integration for
// float. It manages per-connection state in <data-dir>/stripe/connections.json
// and synchronises Stripe transactions into the hledger journal.
package stripeconn

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	storeDir  = "stripe"
	storeFile = "connections.json"
	// SchemaVersion is the current Store.Version.
	SchemaVersion = 1
)

// Account-category constants. Mirror the values Stripe returns for
// FinancialConnections.Account.Category.
const (
	CategoryCash       = "cash"
	CategoryCredit     = "credit"
	CategoryInvestment = "investment"
	CategoryOther      = "other"
)

// Connection is a single Stripe Financial Connections account that has been
// linked to a float installation, including the account mapping needed to
// import transactions into hledger.
type Connection struct {
	// ID is an 8-char hex identifier minted by float (see journal.MintFID).
	// Used in the stripe-connection: tag on every imported transaction.
	ID string `json:"id"`

	// StripeAccountID is the Stripe identifier (fca_...).
	StripeAccountID string `json:"stripe_account_id"`

	// DisplayName is a user-editable label shown in the UI. Defaults to
	// "<institution> · <last4>" when first created.
	DisplayName string `json:"display_name"`

	// InstitutionName is the bank/institution name reported by Stripe.
	InstitutionName string `json:"institution_name"`

	// Last4 is the last four digits of the account number, if available.
	Last4 string `json:"last4"`

	// AccountCategory mirrors Stripe's account.category field
	// (cash | credit | investment | other).
	AccountCategory string `json:"account_category"`

	// AccountSubcategory mirrors Stripe's account.subcategory field
	// (checking, savings, credit_card, ...).
	AccountSubcategory string `json:"account_subcategory"`

	// Currency is the ISO-4217 code (uppercase) used for amounts on this
	// account. Stripe returns lowercase; we normalise to uppercase here so it
	// flows into hledger as a conventional commodity symbol.
	Currency string `json:"currency"`

	// HledgerAccount is the asset or liability account this Stripe account
	// represents (e.g. "assets:chase:checking" or "liabilities:amex").
	// Required before syncing; empty means "not yet mapped".
	HledgerAccount string `json:"hledger_account"`

	// DefaultInflowAccount is the "other side" account used for transactions
	// where Stripe.Amount > 0 and no float rule matched (e.g. income:unknown).
	DefaultInflowAccount string `json:"default_inflow_account"`

	// DefaultOutflowAccount is the "other side" account used for transactions
	// where Stripe.Amount < 0 and no float rule matched (e.g.
	// expenses:unknown).
	DefaultOutflowAccount string `json:"default_outflow_account"`

	// LastSyncedAt is the time of the last successful sync.
	LastSyncedAt time.Time `json:"last_synced_at"`

	// LastTransactionCursor is the Stripe pagination cursor (last seen
	// transaction id) used to continue from where the previous sync left off.
	LastTransactionCursor string `json:"last_transaction_cursor"`

	// ImportedIDs is the full set of Stripe transaction ids that have ever
	// been imported for this connection. Used as a secondary dedup check
	// alongside the stripe-txn-id: tag on journal entries.
	ImportedIDs []string `json:"imported_ids"`

	// CreatedAt records when the connection was first linked.
	CreatedAt time.Time `json:"created_at"`
}

// Store is the on-disk representation of all linked Stripe connections.
type Store struct {
	Version     int          `json:"version"`
	Connections []Connection `json:"connections"`
}

// storePath returns the absolute path to connections.json for dataDir.
func storePath(dataDir string) string {
	return filepath.Join(dataDir, storeDir, storeFile)
}

// Load reads <dataDir>/stripe/connections.json. Returns an empty Store (with
// the current SchemaVersion) if the file does not exist; that is not an
// error.
func Load(dataDir string) (*Store, error) {
	path := storePath(dataDir)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Store{Version: SchemaVersion}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stripeconn: read %s: %w", path, err)
	}
	var s Store
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("stripeconn: parse %s: %w", path, err)
	}
	if s.Version == 0 {
		s.Version = SchemaVersion
	}
	return &s, nil
}

// Save writes the store back to disk, creating the stripe/ subdirectory if
// needed. Must be called within txlock.Do() since it mutates the data
// directory.
func Save(dataDir string, s *Store) error {
	if s == nil {
		return fmt.Errorf("stripeconn: nil store")
	}
	if s.Version == 0 {
		s.Version = SchemaVersion
	}
	// Keep connections in a stable order so JSON diffs are minimal.
	sort.SliceStable(s.Connections, func(i, j int) bool {
		return s.Connections[i].CreatedAt.Before(s.Connections[j].CreatedAt)
	})
	dir := filepath.Join(dataDir, storeDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("stripeconn: mkdir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("stripeconn: marshal: %w", err)
	}
	path := storePath(dataDir)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("stripeconn: write %s: %w", path, err)
	}
	return nil
}

// Find returns a pointer to the connection with the given float-side id, or
// nil if not found. The pointer aliases into s.Connections, so mutations on
// the returned value are persisted by a subsequent Save.
func (s *Store) Find(id string) *Connection {
	for i := range s.Connections {
		if s.Connections[i].ID == id {
			return &s.Connections[i]
		}
	}
	return nil
}

// FindByStripeID returns a pointer to the connection whose StripeAccountID
// matches, or nil if not found.
func (s *Store) FindByStripeID(stripeAccountID string) *Connection {
	for i := range s.Connections {
		if s.Connections[i].StripeAccountID == stripeAccountID {
			return &s.Connections[i]
		}
	}
	return nil
}

// Upsert inserts or replaces a connection (matched on ID). When inserting, if
// CreatedAt is zero it is set to time.Now().UTC().
func (s *Store) Upsert(c Connection) {
	for i := range s.Connections {
		if s.Connections[i].ID == c.ID {
			s.Connections[i] = c
			return
		}
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	s.Connections = append(s.Connections, c)
}

// Delete removes the connection with the given id. Returns true if a
// connection was removed.
func (s *Store) Delete(id string) bool {
	for i := range s.Connections {
		if s.Connections[i].ID == id {
			s.Connections = append(s.Connections[:i], s.Connections[i+1:]...)
			return true
		}
	}
	return false
}

// MarkImported records that stripeTxnID has been successfully imported. Safe
// to call repeatedly with the same id (no-op on duplicates).
func (c *Connection) MarkImported(stripeTxnID string) {
	for _, existing := range c.ImportedIDs {
		if existing == stripeTxnID {
			return
		}
	}
	c.ImportedIDs = append(c.ImportedIDs, stripeTxnID)
}

// HasImported returns true if stripeTxnID is in c.ImportedIDs.
func (c *Connection) HasImported(stripeTxnID string) bool {
	for _, existing := range c.ImportedIDs {
		if existing == stripeTxnID {
			return true
		}
	}
	return false
}
