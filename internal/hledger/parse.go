package hledger

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
)

func parseBalanceReport(data []byte) (*BalanceReport, error) {
	var outer [2]json.RawMessage
	if err := json.Unmarshal(data, &outer); err != nil {
		return nil, fmt.Errorf("parseBalanceReport: unmarshal outer: %w", err)
	}

	var rawRows []json.RawMessage
	if err := json.Unmarshal(outer[0], &rawRows); err != nil {
		return nil, fmt.Errorf("parseBalanceReport: unmarshal rows: %w", err)
	}

	rows := make([]BalanceRow, 0, len(rawRows))
	for i, raw := range rawRows {
		var fields [4]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, fmt.Errorf("parseBalanceReport: row %d unmarshal fields: %w", i, err)
		}
		var row BalanceRow
		if err := json.Unmarshal(fields[0], &row.DisplayName); err != nil {
			return nil, fmt.Errorf("parseBalanceReport: row %d DisplayName: %w", i, err)
		}
		if err := json.Unmarshal(fields[1], &row.FullName); err != nil {
			return nil, fmt.Errorf("parseBalanceReport: row %d FullName: %w", i, err)
		}
		if err := json.Unmarshal(fields[2], &row.Indent); err != nil {
			return nil, fmt.Errorf("parseBalanceReport: row %d Indent: %w", i, err)
		}
		if err := json.Unmarshal(fields[3], &row.Amounts); err != nil {
			return nil, fmt.Errorf("parseBalanceReport: row %d Amounts: %w", i, err)
		}
		rows = append(rows, row)
	}

	var totals []Amount
	if err := json.Unmarshal(outer[1], &totals); err != nil {
		return nil, fmt.Errorf("parseBalanceReport: unmarshal totals: %w", err)
	}

	return &BalanceReport{Rows: rows, Total: totals}, nil
}

func parseRegisterRows(data []byte) ([]RegisterRow, error) {
	var rawRows []json.RawMessage
	if err := json.Unmarshal(data, &rawRows); err != nil {
		return nil, fmt.Errorf("parseRegisterRows: unmarshal outer: %w", err)
	}

	rows := make([]RegisterRow, 0, len(rawRows))
	for i, raw := range rawRows {
		var fields [5]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, fmt.Errorf("parseRegisterRows: row %d unmarshal fields: %w", i, err)
		}
		var row RegisterRow
		if err := json.Unmarshal(fields[0], &row.Date); err != nil {
			return nil, fmt.Errorf("parseRegisterRows: row %d Date: %w", i, err)
		}
		if err := json.Unmarshal(fields[1], &row.Date2); err != nil {
			return nil, fmt.Errorf("parseRegisterRows: row %d Date2: %w", i, err)
		}
		if err := json.Unmarshal(fields[2], &row.Description); err != nil {
			return nil, fmt.Errorf("parseRegisterRows: row %d Description: %w", i, err)
		}
		if err := json.Unmarshal(fields[3], &row.Posting); err != nil {
			return nil, fmt.Errorf("parseRegisterRows: row %d Posting: %w", i, err)
		}
		if err := json.Unmarshal(fields[4], &row.Balance); err != nil {
			return nil, fmt.Errorf("parseRegisterRows: row %d Balance: %w", i, err)
		}
		rows = append(rows, row)
	}

	return rows, nil
}

// splitPayeeNote splits an hledger description on the first "|".
// If no "|" is present, both payee and note equal the full description.
func splitPayeeNote(desc string) (payee, note *string) {
	if i := strings.Index(desc, "|"); i >= 0 {
		p := strings.TrimSpace(desc[:i])
		n := strings.TrimSpace(desc[i+1:])
		return &p, &n
	}
	return nil, nil
}

// enrichTransaction populates the derived FID, Payee, Note, and FloatMeta
// fields on a freshly-unmarshaled Transaction. Safe to call on a zero-value
// Transaction; fields without a matching source are left nil / empty.
func enrichTransaction(t *Transaction) {
	t.FID = t.Code
	t.Payee, t.Note = splitPayeeNote(t.Description)
	for _, kv := range t.Tags {
		if strings.HasPrefix(kv[0], HiddenMetaPrefix) {
			if t.FloatMeta == nil {
				t.FloatMeta = make(map[string]string)
			}
			t.FloatMeta[kv[0]] = kv[1]
		}
	}
}

func parseTransactions(data []byte) ([]Transaction, error) {
	var txns []Transaction
	if err := json.Unmarshal(data, &txns); err != nil {
		return nil, fmt.Errorf("parseTransactions: %w", err)
	}
	for i := range txns {
		enrichTransaction(&txns[i])
	}
	return txns, nil
}

// parseAregisterRows parses the output of `hledger areg -O json`.
// Each row is a heterogeneous 6-element JSON array:
//
//	[ Transaction, Transaction, Bool, []string, []Amount, []Amount ]
//	  source       displayed    flag  others    change    balance
//
// We keep the source transaction (element 0) and discard the duplicate
// displayed transaction (1) and the subaccount-included flag (2).
func parseAregisterRows(data []byte) ([]AregisterRow, error) {
	var rawRows []json.RawMessage
	if err := json.Unmarshal(data, &rawRows); err != nil {
		return nil, fmt.Errorf("parseAregisterRows: unmarshal outer: %w", err)
	}

	rows := make([]AregisterRow, 0, len(rawRows))
	for i, raw := range rawRows {
		var fields [6]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, fmt.Errorf("parseAregisterRows: row %d unmarshal fields: %w", i, err)
		}
		var row AregisterRow
		if err := json.Unmarshal(fields[0], &row.Transaction); err != nil {
			return nil, fmt.Errorf("parseAregisterRows: row %d Transaction: %w", i, err)
		}
		enrichTransaction(&row.Transaction)
		// fields[1] (displayed txn) and fields[2] (subaccount flag) ignored.
		if err := json.Unmarshal(fields[3], &row.OtherAccounts); err != nil {
			return nil, fmt.Errorf("parseAregisterRows: row %d OtherAccounts: %w", i, err)
		}
		if err := json.Unmarshal(fields[4], &row.Change); err != nil {
			return nil, fmt.Errorf("parseAregisterRows: row %d Change: %w", i, err)
		}
		if err := json.Unmarshal(fields[5], &row.Balance); err != nil {
			return nil, fmt.Errorf("parseAregisterRows: row %d Balance: %w", i, err)
		}
		rows = append(rows, row)
	}

	return rows, nil
}

// parseBalanceSheetTimeseries parses the JSON object emitted by
// `hledger bs --monthly -O json`. The format differs substantially from
// `hledger bal`: it is a JSON object with cbrDates, cbrSubreports, and
// cbrTotals rather than a simple two-element array.
func parseBalanceSheetTimeseries(data []byte) (*BalanceSheetTimeseries, error) {
	// Intermediate structs that mirror hledger's JSON schema.
	type dateEntry struct {
		Contents string `json:"contents"`
	}
	type prrRow struct {
		PrrAmounts [][]Amount `json:"prrAmounts"`
	}
	type prSubreport struct {
		PrTotals prrRow `json:"prTotals"`
	}
	type bsJSON struct {
		CbrDates      [][]dateEntry     `json:"cbrDates"`
		CbrSubreports []json.RawMessage `json:"cbrSubreports"`
		CbrTotals     prrRow            `json:"cbrTotals"`
	}

	var raw bsJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parseBalanceSheetTimeseries: unmarshal: %w", err)
	}

	// Extract period start dates.
	periods := make([]string, len(raw.CbrDates))
	for i, pair := range raw.CbrDates {
		if len(pair) < 1 {
			return nil, fmt.Errorf("parseBalanceSheetTimeseries: period %d missing start date", i)
		}
		periods[i] = pair[0].Contents
	}

	// Each element of cbrSubreports is a 2-element JSON array: [name, subreportObject].
	subreports := make([]BSSubreport, 0, len(raw.CbrSubreports))
	for i, rawSub := range raw.CbrSubreports {
		var pair [2]json.RawMessage
		if err := json.Unmarshal(rawSub, &pair); err != nil {
			return nil, fmt.Errorf("parseBalanceSheetTimeseries: subreport %d unmarshal pair: %w", i, err)
		}
		var name string
		if err := json.Unmarshal(pair[0], &name); err != nil {
			return nil, fmt.Errorf("parseBalanceSheetTimeseries: subreport %d name: %w", i, err)
		}
		var sub prSubreport
		if err := json.Unmarshal(pair[1], &sub); err != nil {
			return nil, fmt.Errorf("parseBalanceSheetTimeseries: subreport %d data: %w", i, err)
		}
		subreports = append(subreports, BSSubreport{
			Name:   name,
			Totals: sub.PrTotals.PrrAmounts,
		})
	}

	return &BalanceSheetTimeseries{
		Periods:    periods,
		Subreports: subreports,
		NetWorth:   raw.CbrTotals.PrrAmounts,
	}, nil
}

// parseIncomeStatementTimeseries parses the JSON object emitted by
// `hledger is --monthly --tree -O json`. The format is the same compound
// report structure as `hledger bs`, but each subreport row is a JSON object
// with prrName, prrAmounts, prrTotal, and prrAverage fields.
func parseIncomeStatementTimeseries(data []byte) (*IncomeStatementTimeseries, error) {
	type dateEntry struct {
		Contents string `json:"contents"`
	}
	type prrRowJSON struct {
		PrrAmounts [][]Amount `json:"prrAmounts"`
		PrrAverage []Amount   `json:"prrAverage"` // ignored
		PrrName    string     `json:"prrName"`
		PrrTotal   []Amount   `json:"prrTotal"`
	}
	type prrTotalsJSON struct {
		PrrAmounts [][]Amount `json:"prrAmounts"`
	}
	type isSubreportData struct {
		PrRows   []prrRowJSON  `json:"prRows"`
		PrTotals prrTotalsJSON `json:"prTotals"`
	}
	type isJSON struct {
		CbrDates      [][]dateEntry     `json:"cbrDates"`
		CbrSubreports []json.RawMessage `json:"cbrSubreports"`
		CbrTotals     prrTotalsJSON     `json:"cbrTotals"`
	}

	var raw isJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parseIncomeStatementTimeseries: unmarshal: %w", err)
	}

	periods := make([]string, len(raw.CbrDates))
	for i, pair := range raw.CbrDates {
		if len(pair) < 1 {
			return nil, fmt.Errorf("parseIncomeStatementTimeseries: period %d missing start date", i)
		}
		periods[i] = pair[0].Contents
	}

	subreports := make([]ISSubreport, 0, len(raw.CbrSubreports))
	for i, rawSub := range raw.CbrSubreports {
		var pair [2]json.RawMessage
		if err := json.Unmarshal(rawSub, &pair); err != nil {
			return nil, fmt.Errorf("parseIncomeStatementTimeseries: subreport %d unmarshal pair: %w", i, err)
		}
		var name string
		if err := json.Unmarshal(pair[0], &name); err != nil {
			return nil, fmt.Errorf("parseIncomeStatementTimeseries: subreport %d name: %w", i, err)
		}
		var subData isSubreportData
		if err := json.Unmarshal(pair[1], &subData); err != nil {
			return nil, fmt.Errorf("parseIncomeStatementTimeseries: subreport %d data: %w", i, err)
		}

		rows := make([]ISRow, 0, len(subData.PrRows))
		for _, r := range subData.PrRows {
			row := ISRow{
				FullName:         r.PrrName,
				DisplayName:      isDisplayName(r.PrrName),
				Indent:           strings.Count(r.PrrName, ":"),
				Section:          name,
				PerPeriodAmounts: r.PrrAmounts,
				TotalAmounts:     r.PrrTotal,
			}
			rows = append(rows, row)
		}

		subreports = append(subreports, ISSubreport{
			Name:   name,
			Rows:   rows,
			Totals: subData.PrTotals.PrrAmounts,
		})
	}

	return &IncomeStatementTimeseries{
		Periods:    periods,
		Subreports: subreports,
		NetAmounts: raw.CbrTotals.PrrAmounts,
	}, nil
}

// isDisplayName returns the last colon-separated segment of an account name,
// e.g. "expenses:food" → "food". Returns the full name if no colon is present.
func isDisplayName(fullName string) string {
	if idx := strings.LastIndex(fullName, ":"); idx >= 0 {
		return fullName[idx+1:]
	}
	return fullName
}

// extractAccountType parses the "; type: X" suffix added by hledger --types.
// Returns the trimmed account name and the type letter (or empty string if absent).
func extractAccountType(s string) (name string, typ AccountType) {
	if idx := strings.Index(s, "; type: "); idx >= 0 {
		letter := strings.TrimSpace(s[idx+8:])
		return strings.TrimSpace(s[:idx]), AccountType(letter)
	}
	return strings.TrimSpace(s), ""
}

func parseAccountsTree(text string) ([]*AccountNode, error) {
	lines := strings.Split(text, "\n")
	var roots []*AccountNode
	type stackEntry struct {
		depth int
		node  *AccountNode
	}
	var stack []stackEntry

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		spaces := len(line) - len(strings.TrimLeft(line, " "))
		depth := spaces / 2
		shortName, acctType := extractAccountType(strings.TrimSpace(line))

		node := &AccountNode{Name: shortName, Type: acctType}

		if depth == 0 {
			node.FullName = shortName
			stack = stack[:0]
			roots = append(roots, node)
			stack = append(stack, stackEntry{depth: 0, node: node})
		} else {
			for len(stack) > 0 && stack[len(stack)-1].depth >= depth {
				stack = stack[:len(stack)-1]
			}
			if len(stack) == 0 {
				return nil, fmt.Errorf("parseAccountsTree: no parent for depth %d node %q", depth, shortName)
			}
			parent := stack[len(stack)-1].node
			node.FullName = parent.FullName + ":" + shortName
			parent.Children = append(parent.Children, node)
			stack = append(stack, stackEntry{depth: depth, node: node})
		}
	}

	return roots, nil
}

func parseAccountsFlat(text string) ([]*AccountNode, error) {
	lines := strings.Split(text, "\n")
	var nodes []*AccountNode
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fullName, acctType := extractAccountType(line)
		shortName := fullName
		if idx := strings.LastIndex(fullName, ":"); idx >= 0 {
			shortName = fullName[idx+1:]
		}
		nodes = append(nodes, &AccountNode{
			Name:     shortName,
			FullName: fullName,
			Type:     acctType,
			Children: nil,
		})
	}
	return nodes, nil
}

// parseTags parses `hledger tags` output: one tag name per line.
// Filters out empty lines.
func parseTags(data []byte) []string {
	var tags []string
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

// parsePayees parses `hledger payees` output: one payee name per line.
// Filters out empty lines.
func parsePayees(data []byte) []string {
	var payees []string
	for _, line := range strings.Split(string(data), "\n") {
		p := strings.TrimSpace(line)
		if p != "" {
			payees = append(payees, p)
		}
	}
	return payees
}

// parsePostingRowsCSV parses the output of `hledger print -O csv`, whose
// header is:
//
//	txnidx,date,date2,status,code,description,comment,
//	account,amount,commodity,credit,debit,posting-status,posting-comment
//
// Columns are located by header name rather than by position so a future
// hledger release that adds a column does not silently shift the parse.
func parsePostingRowsCSV(data []byte) ([]PostingRow, error) {
	records, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parsePostingRowsCSV: %w", err)
	}
	if len(records) == 0 {
		return nil, nil
	}
	idx, err := csvHeaderIndex(records[0], "date", "status", "code", "description", "account", "amount", "commodity")
	if err != nil {
		return nil, fmt.Errorf("parsePostingRowsCSV: %w", err)
	}
	rows := make([]PostingRow, 0, len(records)-1)
	for _, rec := range records[1:] {
		if len(rec) < len(records[0]) {
			continue
		}
		rows = append(rows, PostingRow{
			Date:        rec[idx["date"]],
			Status:      rec[idx["status"]],
			Code:        rec[idx["code"]],
			Description: rec[idx["description"]],
			Account:     rec[idx["account"]],
			Amount:      rec[idx["amount"]],
			Commodity:   rec[idx["commodity"]],
		})
	}
	return rows, nil
}

// parsePeriodBalancesCSV parses the output of
// `hledger bal --monthly --layout=bare -O csv`, whose shape is:
//
//	"account","commodity","2016-01","2016-02",...
//	"assets:checking","USD","8567.27","-4286.72",...
//	"Total:","USD",...
//
// The trailing "Total:" row is dropped — it is a report artifact, not an
// account, and callers roll their own totals up the account tree.
func parsePeriodBalancesCSV(data []byte) (*PeriodBalances, error) {
	records, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parsePeriodBalancesCSV: %w", err)
	}
	if len(records) == 0 {
		return &PeriodBalances{}, nil
	}
	header := records[0]
	if len(header) < 2 {
		return nil, fmt.Errorf("parsePeriodBalancesCSV: header has %d columns, need at least 2", len(header))
	}
	out := &PeriodBalances{Periods: append([]string(nil), header[2:]...)}
	for _, rec := range records[1:] {
		if len(rec) < 2 || rec[0] == "Total:" || rec[0] == "" {
			continue
		}
		amounts := make([]string, len(out.Periods))
		for i := range amounts {
			if col := i + 2; col < len(rec) {
				amounts[i] = rec[col]
			}
		}
		out.Rows = append(out.Rows, PeriodBalanceRow{
			Account:   rec[0],
			Commodity: rec[1],
			Amounts:   amounts,
		})
	}
	return out, nil
}

// csvHeaderIndex maps each wanted column name to its position in header,
// erroring if any is absent.
func csvHeaderIndex(header []string, want ...string) (map[string]int, error) {
	pos := make(map[string]int, len(header))
	for i, h := range header {
		pos[strings.TrimSpace(h)] = i
	}
	idx := make(map[string]int, len(want))
	for _, w := range want {
		i, ok := pos[w]
		if !ok {
			return nil, fmt.Errorf("missing expected column %q in header %v", w, header)
		}
		idx[w] = i
	}
	return idx, nil
}

// PayeeOf returns the payee portion of a transaction description, applying the
// same "payee | note" convention as Transaction.Payee. Descriptions without a
// "|" are payees in their entirety.
//
// Exported for internal/cube, which builds a payee dimension from flat
// PostingRow descriptions rather than from full Transaction values.
func PayeeOf(desc string) string {
	if payee, _ := splitPayeeNote(desc); payee != nil {
		return *payee
	}
	return strings.TrimSpace(desc)
}
