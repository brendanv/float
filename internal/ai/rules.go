package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// TxnSummary is a condensed transaction for the AI prompt.
type TxnSummary struct {
	FID         string
	Description string
	Payee       string
	Account     string // category posting account (non-asset/liability side)
	Amount      string
	Date        string
}

// RuleSummary is a condensed existing rule for the AI prompt.
type RuleSummary struct {
	Pattern string
	Payee   string
	Account string
}

// SuggestedRule is one rule suggestion returned by the AI.
type SuggestedRule struct {
	Pattern     string
	Payee       string
	Account     string
	Reasoning   string
	ExampleFIDs []string
}

var suggestRulesSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"rules": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern":      map[string]any{"type": "string", "description": "Go case-insensitive regex matched against transaction description"},
					"payee":        map[string]any{"type": "string", "description": "Suggested payee name (empty string if no change)"},
					"account":      map[string]any{"type": "string", "description": "Suggested category account (e.g. expenses:food:groceries)"},
					"reasoning":    map[string]any{"type": "string", "description": "Brief explanation of why this rule was suggested"},
					"example_fids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "FIDs of input transactions this pattern would match"},
				},
				"required":             []string{"pattern", "payee", "account", "reasoning", "example_fids"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"rules"},
	"additionalProperties": false,
}

// SuggestRules asks the AI to suggest categorization rules for the given
// transactions. Existing rules and the full account list are provided as
// context so the AI avoids duplicates and uses real account names.
func (c *Client) SuggestRules(ctx context.Context, txns []TxnSummary, existingRules []RuleSummary, accounts []string) ([]SuggestedRule, error) {
	type txnInput struct {
		FID         string `json:"fid"`
		Description string `json:"description"`
		Payee       string `json:"payee,omitempty"`
		Amount      string `json:"amount"`
		Date        string `json:"date"`
	}
	type ruleInput struct {
		Pattern string `json:"pattern"`
		Payee   string `json:"payee,omitempty"`
		Account string `json:"account,omitempty"`
	}

	inputTxns := make([]txnInput, len(txns))
	for i, t := range txns {
		inputTxns[i] = txnInput{FID: t.FID, Description: t.Description, Payee: t.Payee, Amount: t.Amount, Date: t.Date}
	}
	inputRules := make([]ruleInput, len(existingRules))
	for i, r := range existingRules {
		inputRules[i] = ruleInput(r)
	}

	userPayload, err := json.Marshal(map[string]any{
		"transactions":   inputTxns,
		"existing_rules": inputRules,
		"accounts":       accounts,
	})
	if err != nil {
		return nil, fmt.Errorf("ai: marshal suggest-rules payload: %w", err)
	}

	systemPrompt := strings.TrimSpace(`
You are a personal finance assistant helping a user create categorization rules for their transactions.
Rules use Go regex patterns (case-insensitive) matched against the transaction description.
Each rule sets a payee name and/or a category account from the user's account hierarchy.

Given a list of transactions, suggest useful rules that would categorize multiple transactions.
Group similar merchants together into one rule where possible.
Prefer rules that are specific enough to avoid false matches but general enough to be reusable.
Do not suggest rules that duplicate existing_rules patterns.
Only use account names from the provided accounts list.
If no suitable account exists, use an empty string for account.
`)

	content, err := c.chat(ctx, []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: string(userPayload)},
	}, "suggest_rules", suggestRulesSchema)
	if err != nil {
		return nil, err
	}

	var response struct {
		Rules []struct {
			Pattern     string   `json:"pattern"`
			Payee       string   `json:"payee"`
			Account     string   `json:"account"`
			Reasoning   string   `json:"reasoning"`
			ExampleFIDs []string `json:"example_fids"`
		} `json:"rules"`
	}
	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return nil, fmt.Errorf("ai: parse suggest-rules response: %w", err)
	}

	var results []SuggestedRule
	for _, r := range response.Rules {
		if r.Pattern == "" {
			continue
		}
		// Validate the pattern compiles before returning it.
		if _, err := regexp.Compile("(?i)" + r.Pattern); err != nil {
			continue
		}
		results = append(results, SuggestedRule{
			Pattern:     r.Pattern,
			Payee:       r.Payee,
			Account:     r.Account,
			Reasoning:   r.Reasoning,
			ExampleFIDs: r.ExampleFIDs,
		})
	}
	return results, nil
}
