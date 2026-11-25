package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// CategorizedExample is a past transaction the user already categorized,
// given to the AI as a precedent.
type CategorizedExample struct {
	Description string
	Account     string
}

// UnknownTxn is a transaction whose posting account contains "unknown".
type UnknownTxn struct {
	FID         string
	Description string
	Payee       string
	Amount      string
	Date        string
	// Counterparty is the known (non-unknown) side, e.g. the card it was paid with.
	Counterparty string
	// Examples are similar past transactions and the accounts the user chose.
	Examples []CategorizedExample
}

// AccountSuggestion is the AI's proposed account for one UnknownTxn.
type AccountSuggestion struct {
	FID        string
	Account    string
	Confidence string // "high" | "medium" | "low"
	Reasoning  string
}

var suggestAccountsSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"suggestions": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"fid":        map[string]any{"type": "string", "description": "fid of the input transaction"},
					"account":    map[string]any{"type": "string", "description": "Account from the accounts list, or empty string if none fits"},
					"confidence": map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}},
					"reasoning":  map[string]any{"type": "string", "description": "One short sentence explaining the choice"},
				},
				"required":             []string{"fid", "account", "confidence", "reasoning"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"suggestions"},
	"additionalProperties": false,
}

// SuggestAccounts asks the AI to pick an account for each transaction from
// accounts. Suggestions for unknown FIDs, accounts outside the list, or empty
// accounts are dropped, so every returned suggestion names a listed account.
func (c *Client) SuggestAccounts(ctx context.Context, txns []UnknownTxn, accounts []string, userGuidelines string) ([]AccountSuggestion, error) {
	if len(txns) == 0 {
		return nil, nil
	}
	type exampleInput struct {
		Description string `json:"description"`
		Account     string `json:"account"`
	}
	type txnInput struct {
		FID          string         `json:"fid"`
		Description  string         `json:"description"`
		Payee        string         `json:"payee,omitempty"`
		Amount       string         `json:"amount"`
		Date         string         `json:"date"`
		Counterparty string         `json:"paid_from_or_to,omitempty"`
		Examples     []exampleInput `json:"similar_past_transactions,omitempty"`
	}

	inputs := make([]txnInput, len(txns))
	wanted := make(map[string]bool, len(txns))
	for i, t := range txns {
		ex := make([]exampleInput, len(t.Examples))
		for j, e := range t.Examples {
			ex[j] = exampleInput(e)
		}
		inputs[i] = txnInput{
			FID:          t.FID,
			Description:  t.Description,
			Payee:        t.Payee,
			Amount:       t.Amount,
			Date:         t.Date,
			Counterparty: t.Counterparty,
			Examples:     ex,
		}
		wanted[t.FID] = true
	}
	allowed := make(map[string]bool, len(accounts))
	for _, a := range accounts {
		allowed[a] = true
	}

	userPayload, err := json.Marshal(map[string]any{
		"transactions": inputs,
		"accounts":     accounts,
	})
	if err != nil {
		return nil, fmt.Errorf("ai: marshal suggest-accounts payload: %w", err)
	}

	guidelinesSection := ""
	if userGuidelines != "" {
		guidelinesSection = "## User guidelines\n\n" + userGuidelines + "\n\n"
	}

	systemPrompt := strings.TrimSpace(guidelinesSection + `
You are a personal finance assistant categorizing transactions that were imported without a category.
For each transaction, choose the single best account from the provided accounts list.
The amount is the uncategorized side of the transaction: positive is money spent (usually an expenses account), negative is money received (usually income, or a transfer from another asset/liability account).
similar_past_transactions shows how the user categorized similar transactions before; follow those precedents unless they clearly do not apply.
Only use account names exactly as they appear in the accounts list. Never invent accounts.
If no account fits, return an empty string for account.
Use "high" confidence only when the merchant or a precedent makes the category unambiguous.
Return one entry per input transaction, keyed by its fid.
`)

	content, err := c.chat(ctx, []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: string(userPayload)},
	}, "suggest_accounts", suggestAccountsSchema)
	if err != nil {
		return nil, err
	}

	var response struct {
		Suggestions []struct {
			FID        string `json:"fid"`
			Account    string `json:"account"`
			Confidence string `json:"confidence"`
			Reasoning  string `json:"reasoning"`
		} `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return nil, fmt.Errorf("ai: parse suggest-accounts response: %w", err)
	}

	seen := make(map[string]bool, len(response.Suggestions))
	var results []AccountSuggestion
	for _, s := range response.Suggestions {
		account := strings.TrimSpace(s.Account)
		if !wanted[s.FID] || seen[s.FID] || !allowed[account] {
			continue
		}
		seen[s.FID] = true
		confidence := s.Confidence
		switch confidence {
		case "high", "medium", "low":
		default:
			confidence = "low"
		}
		results = append(results, AccountSuggestion{
			FID:        s.FID,
			Account:    account,
			Confidence: confidence,
			Reasoning:  s.Reasoning,
		})
	}
	return results, nil
}
