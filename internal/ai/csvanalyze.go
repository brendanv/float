package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// CSVRuleDirective is one hledger CSV rules directive suggested for unusual CSV structures.
type CSVRuleDirective struct {
	Directive string // one or more hledger rules directives (may be multi-line)
	Reasoning string // brief explanation of why this directive is needed
}

var suggestCSVRuleDirectivesSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"directives": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"directive": map[string]any{
						"type":        "string",
						"description": "One or more hledger CSV rules directives (may be multi-line). Must be valid hledger rules syntax.",
					},
					"reasoning": map[string]any{
						"type":        "string",
						"description": "Brief explanation of why this directive is needed given the CSV structure.",
					},
				},
				"required":             []string{"directive", "reasoning"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"directives"},
	"additionalProperties": false,
}

// SuggestCSVRuleDirectives analyzes a CSV sample and suggests hledger rules directives
// for structural parsing edge cases such as multi-currency amounts, commodity purchases,
// or type-column-driven sign conventions. It does NOT suggest account2 categorization
// rules — those belong in float's Rules system.
//
// csvSample is the first ~20 rows of the CSV as raw text.
// detectedFields is the current fields directive content (e.g. "date, description, amount").
// userGuidelines, if non-empty, is prepended to the system prompt as additional instructions.
func (c *Client) SuggestCSVRuleDirectives(ctx context.Context, csvSample string, detectedFields []string, userGuidelines string) ([]CSVRuleDirective, error) {
	type payload struct {
		CSVSample      string   `json:"csv_sample"`
		DetectedFields []string `json:"detected_fields"`
	}

	userPayload, err := json.Marshal(payload{
		CSVSample:      csvSample,
		DetectedFields: detectedFields,
	})
	if err != nil {
		return nil, fmt.Errorf("ai: marshal csv-directives payload: %w", err)
	}

	guidelinesSection := ""
	if userGuidelines != "" {
		guidelinesSection = "## User guidelines\n\n" + userGuidelines + "\n\n"
	}

	systemPrompt := strings.TrimSpace(guidelinesSection + `
You are an expert in hledger CSV rules files helping a user parse bank or brokerage CSV exports.

Your task is to analyze a CSV sample and suggest hledger rules directives that handle STRUCTURAL parsing edge cases. Focus on:

1. **Multi-currency amounts**: if amounts include a currency symbol or currency code (e.g. "$45.00", "€23.50", "100.00 EUR"), suggest how to handle or strip the symbol.
2. **Commodity/investment purchases**: if the CSV has columns for share quantities, ticker symbols, or unit prices (e.g. brokerage account CSVs with BUY/SELL transactions), suggest commodity-related directives like:
   - if %type == BUY
     commodity-amount %shares
     commodity %ticker
3. **Type/transaction-code columns**: if a column distinguishes transaction types (DEBIT, CREDIT, BUY, SELL, FEE, etc.), suggest conditional if-blocks that set the correct sign or account based on the type.
4. **Amount sign conventions**: if the CSV uses unsigned amounts with a separate sign indicator column.
5. **Balance assertions or running totals** that might interfere with parsing.

Do NOT suggest account2 categorization rules (e.g. "if AMAZON → account2 expenses:shopping") — those are managed separately by the user.

If the CSV looks like a standard bank statement (single currency, signed numeric amounts, simple headers), return an empty directives list.

Return only directives that are clearly warranted by the CSV structure. Each directive must be valid hledger CSV rules syntax.
`)

	content, err := c.chat(ctx, []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: string(userPayload)},
	}, "suggest_csv_directives", suggestCSVRuleDirectivesSchema)
	if err != nil {
		return nil, err
	}

	var response struct {
		Directives []struct {
			Directive string `json:"directive"`
			Reasoning string `json:"reasoning"`
		} `json:"directives"`
	}
	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return nil, fmt.Errorf("ai: parse csv-directives response: %w", err)
	}

	var results []CSVRuleDirective
	for _, d := range response.Directives {
		if strings.TrimSpace(d.Directive) == "" {
			continue
		}
		results = append(results, CSVRuleDirective{
			Directive: d.Directive,
			Reasoning: d.Reasoning,
		})
	}
	return results, nil
}
