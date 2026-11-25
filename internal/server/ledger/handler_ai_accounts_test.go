package ledger_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/hledger"
	serverledger "github.com/brendanv/float/internal/server/ledger"
	"github.com/brendanv/float/internal/suggestions"
	"github.com/brendanv/float/internal/txlock"
)

// uu000001/uu000002 are new, unreviewed and uncategorized; uu000003 is
// uncategorized but already reviewed, so it is left alone.
const suggestAccountsJournal = `account assets:checking
account liabilities:card
account expenses:food:groceries
account expenses:entertainment
account expenses:unknown
account expenses:old  ; float-closed:

2026-01-03 * (hh000001) Whole Foods | weekly shop
    expenses:food:groceries   42.10 USD
    liabilities:card

2026-01-04 * (hh000002) NETFLIX.COM
    expenses:entertainment   15.49 USD
    liabilities:card

2026-01-05 * (uu000003) Cash withdrawal
    expenses:unknown   20.00 USD
    assets:checking

2026-02-03 (uu000001) Whole Foods | groceries
    expenses:unknown   55.00 USD
    liabilities:card

2026-02-04 (uu000002) NETFLIX.COM 866-579-7172
    expenses:unknown   15.49 USD
    liabilities:card
`

// suggestAIResponse suggests groceries for uu000001 and an invented account
// for uu000002 (dropped), plus an entry for a fid it wasn't asked about.
const suggestAIResponse = `{"suggestions":[
	{"fid":"uu000001","account":"expenses:food:groceries","confidence":"high","reasoning":"past Whole Foods"},
	{"fid":"uu000002","account":"expenses:streaming","confidence":"medium","reasoning":"invented account"},
	{"fid":"hh000002","account":"expenses:entertainment","confidence":"high","reasoning":"not a target"}
]}`

// newSuggestAccountsHandler writes the fixture journal and returns a handler
// running real hledger over it, plus its data dir.
func newSuggestAccountsHandler(t *testing.T, cfg *config.Config) (*serverledger.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte(suggestAccountsJournal), 0o644); err != nil {
		t.Fatal(err)
	}
	return mustHandlerWithConfig(t, dir, cfg), dir
}

// capturingAIServer records each request body and replies with content, or
// with a 500-style API error when content is empty.
type capturingAIServer struct {
	*httptest.Server
	requests []map[string]any
}

func newCapturingAIServer(t *testing.T, content string) *capturingAIServer {
	t.Helper()
	s := &capturingAIServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.requests = append(s.requests, body)
		w.Header().Set("Content-Type", "application/json")
		if content == "" {
			_, _ = w.Write([]byte(`{"error":{"message":"upstream unavailable"}}`))
			return
		}
		_, _ = w.Write(openRouterResp(content))
	}))
	t.Cleanup(s.Close)
	return s
}

// userPayload decodes the user message of request i.
func (s *capturingAIServer) userPayload(t *testing.T, i int) map[string]any {
	t.Helper()
	msgs := s.requests[i]["messages"].([]any)
	content := msgs[len(msgs)-1].(map[string]any)["content"].(string)
	var payload map[string]any
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		t.Fatalf("decode user payload: %v", err)
	}
	return payload
}

// suggestionsByFID lists transactions and returns their suggested accounts.
func suggestionsByFID(t *testing.T, h *serverledger.Handler) map[string]string {
	t.Helper()
	resp, err := h.ListTransactions(t.Context(), connect.NewRequest(&floatv1.ListTransactionsRequest{}))
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	out := map[string]string{}
	for _, tx := range resp.Msg.Transactions {
		if tx.SuggestedAccount != nil {
			out[tx.Fid] = tx.SuggestedAccount.Account
		}
	}
	return out
}

func TestSuggestPendingAccounts_StoresAndServesSuggestions(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("OPENROUTER_MODEL", "")
	srv := newCapturingAIServer(t, suggestAIResponse)
	h, dir := newSuggestAccountsHandler(t, &config.Config{AI: config.AIConfig{Model: "main/model", DecisionModel: "fast/model"}})
	h.AIBaseURL = srv.URL

	if got := suggestionsByFID(t, h); len(got) != 0 {
		t.Fatalf("suggestions before any pass = %v", got)
	}
	if err := serverledger.ExportedSuggestPendingAccounts(h, t.Context()); err != nil {
		t.Fatalf("suggest pass: %v", err)
	}

	if len(srv.requests) != 1 {
		t.Fatalf("AI calls = %d, want 1", len(srv.requests))
	}
	if got := srv.requests[0]["model"]; got != "fast/model" {
		t.Errorf("model = %v, want the decision model fast/model", got)
	}
	payload := srv.userPayload(t, 0)
	var accounts []string
	for _, a := range payload["accounts"].([]any) {
		accounts = append(accounts, a.(string))
	}
	for _, bad := range []string{"expenses:unknown", "expenses:old"} {
		if slices.Contains(accounts, bad) {
			t.Errorf("accounts offered to AI include %q: %v", bad, accounts)
		}
	}
	txns := payload["transactions"].([]any)
	var fids []string
	for _, tx := range txns {
		fids = append(fids, tx.(map[string]any)["fid"].(string))
	}
	if !slices.Equal(fids, []string{"uu000001", "uu000002"}) {
		t.Errorf("sent fids = %v, want the two unreviewed unknown transactions", fids)
	}
	wf := txns[0].(map[string]any)
	if wf["amount"] != "55.00 USD" || wf["paid_from_or_to"] != "liabilities:card" {
		t.Errorf("whole foods input = %v", wf)
	}
	if ex, _ := wf["similar_past_transactions"].([]any); len(ex) != 1 || ex[0].(map[string]any)["account"] != "expenses:food:groceries" {
		t.Errorf("whole foods examples = %v, want the past groceries transaction", ex)
	}
	nf := txns[1].(map[string]any)
	if ex, _ := nf["similar_past_transactions"].([]any); len(ex) != 1 || ex[0].(map[string]any)["account"] != "expenses:entertainment" {
		t.Errorf("netflix examples = %v, want the past entertainment transaction", ex)
	}

	// Served on the list and the register; the invented account is dropped.
	if got := suggestionsByFID(t, h); len(got) != 1 || got["uu000001"] != "expenses:food:groceries" {
		t.Errorf("listed suggestions = %v, want only uu000001 → groceries", got)
	}
	reg, err := h.GetAccountRegister(t.Context(), connect.NewRequest(&floatv1.GetAccountRegisterRequest{Account: "liabilities:card"}))
	if err != nil {
		t.Fatalf("GetAccountRegister: %v", err)
	}
	var regHit bool
	for _, r := range reg.Msg.Rows {
		if r.Fid == "uu000001" {
			regHit = r.SuggestedAccount.GetAccount() == "expenses:food:groceries" && r.SuggestedAccount.GetConfidence() == "high"
		} else if r.SuggestedAccount != nil {
			t.Errorf("register row %s has suggestion %v", r.Fid, r.SuggestedAccount)
		}
	}
	if !regHit {
		t.Error("register row uu000001 missing its suggestion")
	}

	// Both processed fids are stored (uu000002 as "no suggestion"); the
	// journal is untouched and a fresh handler reloads the store.
	store, err := suggestions.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(store) != 2 || store["uu000001"].Model != "fast/model" || store["uu000002"].Account != "" {
		t.Errorf("stored = %+v", store)
	}
	journal, err := os.ReadFile(filepath.Join(dir, "main.journal"))
	if err != nil {
		t.Fatal(err)
	}
	if string(journal) != suggestAccountsJournal {
		t.Errorf("journal changed:\n%s", journal)
	}
	reloaded := mustHandlerWithConfig(t, dir, &config.Config{})
	if got := suggestionsByFID(t, reloaded); got["uu000001"] != "expenses:food:groceries" {
		t.Errorf("reloaded handler suggestions = %v", got)
	}

	// Nothing new: a second pass doesn't call the AI again.
	if err := serverledger.ExportedSuggestPendingAccounts(h, t.Context()); err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if len(srv.requests) != 1 {
		t.Errorf("AI calls after second pass = %d, want still 1", len(srv.requests))
	}
}

func TestSuggestPendingAccounts_AppliedSuggestionIsHiddenAndPruned(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	srv := newCapturingAIServer(t, suggestAIResponse)
	h, dir := newSuggestAccountsHandler(t, &config.Config{})
	h.AIBaseURL = srv.URL

	if err := serverledger.ExportedSuggestPendingAccounts(h, t.Context()); err != nil {
		t.Fatalf("suggest pass: %v", err)
	}
	if _, err := h.BulkEditTransactions(t.Context(), connect.NewRequest(&floatv1.BulkEditTransactionsRequest{
		Fids: []string{"uu000001"},
		Operations: []*floatv1.BulkEditOperation{{Operation: &floatv1.BulkEditOperation_UpdateUnknownAccount{
			UpdateUnknownAccount: &floatv1.UpdateUnknownAccountOperation{Account: "expenses:food:groceries"},
		}}},
	})); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got := suggestionsByFID(t, h); len(got) != 0 {
		t.Errorf("suggestions after applying = %v, want none", got)
	}

	if err := serverledger.ExportedSuggestPendingAccounts(h, t.Context()); err != nil {
		t.Fatalf("prune pass: %v", err)
	}
	if len(srv.requests) != 1 {
		t.Errorf("AI calls = %d, want 1 (pruning needs no AI)", len(srv.requests))
	}
	store, err := suggestions.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store["uu000001"]; ok || len(store) != 1 {
		t.Errorf("stored after prune = %v, want only uu000002", store)
	}
}

func TestSuggestPendingAccounts_AIFailureRetriesNextPass(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	failing := newCapturingAIServer(t, "")
	h, dir := newSuggestAccountsHandler(t, &config.Config{})
	h.AIBaseURL = failing.URL

	err := serverledger.ExportedSuggestPendingAccounts(h, t.Context())
	if err == nil || !strings.Contains(err.Error(), "upstream unavailable") {
		t.Fatalf("pass error = %v, want the API error", err)
	}
	if _, statErr := os.Stat(suggestions.FilePath(dir)); !os.IsNotExist(statErr) {
		t.Errorf("store written after a failed pass: %v", statErr)
	}

	working := newCapturingAIServer(t, suggestAIResponse)
	h.AIBaseURL = working.URL
	if err := serverledger.ExportedSuggestPendingAccounts(h, t.Context()); err != nil {
		t.Fatalf("retry pass: %v", err)
	}
	if got := suggestionsByFID(t, h); got["uu000001"] != "expenses:food:groceries" {
		t.Errorf("suggestions after retry = %v", got)
	}
}

func TestSuggestPendingAccounts_NoAPIKeyIsNoop(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	srv := newCapturingAIServer(t, suggestAIResponse)
	h, dir := newSuggestAccountsHandler(t, &config.Config{})
	h.AIBaseURL = srv.URL

	if err := serverledger.ExportedSuggestPendingAccounts(h, t.Context()); err != nil {
		t.Fatalf("pass: %v", err)
	}
	if len(srv.requests) != 0 {
		t.Errorf("AI calls = %d, want 0", len(srv.requests))
	}
	if _, err := os.Stat(suggestions.FilePath(dir)); !os.IsNotExist(err) {
		t.Errorf("store written without an API key: %v", err)
	}
}

func TestSetAIDecisionModel(t *testing.T) {
	t.Setenv("OPENROUTER_MODEL", "")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	h := mustHandlerWithConfig(t, dir, &config.Config{AI: config.AIConfig{Model: "main/model"}})

	get := func() *floatv1.GetAIConfigResponse {
		t.Helper()
		resp, err := h.GetAIConfig(t.Context(), connect.NewRequest(&floatv1.GetAIConfigRequest{}))
		if err != nil {
			t.Fatal(err)
		}
		return resp.Msg
	}
	if c := get(); c.DecisionModel != "" || c.EffectiveDecisionModel != "main/model" {
		t.Errorf("before: decision=%q effective=%q", c.DecisionModel, c.EffectiveDecisionModel)
	}

	if _, err := h.SetAIDecisionModel(t.Context(), connect.NewRequest(&floatv1.SetAIDecisionModelRequest{Model: " fast/model "})); err != nil {
		t.Fatalf("SetAIDecisionModel: %v", err)
	}
	if c := get(); c.DecisionModel != "fast/model" || c.EffectiveDecisionModel != "fast/model" {
		t.Errorf("after: decision=%q effective=%q", c.DecisionModel, c.EffectiveDecisionModel)
	}
	saved, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), `decision_model = "fast/model"`) {
		t.Errorf("config.toml missing decision_model:\n%s", saved)
	}

	if _, err := h.SetAIDecisionModel(t.Context(), connect.NewRequest(&floatv1.SetAIDecisionModelRequest{})); err != nil {
		t.Fatalf("clear SetAIDecisionModel: %v", err)
	}
	if c := get(); c.EffectiveDecisionModel != "main/model" {
		t.Errorf("after clear: effective=%q, want main/model", c.EffectiveDecisionModel)
	}
}

// newEchoAIServer suggests account for every transaction it is sent.
func newEchoAIServer(t *testing.T, account string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		var payload struct {
			Transactions []struct {
				FID string `json:"fid"`
			} `json:"transactions"`
		}
		_ = json.Unmarshal([]byte(body.Messages[len(body.Messages)-1].Content), &payload)
		type sugg struct {
			FID        string `json:"fid"`
			Account    string `json:"account"`
			Confidence string `json:"confidence"`
			Reasoning  string `json:"reasoning"`
		}
		out := struct {
			Suggestions []sugg `json:"suggestions"`
		}{Suggestions: []sugg{}}
		for _, tx := range payload.Transactions {
			out.Suggestions = append(out.Suggestions, sugg{tx.FID, account, "high", "echo"})
		}
		content, _ := json.Marshal(out)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(openRouterResp(string(content)))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAccountSuggester_RunsAfterWrite(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	srv := newEchoAIServer(t, "expenses:entertainment")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte(suggestAccountsJournal), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.toml")
	cfg := &config.Config{}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	hl, err := hledger.New("hledger", filepath.Join(dir, "main.journal"))
	if err != nil {
		t.Skipf("hledger unavailable: %v", err)
	}
	lock := txlock.New(dir, hl)
	h := serverledger.NewHandler(hl, lock, dir, configPath, nil, nil, cfg, nil)
	h.AIBaseURL = srv.URL
	lock.OnCommit(h.WakeAccountSuggester)
	go h.StartAccountSuggester(t.Context())

	unmarked := ""
	added, err := h.AddTransaction(t.Context(), connect.NewRequest(&floatv1.AddTransactionRequest{
		Date:        "2026-02-10",
		Description: "HULU",
		Status:      &unmarked,
		Postings: []*floatv1.PostingInput{
			{Account: "expenses:unknown", Commodity: "USD", Quantity: "7.99"},
			{Account: "liabilities:card"},
		},
	}))
	if err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}
	fid := added.Msg.Transaction.Fid

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if suggestionsByFID(t, h)[fid] == "expenses:entertainment" {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("no suggestion for %s after the write", fid)
}
