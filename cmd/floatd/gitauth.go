package main

import (
	"fmt"
	"os"
	"path/filepath"

	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"

	"github.com/brendanv/float/internal/config"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

// buildGitAuth constructs a go-git auth method from the [git] config section.
// When AuthToken is set, HTTPS token auth is used.
// Otherwise SSH key auth is attempted (SSHKeyPath or ~/.ssh/id_rsa).
func buildGitAuth(cfg config.GitConfig) (transport.AuthMethod, error) {
	if cfg.AuthToken != "" {
		return &githttp.BasicAuth{
			Username: "x-token",
			Password: cfg.AuthToken,
		}, nil
	}
	keyPath := cfg.SSHKeyPath
	if keyPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("git auth: resolve home dir: %w", err)
		}
		keyPath = filepath.Join(home, ".ssh", "id_rsa")
	}
	auth, err := gitssh.NewPublicKeysFromFile("git", keyPath, "")
	if err != nil {
		return nil, fmt.Errorf("git auth: load SSH key %s: %w", keyPath, err)
	}
	return auth, nil
}
