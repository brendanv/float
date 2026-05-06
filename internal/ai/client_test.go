package ai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/brendanv/float/internal/ai"
)

// openRouterResponse builds a minimal chat completions response with the given content.
func openRouterResponse(content string) []byte {
	b, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"content": content}},
		},
	})
	return b
}

// openRouterError builds an error response body.
func openRouterError(msg string) []byte {
	b, _ := json.Marshal(map[string]any{
		"error": map[string]any{"message": msg},
	})
	return b
}

// newTestServer returns an httptest.Server that responds once with resp, and
// captures the request body in *reqBody for inspection.
func newTestServer(t *testing.T, status int, resp []byte, reqBody *[]byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reqBody != nil {
			*reqBody, _ = io.ReadAll(r.Body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(resp)
	}))
}

func TestSuggestRules_ReturnsSuggestions(t *testing.T) {
	responseContent := `{"rules":[
		{"pattern":"WHOLE FOODS","payee":"Whole Foods","account":"expenses:food:groceries","reasoning":"grocery store","example_fids":["abc12345"]},
		{"pattern":"NETFLIX","payee":"Netflix","account":"expenses:entertainment","reasoning":"streaming service","example_fids":["def67890"]}
	]}`

	srv := newTestServer(t, http.StatusOK, openRouterResponse(responseContent), nil)
	defer srv.Close()

	cl := ai.NewClient("test-key", "test-model", ai.WithBaseURL(srv.URL))

	txns := []ai.TxnSummary{
		{FID: "abc12345", Description: "WHOLE FOODS MARKET #123", Amount: "-45.67 $", Date: "2026-01-05"},
		{FID: "def67890", Description: "NETFLIX.COM", Amount: "-15.99 $", Date: "2026-01-06"},
	}
	suggestions, err := cl.SuggestRules(t.Context(), txns, nil, []string{"expenses:food:groceries", "expenses:entertainment"})
	if err != nil {
		t.Fatalf("SuggestRules: %v", err)
	}

	if len(suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got %d", len(suggestions))
	}
	if suggestions[0].Pattern != "WHOLE FOODS" {
		t.Errorf("suggestion[0].Pattern = %q, want %q", suggestions[0].Pattern, "WHOLE FOODS")
	}
	if suggestions[0].Account != "expenses:food:groceries" {
		t.Errorf("suggestion[0].Account = %q, want %q", suggestions[0].Account, "expenses:food:groceries")
	}
	if len(suggestions[0].ExampleFIDs) != 1 || suggestions[0].ExampleFIDs[0] != "abc12345" {
		t.Errorf("suggestion[0].ExampleFIDs = %v, want [abc12345]", suggestions[0].ExampleFIDs)
	}
}

func TestSuggestRules_FiltersInvalidRegex(t *testing.T) {
	// One valid pattern, one invalid regex — the invalid one should be dropped.
	responseContent := `{"rules":[
		{"pattern":"AMAZON","payee":"Amazon","account":"expenses:shopping","reasoning":"online retail","example_fids":["aaa11111"]},
		{"pattern":"[invalid","payee":"Bad","account":"expenses:other","reasoning":"bad regex","example_fids":["bbb22222"]}
	]}`

	srv := newTestServer(t, http.StatusOK, openRouterResponse(responseContent), nil)
	defer srv.Close()

	cl := ai.NewClient("test-key", "test-model", ai.WithBaseURL(srv.URL))
	suggestions, err := cl.SuggestRules(t.Context(), []ai.TxnSummary{
		{FID: "aaa11111", Description: "AMAZON.COM", Amount: "-29.99 $", Date: "2026-01-01"},
	}, nil, nil)
	if err != nil {
		t.Fatalf("SuggestRules: %v", err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion (invalid regex filtered), got %d", len(suggestions))
	}
	if suggestions[0].Pattern != "AMAZON" {
		t.Errorf("expected AMAZON pattern, got %q", suggestions[0].Pattern)
	}
}

func TestSuggestRules_EmptyRulesInResponse(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, openRouterResponse(`{"rules":[]}`), nil)
	defer srv.Close()

	cl := ai.NewClient("test-key", "test-model", ai.WithBaseURL(srv.URL))
	suggestions, err := cl.SuggestRules(t.Context(), []ai.TxnSummary{
		{FID: "aaa11111", Description: "UNKNOWN MERCHANT", Amount: "-10.00 $", Date: "2026-01-01"},
	}, nil, nil)
	if err != nil {
		t.Fatalf("SuggestRules: %v", err)
	}
	if len(suggestions) != 0 {
		t.Errorf("expected 0 suggestions, got %d", len(suggestions))
	}
}

func TestSuggestRules_APIError(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, openRouterError("rate limit exceeded"), nil)
	defer srv.Close()

	cl := ai.NewClient("test-key", "test-model", ai.WithBaseURL(srv.URL))
	_, err := cl.SuggestRules(t.Context(), []ai.TxnSummary{
		{FID: "aaa11111", Description: "SOME MERCHANT", Amount: "-10.00 $", Date: "2026-01-01"},
	}, nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSuggestRules_SendsTransactionsAndAccounts(t *testing.T) {
	responseContent := `{"rules":[{"pattern":"STARBUCKS","payee":"Starbucks","account":"expenses:food:coffee","reasoning":"coffee","example_fids":["aaa11111"]}]}`

	var captured []byte
	srv := newTestServer(t, http.StatusOK, openRouterResponse(responseContent), &captured)
	defer srv.Close()

	cl := ai.NewClient("test-key", "test-model", ai.WithBaseURL(srv.URL))
	txns := []ai.TxnSummary{{FID: "aaa11111", Description: "STARBUCKS #1234", Amount: "-5.50 $", Date: "2026-01-10"}}
	existingRules := []ai.RuleSummary{{Pattern: "AMAZON", Payee: "Amazon", Account: "expenses:shopping"}}
	accounts := []string{"expenses:food:coffee", "expenses:shopping"}

	_, err := cl.SuggestRules(t.Context(), txns, existingRules, accounts)
	if err != nil {
		t.Fatalf("SuggestRules: %v", err)
	}

	// Verify the request body contains the transactions, existing rules, and accounts.
	var body map[string]any
	if err := json.Unmarshal(captured, &body); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	messages, ok := body["messages"].([]any)
	if !ok || len(messages) < 2 {
		t.Fatalf("expected at least 2 messages in request, got: %v", body["messages"])
	}
	userMsg := messages[1].(map[string]any)["content"].(string)
	var payload map[string]any
	if err := json.Unmarshal([]byte(userMsg), &payload); err != nil {
		t.Fatalf("unmarshal user message payload: %v", err)
	}
	if _, ok := payload["transactions"]; !ok {
		t.Error("request payload missing 'transactions' field")
	}
	if _, ok := payload["existing_rules"]; !ok {
		t.Error("request payload missing 'existing_rules' field")
	}
	if _, ok := payload["accounts"]; !ok {
		t.Error("request payload missing 'accounts' field")
	}
}

func TestTranslateQuery_ReturnsQueryAndExplanation(t *testing.T) {
	responseContent := `{"query":"expenses:food date:lastmonth","explanation":"All food expenses in the previous calendar month"}`

	srv := newTestServer(t, http.StatusOK, openRouterResponse(responseContent), nil)
	defer srv.Close()

	cl := ai.NewClient("test-key", "test-model", ai.WithBaseURL(srv.URL))
	query, explanation, err := cl.TranslateQuery(t.Context(), "how much did I spend on food last month?", []string{"expenses:food", "assets:checking"})
	if err != nil {
		t.Fatalf("TranslateQuery: %v", err)
	}
	if query != "expenses:food date:lastmonth" {
		t.Errorf("query = %q, want %q", query, "expenses:food date:lastmonth")
	}
	if explanation == "" {
		t.Error("explanation should not be empty")
	}
}

func TestTranslateQuery_APIError(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, openRouterError("invalid API key"), nil)
	defer srv.Close()

	cl := ai.NewClient("test-key", "test-model", ai.WithBaseURL(srv.URL))
	_, _, err := cl.TranslateQuery(t.Context(), "show me expenses", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestTranslateQuery_SendsAccountsAndQuestion(t *testing.T) {
	responseContent := `{"query":"expenses:rent","explanation":"Rent payments"}`
	var captured []byte
	srv := newTestServer(t, http.StatusOK, openRouterResponse(responseContent), &captured)
	defer srv.Close()

	cl := ai.NewClient("test-key", "test-model", ai.WithBaseURL(srv.URL))
	accounts := []string{"expenses:rent", "assets:checking"}
	_, _, err := cl.TranslateQuery(t.Context(), "show me my rent payments", accounts)
	if err != nil {
		t.Fatalf("TranslateQuery: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(captured, &body); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	messages := body["messages"].([]any)
	// System message should mention today's date and account list.
	sysContent := messages[0].(map[string]any)["content"].(string)
	if len(sysContent) == 0 {
		t.Error("system message should not be empty")
	}
	// User message should be the question.
	userContent := messages[1].(map[string]any)["content"].(string)
	if userContent != "show me my rent payments" {
		t.Errorf("user message = %q, want question", userContent)
	}
}

func TestClient_UsesAuthorizationHeader(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(openRouterResponse(`{"rules":[]}`))
	}))
	defer srv.Close()

	cl := ai.NewClient("my-secret-key", "test-model", ai.WithBaseURL(srv.URL))
	_, _ = cl.SuggestRules(context.Background(), []ai.TxnSummary{
		{FID: "x", Description: "TEST", Amount: "-1.00 $", Date: "2026-01-01"},
	}, nil, nil)

	if capturedAuth != "Bearer my-secret-key" {
		t.Errorf("Authorization = %q, want %q", capturedAuth, "Bearer my-secret-key")
	}
}

func TestClient_SendsStructuredOutputSchema(t *testing.T) {
	var captured []byte
	srv := newTestServer(t, http.StatusOK, openRouterResponse(`{"query":"expenses","explanation":"all expenses"}`), &captured)
	defer srv.Close()

	cl := ai.NewClient("test-key", "test-model", ai.WithBaseURL(srv.URL))
	_, _, _ = cl.TranslateQuery(t.Context(), "show expenses", nil)

	var body map[string]any
	if err := json.Unmarshal(captured, &body); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	rf, ok := body["response_format"].(map[string]any)
	if !ok {
		t.Fatal("request body missing response_format")
	}
	if rf["type"] != "json_schema" {
		t.Errorf("response_format.type = %v, want json_schema", rf["type"])
	}
	js, ok := rf["json_schema"].(map[string]any)
	if !ok {
		t.Fatal("response_format missing json_schema")
	}
	if js["strict"] != true {
		t.Errorf("json_schema.strict = %v, want true", js["strict"])
	}
}
