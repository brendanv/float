package ledger_test

import (
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/brendanv/float/internal/auth"
	"github.com/brendanv/float/internal/cache"
	"github.com/brendanv/float/internal/cube"
	"github.com/brendanv/float/internal/hledger"
	serverledger "github.com/brendanv/float/internal/server/ledger"
	"github.com/brendanv/float/internal/testgen"
	"github.com/brendanv/float/internal/txlock"
)

// cubeTestServer builds a handler over a real data dir and serves the cube
// routes the way cmd/floatd wires them, including the auth wrapper.
func cubeTestServer(t *testing.T, passphrase string) (*httptest.Server, *txlock.TxLock) {
	t.Helper()
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 90, NumTxns: 25, WithFIDs: true})
	c, err := hledger.New("hledger", dir+"/main.journal")
	if err != nil {
		t.Skipf("hledger unavailable: %v", err)
	}
	lock := txlock.New(dir, c)
	h := serverledger.NewHandler(c, lock, dir, "", cache.New[any](lock.Generation), nil, nil, nil)

	authn := auth.New(passphrase)
	mux := http.NewServeMux()
	mux.Handle("/api/generation", authn.RequireAuth(h.GenerationHandler()))
	mux.Handle("/api/cube/", authn.RequireAuth(h.CubeHandler()))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, lock
}

// get issues a request without following the transparent gzip path, so header
// assertions see exactly what the handler wrote.
func get(t *testing.T, srv *httptest.Server, path string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func currentGeneration(t *testing.T, srv *httptest.Server) uint64 {
	t.Helper()
	resp := get(t, srv, "/api/generation", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/generation: status %d", resp.StatusCode)
	}
	var body struct {
		Generation uint64 `json:"generation"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode generation: %v", err)
	}
	return body.Generation
}

func TestCubeHandlerServesCurrentGeneration(t *testing.T) {
	srv, _ := cubeTestServer(t, "")
	gen := currentGeneration(t, srv)

	resp := get(t, srv, "/api/cube/"+strconv.FormatUint(gen, 10)+".bin", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/octet-stream" {
		t.Errorf("Content-Type: got %q", got)
	}
	// The URL is content-addressed by generation, which is what makes an
	// immutable cache directive safe.
	cc := resp.Header.Get("Cache-Control")
	if !strings.Contains(cc, "immutable") || !strings.Contains(cc, "max-age=31536000") {
		t.Errorf("Cache-Control: got %q, want public/immutable with a long max-age", cc)
	}
	if got := resp.Header.Get(serverledger.GenerationHeader); got != strconv.FormatUint(gen, 10) {
		t.Errorf("%s: got %q, want %d", serverledger.GenerationHeader, got, gen)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if got := string(body[:len(cube.Magic)]); got != cube.Magic {
		t.Fatalf("magic: got %q, want %q", got, cube.Magic)
	}
	headerLen := int(binary.LittleEndian.Uint32(body[len(cube.Magic):]))
	var hdr struct {
		Generation uint64 `json:"generation"`
		Tables     map[string]struct {
			Rows int `json:"rows"`
		} `json:"tables"`
	}
	if err := json.Unmarshal(body[len(cube.Magic)+4:len(cube.Magic)+4+headerLen], &hdr); err != nil {
		t.Fatalf("header: %v", err)
	}
	if hdr.Generation != gen {
		t.Errorf("payload generation: got %d, want %d", hdr.Generation, gen)
	}
	if hdr.Tables["postings"].Rows == 0 {
		t.Error("payload has no postings")
	}
}

func TestCubeHandlerGzip(t *testing.T) {
	srv, _ := cubeTestServer(t, "")
	gen := currentGeneration(t, srv)
	path := "/api/cube/" + strconv.FormatUint(gen, 10) + ".bin"

	plain := get(t, srv, path, map[string]string{"Accept-Encoding": "identity"})
	plainBody, _ := io.ReadAll(plain.Body)

	resp := get(t, srv, path, map[string]string{"Accept-Encoding": "gzip"})
	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding: got %q, want gzip", got)
	}
	if got := resp.Header.Get("Vary"); !strings.Contains(got, "Accept-Encoding") {
		t.Errorf("Vary: got %q, want it to include Accept-Encoding", got)
	}
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	unzipped, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	if string(unzipped) != string(plainBody) {
		t.Error("gzipped payload does not match the identity payload")
	}
}

func TestCubeHandlerRejectsStaleGeneration(t *testing.T) {
	srv, _ := cubeTestServer(t, "")
	gen := currentGeneration(t, srv)

	// A generation the server is not on must not be served: the client would
	// cache it immutably under a URL claiming it never changes.
	resp := get(t, srv, "/api/cube/"+strconv.FormatUint(gen+7, 10)+".bin", nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status %d, want 409", resp.StatusCode)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control: got %q, want no-store", got)
	}
	var body struct {
		Generation uint64 `json:"generation"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Generation != gen {
		t.Errorf("reported generation: got %d, want %d", body.Generation, gen)
	}
}

func TestCubeURLChangesWithGeneration(t *testing.T) {
	srv, lock := cubeTestServer(t, "")
	before := currentGeneration(t, srv)

	lock.BumpGeneration()

	after := currentGeneration(t, srv)
	if after == before {
		t.Fatal("generation did not advance")
	}
	// The old URL stops resolving, which is what makes the immutable cache
	// entry unreachable rather than merely unlikely to be used.
	if resp := get(t, srv, "/api/cube/"+strconv.FormatUint(before, 10)+".bin", nil); resp.StatusCode != http.StatusConflict {
		t.Errorf("stale URL: status %d, want 409", resp.StatusCode)
	}
	if resp := get(t, srv, "/api/cube/"+strconv.FormatUint(after, 10)+".bin", nil); resp.StatusCode != http.StatusOK {
		t.Errorf("current URL: status %d, want 200", resp.StatusCode)
	}
}

// TestCubeRequiresAuth is the one that matters most here: the payload is the
// entire ledger, and the static assets next to it are deliberately public.
func TestCubeRequiresAuth(t *testing.T) {
	const passphrase = "correct horse battery staple"
	srv, lock := cubeTestServer(t, passphrase)
	gen := lock.Generation()
	path := "/api/cube/" + strconv.FormatUint(gen, 10) + ".bin"

	for _, tc := range []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{name: "no credential", want: http.StatusUnauthorized},
		{name: "wrong passphrase", headers: map[string]string{"Authorization": "Bearer nope"}, want: http.StatusUnauthorized},
		{name: "correct passphrase", headers: map[string]string{"Authorization": "Bearer " + passphrase}, want: http.StatusOK},
		{name: "session token", headers: map[string]string{"Authorization": "Bearer " + auth.New(passphrase).Token()}, want: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if resp := get(t, srv, path, tc.headers); resp.StatusCode != tc.want {
				t.Errorf("status %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}

	t.Run("generation endpoint", func(t *testing.T) {
		if resp := get(t, srv, "/api/generation", nil); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status %d, want 401", resp.StatusCode)
		}
	})

	t.Run("session cookie", func(t *testing.T) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+path, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: auth.New(passphrase).Token()})
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("do: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status %d, want 200", resp.StatusCode)
		}
	})
}

func TestCubeHandlerBadPaths(t *testing.T) {
	srv, lock := cubeTestServer(t, "")
	gen := strconv.FormatUint(lock.Generation(), 10)

	for _, tc := range []struct {
		path string
		want int
	}{
		{path: "/api/cube/", want: http.StatusNotFound},
		{path: "/api/cube/" + gen, want: http.StatusNotFound},
		{path: "/api/cube/abc.bin", want: http.StatusBadRequest},
		{path: "/api/cube/-1.bin", want: http.StatusBadRequest},
	} {
		t.Run(tc.path, func(t *testing.T) {
			if resp := get(t, srv, tc.path, nil); resp.StatusCode != tc.want {
				t.Errorf("status %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}
