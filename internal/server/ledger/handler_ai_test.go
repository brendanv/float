package ledger_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/hledger"
	serverledger "github.com/brendanv/float/internal/server/ledger"
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

// --- FindRuleIssues ---

// mustHandlerWithRules creates a handler backed by a fake hledger client and
// a temp data dir containing the given rules.json content. FindRuleIssues
// never calls hledger, so the fake runner just needs to satisfy client setup.
func mustHandlerWithRules(t *testing.T, rulesJSON string) *serverledger.Handler {
	t.Helper()
	dir := t.TempDir()
	if rulesJSON != "" {
		if err := os.WriteFile(filepath.Join(dir, "rules.json"), []byte(rulesJSON), 0o644); err != nil {
			t.Fatalf("write rules.json: %v", err)
		}
	}
	c, err := hledger.NewWithRunner("hledger", filepath.Join(dir, "main.journal"), versionRunner(t, nil))
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	return serverledger.NewHandler(c, nil, dir, "", nil, nil, nil, nil)
}

const twoRulesJSON = `[
	{"id":"aaa11111","pattern":"AMAZON","payee":"Amazon","account":"expenses:shopping","tags":{},"priority":10,"auto_reviewed":false,"match_account":""},
	{"id":"bbb22222","pattern":"AMAZON","payee":"Amazon","account":"expenses:shopping","tags":{},"priority":20,"auto_reviewed":false,"match_account":""}
]`

func TestFindRuleIssues_MissingAPIKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	h := mustHandlerWithRules(t, twoRulesJSON)

	_, err := h.FindRuleIssues(t.Context(), connect.NewRequest(&floatv1.FindRuleIssuesRequest{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("code = %v, want FailedPrecondition", connect.CodeOf(err))
	}
}

func TestFindRuleIssues_FewerThanTwoRules_ReturnsEmptyWithoutAICall(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	calls := 0
	srv := newAIServer(t, openRouterResp(`{"issues":[]}`), &calls)
	defer srv.Close()

	h := mustHandlerWithRules(t, `[{"id":"aaa11111","pattern":"AMAZON","payee":"","account":"","tags":{},"priority":10,"auto_reviewed":false,"match_account":""}]`)
	h.AIBaseURL = srv.URL

	resp, err := h.FindRuleIssues(t.Context(), connect.NewRequest(&floatv1.FindRuleIssuesRequest{}))
	if err != nil {
		t.Fatalf("FindRuleIssues: %v", err)
	}
	if len(resp.Msg.Issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(resp.Msg.Issues))
	}
	if calls != 0 {
		t.Errorf("expected 0 AI calls with fewer than 2 rules, got %d", calls)
	}
}

func TestFindRuleIssues_ReturnsIssues(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	aiBody := openRouterResp(`{"issues":[
		{"issue_type":"duplicate","rule_ids":["aaa11111","bbb22222"],"explanation":"both match AMAZON and set the same account"}
	]}`)
	srv := newAIServer(t, aiBody, nil)
	defer srv.Close()

	h := mustHandlerWithRules(t, twoRulesJSON)
	h.AIBaseURL = srv.URL

	resp, err := h.FindRuleIssues(t.Context(), connect.NewRequest(&floatv1.FindRuleIssuesRequest{}))
	if err != nil {
		t.Fatalf("FindRuleIssues: %v", err)
	}
	if len(resp.Msg.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(resp.Msg.Issues))
	}
	issue := resp.Msg.Issues[0]
	if issue.IssueType != "duplicate" {
		t.Errorf("IssueType = %q, want duplicate", issue.IssueType)
	}
	if len(issue.RuleIds) != 2 || issue.RuleIds[0] != "aaa11111" || issue.RuleIds[1] != "bbb22222" {
		t.Errorf("RuleIds = %v, want [aaa11111 bbb22222]", issue.RuleIds)
	}
	if issue.Explanation == "" {
		t.Error("Explanation should not be empty")
	}
}

func TestFindRuleIssues_NoRulesFile_ReturnsEmptyWithoutAICall(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	calls := 0
	srv := newAIServer(t, openRouterResp(`{"issues":[]}`), &calls)
	defer srv.Close()

	h := mustHandlerWithRules(t, "") // no rules.json at all
	h.AIBaseURL = srv.URL

	resp, err := h.FindRuleIssues(t.Context(), connect.NewRequest(&floatv1.FindRuleIssuesRequest{}))
	if err != nil {
		t.Fatalf("FindRuleIssues: %v", err)
	}
	if len(resp.Msg.Issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(resp.Msg.Issues))
	}
	if calls != 0 {
		t.Errorf("expected 0 AI calls, got %d", calls)
	}
}
