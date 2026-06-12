package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"

	"github.com/brendanv/float/internal/auth"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/gen/float/v1/floatv1connect"
)

// stubLedger implements just enough of LedgerService to exercise the
// interceptor's unary and streaming paths.
type stubLedger struct {
	floatv1connect.UnimplementedLedgerServiceHandler
}

func (s *stubLedger) GetGeneralConfig(
	ctx context.Context,
	req *connect.Request[floatv1.GetGeneralConfigRequest],
) (*connect.Response[floatv1.GetGeneralConfigResponse], error) {
	return connect.NewResponse(&floatv1.GetGeneralConfigResponse{}), nil
}

func (s *stubLedger) StreamLogs(
	ctx context.Context,
	req *connect.Request[floatv1.StreamLogsRequest],
	stream *connect.ServerStream[floatv1.StreamLogsResponse],
) error {
	return stream.Send(&floatv1.StreamLogsResponse{})
}

func newTestServer(t *testing.T, a *auth.Auth) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := floatv1connect.NewLedgerServiceHandler(
		&stubLedger{},
		connect.WithInterceptors(auth.NewServerInterceptor(a)),
	)
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestServerInterceptor(t *testing.T) {
	const passphrase = "open-sesame"
	enabled := auth.New(passphrase)

	tests := []struct {
		name     string
		auth     *auth.Auth
		header   http.Header
		wantCode connect.Code // 0 = success
	}{
		{
			name:     "no credential rejected",
			auth:     enabled,
			wantCode: connect.CodeUnauthenticated,
		},
		{
			name:   "bearer passphrase accepted",
			auth:   enabled,
			header: http.Header{"Authorization": {"Bearer " + passphrase}},
		},
		{
			name:   "bearer token accepted",
			auth:   enabled,
			header: http.Header{"Authorization": {"Bearer " + enabled.Token()}},
		},
		{
			name:   "cookie accepted",
			auth:   enabled,
			header: http.Header{"Cookie": {auth.CookieName + "=" + enabled.Token()}},
		},
		{
			name:     "wrong bearer rejected",
			auth:     enabled,
			header:   http.Header{"Authorization": {"Bearer nope"}},
			wantCode: connect.CodeUnauthenticated,
		},
		{
			name:     "wrong cookie rejected",
			auth:     enabled,
			header:   http.Header{"Cookie": {auth.CookieName + "=nope"}},
			wantCode: connect.CodeUnauthenticated,
		},
		{
			name: "disabled auth allows anonymous",
			auth: auth.New(""),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t, tt.auth)
			client := floatv1connect.NewLedgerServiceClient(srv.Client(), srv.URL)

			req := connect.NewRequest(&floatv1.GetGeneralConfigRequest{})
			for k, vs := range tt.header {
				for _, v := range vs {
					req.Header().Add(k, v)
				}
			}
			_, err := client.GetGeneralConfig(t.Context(), req)
			checkCode(t, err, tt.wantCode)
		})
	}
}

func TestServerInterceptor_Streaming(t *testing.T) {
	enabled := auth.New("open-sesame")
	srv := newTestServer(t, enabled)
	client := floatv1connect.NewLedgerServiceClient(srv.Client(), srv.URL)

	t.Run("no credential rejected", func(t *testing.T) {
		stream, err := client.StreamLogs(t.Context(), connect.NewRequest(&floatv1.StreamLogsRequest{}))
		if err == nil {
			stream.Receive()
			err = stream.Err()
			_ = stream.Close()
		}
		checkCode(t, err, connect.CodeUnauthenticated)
	})

	t.Run("bearer token accepted", func(t *testing.T) {
		req := connect.NewRequest(&floatv1.StreamLogsRequest{})
		req.Header().Set("Authorization", "Bearer "+enabled.Token())
		stream, err := client.StreamLogs(t.Context(), req)
		if err != nil {
			t.Fatalf("StreamLogs: %v", err)
		}
		if !stream.Receive() {
			t.Fatalf("expected one message, got error: %v", stream.Err())
		}
		_ = stream.Close()
	})
}

func TestClientInterceptor(t *testing.T) {
	const passphrase = "open-sesame"
	enabled := auth.New(passphrase)
	srv := newTestServer(t, enabled)

	src := auth.NewTokenSource("")
	client := floatv1connect.NewLedgerServiceClient(
		srv.Client(), srv.URL,
		connect.WithInterceptors(auth.NewClientInterceptor(src)),
	)

	// Empty source: no header sent, request rejected.
	_, err := client.GetGeneralConfig(t.Context(), connect.NewRequest(&floatv1.GetGeneralConfigRequest{}))
	checkCode(t, err, connect.CodeUnauthenticated)

	// After setting the token, unary and streaming both succeed.
	src.Set(enabled.Token())
	if _, err := client.GetGeneralConfig(t.Context(), connect.NewRequest(&floatv1.GetGeneralConfigRequest{})); err != nil {
		t.Fatalf("GetGeneralConfig with token: %v", err)
	}
	stream, err := client.StreamLogs(t.Context(), connect.NewRequest(&floatv1.StreamLogsRequest{}))
	if err != nil {
		t.Fatalf("StreamLogs with token: %v", err)
	}
	if !stream.Receive() {
		t.Fatalf("expected one message, got error: %v", stream.Err())
	}
	_ = stream.Close()
}

func checkCode(t *testing.T, err error, want connect.Code) {
	t.Helper()
	if want == 0 {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected connect error with code %v, got %v", want, err)
	}
	if connectErr.Code() != want {
		t.Fatalf("code = %v, want %v", connectErr.Code(), want)
	}
}
