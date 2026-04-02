package hledger

import (
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

func parseTransactions(data []byte) ([]Transaction, error) {
	var txns []Transaction
	if err := json.Unmarshal(data, &txns); err != nil {
		return nil, fmt.Errorf("parseTransactions: %w", err)
	}
	for i := range txns {
		txns[i].FID = txns[i].Code
		txns[i].Payee, txns[i].Note = splitPayeeNote(txns[i].Description)
		for _, kv := range txns[i].Tags {
			if strings.HasPrefix(kv[0], HiddenMetaPrefix) {
				if txns[i].FloatMeta == nil {
					txns[i].FloatMeta = make(map[string]string)
				}
				txns[i].FloatMeta[kv[0]] = kv[1]
			}
		}
	}
	return txns, nil
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
		CbrDates      [][]dateEntry      `json:"cbrDates"`
		CbrSubreports []json.RawMessage  `json:"cbrSubreports"`
		CbrTotals     prrRow             `json:"cbrTotals"`
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
