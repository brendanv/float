package main

import (
	"os"
	"path/filepath"
	"testing"

	"charm.land/wish/v2"
	"charm.land/wish/v2/activeterm"
	"charm.land/wish/v2/bubbletea"
	"charm.land/wish/v2/logging"
	"charm.land/wish/v2/testsession"
	"github.com/charmbracelet/ssh"
)

// TestSSHTUIHandlerReturnsModel verifies that sshTUIHandler produces a valid
// bubbletea Handler that returns a non-nil model. The ssh.Session is unused
// in the handler body (the model only needs the gRPC client), so nil is safe.
func TestSSHTUIHandlerReturnsModel(t *testing.T) {
	handler := sshTUIHandler("localhost:8080")
	model, opts := handler(nil)
	if model == nil {
		t.Fatal("expected non-nil model")
	}
	if opts == nil {
		t.Fatal("expected non-nil options slice (may be empty)")
	}
}

// TestSSHServerHostKeyGeneration verifies that wish.NewServer creates an ed25519
// host key at the configured path when none already exists.
func TestSSHServerHostKeyGeneration(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "ssh_host_key")

	if _, err := wish.NewServer(wish.WithHostKeyPath(keyPath)); err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	if _, err := os.Stat(keyPath); err != nil {
		t.Fatalf("expected host key at %s after server creation: %v", keyPath, err)
	}
}

// TestSSHServerReusesExistingHostKey verifies that the server loads an
// existing key rather than overwriting it on a subsequent startup.
func TestSSHServerReusesExistingHostKey(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "ssh_host_key")

	// First startup — generates the key.
	if _, err := wish.NewServer(wish.WithHostKeyPath(keyPath)); err != nil {
		t.Fatalf("first NewServer: %v", err)
	}
	info1, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat after first start: %v", err)
	}

	// Second startup — should reuse the existing key (same mtime).
	if _, err := wish.NewServer(wish.WithHostKeyPath(keyPath)); err != nil {
		t.Fatalf("second NewServer: %v", err)
	}
	info2, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat after second start: %v", err)
	}

	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Fatal("host key was overwritten on second startup")
	}
}

// TestSSHServerRejectsNoPTY verifies that the activeterm middleware in our
// middleware stack rejects SSH connections that do not request a PTY.
func TestSSHServerRejectsNoPTY(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "ssh_host_key")

	srv, err := wish.NewServer(
		wish.WithHostKeyPath(keyPath),
		wish.WithMiddleware(
			bubbletea.Middleware(sshTUIHandler("localhost:8080")),
			activeterm.Middleware(),
			logging.Middleware(),
		),
	)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	// testsession.New connects without requesting a PTY by default.
	sess := testsession.New(t, srv, nil)
	out, err := sess.Output("")
	if err == nil {
		t.Fatal("expected non-zero exit for non-PTY connection, got nil")
	}
	if string(out) != "Requires an active PTY\n" {
		t.Errorf("unexpected rejection message: %q", string(out))
	}
}

// TestSSHMiddlewareStackBuilds verifies that wish.NewServer accepts our full
// middleware configuration without error (smoke test for the option wiring).
func TestSSHMiddlewareStackBuilds(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "ssh_host_key")

	_, err := wish.NewServer(
		wish.WithAddress(":0"),
		wish.WithHostKeyPath(keyPath),
		wish.WithMiddleware(
			bubbletea.Middleware(sshTUIHandler("localhost:8080")),
			activeterm.Middleware(),
			logging.Middleware(),
		),
	)
	if err != nil {
		t.Fatalf("server construction failed: %v", err)
	}
}

// TestSSHHandlerIsWrappedByActiveterm verifies the same PTY-guard behavior
// at the middleware level without going through wish.NewServer, using an
// ssh.Server directly (as wish's own tests do).
func TestSSHHandlerIsWrappedByActiveterm(t *testing.T) {
	inner := activeterm.Middleware()(func(s ssh.Session) {
		_, _ = s.Write([]byte("hello"))
	})
	sess := testsession.New(t, &ssh.Server{Handler: inner}, nil)
	_, err := sess.Output("")
	if err == nil {
		t.Fatal("expected rejection without PTY")
	}
}
