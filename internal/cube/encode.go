package cube

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"time"
)

// Magic identifies the payload and its format revision. Bump the trailing
// digit on any breaking layout change; the client refuses a payload it does not
// recognize rather than misreading it.
const Magic = "FLTCUBE1"

// headerPrefixLen is the magic plus the uint32 header length that precedes the
// JSON header.
const headerPrefixLen = len(Magic) + 4

// columnAlign is the byte alignment every column start is padded to.
// Float64Array construction throws on a byte offset that is not a multiple of
// 8, so aligning every column to 8 keeps the client's decode zero-copy for all
// three column widths.
const columnAlign = 8

// maxSafeInteger is 2^53, above which a float64 can no longer represent every
// integer. Amounts are carried as float64 so the client can use Float64Array
// (measured 20x faster than BigInt64Array for the same aggregation) and the
// encoder verifies that every amount stays exactly representable.
const maxSafeInteger = 1 << 53

// Column types understood by the client decoder.
const (
	typeU16 = "u16"
	typeU32 = "u32"
	typeF64 = "f64"
)

func columnWidth(t string) int {
	switch t {
	case typeU16:
		return 2
	case typeU32:
		return 4
	case typeF64:
		return 8
	}
	return 0
}

// wireColumn describes one column's location and meaning. Offset is relative to
// the start of the data section, not to the start of the file, so that the
// header's own length does not feed back into the offsets it contains.
type wireColumn struct {
	Type     string      `json:"type"`
	Offset   int         `json:"offset"`
	Units    string      `json:"units,omitempty"`
	Summable Summability `json:"summable,omitempty"`
}

type wireTable struct {
	Rows     int                   `json:"rows"`
	SortedBy string                `json:"sortedBy,omitempty"`
	Columns  map[string]wireColumn `json:"columns"`
}

type wireHeader struct {
	Generation        uint64               `json:"generation"`
	BuiltAt           string               `json:"builtAt"`
	ConfigHash        string               `json:"configHash"`
	ReportingCurrency string               `json:"reportingCurrency"`
	EpochDate         string               `json:"epochDate"`
	Accounts          []AccountMeta        `json:"accounts"`
	Payees            []string             `json:"payees"`
	Commodities       []Commodity          `json:"commodities"`
	Periods           []string             `json:"periods"`
	Tables            map[string]wireTable `json:"tables"`
}

// column is one column staged for writing: its wire metadata plus the values.
type column struct {
	name  string
	typ   string
	u16   []uint16
	u32   []uint32
	i64   []int64 // written as float64
	units string
	sum   Summability
}

func align(n int) int {
	if r := n % columnAlign; r != 0 {
		return n + columnAlign - r
	}
	return n
}

// Encode serializes the cube to its wire format:
//
//	"FLTCUBE1" | uint32 headerLen | JSON header | pad | columns
//
// Every column is 8-byte aligned within the data section, and the data section
// itself starts at the first 8-byte boundary at or after the header.
func Encode(c *Cube) ([]byte, error) {
	tables := map[string][]column{
		"postings": {
			{name: "date", typ: typeU16, u16: c.Postings.Date},
			{name: "account", typ: typeU32, u32: c.Postings.Account},
			{name: "payee", typ: typeU32, u32: c.Postings.Payee},
			{name: "commodity", typ: typeU16, u16: c.Postings.Commodity},
			{name: "amount", typ: typeF64, i64: c.Postings.Amount, units: "minor", sum: SumBoth},
		},
		"valued": {
			{name: "period", typ: typeU16, u16: c.Valued.Period},
			{name: "account", typ: typeU32, u32: c.Valued.Account},
			{name: "commodity", typ: typeU16, u16: c.Valued.Commodity},
			{name: "amount", typ: typeF64, i64: c.Valued.Amount, units: "minor", sum: SumAccountOnly},
		},
		"cost": {
			{name: "period", typ: typeU16, u16: c.Cost.Period},
			{name: "account", typ: typeU32, u32: c.Cost.Account},
			{name: "commodity", typ: typeU16, u16: c.Cost.Commodity},
			{name: "amount", typ: typeF64, i64: c.Cost.Amount, units: "minor", sum: SumAccountOnly},
		},
	}
	rowCounts := map[string]int{
		"postings": c.Postings.Len(),
		"valued":   c.Valued.Len(),
		"cost":     c.Cost.Len(),
	}
	sortedBy := map[string]string{"postings": "date"}

	// Lay the columns out in a deterministic order so the payload is
	// reproducible for a given cube.
	order := []string{"postings", "valued", "cost"}

	hdr := wireHeader{
		Generation:        c.Generation,
		BuiltAt:           c.BuiltAt.Format(time.RFC3339),
		ConfigHash:        c.ConfigHash,
		ReportingCurrency: c.ReportingCurrency,
		Accounts:          AccountHierarchy(c.Accounts),
		Payees:            c.Payees.Values(),
		Commodities:       c.Commodities,
		Periods:           c.Periods,
		Tables:            make(map[string]wireTable, len(order)),
	}
	if !c.EpochDate.IsZero() {
		hdr.EpochDate = c.EpochDate.Format(dateLayout)
	}
	// Empty slices must serialize as [] rather than null: the client indexes
	// straight into these arrays.
	if hdr.Payees == nil {
		hdr.Payees = []string{}
	}
	if hdr.Periods == nil {
		hdr.Periods = []string{}
	}

	offset := 0
	for _, tableName := range order {
		cols := tables[tableName]
		rows := rowCounts[tableName]
		wt := wireTable{Rows: rows, SortedBy: sortedBy[tableName], Columns: make(map[string]wireColumn, len(cols))}
		for _, col := range cols {
			offset = align(offset)
			wt.Columns[col.name] = wireColumn{
				Type:     col.typ,
				Offset:   offset,
				Units:    col.units,
				Summable: col.sum,
			}
			offset += columnWidth(col.typ) * rows
		}
		hdr.Tables[tableName] = wt
	}
	dataLen := align(offset)

	headerJSON, err := json.Marshal(hdr)
	if err != nil {
		return nil, fmt.Errorf("cube: encode header: %w", err)
	}
	dataStart := align(headerPrefixLen + len(headerJSON))

	out := make([]byte, dataStart+dataLen)
	copy(out, Magic)
	binary.LittleEndian.PutUint32(out[len(Magic):], uint32(len(headerJSON)))
	copy(out[headerPrefixLen:], headerJSON)

	for _, tableName := range order {
		wt := hdr.Tables[tableName]
		for _, col := range tables[tableName] {
			at := dataStart + wt.Columns[col.name].Offset
			if err := writeColumn(out, at, col, wt.Rows); err != nil {
				return nil, fmt.Errorf("cube: encode %s.%s: %w", tableName, col.name, err)
			}
		}
	}
	return out, nil
}

func writeColumn(out []byte, at int, col column, rows int) error {
	switch col.typ {
	case typeU16:
		if len(col.u16) != rows {
			return fmt.Errorf("have %d values, table has %d rows", len(col.u16), rows)
		}
		for i, v := range col.u16 {
			binary.LittleEndian.PutUint16(out[at+i*2:], v)
		}
	case typeU32:
		if len(col.u32) != rows {
			return fmt.Errorf("have %d values, table has %d rows", len(col.u32), rows)
		}
		for i, v := range col.u32 {
			binary.LittleEndian.PutUint32(out[at+i*4:], v)
		}
	case typeF64:
		if len(col.i64) != rows {
			return fmt.Errorf("have %d values, table has %d rows", len(col.i64), rows)
		}
		for i, v := range col.i64 {
			if v >= maxSafeInteger || v <= -maxSafeInteger {
				return fmt.Errorf("minor-unit amount %d exceeds float64 exact-integer range", v)
			}
			binary.LittleEndian.PutUint64(out[at+i*8:], math.Float64bits(float64(v)))
		}
	default:
		return fmt.Errorf("unknown column type %q", col.typ)
	}
	return nil
}
