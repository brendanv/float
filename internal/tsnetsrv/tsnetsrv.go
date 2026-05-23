// Package tsnetsrv embeds Tailscale's tsnet library so floatd can join the
// user's tailnet as its own node and expose a dedicated HTTP listener — only
// the routes registered on Server.Mux are reachable, so it's safe to attach
// the Tailscale Funnel listener here without leaking the main floatd API.
package tsnetsrv

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/brendanv/float/internal/config"
	"tailscale.com/tsnet"
)

// Server wraps a *tsnet.Server, the listener it produces, and the http.Server
// that drives it. Routes are registered on Mux before calling Serve.
type Server struct {
	ts      *tsnet.Server
	ln      net.Listener
	httpSrv *http.Server
	logger  *slog.Logger
	Mux     *http.ServeMux
}

// New brings up a tsnet.Server, joins the tailnet, and opens a listener.
// When cfg.Funnel is true the listener is exposed publicly via Tailscale Funnel
// on :443; otherwise it's a tailnet-only listener on :443 (useful for testing
// without enabling Funnel in tailnet ACLs).
//
// Returns (nil, nil) when cfg.Enabled is false so callers can skip wiring.
func New(ctx context.Context, cfg config.TailscaleConfig, dataDir string, logger *slog.Logger) (*Server, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if logger == nil {
		logger = slog.Default()
	}

	authKey := cfg.AuthKey
	if env := os.Getenv("TS_AUTHKEY"); env != "" {
		authKey = env
	}

	stateDir := cfg.StateDir
	if stateDir == "" {
		stateDir = filepath.Join(dataDir, "tsnet")
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("tsnet: create state dir: %w", err)
	}

	hostname := cfg.Hostname
	if hostname == "" {
		hostname = "floatd"
	}

	tsLogger := logger.With("component", "tsnet")
	ts := &tsnet.Server{
		Dir:      stateDir,
		Hostname: hostname,
		AuthKey:  authKey,
		Logf:     func(format string, args ...any) { tsLogger.Debug(fmt.Sprintf(format, args...)) },
	}

	if _, err := ts.Up(ctx); err != nil {
		_ = ts.Close()
		return nil, fmt.Errorf("tsnet: bring up tailnet node: %w", err)
	}

	var ln net.Listener
	var err error
	if cfg.Funnel {
		ln, err = ts.ListenFunnel("tcp", ":443")
		if err != nil {
			_ = ts.Close()
			return nil, fmt.Errorf("tsnet: listen funnel: %w", err)
		}
	} else {
		ln, err = ts.ListenTLS("tcp", ":443")
		if err != nil {
			_ = ts.Close()
			return nil, fmt.Errorf("tsnet: listen tls: %w", err)
		}
	}

	mux := http.NewServeMux()
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info("tsnet: listening",
		"hostname", hostname,
		"funnel", cfg.Funnel,
		"addr", ln.Addr().String(),
	)

	return &Server{
		ts:      ts,
		ln:      ln,
		httpSrv: srv,
		logger:  logger,
		Mux:     mux,
	}, nil
}

// Serve blocks until the listener returns an error. It is safe to call once.
func (s *Server) Serve(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if err := s.httpSrv.Serve(s.ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown stops the HTTP server and tears down the tsnet node.
func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = s.httpSrv.Shutdown(shutdownCtx)
	return s.ts.Close()
}
