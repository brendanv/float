package mcpserver_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/brendanv/float/gen/float/v1/floatv1connect"
	"github.com/brendanv/float/internal/auth"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/gitsnap"
	"github.com/brendanv/float/internal/hledger"
	serverledger "github.com/brendanv/float/internal/server/ledger"
	"github.com/brendanv/float/internal/server/mcpserver"
	"github.com/brendanv/float/internal/txlock"
)

const testPassphrase = "mcp-test-pass"

// journalWith returns a journal of n grocery transactions in January 2026.
func journalWith(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "2026-01-%02d * Grocery %d\n    expenses:food  %d.00 USD\n    assets:checking\n\n", i%28+1, i, i+1)
	}
	return b.String()
}

// startMCP serves the MCP endpoint backed by a real handler, hledger and
// gitsnap on a fresh data dir, gated by auth like floatd. It returns the
// endpoint URL and the data dir.
func startMCP(t *testing.T, journal string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte(journal), 0o644); err != nil {
		t.Fatal(err)
	}
	hl, err := hledger.New("hledger", filepath.Join(dir, "main.journal"))
	if err != nil {
		t.Skipf("hledger unavailable: %v", err)
	}
	lock := txlock.New(dir, hl)
	snap, err := gitsnap.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	lock.SetSnap(snap)
	h := serverledger.NewHandler(hl, lock, dir, filepath.Join(dir, "config.toml"), nil, snap, &config.Config{}, nil)

	authn := auth.New(testPassphrase)
	_, svc := floatv1connect.NewLedgerServiceHandler(h, connect.WithInterceptors(auth.NewServerInterceptor(authn)))
	inproc := mcpserver.NewInProcess(svc)
	t.Cleanup(func() { _ = inproc.Shutdown(t.Context()) })

	mh, err := mcpserver.NewHandler(inproc.LedgerClient(), mcpserver.Options{AuthEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(authn.RequireBearer(mh))
	t.Cleanup(ts.Close)
	return ts.URL + mcpserver.Path, dir
}

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func connectMCP(t *testing.T, endpoint, token string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint:   endpoint,
		HTTPClient: &http.Client{Transport: bearerTransport{token}},
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// call invokes a tool and decodes its JSON result. It fails the test on a
// tool error unless wantErr is set, in which case it returns the error text.
func call(t *testing.T, s *mcp.ClientSession, name string, args map[string]any) (map[string]any, string) {
	t.Helper()
	res, err := s.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if len(res.Content) != 1 {
		t.Fatalf("%s: %d content blocks, want 1", name, len(res.Content))
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if res.IsError {
		return nil, text
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("%s: result is not JSON: %v\n%s", name, err, text)
	}
	return out, ""
}

func mustCall(t *testing.T, s *mcp.ClientSession, name string, args map[string]any) map[string]any {
	t.Helper()
	out, errText := call(t, s, name, args)
	if errText != "" {
		t.Fatalf("%s: tool error: %s", name, errText)
	}
	return out
}

func TestAuthRequired(t *testing.T) {
	endpoint, _ := startMCP(t, journalWith(1))
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	for _, tc := range []struct {
		name, header string
	}{
		{"no credential", ""},
		{"wrong passphrase", "Bearer nope"},
		{"cookie only", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			if tc.name == "cookie only" {
				req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: auth.New(testPassphrase).Token()})
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", resp.StatusCode)
			}
			if resp.Header.Get("WWW-Authenticate") == "" {
				t.Error("missing WWW-Authenticate challenge")
			}
		})
	}

	t.Run("session token works", func(t *testing.T) {
		s := connectMCP(t, endpoint, auth.New(testPassphrase).Token())
		mustCall(t, s, "list_accounts", nil)
	})
}

func TestListTools(t *testing.T) {
	endpoint, _ := startMCP(t, journalWith(1))
	s := connectMCP(t, endpoint, testPassphrase)

	if got := s.InitializeResult().Instructions; !strings.Contains(got, "import_structured_transactions") {
		t.Errorf("instructions missing workflow guidance:\n%s", got)
	}

	res, err := s.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tool := range res.Tools {
		byName[tool.Name] = tool
		if tool.Description == "" || tool.Title == "" {
			t.Errorf("%s: missing description or title", tool.Name)
		}
		schema := tool.InputSchema.(map[string]any)
		if schema["type"] != "object" {
			t.Errorf("%s: schema type = %v", tool.Name, schema["type"])
		}
	}
	for _, name := range []string{"list_transactions", "import_structured_transactions", "bulk_delete_transactions", "restore_snapshot", "run_hledger_query"} {
		if byName[name] == nil {
			t.Fatalf("tool %s missing; have %v", name, slices.Sorted(func(yield func(string) bool) {
				for n := range byName {
					if !yield(n) {
						return
					}
				}
			}))
		}
	}

	for _, tc := range []struct {
		name                   string
		readOnly, destructive  bool
		destructiveHintPresent bool
	}{
		{"list_transactions", true, false, false},
		{"import_structured_transactions", false, false, true},
		{"bulk_delete_transactions", false, true, true},
	} {
		a := byName[tc.name].Annotations
		if a.ReadOnlyHint != tc.readOnly {
			t.Errorf("%s readOnlyHint = %v, want %v", tc.name, a.ReadOnlyHint, tc.readOnly)
		}
		if (a.DestructiveHint != nil) != tc.destructiveHintPresent || (a.DestructiveHint != nil && *a.DestructiveHint != tc.destructive) {
			t.Errorf("%s destructiveHint = %v, want present=%v value=%v", tc.name, a.DestructiveHint, tc.destructiveHintPresent, tc.destructive)
		}
	}

	// Field descriptions come from the .proto comments, nested messages are
	// expanded, and fields use proto names.
	schema, _ := json.Marshal(byName["import_structured_transactions"].InputSchema)
	for _, want := range []string{`"dry_run"`, `"include_duplicates"`, `"postings"`, `"balance_assertion"`, "Validate and preview only"} {
		if !strings.Contains(string(schema), want) {
			t.Errorf("import_structured_transactions schema missing %s:\n%s", want, schema)
		}
	}
}

func TestReadTools(t *testing.T) {
	endpoint, _ := startMCP(t, journalWith(120))
	s := connectMCP(t, endpoint, testPassphrase)

	t.Run("list_transactions defaults to a page of 100", func(t *testing.T) {
		out := mustCall(t, s, "list_transactions", nil)
		if n := len(out["transactions"].([]any)); n != 100 {
			t.Errorf("got %d transactions, want 100", n)
		}
		if out["total"] != float64(120) || out["has_next"] != true {
			t.Errorf("total=%v has_next=%v, want 120 true", out["total"], out["has_next"])
		}
	})

	t.Run("list_transactions honours query and limit", func(t *testing.T) {
		out := mustCall(t, s, "list_transactions", map[string]any{"query": []string{"desc:Grocery 7$"}, "limit": 5})
		txns := out["transactions"].([]any)
		if len(txns) != 1 || txns[0].(map[string]any)["description"] != "Grocery 7" {
			t.Errorf("transactions = %v, want only Grocery 7", txns)
		}
	})

	t.Run("get_balances", func(t *testing.T) {
		out := mustCall(t, s, "get_balances", map[string]any{"query": []string{"expenses"}})
		b, _ := json.Marshal(out)
		if !strings.Contains(string(b), `"expenses:food"`) || !strings.Contains(string(b), "7260") {
			t.Errorf("balances missing expenses:food 7260:\n%s", b)
		}
	})

	t.Run("run_hledger_query", func(t *testing.T) {
		out := mustCall(t, s, "run_hledger_query", map[string]any{"args": "bal expenses"})
		if !strings.Contains(out["stdout"].(string), "expenses:food") {
			t.Errorf("stdout = %q", out["stdout"])
		}
	})

	t.Run("RPC errors are tool errors", func(t *testing.T) {
		_, errText := call(t, s, "get_account_register", map[string]any{})
		if !strings.HasPrefix(errText, "invalid_argument: ") || !strings.Contains(errText, "account") {
			t.Errorf("error = %q, want invalid_argument about account", errText)
		}
		_, errText = call(t, s, "run_hledger_query", map[string]any{"args": "add"})
		if !strings.HasPrefix(errText, "invalid_argument: ") {
			t.Errorf("write command error = %q, want invalid_argument", errText)
		}
	})

	t.Run("malformed arguments are tool errors", func(t *testing.T) {
		_, errText := call(t, s, "list_transactions", map[string]any{"limit": "lots"})
		if !strings.HasPrefix(errText, "invalid arguments: ") {
			t.Errorf("error = %q", errText)
		}
		_, errText = call(t, s, "list_transactions", map[string]any{"no_such_field": 1})
		if !strings.HasPrefix(errText, "invalid arguments: ") {
			t.Errorf("unknown field error = %q", errText)
		}
	})
}

func TestWriteToolsRoundTrip(t *testing.T) {
	endpoint, dir := startMCP(t, journalWith(2))
	s := connectMCP(t, endpoint, testPassphrase)

	importArgs := map[string]any{
		"source": "agent",
		"transactions": []map[string]any{{
			"date":        "2026-02-03",
			"description": "Coffee shop",
			"postings": []map[string]any{
				{"account": "expenses:coffee", "quantity": "4.50", "commodity": "USD"},
				{"account": "assets:checking"},
			},
		}},
	}

	dry := mustCall(t, s, "import_structured_transactions", withArg(importArgs, "dry_run", true))
	if dry["import_batch_id"] != nil || len(dry["candidates"].([]any)) != 1 {
		t.Fatalf("dry run = %v, want one candidate and no batch", dry)
	}
	if strings.Contains(readJournals(t, dir), "Coffee shop") {
		t.Fatal("dry run wrote to the journal")
	}

	res := mustCall(t, s, "import_structured_transactions", importArgs)
	if res["imported_count"] != float64(1) {
		t.Fatalf("import = %v, want imported_count 1", res)
	}
	fid := res["transactions"].([]any)[0].(map[string]any)["fid"].(string)
	if !strings.Contains(readJournals(t, dir), "Coffee shop") {
		t.Fatal("import did not reach the journal")
	}

	again := mustCall(t, s, "import_structured_transactions", importArgs)
	if again["skipped_duplicate_count"] != float64(1) || again["imported_count"] != nil {
		t.Fatalf("re-import = %v, want the duplicate skipped", again)
	}

	snaps := mustCall(t, s, "list_snapshots", nil)["snapshots"].([]any)
	if len(snaps) == 0 {
		t.Fatal("no snapshot after a write")
	}

	// Server-streaming RPC: the tool returns the final message.
	del := mustCall(t, s, "bulk_delete_transactions", map[string]any{"fids": []string{fid}})
	if del["result"].(map[string]any)["deleted_count"] != float64(1) {
		t.Fatalf("bulk delete = %v, want deleted_count 1", del)
	}
	if strings.Contains(readJournals(t, dir), "Coffee shop") {
		t.Fatal("bulk delete left the transaction in the journal")
	}
}

func withArg(m map[string]any, k string, v any) map[string]any {
	out := map[string]any{k: v}
	for key, val := range m {
		out[key] = val
	}
	return out
}

// readJournals concatenates every .journal file under dir.
func readJournals(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if strings.HasSuffix(path, ".journal") {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			b.Write(data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}
