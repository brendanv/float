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
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/rules"
	"github.com/brendanv/float/internal/slogctx"
)

const defaultAIModel = "google/gemini-2.0-flash-001"

// effectiveAIModel returns the model to use, in priority order:
//  1. OPENROUTER_MODEL env var (allows runtime override without editing config)
//  2. h.cfg.AI.Model (stored in config.toml, editable from the settings page)
//  3. defaultAIModel
func (h *Handler) effectiveAIModel() string {
	if m := os.Getenv("OPENROUTER_MODEL"); m != "" {
		return m
	}
	cfg := h.loadCfg()
	if cfg != nil && cfg.AI.Model != "" {
		return cfg.AI.Model
	}
	return defaultAIModel
}

// aiClient constructs an ai.Client. Returns FailedPrecondition if
// OPENROUTER_API_KEY is not set.
func (h *Handler) aiClient() (*ai.Client, error) {
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			errors.New("OPENROUTER_API_KEY environment variable is not set"))
	}
	opts := []ai.Option{}
	if h.AIBaseURL != "" {
		opts = append(opts, ai.WithBaseURL(h.AIBaseURL))
	}
	return ai.NewClient(key, h.effectiveAIModel(), opts...), nil
}

// GetAIConfig returns the current AI model and prompt configuration.
func (h *Handler) GetAIConfig(ctx context.Context, req *connect.Request[floatv1.GetAIConfigRequest]) (*connect.Response[floatv1.GetAIConfigResponse], error) {
	cfg := h.loadCfg()
	if cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	return connect.NewResponse(&floatv1.GetAIConfigResponse{
		Model:          cfg.AI.Model,
		EffectiveModel: h.effectiveAIModel(),
		Prompt:         cfg.AI.Prompt,
		Enabled:        os.Getenv("OPENROUTER_API_KEY") != "",
	}), nil
}

// SetAIPrompt updates the AI user guidelines in config.toml. An empty prompt
// clears the guidelines so only the built-in system prompt is used.
func (h *Handler) SetAIPrompt(ctx context.Context, req *connect.Request[floatv1.SetAIPromptRequest]) (*connect.Response[floatv1.SetAIPromptResponse], error) {
	if h.loadCfg() == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	if h.configPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server config path not set"))
	}

	err := h.lock.DoWith(ctx, "set AI prompt", []string{h.configPath}, func() error {
		cur := h.loadCfg()
		newCfg := *cur
		newCfg.AI.Prompt = req.Msg.Prompt
		if err := config.Save(h.configPath, &newCfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		h.cfg.Store(&newCfg)
		return nil
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "set AI prompt failed")
	}

	slogctx.FromContext(ctx).InfoContext(ctx, "updated AI prompt")
	return connect.NewResponse(&floatv1.SetAIPromptResponse{}), nil
}

// SetAIModel updates the AI model in config.toml. An empty model string clears
// the override and reverts to the default.
func (h *Handler) SetAIModel(ctx context.Context, req *connect.Request[floatv1.SetAIModelRequest]) (*connect.Response[floatv1.SetAIModelResponse], error) {
	if h.loadCfg() == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	if h.configPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server config path not set"))
	}

	err := h.lock.DoWith(ctx, "set AI model", []string{h.configPath}, func() error {
		cur := h.loadCfg()
		newCfg := *cur
		newCfg.AI.Model = req.Msg.Model
		if err := config.Save(h.configPath, &newCfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		h.cfg.Store(&newCfg)
		return nil
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "set AI model failed")
	}

	slogctx.FromContext(ctx).InfoContext(ctx, "updated AI model", "model", req.Msg.Model)
	return connect.NewResponse(&floatv1.SetAIModelResponse{}), nil
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
		txns, err = cachedTransactions(ctx, h.cache, h.hl, []string{journal.BuildFIDQuery(req.Msg.Fids)})
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("fetch transactions: %w", err))
		}
	} else {
		query := req.Msg.Query
		if query == "" {
			// Default: unreviewed (not-cleared) transactions.
			query = "not:status:*"
		}
		txns, err = cachedTransactions(ctx, h.cache, h.hl, []string{query})
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
	accounts, err := cachedAccounts(ctx, h.cache, h.hl)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("fetch accounts: %w", err))
	}
	accountNames := make([]string, len(accounts))
	for i, a := range accounts {
		accountNames[i] = a.FullName
	}

	userGuidelines := ""
	if cfg := h.loadCfg(); cfg != nil {
		userGuidelines = cfg.AI.Prompt
	}
	suggestions, err := aiCl.SuggestRules(ctx, summaries, ruleSummaries, accountNames, userGuidelines)
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

// FindRuleIssues asks the AI to review all categorization rules and flag
// groups that are duplicates, contradict each other, or could be combined
// into a single rule. It does not modify any rules.
func (h *Handler) FindRuleIssues(ctx context.Context, req *connect.Request[floatv1.FindRuleIssuesRequest]) (*connect.Response[floatv1.FindRuleIssuesResponse], error) {
	aiCl, err := h.aiClient()
	if err != nil {
		return nil, err
	}

	existingRules, err := rules.Load(h.dataDir)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load rules: %w", err))
	}
	if len(existingRules) < 2 {
		// Fewer than 2 rules can't have duplicates/contradictions; skip the AI call.
		return connect.NewResponse(&floatv1.FindRuleIssuesResponse{}), nil
	}

	details := make([]ai.RuleDetail, len(existingRules))
	for i, r := range existingRules {
		details[i] = ai.RuleDetail{
			ID:           r.ID,
			Pattern:      r.Pattern,
			Payee:        r.Payee,
			Account:      r.Account,
			Tags:         r.Tags,
			Priority:     r.Priority,
			AutoReviewed: r.AutoReviewed,
			MatchAccount: r.MatchAccount,
		}
	}

	userGuidelines := ""
	if cfg := h.loadCfg(); cfg != nil {
		userGuidelines = cfg.AI.Prompt
	}
	issues, err := aiCl.FindRuleIssues(ctx, details, userGuidelines)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("AI find rule issues: %w", err))
	}

	proto := make([]*floatv1.RuleIssueGroup, 0, len(issues))
	for _, iss := range issues {
		proto = append(proto, &floatv1.RuleIssueGroup{
			IssueType:   iss.IssueType,
			RuleIds:     iss.RuleIDs,
			Explanation: iss.Explanation,
		})
	}
	return connect.NewResponse(&floatv1.FindRuleIssuesResponse{Issues: proto}), nil
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

	accounts, err := cachedAccounts(ctx, h.cache, h.hl)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("fetch accounts: %w", err))
	}
	accountNames := make([]string, len(accounts))
	for i, a := range accounts {
		accountNames[i] = a.FullName
	}

	userGuidelines := ""
	if cfg := h.loadCfg(); cfg != nil {
		userGuidelines = cfg.AI.Prompt
	}
	query, explanation, err := aiCl.TranslateQuery(ctx, req.Msg.Question, accountNames, userGuidelines)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("AI translate query: %w", err))
	}

	return connect.NewResponse(&floatv1.TranslateQueryResponse{
		HledgerQuery: query,
		Explanation:  explanation,
	}), nil
}

// AskQuestion translates a plain-English finance question into a hledger
// command, executes it, and returns an AI-generated plain-English answer.
func (h *Handler) AskQuestion(ctx context.Context, req *connect.Request[floatv1.AskQuestionRequest]) (*connect.Response[floatv1.AskQuestionResponse], error) {
	if strings.TrimSpace(req.Msg.Question) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("question must not be empty"))
	}

	aiCl, err := h.aiClient()
	if err != nil {
		return nil, err
	}

	accounts, err := cachedAccounts(ctx, h.cache, h.hl)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("fetch accounts: %w", err))
	}
	accountNames := make([]string, len(accounts))
	for i, a := range accounts {
		accountNames[i] = a.FullName
	}

	userGuidelines := ""
	if cfg := h.loadCfg(); cfg != nil {
		userGuidelines = cfg.AI.Prompt
	}

	hledgerArgs, err := aiCl.PlanQuery(ctx, req.Msg.Question, accountNames, userGuidelines)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("AI plan query: %w", err))
	}

	stdout, _, _, runErr := h.hl.RunQuery(ctx, hledgerArgs)
	querySuccess := runErr == nil

	answer, err := aiCl.ExplainResults(ctx, req.Msg.Question, hledgerArgs, string(stdout), querySuccess, userGuidelines)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("AI explain results: %w", err))
	}

	return connect.NewResponse(&floatv1.AskQuestionResponse{
		HledgerArgs:  hledgerArgs,
		Answer:       answer,
		RawOutput:    string(stdout),
		QuerySuccess: querySuccess,
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
