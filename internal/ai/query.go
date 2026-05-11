package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

var planQuerySchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"hledger_args": map[string]any{"type": "string", "description": "Full hledger args including command and filters"},
	},
	"required":             []string{"hledger_args"},
	"additionalProperties": false,
}

var explainResultsSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"answer": map[string]any{"type": "string", "description": "Plain-English answer to the user's question"},
	},
	"required":             []string{"answer"},
	"additionalProperties": false,
}

// PlanQuery converts a natural-language finance question into a full hledger
// command + args string (e.g. "balance expenses:food date:lastmonth").
// Unlike TranslateQuery, this includes the hledger subcommand so the result
// can be executed directly.
func (c *Client) PlanQuery(ctx context.Context, question string, accounts []string, userGuidelines string) (hledgerArgs string, err error) {
	today := time.Now().Format("2006-01-02")

	guidelinesSection := ""
	if userGuidelines != "" {
		guidelinesSection = "## User guidelines\n\n" + userGuidelines + "\n\n"
	}

	systemPrompt := strings.TrimSpace(guidelinesSection + `
You are a personal finance assistant. Given a plain-English finance question,
produce the hledger command and arguments needed to answer it.

Today's date: ` + today + `

## hledger commands

balance (or bal)  — show account balances; use for "how much", "total", "spend" questions
register (or reg) — show transaction-level detail; use for "show me transactions", "list" questions
print             — show full journal entries; use for inspecting individual transactions
incomestatement (or is) — revenue vs expenses summary
balancesheet (or bs)    — assets and liabilities
cashflow (or cf)        — cash flow statement

## hledger query filters (append after command)

Date:
  date:YYYY-MM-DD, date:YYYY-MM-DD..YYYY-MM-DD
  date:today, date:thisweek, date:thismonth, date:lastmonth
  date:thisquarter, date:lastquarter, date:thisyear, date:lastyear

Account:
  expenses:food        — account name substring
  acct:REGEX           — regex match
  ^expenses            — starts with

Amount:
  amt:>N, amt:<N, amt:>=N, amt:<=N

Description/payee:
  desc:REGEX, payee:REGEX

Status:
  status:*  (cleared)   status:!  (pending)

Tags:
  tag:NAME, tag:NAME=VALUE

Negation:
  not:FILTER

Depth (for balance/balancesheet/etc):
  --depth 2

## User's accounts
` + strings.Join(accounts, "\n") + `

Return only the hledger args — no explanation, no markdown, no 'hledger' binary prefix.
Example output: balance expenses:food date:lastmonth
`)

	content, err := c.chat(ctx, []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: question},
	}, "plan_query", planQuerySchema)
	if err != nil {
		return "", err
	}

	var response struct {
		HledgerArgs string `json:"hledger_args"`
	}
	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return "", fmt.Errorf("ai: parse plan-query response: %w", err)
	}
	return response.HledgerArgs, nil
}

// ExplainResults interprets the output of an hledger command and returns a
// plain-English answer to the user's original question.
func (c *Client) ExplainResults(ctx context.Context, question, hledgerArgs, output string, querySuccess bool, userGuidelines string) (answer string, err error) {
	guidelinesSection := ""
	if userGuidelines != "" {
		guidelinesSection = "## User guidelines\n\n" + userGuidelines + "\n\n"
	}

	var outputSection string
	if querySuccess {
		if strings.TrimSpace(output) == "" {
			outputSection = "(no output — the query returned no matching data)"
		} else {
			outputSection = output
		}
	} else {
		outputSection = "(query failed — hledger returned an error)"
	}

	systemPrompt := strings.TrimSpace(guidelinesSection + `
You are a personal finance assistant. The user asked a question about their finances.
An hledger query was run to answer it. Interpret the results and give a direct,
concise plain-English answer. Focus on answering the question — don't explain
what hledger is or describe the command that was run.

If the query returned no data, say so clearly.
If the query failed, explain that the question could not be answered.
Use the same currency symbols that appear in the output.
`)

	userMsg := fmt.Sprintf("Question: %s\n\nCommand run: hledger %s\n\nOutput:\n%s", question, hledgerArgs, outputSection)

	content, err := c.chat(ctx, []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMsg},
	}, "explain_results", explainResultsSchema)
	if err != nil {
		return "", err
	}

	var response struct {
		Answer string `json:"answer"`
	}
	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return "", fmt.Errorf("ai: parse explain-results response: %w", err)
	}
	return response.Answer, nil
}

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
// userGuidelines, if non-empty, is prepended to the system prompt as additional instructions.
func (c *Client) TranslateQuery(ctx context.Context, question string, accounts []string, userGuidelines string) (query, explanation string, err error) {
	today := time.Now().Format("2006-01-02")

	guidelinesSection := ""
	if userGuidelines != "" {
		guidelinesSection = "## User guidelines\n\n" + userGuidelines + "\n\n"
	}

	systemPrompt := strings.TrimSpace(guidelinesSection + `
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
