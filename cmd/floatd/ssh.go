package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/net/http2"

	"github.com/brendanv/float/cmd/float/ui"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
	"github.com/charmbracelet/keygen"
	"github.com/charmbracelet/ssh"
)

// startSSHServer starts an SSH server that serves the float TUI per session.
// dataDir is used to store/load the SSH host key.
// floatdAddr is the local HTTP address of the floatd gRPC server (e.g. ":8080").
// sshPort is the port to listen on.
func startSSHServer(ctx context.Context, dataDir, floatdAddr string, sshPort int) {
	addr := fmt.Sprintf(":%d", sshPort)
	hostKeyPath := filepath.Join(dataDir, "ssh_host_key")

	// Generate ed25519 host key on first run.
	if _, err := os.Stat(hostKeyPath); os.IsNotExist(err) {
		if _, err := keygen.New(hostKeyPath, keygen.WithKeyType(keygen.Ed25519), keygen.WithWrite()); err != nil {
			slog.Error("ssh server: generate host key", "error", err)
			return
		}
	}

	srv := &ssh.Server{
		Addr:    addr,
		Handler: sshTUIHandler(floatdAddr),
	}
	if err := srv.SetOption(ssh.HostKeyFile(hostKeyPath)); err != nil {
		slog.Error("ssh server: load host key", "error", err)
		return
	}

	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()

	slog.Info("floatd ssh listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("ssh server stopped", "error", err)
	}
}

// sshTUIHandler returns the SSH session handler that runs the float TUI.
func sshTUIHandler(floatdAddr string) ssh.Handler {
	return func(sess ssh.Session) {
		pty, windowChanges, hasPTY := sess.Pty()
		if !hasPTY {
			_, _ = fmt.Fprintln(sess.Stderr(), "error: a PTY is required to run the float TUI")
			_ = sess.Exit(1)
			return
		}

		// Each session gets its own gRPC client (stateless, safe across sessions).
		plainClient := &http.Client{
			Transport: &http2.Transport{
				AllowHTTP: true,
				DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
					return net.Dial(network, addr)
				},
			},
		}
		client := floatv1connect.NewLedgerServiceClient(plainClient, "http://"+floatdAddr)

		model := ui.New(client)

		sessionCtx, cancel := context.WithCancel(sess.Context())
		defer cancel()

		p := tea.NewProgram(
			model,
			tea.WithInput(sess),
			tea.WithOutput(sess),
			tea.WithContext(sessionCtx),
			tea.WithWindowSize(pty.Window.Width, pty.Window.Height),
			tea.WithEnvironment(sess.Environ()), // use client terminal env for color detection
			tea.WithoutSignalHandler(),           // don't intercept SIGINT at the process level
		)

		// Forward PTY window resize events to the bubbletea program.
		go func() {
			for {
				select {
				case <-sessionCtx.Done():
					return
				case w, ok := <-windowChanges:
					if !ok {
						return
					}
					p.Send(tea.WindowSizeMsg{Width: w.Width, Height: w.Height})
				}
			}
		}()

		if _, err := p.Run(); err != nil {
			slog.Warn("ssh tui session ended with error", "error", err, "user", sess.User())
		}
		p.Kill() // restore terminal state
	}
}
