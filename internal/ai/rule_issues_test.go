package ai_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/brendanv/float/internal/ai"
)

func TestFindRuleIssues_ReturnsIssues(t *testing.T) {
	responseContent := `{"issues":[
		{"issue_type":"duplicate","rule_ids":["aaa11111","bbb22222"],"explanation":"same pattern and account"},
		{"issue_type":"contradiction","rule_ids":["ccc33333","ddd44444"],"explanation":"overlapping patterns set different accounts"}
	]}`

	srv := newTestServer(t, http.StatusOK, openRouterResponse(responseContent), nil)
	defer srv.Close()

	cl := ai.NewClient("test-key", "test-model", ai.WithBaseURL(srv.URL))
	rules := []ai.RuleDetail{
		{ID: "aaa11111", Pattern: "AMAZON", Account: "expenses:shopping"},
		{ID: "bbb22222", Pattern: "AMAZON", Account: "expenses:shopping"},
		{ID: "ccc33333", Pattern: "WHOLE FOODS", Account: "expenses:food:groceries"},
		{ID: "ddd44444", Pattern: "WHOLE", Account: "expenses:other"},
	}

	issues, err := cl.FindRuleIssues(t.Context(), rules, "")
	if err != nil {
		t.Fatalf("FindRuleIssues: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}
	if issues[0].IssueType != "duplicate" {
		t.Errorf("issues[0].IssueType = %q, want duplicate", issues[0].IssueType)
	}
	if len(issues[0].RuleIDs) != 2 || issues[0].RuleIDs[0] != "aaa11111" || issues[0].RuleIDs[1] != "bbb22222" {
		t.Errorf("issues[0].RuleIDs = %v, want [aaa11111 bbb22222]", issues[0].RuleIDs)
	}
	if issues[1].IssueType != "contradiction" {
		t.Errorf("issues[1].IssueType = %q, want contradiction", issues[1].IssueType)
	}
}

func TestFindRuleIssues_FiltersUnknownRuleIDs(t *testing.T) {
	// One issue references a hallucinated ID alongside a real one (should be
	// dropped since fewer than 2 valid IDs remain); another references only
	// known IDs and should survive.
	responseContent := `{"issues":[
		{"issue_type":"duplicate","rule_ids":["aaa11111","doesnotexist"],"explanation":"bad"},
		{"issue_type":"duplicate","rule_ids":["aaa11111","bbb22222"],"explanation":"good"}
	]}`

	srv := newTestServer(t, http.StatusOK, openRouterResponse(responseContent), nil)
	defer srv.Close()

	cl := ai.NewClient("test-key", "test-model", ai.WithBaseURL(srv.URL))
	rules := []ai.RuleDetail{
		{ID: "aaa11111", Pattern: "AMAZON"},
		{ID: "bbb22222", Pattern: "AMAZON"},
	}

	issues, err := cl.FindRuleIssues(t.Context(), rules, "")
	if err != nil {
		t.Fatalf("FindRuleIssues: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue after filtering, got %d", len(issues))
	}
	if issues[0].Explanation != "good" {
		t.Errorf("Explanation = %q, want good", issues[0].Explanation)
	}
}

func TestFindRuleIssues_EmptyIssuesInResponse(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, openRouterResponse(`{"issues":[]}`), nil)
	defer srv.Close()

	cl := ai.NewClient("test-key", "test-model", ai.WithBaseURL(srv.URL))
	issues, err := cl.FindRuleIssues(t.Context(), []ai.RuleDetail{
		{ID: "aaa11111", Pattern: "AMAZON"},
		{ID: "bbb22222", Pattern: "TARGET"},
	}, "")
	if err != nil {
		t.Fatalf("FindRuleIssues: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestFindRuleIssues_APIError(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, openRouterError("rate limit exceeded"), nil)
	defer srv.Close()

	cl := ai.NewClient("test-key", "test-model", ai.WithBaseURL(srv.URL))
	_, err := cl.FindRuleIssues(t.Context(), []ai.RuleDetail{
		{ID: "aaa11111", Pattern: "AMAZON"},
	}, "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFindRuleIssues_SendsRuleDetails(t *testing.T) {
	responseContent := `{"issues":[]}`

	var captured []byte
	srv := newTestServer(t, http.StatusOK, openRouterResponse(responseContent), &captured)
	defer srv.Close()

	cl := ai.NewClient("test-key", "test-model", ai.WithBaseURL(srv.URL))
	rules := []ai.RuleDetail{
		{ID: "aaa11111", Pattern: "AMAZON", Payee: "Amazon", Account: "expenses:shopping", Priority: 5, MatchAccount: "assets:checking"},
	}

	_, err := cl.FindRuleIssues(t.Context(), rules, "")
	if err != nil {
		t.Fatalf("FindRuleIssues: %v", err)
	}

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
	rulesField, ok := payload["rules"].([]any)
	if !ok || len(rulesField) != 1 {
		t.Fatalf("expected 1 rule in payload, got: %v", payload["rules"])
	}
	rule := rulesField[0].(map[string]any)
	if rule["id"] != "aaa11111" {
		t.Errorf("rule id = %v, want aaa11111", rule["id"])
	}
	if rule["match_account"] != "assets:checking" {
		t.Errorf("rule match_account = %v, want assets:checking", rule["match_account"])
	}
}
