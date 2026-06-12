// Package auth implements floatd's shared-secret authentication layer.
//
// A single passphrase (from the FLOAT_AUTH_PASSPHRASE environment variable)
// gates all LedgerService RPCs. Clients authenticate with either:
//
//   - an Authorization: Bearer header carrying the raw passphrase or the
//     derived session token, or
//   - a float_session cookie carrying the session token (set by POST /api/login).
//
// The session token is a static HMAC derived from the passphrase, so no
// per-session state is stored and changing the passphrase invalidates all
// outstanding cookies and tokens. An empty passphrase disables auth entirely.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

// CookieName is the session cookie set by the login endpoint and accepted by
// the server interceptor.
const CookieName = "float_session"

// Auth verifies shared-secret credentials. The zero value is unusable; use New.
type Auth struct {
	passphrase string
	token      string // hex HMAC, empty when disabled
}

// New returns an Auth gating access with the given passphrase. An empty
// passphrase returns a disabled Auth that rejects all credentials but reports
// Enabled() == false, which callers treat as open access.
func New(passphrase string) *Auth {
	a := &Auth{passphrase: passphrase}
	if passphrase != "" {
		key := sha256.Sum256([]byte("float-auth-v1\x00" + passphrase))
		mac := hmac.New(sha256.New, key[:])
		mac.Write([]byte("float-session-v1"))
		a.token = hex.EncodeToString(mac.Sum(nil))
	}
	return a
}

// Enabled reports whether a passphrase is configured.
func (a *Auth) Enabled() bool {
	return a.token != ""
}

// Token returns the session token derived from the passphrase, or "" when
// auth is disabled.
func (a *Auth) Token() string {
	return a.token
}

// VerifyPassphrase reports whether s matches the configured passphrase.
// Always false when auth is disabled.
func (a *Auth) VerifyPassphrase(s string) bool {
	return a.Enabled() && subtle.ConstantTimeCompare([]byte(s), []byte(a.passphrase)) == 1
}

// VerifyToken reports whether s is the valid session token. Always false when
// auth is disabled.
func (a *Auth) VerifyToken(s string) bool {
	return a.Enabled() && subtle.ConstantTimeCompare([]byte(s), []byte(a.token)) == 1
}

// VerifyCredential reports whether s is acceptable as a bearer credential:
// either the raw passphrase or the session token.
func (a *Auth) VerifyCredential(s string) bool {
	// Evaluate both to keep timing independent of which one matches.
	okPass := a.VerifyPassphrase(s)
	okToken := a.VerifyToken(s)
	return okPass || okToken
}
