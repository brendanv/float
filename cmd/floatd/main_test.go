package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/net/http2"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/gen/float/v1/floatv1connect"
	serverledger "github.com/brendanv/float/internal/server/ledger"
)

// These tests start the whole server through run() — config, startup
// migrations, txlock + gitsnap, auth, interceptors, reflection and the HTTP
// mux — against a real hledger, so wiring regressions that the per-package
// tests can't see fail here.

const smokePassphrase = "smoke-pass"

const smokeJournal = `2026-01-05 * Paycheck
    assets:checking          1000.00 USD
    income:salary

2026-01-10 * Grocery store
    expenses:food              45.50 USD
    assets:checking
`

// syncBuffer is a goroutine-safe log sink; run() logs from several goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func writeDataDir(t *testing.T, journal string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("[server]\nport = 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte(journal), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// startServer runs floatd on a free loopback port and returns its base URL.
// The server is shut down (and run's return value checked) at test cleanup.
func startServer(t *testing.T, dataDir string) string {
	t.Helper()
	logs := &syncBuffer{}
	ctx, cancel := context.WithCancel(t.Context())
	addrCh := make(chan net.Addr, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, options{
			dataDir:    dataDir,
			addr:       "127.0.0.1:0",
			passphrase: smokePassphrase,
			logOut:     logs,
			ready:      func(a net.Addr) { addrCh <- a },
		})
	}()

	var addr net.Addr
	select {
	case addr = <-addrCh:
	case err := <-errCh:
		cancel()
		t.Fatalf("run exited before serving: %v\nlogs:\n%s", err, logs)
	case <-time.After(60 * time.Second):
		cancel()
		t.Fatalf("floatd did not start within 60s\nlogs:\n%s", logs)
	}

	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil {
				t.Errorf("run returned error on shutdown: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Errorf("floatd did not shut down within 30s")
		}
		if t.Failed() {
			t.Logf("floatd logs:\n%s", logs)
		}
	})
	return "http://" + addr.String()
}

// h2cClient speaks HTTP/2 without TLS, which the gRPC protocol requires.
func h2cClient() *http.Client {
	return &http.Client{Transport: &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
	}}
}

func bearer(token string) connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			req.Header().Set("Authorization", "Bearer "+token)
			return next(ctx, req)
		}
	})
}

func usdBalance(t *testing.T, resp *floatv1.GetBalancesResponse, account string) string {
	t.Helper()
	for _, row := range resp.GetReport().GetRows() {
		if row.GetFullName() != account {
			continue
		}
		var got []string
		for _, a := range row.GetAmounts() {
			got = append(got, a.GetQuantity()+" "+a.GetCommodity())
		}
		if len(got) != 1 || !strings.HasSuffix(got[0], " USD") {
			t.Fatalf("%s amounts = %v, want a single USD amount", account, got)
		}
		return strings.TrimSuffix(got[0], " USD")
	}
	t.Fatalf("no balance row for %s", account)
	return ""
}

func TestServerSmoke(t *testing.T) {
	dataDir := writeDataDir(t, smokeJournal)
	base := startServer(t, dataDir)
	ctx := t.Context()

	t.Run("auth status is public and reports enabled", func(t *testing.T) {
		resp, err := http.Get(base + "/api/auth")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK || !body.Enabled {
			t.Fatalf("GET /api/auth = %d %+v, want 200 enabled", resp.StatusCode, body)
		}
	})

	t.Run("RPC without credentials is unauthenticated", func(t *testing.T) {
		client := floatv1connect.NewLedgerServiceClient(http.DefaultClient, base)
		_, err := client.GetBalances(ctx, connect.NewRequest(&floatv1.GetBalancesRequest{}))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Fatalf("GetBalances error = %v, want unauthenticated", err)
		}
	})

	t.Run("wrong passphrase is rejected", func(t *testing.T) {
		resp, err := http.Post(base+"/api/login", "application/json", strings.NewReader(`{"passphrase":"nope"}`))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("POST /api/login with wrong passphrase = %d, want 401", resp.StatusCode)
		}
	})

	t.Run("login cookie authorizes RPCs", func(t *testing.T) {
		jar, err := cookiejar.New(nil)
		if err != nil {
			t.Fatal(err)
		}
		hc := &http.Client{Jar: jar}
		resp, err := hc.Post(base+"/api/login", "application/json", strings.NewReader(`{"passphrase":"`+smokePassphrase+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST /api/login = %d, want 200", resp.StatusCode)
		}
		client := floatv1connect.NewLedgerServiceClient(hc, base)
		bal, err := client.GetBalances(ctx, connect.NewRequest(&floatv1.GetBalancesRequest{}))
		if err != nil {
			t.Fatalf("GetBalances with session cookie: %v", err)
		}
		if got := usdBalance(t, bal.Msg, "assets:checking"); got != "954.50" {
			t.Fatalf("assets:checking = %s, want 954.50", got)
		}
	})

	// gRPC (HTTP/2, h2c) for both the service and reflection, with a bearer.
	client := floatv1connect.NewLedgerServiceClient(h2cClient(), base, connect.WithGRPC(), connect.WithInterceptors(bearer(smokePassphrase)))

	t.Run("reflection lists LedgerService and requires auth", func(t *testing.T) {
		list := func(opts ...connect.ClientOption) ([]string, error) {
			stream := grpcreflect.NewClient(h2cClient(), base, opts...).NewStream(ctx)
			defer func() { _, _ = stream.Close() }()
			names, err := stream.ListServices()
			out := make([]string, len(names))
			for i, n := range names {
				out[i] = string(n)
			}
			return out, err
		}
		if _, err := list(connect.WithGRPC()); connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Fatalf("unauthenticated ListServices error = %v, want unauthenticated", err)
		}
		names, err := list(connect.WithGRPC(), connect.WithInterceptors(bearerStream(smokePassphrase)))
		if err != nil {
			t.Fatalf("ListServices: %v", err)
		}
		if !slices.Contains(names, floatv1connect.LedgerServiceName) {
			t.Fatalf("ListServices = %v, want %s", names, floatv1connect.LedgerServiceName)
		}
	})

	t.Run("write goes through txlock, hledger and a git snapshot", func(t *testing.T) {
		before, err := client.ListSnapshots(ctx, connect.NewRequest(&floatv1.ListSnapshotsRequest{}))
		if err != nil {
			t.Fatalf("ListSnapshots: %v", err)
		}
		_, err = client.AddTransaction(ctx, connect.NewRequest(&floatv1.AddTransactionRequest{
			Date:        "2026-01-15",
			Description: "Smoke coffee",
			Postings: []*floatv1.PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "4.50"},
				{Account: "assets:checking"},
			},
		}))
		if err != nil {
			t.Fatalf("AddTransaction: %v", err)
		}

		bal, err := client.GetBalances(ctx, connect.NewRequest(&floatv1.GetBalancesRequest{}))
		if err != nil {
			t.Fatalf("GetBalances: %v", err)
		}
		if got := usdBalance(t, bal.Msg, "assets:checking"); got != "950.00" {
			t.Fatalf("assets:checking after write = %s, want 950.00 (cache not invalidated?)", got)
		}

		monthFile, err := os.ReadFile(filepath.Join(dataDir, "2026", "01.journal"))
		if err != nil {
			t.Fatalf("read month journal: %v", err)
		}
		if !strings.Contains(string(monthFile), "Smoke coffee") {
			t.Fatalf("2026/01.journal missing new transaction:\n%s", monthFile)
		}

		after, err := client.ListSnapshots(ctx, connect.NewRequest(&floatv1.ListSnapshotsRequest{}))
		if err != nil {
			t.Fatalf("ListSnapshots: %v", err)
		}
		if len(after.Msg.GetSnapshots()) <= len(before.Msg.GetSnapshots()) {
			t.Fatalf("snapshots: %d before, %d after write; want a new snapshot",
				len(before.Msg.GetSnapshots()), len(after.Msg.GetSnapshots()))
		}
	})

	t.Run("MCP endpoint requires a bearer and reaches the ledger", func(t *testing.T) {
		resp, err := http.Post(base+"/mcp", "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("POST /mcp without credentials = %d, want 401", resp.StatusCode)
		}

		session, err := mcp.NewClient(&mcp.Implementation{Name: "smoke"}, nil).Connect(ctx, &mcp.StreamableClientTransport{
			Endpoint:   base + "/mcp",
			HTTPClient: &http.Client{Transport: bearerRoundTripper(smokePassphrase)},
		}, nil)
		if err != nil {
			t.Fatalf("MCP connect: %v", err)
		}
		defer func() { _ = session.Close() }()
		res, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "list_transactions",
			Arguments: map[string]any{"query": []string{"desc:Grocery"}},
		})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		text := res.Content[0].(*mcp.TextContent).Text
		if res.IsError || !strings.Contains(text, `"Grocery store"`) {
			t.Fatalf("list_transactions = %s (isError=%v)", text, res.IsError)
		}
	})

	t.Run("undoable write returns a snapshot trailer that UndoSnapshot reverts", func(t *testing.T) {
		// The transaction written by the earlier subtest (fixture ones have no code).
		list, err := client.ListTransactions(ctx, connect.NewRequest(&floatv1.ListTransactionsRequest{Query: []string{"desc:Smoke coffee"}}))
		if err != nil || len(list.Msg.Transactions) == 0 {
			t.Fatalf("ListTransactions: %v (%d found)", err, len(list.Msg.GetTransactions()))
		}
		fid := list.Msg.Transactions[0].Fid
		del, err := client.DeleteTransaction(ctx, connect.NewRequest(&floatv1.DeleteTransactionRequest{Fid: fid}))
		if err != nil {
			t.Fatalf("DeleteTransaction: %v", err)
		}
		snapshot := del.Trailer().Get(serverledger.SnapshotTrailer)
		if snapshot == "" {
			t.Fatal("DeleteTransaction returned no snapshot trailer through the full interceptor chain")
		}
		if _, err := client.UndoSnapshot(ctx, connect.NewRequest(&floatv1.UndoSnapshotRequest{Hash: snapshot})); err != nil {
			t.Fatalf("UndoSnapshot: %v", err)
		}
		again, err := client.ListTransactions(ctx, connect.NewRequest(&floatv1.ListTransactionsRequest{Query: []string{"code:" + fid}}))
		if err != nil || len(again.Msg.Transactions) != 1 {
			t.Fatalf("after undo, code:%s matched %d transactions (err %v), want 1", fid, len(again.Msg.GetTransactions()), err)
		}
	})

	t.Run("invalid transaction is rejected and nothing is written", func(t *testing.T) {
		_, err := client.AddTransaction(ctx, connect.NewRequest(&floatv1.AddTransactionRequest{
			Date:        "2026-01-16",
			Description: "Unbalanced",
			Postings: []*floatv1.PostingInput{
				{Account: "expenses:food", Commodity: "USD", Quantity: "1.00"},
				{Account: "assets:checking", Commodity: "USD", Quantity: "-2.00"},
			},
		}))
		if err == nil {
			t.Fatal("AddTransaction of an unbalanced transaction succeeded")
		}
		monthFile, readErr := os.ReadFile(filepath.Join(dataDir, "2026", "01.journal"))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if strings.Contains(string(monthFile), "Unbalanced") {
			t.Fatalf("rejected transaction was left on disk:\n%s", monthFile)
		}
	})
}

// bearerRoundTripper adds a bearer header to every plain HTTP request.
type bearerRoundTripper string

func (b bearerRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+string(b))
	return http.DefaultTransport.RoundTrip(r)
}

func bearerStream(token string) connect.Interceptor {
	return streamHeaderInterceptor{token: token}
}

// streamHeaderInterceptor adds a bearer header to streaming calls (reflection
// is a bidi stream, which connect.UnaryInterceptorFunc does not touch).
type streamHeaderInterceptor struct{ token string }

func (i streamHeaderInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc { return next }

func (i streamHeaderInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		conn.RequestHeader().Set("Authorization", "Bearer "+i.token)
		return conn
	}
}

func (i streamHeaderInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

func TestRunFailsOnInvalidJournal(t *testing.T) {
	dataDir := writeDataDir(t, `2026-01-05 * Unbalanced
    assets:checking          1.00 USD
    income:salary           -2.00 USD
`)
	err := run(t.Context(), options{dataDir: dataDir, addr: "127.0.0.1:0", logOut: io.Discard})
	if err == nil {
		t.Fatal("run succeeded on an unbalanced journal")
	}
	if !strings.Contains(err.Error(), "startup") {
		t.Fatalf("run error = %v, want a startup failure", err)
	}
}

func TestRunRequiresDataDir(t *testing.T) {
	err := run(t.Context(), options{logOut: io.Discard})
	if err == nil || !strings.Contains(err.Error(), "--data-dir") {
		t.Fatalf("run error = %v, want --data-dir required", err)
	}
}
