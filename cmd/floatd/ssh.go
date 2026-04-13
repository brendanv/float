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
	"github.com/brendanv/float/cmd/float/ui"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
	"github.com/charmbracelet/ssh"
	"golang.org/x/net/http2"
)

// startSSHServer starts a wish SSH server that serves the float TUI per session.
// dataDir is used to store/load the SSH host key.
// floatdAddr is the local HTTP address of the floatd gRPC server (e.g. ":8080").
// sshPort is the port to listen on.
func startSSHServer(ctx context.Context, dataDir, floatdAddr string, sshPort int) {
	addr := fmt.Sprintf(":%d", sshPort)
	hostKeyPath := filepath.Join(dataDir, "ssh_host_key")

	s, err := wish.NewServer(
		wish.WithAddress(addr),
		wish.WithHostKeyPath(hostKeyPath),
		wish.WithMiddleware(
			bubbletea.Middleware(sshTUIHandler(floatdAddr)),
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
// forwarding, and program lifecycle.
func sshTUIHandler(floatdAddr string) bubbletea.Handler {
	return func(sess ssh.Session) (tea.Model, []tea.ProgramOption) {
		plainClient := &http.Client{
			Transport: &http2.Transport{
				AllowHTTP: true,
				DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
					return net.Dial(network, addr)
				},
			},
		}
		client := floatv1connect.NewLedgerServiceClient(plainClient, "http://"+floatdAddr)
		return ui.New(client), []tea.ProgramOption{}
	}
}
