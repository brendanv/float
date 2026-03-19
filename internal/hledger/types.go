package hledger

import "encoding/json"

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

	// FID is the value of the "fid" tag (e.g. "aa001100"), extracted from Tags.
	// Empty string if no fid tag is present.
	FID string `json:"-"`
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

type CheckError struct {
	Output string
}

func (e *CheckError) Error() string { return e.Output }
