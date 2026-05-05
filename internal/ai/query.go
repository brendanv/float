package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

var translateQuerySchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"query":       map[string]any{"type": "string", "description": "hledger query string"},
		"explanation": map[string]any{"type": "string", "description": "Plain-English explanation of what the query does"},
	},
	"required":             []string{"query", "explanation"},
	"additionalProperties": false,
}

// TranslateQuery converts a natural-language finance question into a hledger
// query string. accounts is the user's full account list for context.
func (c *Client) TranslateQuery(ctx context.Context, question string, accounts []string) (query, explanation string, err error) {
	today := time.Now().Format("2006-01-02")

	systemPrompt := strings.TrimSpace(`
You are a personal finance assistant that translates plain-English questions into hledger query syntax.

Today's date: ` + today + `

## hledger query syntax reference

Queries are space-separated tokens passed to hledger commands like 'print', 'balance', 'register'.

Date filters:
  date:YYYY-MM-DD          exact date
  date:YYYY-MM-DD..        on or after
  date:..YYYY-MM-DD        on or before
  date:YYYY-MM-DD..YYYY-MM-DD  range
  date:today, date:yesterday
  date:thisweek, date:lastweek
  date:thismonth, date:lastmonth
  date:thisquarter, date:lastquarter
  date:thisyear, date:lastyear

Account filters:
  ACCOUNTNAME              matches account names containing this string
  acct:REGEX               regex match on account name
  ^expenses                starts with "expenses"

Amount filters:
  amt:N                    exactly N
  amt:>N, amt:<N           greater/less than
  amt:>=N, amt:<=N

Description/payee:
  desc:REGEX               regex match on description
  payee:REGEX              regex match on payee

Status:
  status:*  or  *          cleared transactions
  status:!  or  !          pending transactions
  status:   or  unmarked   unmarked transactions

Tags:
  tag:TAGNAME              has tag
  tag:TAGNAME=VALUE        tag with value

Other:
  not:QUERY                negation
  real:1                   only real (non-virtual) postings

## User's accounts
` + strings.Join(accounts, "\n") + `

Translate the user's question into the most appropriate hledger query.
Return only the query tokens (no 'hledger' command prefix).
If the question is ambiguous, prefer the most common interpretation.
`)

	content, err := c.chat(ctx, []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: question},
	}, "translate_query", translateQuerySchema)
	if err != nil {
		return "", "", err
	}

	var response struct {
		Query       string `json:"query"`
		Explanation string `json:"explanation"`
	}
	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return "", "", fmt.Errorf("ai: parse translate-query response: %w", err)
	}
	return response.Query, response.Explanation, nil
}
