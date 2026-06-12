package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"charm.land/wish/v2"
	"charm.land/wish/v2/activeterm"
	"charm.land/wish/v2/bubbletea"
	"charm.land/wish/v2/logging"
	"connectrpc.com/connect"
	"github.com/brendanv/float/cmd/float/ui"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
	"github.com/brendanv/float/internal/auth"
	"github.com/charmbracelet/ssh"
	"golang.org/x/net/http2"
)

// startSSHServer starts a wish SSH server that serves the float TUI per session.
// dataDir is used to store/load the SSH host key.
// floatdAddr is the local HTTP address of the floatd gRPC server (e.g. ":8080").
// sshPort is the port to listen on.
// authn gates each session behind an in-TUI passphrase prompt when enabled.
func startSSHServer(ctx context.Context, dataDir, floatdAddr string, sshPort int, authn *auth.Auth) {
	addr := fmt.Sprintf(":%d", sshPort)
	hostKeyPath := filepath.Join(dataDir, "ssh_host_key")

	s, err := wish.NewServer(
		wish.WithAddress(addr),
		wish.WithHostKeyPath(hostKeyPath),
		wish.WithMiddleware(
			bubbletea.Middleware(sshTUIHandler(floatdAddr, authn)),
			activeterm.Middleware(), // Bubble Tea apps require a PTY.
			logging.Middleware(),
		),
	)
	if err != nil {
		slog.Error("ssh server: create server", "error", err)
		return
	}

	go func() {
		<-ctx.Done()
		_ = s.Shutdown(context.Background())
	}()

	slog.Info("floatd ssh listening", "addr", addr)
	if err := s.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
		slog.Error("ssh server stopped", "error", err)
	}
}

// sshTUIHandler returns a wish/bubbletea Handler that creates a float TUI model
// per SSH session. The wish bubbletea middleware handles PTY setup, window resize
// forwarding, and program lifecycle. When auth is enabled, each session is gated
// behind a passphrase prompt: the passphrase is verified in-process and the
// session's client then carries the bearer token through the auth interceptor.
func sshTUIHandler(floatdAddr string, authn *auth.Auth) bubbletea.Handler {
	return func(sess ssh.Session) (tea.Model, []tea.ProgramOption) {
		plainClient := &http.Client{
			Transport: &http2.Transport{
				AllowHTTP: true,
				DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
					return net.Dial(network, addr)
				},
			},
		}
		tokenSrc := auth.NewTokenSource("")
		client := floatv1connect.NewLedgerServiceClient(
			plainClient,
			"http://"+floatdAddr,
			connect.WithInterceptors(auth.NewClientInterceptor(tokenSrc)),
		)
		inner := ui.New(client)
		if !authn.Enabled() {
			return inner, []tea.ProgramOption{}
		}
		login := func(_ context.Context, passphrase string) (string, error) {
			if !authn.VerifyPassphrase(passphrase) {
				return "", errors.New("incorrect passphrase")
			}
			return authn.Token(), nil
		}
		// nil probe: there is no saved credential, so prompt immediately.
		gate := ui.NewAuthGate(inner, nil, login, tokenSrc.Set)
		return gate, []tea.ProgramOption{}
	}
}
