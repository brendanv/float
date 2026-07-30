package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// RuleDetail is a full existing rule for the AI prompt when analyzing rules
// for duplicates, contradictions, or merge opportunities.
type RuleDetail struct {
	ID           string
	Pattern      string
	Payee        string
	Account      string
	Tags         map[string]string
	Priority     int
	AutoReviewed bool
	MatchAccount string
}

// RuleIssue flags a group of existing rules that are duplicates, contradict
// each other, or could be combined into a single rule.
type RuleIssue struct {
	IssueType   string // "duplicate" | "contradiction" | "combinable"
	RuleIDs     []string
	Explanation string
}

var findRuleIssuesSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"issues": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"issue_type": map[string]any{
						"type":        "string",
						"enum":        []string{"duplicate", "contradiction", "combinable"},
						"description": "duplicate: rules that do the same thing; contradiction: rules whose patterns can match the same transaction but set conflicting payee/account/tags; combinable: distinct rules that always produce the same outcome and could be merged into one rule",
					},
					"rule_ids":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "IDs of the rules involved in this issue (at least 2)"},
					"explanation": map[string]any{"type": "string", "description": "Brief explanation of the issue and how to fix it"},
				},
				"required":             []string{"issue_type", "rule_ids", "explanation"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"issues"},
	"additionalProperties": false,
}

// FindRuleIssues asks the AI to review the full set of categorization rules
// and flag groups of two or more rules that are duplicates, contradict each
// other, or could be combined into a single rule. It does not modify any
// rules; the caller is responsible for presenting the flagged groups to the
// user. userGuidelines, if non-empty, is prepended to the system prompt as
// additional instructions.
func (c *Client) FindRuleIssues(ctx context.Context, existingRules []RuleDetail, userGuidelines string) ([]RuleIssue, error) {
	type ruleInput struct {
		ID           string            `json:"id"`
		Pattern      string            `json:"pattern"`
		Payee        string            `json:"payee,omitempty"`
		Account      string            `json:"account,omitempty"`
		Tags         map[string]string `json:"tags,omitempty"`
		Priority     int               `json:"priority"`
		AutoReviewed bool              `json:"auto_reviewed,omitempty"`
		MatchAccount string            `json:"match_account,omitempty"`
	}

	knownIDs := make(map[string]bool, len(existingRules))
	inputRules := make([]ruleInput, len(existingRules))
	for i, r := range existingRules {
		knownIDs[r.ID] = true
		inputRules[i] = ruleInput(r)
	}

	userPayload, err := json.Marshal(map[string]any{"rules": inputRules})
	if err != nil {
		return nil, fmt.Errorf("ai: marshal find-rule-issues payload: %w", err)
	}

	guidelinesSection := ""
	if userGuidelines != "" {
		guidelinesSection = "## User guidelines\n\n" + userGuidelines + "\n\n"
	}

	systemPrompt := strings.TrimSpace(guidelinesSection + `
You are a personal finance assistant reviewing a user's transaction categorization rules.
Each rule matches transaction descriptions with a Go case-insensitive regex pattern, optionally
scoped to source accounts with match_account, and can set a payee, a category account, and tags.
Lower priority numbers are matched first, and only the first matching rule applies to a given
transaction.

Given the full list of rules, identify groups of two or more rules that have one of these issues:

- "duplicate": rules with effectively the same pattern (allowing for trivial regex variations)
  that set the same payee/account/tags, making one of them redundant.
- "contradiction": rules whose patterns could match some of the same transactions but set
  different payees, accounts, or tags, so the actual result silently depends on priority order.
- "combinable": distinct rules that always set the same payee/account/tags and could be merged
  into a single rule (e.g. with a regex alternation) without changing behavior.

Only report genuine issues. Do not flag rules that are merely similar merchants correctly
categorized differently, and do not flag a rule against itself. For each issue, explain briefly
what's wrong and how to fix it. If there are no issues, return an empty issues array.
`)

	content, err := c.chat(ctx, []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: string(userPayload)},
	}, "find_rule_issues", findRuleIssuesSchema)
	if err != nil {
		return nil, err
	}

	var response struct {
		Issues []struct {
			IssueType   string   `json:"issue_type"`
			RuleIDs     []string `json:"rule_ids"`
			Explanation string   `json:"explanation"`
		} `json:"issues"`
	}
	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return nil, fmt.Errorf("ai: parse find-rule-issues response: %w", err)
	}

	var results []RuleIssue
	for _, iss := range response.Issues {
		var validIDs []string
		for _, id := range iss.RuleIDs {
			if knownIDs[id] {
				validIDs = append(validIDs, id)
			}
		}
		if len(validIDs) < 2 {
			continue // hallucinated or self-referential group; not a meaningful issue
		}
		results = append(results, RuleIssue{
			IssueType:   iss.IssueType,
			RuleIDs:     validIDs,
			Explanation: iss.Explanation,
		})
	}
	return results, nil
}
