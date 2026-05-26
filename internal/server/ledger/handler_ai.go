package ledger

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/ai"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/hledger"
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
	if h.cfg != nil && h.cfg.AI.Model != "" {
		return h.cfg.AI.Model
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
	if h.cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	return connect.NewResponse(&floatv1.GetAIConfigResponse{
		Model:          h.cfg.AI.Model,
		EffectiveModel: h.effectiveAIModel(),
		Prompt:         h.cfg.AI.Prompt,
		Enabled:        os.Getenv("OPENROUTER_API_KEY") != "",
	}), nil
}

// SetAIPrompt updates the AI user guidelines in config.toml. An empty prompt
// clears the guidelines so only the built-in system prompt is used.
func (h *Handler) SetAIPrompt(ctx context.Context, req *connect.Request[floatv1.SetAIPromptRequest]) (*connect.Response[floatv1.SetAIPromptResponse], error) {
	if h.cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	if h.configPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server config path not set"))
	}

	oldPrompt := h.cfg.AI.Prompt
	err := h.lock.Do(ctx, "set AI prompt", func() error {
		h.cfg.AI.Prompt = req.Msg.Prompt
		if err := config.Save(h.configPath, h.cfg); err != nil {
			h.cfg.AI.Prompt = oldPrompt
			return fmt.Errorf("save config: %w", err)
		}
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
	if h.cfg == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server has no config loaded"))
	}
	if h.configPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server config path not set"))
	}

	oldModel := h.cfg.AI.Model
	err := h.lock.Do(ctx, "set AI model", func() error {
		h.cfg.AI.Model = req.Msg.Model
		if err := config.Save(h.configPath, h.cfg); err != nil {
			h.cfg.AI.Model = oldModel
			return fmt.Errorf("save config: %w", err)
		}
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
			// Default: unreviewed (not-cleared) transactions.
			query = "not:status:*"
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

	userGuidelines := ""
	if h.cfg != nil {
		userGuidelines = h.cfg.AI.Prompt
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

	userGuidelines := ""
	if h.cfg != nil {
		userGuidelines = h.cfg.AI.Prompt
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

	accounts, err := h.hl.Accounts(ctx, false)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("fetch accounts: %w", err))
	}
	accountNames := make([]string, len(accounts))
	for i, a := range accounts {
		accountNames[i] = a.FullName
	}

	userGuidelines := ""
	if h.cfg != nil {
		userGuidelines = h.cfg.AI.Prompt
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

// csvAnalysis holds the results of inspecting a CSV file's structure.
type csvAnalysis struct {
	SkipCount   int      // number of header rows before data
	FieldNames  []string // hledger field name per column
	DateFormat  string   // strftime-style date format
	SampleRows  [][]string // first data rows (for AI analysis)
	HeaderRow   []string   // detected header row
}

var (
	reDate        = regexp.MustCompile(`(?i)date|posted|trans.*date`)
	reDescription = regexp.MustCompile(`(?i)desc|narr|memo|detail|merchant|payee|ref`)
	reAmount      = regexp.MustCompile(`(?i)^amount$|^amt$|^value$|transaction amount`)
	reDebit       = regexp.MustCompile(`(?i)debit|withdrawal`)
	reCredit      = regexp.MustCompile(`(?i)credit|deposit`)
	reBalance     = regexp.MustCompile(`(?i)balance|bal\.?$`)

	reDateISO1  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	reDateISO2  = regexp.MustCompile(`^\d{4}/\d{2}/\d{2}$`)
	reDateUS    = regexp.MustCompile(`^\d{1,2}/\d{1,2}/\d{4}$`)
	reDateUS2   = regexp.MustCompile(`^\d{2}-\d{2}-\d{4}$`)
	reDateEU    = regexp.MustCompile(`^\d{2}\.\d{2}\.\d{4}$`)
	reDateCompact = regexp.MustCompile(`^\d{8}$`)
	reNumeric   = regexp.MustCompile(`^-?[\d,]+\.?\d*$`)
)

func guessFieldName(header string) string {
	h := strings.TrimSpace(header)
	switch {
	case reDate.MatchString(h):
		return "date"
	case reDescription.MatchString(h):
		return "description"
	case reAmount.MatchString(h):
		return "amount"
	case reDebit.MatchString(h):
		return "amount-out"
	case reCredit.MatchString(h):
		return "amount-in"
	case reBalance.MatchString(h):
		return "balance"
	default:
		return "_"
	}
}

func detectDateFormat(sample string) string {
	s := strings.TrimSpace(sample)
	switch {
	case reDateISO1.MatchString(s):
		return "%Y-%m-%d"
	case reDateISO2.MatchString(s):
		return "%Y/%m/%d"
	case reDateUS.MatchString(s):
		return "%m/%d/%Y"
	case reDateUS2.MatchString(s):
		return "%m-%d-%Y"
	case reDateEU.MatchString(s):
		return "%d.%m.%Y"
	case reDateCompact.MatchString(s):
		return "%Y%m%d"
	default:
		return "%Y-%m-%d"
	}
}

func looksLikeDateValue(s string) bool {
	s = strings.TrimSpace(s)
	return reDateISO1.MatchString(s) || reDateISO2.MatchString(s) ||
		reDateUS.MatchString(s) || reDateUS2.MatchString(s) ||
		reDateEU.MatchString(s) || reDateCompact.MatchString(s)
}

func looksLikeNumericValue(s string) bool {
	s = strings.TrimSpace(s)
	// Strip leading currency symbol if present
	s = strings.TrimLeft(s, "$€£¥-+ ")
	return reNumeric.MatchString(s) && s != ""
}

// analyzeCSV parses raw CSV bytes and extracts structural information.
func analyzeCSV(data []byte) (csvAnalysis, error) {
	// Strip UTF-8 BOM if present.
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	r := csv.NewReader(bytes.NewReader(data))
	r.LazyQuotes = true
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1 // allow rows with different field counts (e.g. bank headers)

	allRows, err := r.ReadAll()
	if err != nil {
		return csvAnalysis{}, fmt.Errorf("parse CSV: %w", err)
	}
	if len(allRows) == 0 {
		return csvAnalysis{}, errors.New("CSV file is empty")
	}

	// Determine skip count: number of rows before the first row where at least
	// one cell looks like a date AND at least one looks like a number.
	skipCount := 0
	for i, row := range allRows {
		hasDate := false
		hasNum := false
		for _, cell := range row {
			if looksLikeDateValue(cell) {
				hasDate = true
			}
			if looksLikeNumericValue(cell) {
				hasNum = true
			}
		}
		if hasDate && hasNum {
			skipCount = i
			break
		}
		// If we never find a data row, assume single header row.
		if i == len(allRows)-1 {
			skipCount = 1
		}
	}
	if skipCount < 1 {
		skipCount = 1
	}

	// Use the row just before the first data row as the header.
	headerIdx := skipCount - 1
	if headerIdx < 0 || headerIdx >= len(allRows) {
		headerIdx = 0
	}
	headerRow := allRows[headerIdx]

	// Map each header to a hledger field name.
	fieldNames := make([]string, len(headerRow))
	for i, h := range headerRow {
		fieldNames[i] = guessFieldName(h)
	}

	// Detect date format from the first data row.
	dateFormat := "%Y-%m-%d"
	dateColIdx := -1
	for i, f := range fieldNames {
		if f == "date" {
			dateColIdx = i
			break
		}
	}
	if dateColIdx >= 0 && skipCount < len(allRows) {
		firstDataRow := allRows[skipCount]
		if dateColIdx < len(firstDataRow) {
			dateFormat = detectDateFormat(firstDataRow[dateColIdx])
		}
	}

	// Collect sample data rows for AI analysis (up to 20).
	end := skipCount + 20
	if end > len(allRows) {
		end = len(allRows)
	}
	sampleRows := allRows[skipCount:end]

	return csvAnalysis{
		SkipCount:  skipCount,
		FieldNames: fieldNames,
		DateFormat: dateFormat,
		SampleRows: sampleRows,
		HeaderRow:  headerRow,
	}, nil
}

// buildStructuralRules generates the structural portion of a hledger CSV rules file.
func buildStructuralRules(info csvAnalysis, account1 string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "skip %d\n", info.SkipCount)
	fmt.Fprintf(&sb, "fields %s\n", strings.Join(info.FieldNames, ", "))
	fmt.Fprintf(&sb, "date-format %s\n", info.DateFormat)
	fmt.Fprintf(&sb, "account1 %s\n", account1)
	sb.WriteString("currency USD\n")
	return sb.String()
}

// GenerateBankProfileRules analyzes a CSV file and returns a draft hledger .rules file.
// It detects skip count, column types, and date format. If AI is configured, it also
// suggests directives for structural edge cases (multi-currency, commodity purchases, etc.).
func (h *Handler) GenerateBankProfileRules(
	ctx context.Context,
	req *connect.Request[floatv1.GenerateBankProfileRulesRequest],
) (*connect.Response[floatv1.GenerateBankProfileRulesResponse], error) {
	if len(req.Msg.CsvData) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("csv_data is required"))
	}

	info, err := analyzeCSV(req.Msg.CsvData)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("analyze CSV: %w", err))
	}

	account1 := strings.TrimSpace(req.Msg.Account1)
	if account1 == "" {
		account1 = "assets:checking"
	}

	rulesContent := buildStructuralRules(info, account1)

	// Optional AI analysis for structural edge cases.
	aiCl, aiErr := h.aiClient()
	if aiErr == nil && len(info.SampleRows) > 0 {
		// Reconstruct a CSV sample string for the AI.
		var csvBuf strings.Builder
		csvBuf.WriteString(strings.Join(info.HeaderRow, ",") + "\n")
		for _, row := range info.SampleRows {
			csvBuf.WriteString(strings.Join(row, ",") + "\n")
		}

		userGuidelines := ""
		if h.cfg != nil {
			userGuidelines = h.cfg.AI.Prompt
		}

		directives, err := aiCl.SuggestCSVRuleDirectives(ctx, csvBuf.String(), info.FieldNames, userGuidelines)
		if err != nil {
			slogctx.FromContext(ctx).WarnContext(ctx, "AI CSV directive suggestion failed, continuing without AI", "err", err)
		} else if len(directives) > 0 {
			rulesContent += "\n"
			for _, d := range directives {
				if d.Reasoning != "" {
					rulesContent += "# " + d.Reasoning + "\n"
				}
				rulesContent += d.Directive + "\n\n"
			}
		}
	}

	rulesContent += "\n# Add conditional rules if needed:\n# if PATTERN\n#   account2 expenses:category\n"

	return connect.NewResponse(&floatv1.GenerateBankProfileRulesResponse{
		RulesContent: []byte(rulesContent),
	}), nil
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
