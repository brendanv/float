package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	"connectrpc.com/connect"
)

var errUnauthenticated = connect.NewError(
	connect.CodeUnauthenticated,
	errors.New("authentication required: provide an Authorization: Bearer credential or log in at /api/login"),
)

// NewServerInterceptor returns a ConnectRPC interceptor that rejects requests
// lacking a valid credential with CodeUnauthenticated. It accepts an
// Authorization: Bearer header (passphrase or session token) or a
// float_session cookie. When auth is disabled it passes everything through.
func NewServerInterceptor(a *Auth) connect.Interceptor {
	return &serverInterceptor{a: a}
}

type serverInterceptor struct {
	a *Auth
}

func (si *serverInterceptor) authorized(h http.Header) bool {
	if !si.a.Enabled() {
		return true
	}
	if bearer, ok := strings.CutPrefix(h.Get("Authorization"), "Bearer "); ok {
		if si.a.VerifyCredential(bearer) {
			return true
		}
	}
	// Reuse net/http cookie parsing for the Cookie header.
	if c, err := (&http.Request{Header: h}).Cookie(CookieName); err == nil {
		return si.a.VerifyToken(c.Value)
	}
	return false
}

func (si *serverInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if !si.authorized(req.Header()) {
			return nil, errUnauthenticated
		}
		return next(ctx, req)
	}
}

func (si *serverInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if !si.authorized(conn.RequestHeader()) {
			return errUnauthenticated
		}
		return next(ctx, conn)
	}
}

func (si *serverInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// TokenSource holds a bearer credential that can be set after the client is
// constructed (e.g. once the user has entered the passphrase). Safe for
// concurrent use.
type TokenSource struct {
	mu    sync.Mutex
	token string
}

// NewTokenSource returns a TokenSource seeded with token (may be empty).
func NewTokenSource(token string) *TokenSource {
	return &TokenSource{token: token}
}

// Set replaces the credential.
func (ts *TokenSource) Set(token string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.token = token
}

// Get returns the current credential, or "" if none is set.
func (ts *TokenSource) Get() string {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.token
}

// NewClientInterceptor returns a ConnectRPC interceptor that attaches the
// TokenSource's credential as an Authorization: Bearer header on every
// request. Requests are sent without the header while the source is empty.
func NewClientInterceptor(src *TokenSource) connect.Interceptor {
	return &clientInterceptor{src: src}
}

type clientInterceptor struct {
	src *TokenSource
}

func (ci *clientInterceptor) setHeader(h http.Header) {
	if token := ci.src.Get(); token != "" {
		h.Set("Authorization", "Bearer "+token)
	}
}

func (ci *clientInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		ci.setHeader(req.Header())
		return next(ctx, req)
	}
}

func (ci *clientInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		ci.setHeader(conn.RequestHeader())
		return conn
	}
}

func (ci *clientInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}
