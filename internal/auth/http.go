package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// cookieMaxAge is one year; sessions are effectively permanent until the
// passphrase changes.
const cookieMaxAge = 365 * 24 * 60 * 60

// sessionCookie builds the session cookie. No Secure attribute: floatd serves
// plain h2c behind a trusted network.
func sessionCookie(token string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

// StatusHandler serves GET /api/auth, reporting whether auth is enabled so
// clients know whether to show a login flow.
func (a *Auth) StatusHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, map[string]any{"enabled": a.Enabled()})
	})
}

// LoginHandler serves POST /api/login. On success it returns the session
// token and sets the session cookie; a wrong passphrase returns 401. When
// auth is disabled it returns an empty token.
func (a *Auth) LoginHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Passphrase string `json:"passphrase"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if !a.Enabled() {
			writeJSON(w, map[string]any{"token": ""})
			return
		}
		if !a.VerifyPassphrase(body.Passphrase) {
			http.Error(w, "incorrect passphrase", http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, sessionCookie(a.Token(), cookieMaxAge))
		writeJSON(w, map[string]any{"token": a.Token()})
	})
}

// LogoutHandler serves POST /api/logout, clearing the session cookie.
func (a *Auth) LogoutHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.SetCookie(w, sessionCookie("", -1))
		w.WriteHeader(http.StatusNoContent)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// LoginHTTP exchanges a passphrase for a session token via POST /api/login.
// baseURL is the server root, e.g. "http://localhost:8080".
func LoginHTTP(ctx context.Context, client *http.Client, baseURL, passphrase string) (string, error) {
	body, err := json.Marshal(map[string]string{"passphrase": passphrase})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/login", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("incorrect passphrase")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed: %s", resp.Status)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("login: decode response: %w", err)
	}
	return out.Token, nil
}

// RequireAuth wraps an http.Handler with the same credential check the Connect
// interceptor applies, rejecting unauthenticated requests with 401.
//
// Plain HTTP endpoints that serve ledger data need this explicitly. The static
// web assets are deliberately served without auth so the SPA shell can render
// the login page, so "it is served over HTTP by floatd" is not by itself a
// reason to consider an endpoint protected.
func (a *Auth) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.authorizedRequest(r) {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// authorizedRequest reports whether r carries a valid credential: an
// Authorization: Bearer header (passphrase or session token) or the session
// cookie. Always true when auth is disabled.
func (a *Auth) authorizedRequest(r *http.Request) bool {
	if !a.Enabled() {
		return true
	}
	if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		if a.VerifyCredential(bearer) {
			return true
		}
	}
	if c, err := r.Cookie(CookieName); err == nil {
		return a.VerifyToken(c.Value)
	}
	return false
}
