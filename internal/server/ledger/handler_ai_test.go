package ledger_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
