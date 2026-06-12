package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/brendanv/float/internal/auth"
)

func newHTTPServer(a *auth.Auth) *httptest.Server {
	mux := http.NewServeMux()
	mux.Handle("/api/auth", a.StatusHandler())
	mux.Handle("/api/login", a.LoginHandler())
	mux.Handle("/api/logout", a.LogoutHandler())
	return httptest.NewServer(mux)
}

func TestStatusHandler(t *testing.T) {
	tests := []struct {
		name        string
		auth        *auth.Auth
		wantEnabled bool
	}{
		{name: "enabled", auth: auth.New("secret"), wantEnabled: true},
		{name: "disabled", auth: auth.New(""), wantEnabled: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newHTTPServer(tt.auth)
			defer srv.Close()
			resp, err := srv.Client().Get(srv.URL + "/api/auth")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			var out struct {
				Enabled bool `json:"enabled"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				t.Fatal(err)
			}
			if out.Enabled != tt.wantEnabled {
				t.Errorf("enabled = %v, want %v", out.Enabled, tt.wantEnabled)
			}
		})
	}
}

func TestLoginHandler(t *testing.T) {
	const passphrase = "secret"
	tests := []struct {
		name       string
		auth       *auth.Auth
		body       string
		wantStatus int
		wantToken  bool
		wantCookie bool
	}{
		{
			name:       "correct passphrase",
			auth:       auth.New(passphrase),
			body:       `{"passphrase":"secret"}`,
			wantStatus: http.StatusOK,
			wantToken:  true,
			wantCookie: true,
		},
		{
			name:       "wrong passphrase",
			auth:       auth.New(passphrase),
			body:       `{"passphrase":"wrong"}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid body",
			auth:       auth.New(passphrase),
			body:       `not json`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "disabled auth returns empty token",
			auth:       auth.New(""),
			body:       `{"passphrase":""}`,
			wantStatus: http.StatusOK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newHTTPServer(tt.auth)
			defer srv.Close()
			resp, err := srv.Client().Post(srv.URL+"/api/login", "application/json", strings.NewReader(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			var out struct {
				Token string `json:"token"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				t.Fatal(err)
			}
			if got := out.Token != ""; got != tt.wantToken {
				t.Errorf("token present = %v, want %v", got, tt.wantToken)
			}
			if tt.wantToken && out.Token != tt.auth.Token() {
				t.Errorf("token = %q, want %q", out.Token, tt.auth.Token())
			}
			cookie := findSessionCookie(resp.Cookies())
			if tt.wantCookie {
				if cookie == nil {
					t.Fatal("session cookie not set")
				}
				if cookie.Value != tt.auth.Token() {
					t.Errorf("cookie value = %q, want token", cookie.Value)
				}
				if !cookie.HttpOnly || cookie.Path != "/" || cookie.MaxAge <= 0 {
					t.Errorf("cookie attributes: %+v", cookie)
				}
			} else if cookie != nil {
				t.Errorf("unexpected session cookie: %+v", cookie)
			}
		})
	}
}

func TestLogoutHandler(t *testing.T) {
	srv := newHTTPServer(auth.New("secret"))
	defer srv.Close()
	resp, err := srv.Client().Post(srv.URL+"/api/logout", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	cookie := findSessionCookie(resp.Cookies())
	if cookie == nil {
		t.Fatal("expected clearing cookie")
	}
	if cookie.Value != "" || cookie.MaxAge >= 0 {
		t.Errorf("cookie not cleared: %+v", cookie)
	}
}

func TestLoginHTTP(t *testing.T) {
	a := auth.New("secret")
	srv := newHTTPServer(a)
	defer srv.Close()

	token, err := auth.LoginHTTP(t.Context(), srv.Client(), srv.URL, "secret")
	if err != nil {
		t.Fatalf("LoginHTTP: %v", err)
	}
	if token != a.Token() {
		t.Errorf("token = %q, want %q", token, a.Token())
	}

	if _, err := auth.LoginHTTP(t.Context(), srv.Client(), srv.URL, "wrong"); err == nil {
		t.Error("LoginHTTP with wrong passphrase succeeded")
	}
}

func findSessionCookie(cookies []*http.Cookie) *http.Cookie {
	for _, c := range cookies {
		if c.Name == auth.CookieName {
			return c
		}
	}
	return nil
}
