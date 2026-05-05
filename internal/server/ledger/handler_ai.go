package ledger

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/ai"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/rules"
)

const defaultAIModel = "google/gemini-2.0-flash-001"

// aiClient constructs an ai.Client from environment variables.
// Returns FailedPrecondition if OPENROUTER_API_KEY is not set.
func (h *Handler) aiClient() (*ai.Client, error) {
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			errors.New("OPENROUTER_API_KEY environment variable is not set"))
	}
	model := os.Getenv("OPENROUTER_MODEL")
	if model == "" {
		model = defaultAIModel
	}
	return ai.NewClient(key, model), nil
}

// SuggestRules fetches transactions (by FID list or hledger query), then asks
// the AI to suggest categorization rules that could be added via AddRule.
func (h *Handler) SuggestRules(ctx context.Context, req *connect.Request[floatv1.SuggestRulesRequest]) (*connect.Response[floatv1.SuggestRulesResponse], error) {
	aiCl, err := h.aiClient()
	if err != nil {
		return nil, err
	}

	// Fetch the target transactions.
	var txns []hledger.Transaction
	if len(req.Msg.Fids) > 0 {
		for _, fid := range req.Msg.Fids {
			t, err := h.hl.Transactions(ctx, "code:"+fid)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("fetch transaction %s: %w", fid, err))
			}
			txns = append(txns, t...)
		}
	} else {
		query := req.Msg.Query
		if query == "" {
			// Default: pending (unreviewed) transactions.
			query = "status:!"
		}
		txns, err = h.hl.Transactions(ctx, query)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("fetch transactions: %w", err))
		}
	}

	if len(txns) == 0 {
		return connect.NewResponse(&floatv1.SuggestRulesResponse{}), nil
	}

	// Build TxnSummary list for the AI.
	summaries := make([]ai.TxnSummary, 0, len(txns))
	for _, t := range txns {
		if t.FID == "" {
			continue // skip transactions without a FID
		}
		payee := ""
		if t.Payee != nil {
			payee = *t.Payee
		}
		summaries = append(summaries, ai.TxnSummary{
			FID:         t.FID,
			Description: t.Description,
			Payee:       payee,
			Account:     categoryAccount(t),
			Amount:      primaryAmount(t),
			Date:        t.Date,
		})
	}

	// Provide existing rules as context so the AI avoids duplicates.
	existingRules, err := rules.Load(h.dataDir)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load rules: %w", err))
	}
	ruleSummaries := make([]ai.RuleSummary, len(existingRules))
	for i, r := range existingRules {
		ruleSummaries[i] = ai.RuleSummary{Pattern: r.Pattern, Payee: r.Payee, Account: r.Account}
	}

	// Provide the full account list so the AI can suggest valid account names.
	accounts, err := h.hl.Accounts(ctx, false)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("fetch accounts: %w", err))
	}
	accountNames := make([]string, len(accounts))
	for i, a := range accounts {
		accountNames[i] = a.FullName
	}

	suggestions, err := aiCl.SuggestRules(ctx, summaries, ruleSummaries, accountNames)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("AI suggest rules: %w", err))
	}

	proto := make([]*floatv1.SuggestedRule, 0, len(suggestions))
	for _, s := range suggestions {
		proto = append(proto, &floatv1.SuggestedRule{
			Pattern:     s.Pattern,
			Payee:       s.Payee,
			Account:     s.Account,
			Reasoning:   s.Reasoning,
			ExampleFids: s.ExampleFIDs,
		})
	}
	return connect.NewResponse(&floatv1.SuggestRulesResponse{Suggestions: proto}), nil
}

// TranslateQuery converts a plain-English finance question into a hledger
// query string.
func (h *Handler) TranslateQuery(ctx context.Context, req *connect.Request[floatv1.TranslateQueryRequest]) (*connect.Response[floatv1.TranslateQueryResponse], error) {
	if strings.TrimSpace(req.Msg.Question) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("question must not be empty"))
	}

	aiCl, err := h.aiClient()
	if err != nil {
		return nil, err
	}

	accounts, err := h.hl.Accounts(ctx, false)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("fetch accounts: %w", err))
	}
	accountNames := make([]string, len(accounts))
	for i, a := range accounts {
		accountNames[i] = a.FullName
	}

	query, explanation, err := aiCl.TranslateQuery(ctx, req.Msg.Question, accountNames)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("AI translate query: %w", err))
	}

	return connect.NewResponse(&floatv1.TranslateQueryResponse{
		HledgerQuery: query,
		Explanation:  explanation,
	}), nil
}

// categoryAccount returns the category (non-asset/liability) posting account
// for a 2-posting transaction, or empty string for multi-posting transactions.
func categoryAccount(txn hledger.Transaction) string {
	if len(txn.Postings) != 2 {
		return ""
	}
	for _, p := range txn.Postings {
		lower := strings.ToLower(p.Account)
		if !strings.HasPrefix(lower, "assets") &&
			!strings.HasPrefix(lower, "liabilities") &&
			!strings.HasPrefix(lower, "asset:") &&
			!strings.HasPrefix(lower, "liability:") {
			return p.Account
		}
	}
	return ""
}

// primaryAmount formats the first amount of the first non-zero posting.
func primaryAmount(txn hledger.Transaction) string {
	for _, p := range txn.Postings {
		for _, a := range p.Amounts {
			if a.Quantity.DecimalMantissa != 0 {
				divisor := math.Pow10(a.Quantity.DecimalPlaces)
				val := float64(a.Quantity.DecimalMantissa) / divisor
				return fmt.Sprintf("%.2f %s", val, a.Commodity)
			}
		}
	}
	return ""
}
