package ledger_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
)

// openRouterResp builds a minimal chat completions response wrapping content.
func openRouterResp(content string) []byte {
	b, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"content": content}},
		},
	})
	return b
}

// newAIServer starts an httptest.Server that returns a fixed JSON body and
// records the number of requests received.
func newAIServer(t *testing.T, body []byte, calls *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			*calls++
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
}

// --- SuggestRules ---

func TestSuggestRules_MissingAPIKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	h := mustHandler(t, nil)

	_, err := h.SuggestRules(t.Context(), connect.NewRequest(&floatv1.SuggestRulesRequest{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("code = %v, want FailedPrecondition", connect.CodeOf(err))
	}
}

func TestSuggestRules_NoTransactions_ReturnsEmpty(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	calls := 0
	srv := newAIServer(t, openRouterResp(`{"rules":[]}`), &calls)
	defer srv.Close()

	// hledger returns no transactions for the query.
	h := mustHandler(t, map[string][]byte{
		"print":    []byte("[]"),
		"accounts": []byte(accountsText),
	})
	h.AIBaseURL = srv.URL

	resp, err := h.SuggestRules(t.Context(), connect.NewRequest(&floatv1.SuggestRulesRequest{
		Query: "status:!",
	}))
	if err != nil {
		t.Fatalf("SuggestRules: %v", err)
	}
	if len(resp.Msg.Suggestions) != 0 {
		t.Errorf("expected 0 suggestions, got %d", len(resp.Msg.Suggestions))
	}
	// AI should not be called when there are no transactions.
	if calls != 0 {
		t.Errorf("expected 0 AI calls, got %d", calls)
	}
}

func TestSuggestRules_ReturnsSuggestions(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	aiBody := openRouterResp(`{"rules":[
		{"pattern":"WHOLE FOODS","payee":"Whole Foods","account":"expenses:food:groceries","reasoning":"grocery chain","example_fids":["aa001100"]},
		{"pattern":"NETFLIX","payee":"Netflix","account":"expenses:entertainment","reasoning":"streaming","example_fids":["bb002200"]}
	]}`)

	srv := newAIServer(t, aiBody, nil)
	defer srv.Close()

	h := mustHandler(t, map[string][]byte{
		"print":    []byte(printJSON), // one transaction with FID aa001100
		"accounts": []byte(accountsText),
	})
	h.AIBaseURL = srv.URL

	resp, err := h.SuggestRules(t.Context(), connect.NewRequest(&floatv1.SuggestRulesRequest{}))
	if err != nil {
		t.Fatalf("SuggestRules: %v", err)
	}
	if len(resp.Msg.Suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got %d", len(resp.Msg.Suggestions))
	}
	s := resp.Msg.Suggestions[0]
	if s.Pattern != "WHOLE FOODS" {
		t.Errorf("Pattern = %q, want WHOLE FOODS", s.Pattern)
	}
	if s.Account != "expenses:food:groceries" {
		t.Errorf("Account = %q, want expenses:food:groceries", s.Account)
	}
	if s.Reasoning == "" {
		t.Error("Reasoning should not be empty")
	}
}

func TestSuggestRules_WithFIDs(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	aiBody := openRouterResp(`{"rules":[
		{"pattern":"PAYROLL","payee":"Employer","account":"income:salary","reasoning":"payroll deposit","example_fids":["aa001100"]}
	]}`)
	srv := newAIServer(t, aiBody, nil)
	defer srv.Close()

	h := mustHandler(t, map[string][]byte{
		"print":    []byte(printJSON),
		"accounts": []byte(accountsText),
	})
	h.AIBaseURL = srv.URL

	resp, err := h.SuggestRules(t.Context(), connect.NewRequest(&floatv1.SuggestRulesRequest{
		Fids: []string{"aa001100"},
	}))
	if err != nil {
		t.Fatalf("SuggestRules with FIDs: %v", err)
	}
	if len(resp.Msg.Suggestions) != 1 {
		t.Errorf("expected 1 suggestion, got %d", len(resp.Msg.Suggestions))
	}
}

func TestSuggestRules_DefaultsToUnreviewed(t *testing.T) {
	// When no query is provided, the handler should default to "not:status:*" and
	// succeed (returning empty when no transactions match).
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	srv := newAIServer(t, openRouterResp(`{"rules":[]}`), nil)
	defer srv.Close()

	h := mustHandler(t, map[string][]byte{
		"print":    []byte("[]"),
		"accounts": []byte(accountsText),
	})
	h.AIBaseURL = srv.URL

	_, err := h.SuggestRules(t.Context(), connect.NewRequest(&floatv1.SuggestRulesRequest{}))
	if err != nil {
		t.Fatalf("SuggestRules with empty query: %v", err)
	}
}

// --- TranslateQuery ---

func TestTranslateQuery_MissingAPIKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	h := mustHandler(t, nil)

	_, err := h.TranslateQuery(t.Context(), connect.NewRequest(&floatv1.TranslateQueryRequest{
		Question: "how much did I spend?",
	}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("code = %v, want FailedPrecondition", connect.CodeOf(err))
	}
}

func TestTranslateQuery_EmptyQuestion(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	h := mustHandler(t, nil)

	_, err := h.TranslateQuery(t.Context(), connect.NewRequest(&floatv1.TranslateQueryRequest{
		Question: "",
	}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestTranslateQuery_WhitespaceOnlyQuestion(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	h := mustHandler(t, nil)

	_, err := h.TranslateQuery(t.Context(), connect.NewRequest(&floatv1.TranslateQueryRequest{
		Question: "   ",
	}))
	if err == nil {
		t.Fatal("expected error for whitespace-only question, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestTranslateQuery_ReturnsQueryAndExplanation(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	aiBody := openRouterResp(`{"query":"expenses:food date:lastmonth","explanation":"Food expenses last month"}`)
	srv := newAIServer(t, aiBody, nil)
	defer srv.Close()

	h := mustHandler(t, map[string][]byte{
		"accounts": []byte(accountsText),
	})
	h.AIBaseURL = srv.URL

	resp, err := h.TranslateQuery(t.Context(), connect.NewRequest(&floatv1.TranslateQueryRequest{
		Question: "how much did I spend on food last month?",
	}))
	if err != nil {
		t.Fatalf("TranslateQuery: %v", err)
	}
	if resp.Msg.HledgerQuery != "expenses:food date:lastmonth" {
		t.Errorf("HledgerQuery = %q, want expenses:food date:lastmonth", resp.Msg.HledgerQuery)
	}
	if resp.Msg.Explanation == "" {
		t.Error("Explanation should not be empty")
	}
}

// --- GenerateBankProfileRules ---

const simpleCSV = "Date,Description,Amount\n2026-04-01,AMAZON,-45.00\n2026-04-02,PAYROLL,2000.00\n"

const debitCreditCSV = "Date,Description,Debit,Credit\n04/01/2026,STARBUCKS,5.50,\n04/02/2026,PAYROLL,,2000.00\n"

const twoHeaderCSV = "My Bank Statement\nDate,Description,Amount\n2026-04-01,AMAZON,-45.00\n2026-04-02,PAYROLL,2000.00\n"

func TestGenerateBankProfileRules_MissingCsvData(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	h := mustHandler(t, nil)

	_, err := h.GenerateBankProfileRules(t.Context(), connect.NewRequest(&floatv1.GenerateBankProfileRulesRequest{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestGenerateBankProfileRules_NoAI_StructuralOnly(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	h := mustHandler(t, nil)

	resp, err := h.GenerateBankProfileRules(t.Context(), connect.NewRequest(&floatv1.GenerateBankProfileRulesRequest{
		CsvData: []byte(simpleCSV),
	}))
	if err != nil {
		t.Fatalf("GenerateBankProfileRules: %v", err)
	}
	content := string(resp.Msg.RulesContent)
	for _, want := range []string{"skip 1", "fields date, description, amount", "date-format %Y-%m-%d", "account1 assets:checking", "currency USD"} {
		if !containsStr(content, want) {
			t.Errorf("rules content missing %q\ncontent:\n%s", want, content)
		}
	}
	// Must not include uncommented account2 categorization (comments are fine as examples).
	for _, line := range splitLines(content) {
		if trimmed := strings.TrimSpace(line); !strings.HasPrefix(trimmed, "#") && containsStr(trimmed, "account2") {
			t.Errorf("rules content has unexpected uncommented account2 line %q\ncontent:\n%s", line, content)
		}
	}
}

func TestGenerateBankProfileRules_CustomAccount1(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	h := mustHandler(t, nil)

	resp, err := h.GenerateBankProfileRules(t.Context(), connect.NewRequest(&floatv1.GenerateBankProfileRulesRequest{
		CsvData:  []byte(simpleCSV),
		Account1: "assets:savings",
	}))
	if err != nil {
		t.Fatalf("GenerateBankProfileRules: %v", err)
	}
	content := string(resp.Msg.RulesContent)
	if !containsStr(content, "account1 assets:savings") {
		t.Errorf("expected account1 assets:savings in:\n%s", content)
	}
}

func TestGenerateBankProfileRules_DetectsSkipCount(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	h := mustHandler(t, nil)

	resp, err := h.GenerateBankProfileRules(t.Context(), connect.NewRequest(&floatv1.GenerateBankProfileRulesRequest{
		CsvData: []byte(twoHeaderCSV),
	}))
	if err != nil {
		t.Fatalf("GenerateBankProfileRules: %v", err)
	}
	content := string(resp.Msg.RulesContent)
	if !containsStr(content, "skip 2") {
		t.Errorf("expected skip 2 for two-header CSV, got:\n%s", content)
	}
}

func TestGenerateBankProfileRules_DebitCreditColumns(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	h := mustHandler(t, nil)

	resp, err := h.GenerateBankProfileRules(t.Context(), connect.NewRequest(&floatv1.GenerateBankProfileRulesRequest{
		CsvData: []byte(debitCreditCSV),
	}))
	if err != nil {
		t.Fatalf("GenerateBankProfileRules: %v", err)
	}
	content := string(resp.Msg.RulesContent)
	if !containsStr(content, "amount-out") {
		t.Errorf("expected amount-out field for Debit column, got:\n%s", content)
	}
	if !containsStr(content, "amount-in") {
		t.Errorf("expected amount-in field for Credit column, got:\n%s", content)
	}
}

func TestGenerateBankProfileRules_WithAI_AppendsDirectives(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	aiBody := openRouterResp(`{"directives":[{"directive":"if %type == BUY\n  commodity %ticker","reasoning":"BUY transactions need commodity mapping"}]}`)
	srv := newAIServer(t, aiBody, nil)
	defer srv.Close()

	h := mustHandler(t, map[string][]byte{
		"accounts": []byte(accountsText),
	})
	h.AIBaseURL = srv.URL

	resp, err := h.GenerateBankProfileRules(t.Context(), connect.NewRequest(&floatv1.GenerateBankProfileRulesRequest{
		CsvData: []byte(simpleCSV),
	}))
	if err != nil {
		t.Fatalf("GenerateBankProfileRules: %v", err)
	}
	content := string(resp.Msg.RulesContent)
	if !containsStr(content, "if %type == BUY") {
		t.Errorf("expected AI directive in output, got:\n%s", content)
	}
}

func TestGenerateBankProfileRules_AIFailure_FallsBackToStructural(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	// AI server returns HTTP 500.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	h := mustHandler(t, map[string][]byte{
		"accounts": []byte(accountsText),
	})
	h.AIBaseURL = srv.URL

	resp, err := h.GenerateBankProfileRules(t.Context(), connect.NewRequest(&floatv1.GenerateBankProfileRulesRequest{
		CsvData: []byte(simpleCSV),
	}))
	if err != nil {
		t.Fatalf("expected fallback success, got error: %v", err)
	}
	content := string(resp.Msg.RulesContent)
	if !containsStr(content, "skip 1") {
		t.Errorf("expected structural rules even on AI failure, got:\n%s", content)
	}
}

func containsStr(s, substr string) bool {
	return strings.Contains(s, substr)
}

func splitLines(s string) []string {
	return strings.Split(s, "\n")
}

func TestTranslateQuery_UsesCustomModel(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("OPENROUTER_MODEL", "anthropic/claude-opus-4-7")

	var capturedModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if m, ok := body["model"].(string); ok {
			capturedModel = m
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(openRouterResp(`{"query":"expenses","explanation":"all expenses"}`))
	}))
	defer srv.Close()

	h := mustHandler(t, map[string][]byte{
		"accounts": []byte(accountsText),
	})
	h.AIBaseURL = srv.URL

	_, err := h.TranslateQuery(t.Context(), connect.NewRequest(&floatv1.TranslateQueryRequest{
		Question: "show me expenses",
	}))
	if err != nil {
		t.Fatalf("TranslateQuery: %v", err)
	}
	if capturedModel != "anthropic/claude-opus-4-7" {
		t.Errorf("model sent to API = %q, want anthropic/claude-opus-4-7", capturedModel)
	}
}
