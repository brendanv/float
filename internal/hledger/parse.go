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

func parseTransactions(data []byte) ([]Transaction, error) {
	var txns []Transaction
	if err := json.Unmarshal(data, &txns); err != nil {
		return nil, fmt.Errorf("parseTransactions: %w", err)
	}
	for i := range txns {
		for _, tag := range txns[i].Tags {
			if tag[0] == "fid" {
				// hledger extends tag values to end of line unless terminated by
				// a comma, so the value may include trailing comment text
				// (e.g. "908dc69c my note"). Extract only the 8-char hex prefix.
				v := tag[1]
				if len(v) > FIDLen {
					v = v[:FIDLen]
				}
				txns[i].FID = v
				break
			}
		}
	}
	return txns, nil
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
