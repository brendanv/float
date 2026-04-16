package hledger

import "encoding/json"

// FIDLen is the length in characters of a float transaction ID (fid tag).
const FIDLen = 8

// HiddenMetaPrefix is the tag key prefix reserved for float internal metadata.
// Tags with this prefix are stored in the journal but filtered from the gRPC API.
// Example: "float-import-id", "float-updated-at".
const HiddenMetaPrefix = "float-"

type AmountQuantity struct {
	DecimalMantissa int64   `json:"decimalMantissa"`
	DecimalPlaces   int     `json:"decimalPlaces"`
	FloatingPoint   float64 `json:"floatingPoint"`
}

type Amount struct {
	Commodity string           `json:"acommodity"`
	Quantity  AmountQuantity   `json:"aquantity"`
	Cost      *json.RawMessage `json:"acost"`
}

type Posting struct {
	Account          string      `json:"paccount"`
	Amounts          []Amount    `json:"pamount"`
	Comment          string      `json:"pcomment"`
	Tags             [][2]string `json:"ptags"`
	Status           string      `json:"pstatus"`
	Type             string      `json:"ptype"`
	TransactionIndex string      `json:"ptransaction_"`
	Date             *string     `json:"pdate"`
	Date2            *string     `json:"pdate2"`
}

// SourcePos is a file location emitted by hledger in its JSON output as
// {"sourceName": "/path/to/file.journal", "sourceLine": 6, "sourceColumn": 1}.
type SourcePos struct {
	File   string `json:"sourceName"`
	Line   int    `json:"sourceLine"`
	Column int    `json:"sourceColumn"`
}

type Transaction struct {
	Index            int         `json:"tindex"`
	Date             string      `json:"tdate"`
	Date2            *string     `json:"tdate2"`
	Description      string      `json:"tdescription"`
	Code             string      `json:"tcode"`
	Comment          string      `json:"tcomment"`
	Tags             [][2]string `json:"ttags"`
	Postings         []Posting   `json:"tpostings"`
	Status           string      `json:"tstatus"`
	PrecedingComment string      `json:"tprecedingcomment"`
	// SourcePos is the [start, end] source file position from hledger's
	// tsourcepos field. SourcePos[0] is the transaction header line;
	// SourcePos[1] is the line after the last posting. Always populated
	// when returned by Transactions() or PrintCSV(); zero-value for
	// programmatically constructed transactions.
	SourcePos [2]SourcePos `json:"tsourcepos"`

	// FID is the transaction code (e.g. "aa001100"), extracted from Code.
	// Empty string if no code is present.
	FID string `json:"-"`

	// Payee is the part before the first "|" in Description (trimmed).
	// Nil if no "|" is present.
	Payee *string `json:"-"`
	// Note is the part after the first "|" in Description (trimmed).
	// Nil if no "|" is present.
	Note *string `json:"-"`

	// FloatMeta contains tags whose keys start with HiddenMetaPrefix.
	// These are internal float metadata, not exposed via the gRPC API.
	// Nil if no hidden meta tags are present.
	FloatMeta map[string]string `json:"-"`
}

// RegisterRow is one row from `hledger reg -O json`.
// Each row is a heterogeneous 5-element JSON array — see parseRegisterRows.
// Date and Description are non-nil only for the first posting of each transaction.
type RegisterRow struct {
	Date        *string
	Date2       *string
	Description *string
	Posting     Posting
	Balance     []Amount
}

// AregisterRow is one row from `hledger areg <account> -O json`.
// Unlike RegisterRow (one row per posting), each AregisterRow corresponds to
// a single transaction that touches the focused account. The JSON wire format
// is a heterogeneous 6-element array — see parseAregisterRows.
type AregisterRow struct {
	// Transaction is the source transaction from element [0] of the hledger row.
	// FID/Payee/Note/FloatMeta are populated by the parser, matching the behavior
	// of Transactions().
	Transaction Transaction
	// OtherAccounts are the accounts in the transaction that are NOT in or under
	// the focused account (element [3] of the hledger row).
	OtherAccounts []string
	// Change is the signed net change to the focused account for this
	// transaction (element [4]).
	Change []Amount
	// Balance is the running balance of the focused account after this row
	// (element [5]).
	Balance []Amount
}

// BalanceRow is one account entry from `hledger bal -O json`.
// The JSON encodes each row as a heterogeneous 4-element array — see parseBalanceReport.
type BalanceRow struct {
	DisplayName string
	FullName    string
	Indent      int
	Amounts     []Amount
}

type BalanceReport struct {
	Rows  []BalanceRow
	Total []Amount
}

// AccountType represents the hledger account type letter (A, L, E, R, X, C, V).
type AccountType string

const (
	AccountTypeAsset      AccountType = "A"
	AccountTypeLiability  AccountType = "L"
	AccountTypeEquity     AccountType = "E"
	AccountTypeRevenue    AccountType = "R"
	AccountTypeExpense    AccountType = "X"
	AccountTypeCash       AccountType = "C"
	AccountTypeConversion AccountType = "V"
)

// AccountNode is a node in the account tree.
// Returned by Accounts(tree=true) with children populated,
// or Accounts(tree=false) as a flat list with no children.
type AccountNode struct {
	Name     string      // short segment (e.g. "checking")
	FullName string      // full colon path (e.g. "assets:checking")
	Type     AccountType // hledger type letter; empty if unknown
	Children []*AccountNode
}

// BalanceSheetTimeseries is returned by hledger bs --monthly -O json.
type BalanceSheetTimeseries struct {
	// Periods[i] is the start date "YYYY-MM-DD" of period i (period end is exclusive).
	Periods []string
	// Subreports holds the per-section (e.g. "Assets", "Liabilities") totals.
	Subreports []BSSubreport
	// NetWorth[i] contains the net worth amounts for period i.
	NetWorth [][]Amount
}

// BSSubreport holds per-period totals for one section of the balance sheet.
type BSSubreport struct {
	Name string
	// Totals[i] contains the total amounts for period i.
	Totals [][]Amount
}

type CheckError struct {
	Output string
}

func (e *CheckError) Error() string { return e.Output }
