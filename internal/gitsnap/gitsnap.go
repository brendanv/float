// Package gitsnap provides git-backed snapshots of the float data directory.
// Each successful journal write creates a commit, giving a complete history
// that can be listed and restored.
package gitsnap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Repo wraps a go-git repository in the float data directory.
type Repo struct {
	dir  string
	repo *git.Repository
}

// Snapshot is a single git commit representing a point-in-time state of the data.
type Snapshot struct {
	Hash      string
	Message   string
	Timestamp time.Time
}

// New opens the git repository in dir, initializing it if it does not exist.
// It also writes a .gitignore to exclude config.toml (which may contain
// passphrase hashes), and creates an initial empty commit if the repo is new.
func New(dir string) (*Repo, error) {
	r, err := git.PlainOpen(dir)
	if errors.Is(err, git.ErrRepositoryNotExists) {
		r, err = initRepo(dir)
		if err != nil {
			return nil, fmt.Errorf("gitsnap: init: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("gitsnap: open: %w", err)
	}
	return &Repo{dir: dir, repo: r}, nil
}

// initRepo initializes a new git repository and creates an initial empty commit.
func initRepo(dir string) (*git.Repository, error) {
	if err := writeGitignore(dir); err != nil {
		return nil, err
	}
	r, err := git.PlainInit(dir, false)
	if err != nil {
		return nil, fmt.Errorf("plain init: %w", err)
	}
	// Create an initial empty commit so Log() works on a fresh repo.
	wt, err := r.Worktree()
	if err != nil {
		return nil, fmt.Errorf("worktree: %w", err)
	}
	// Stage the .gitignore we just wrote.
	if _, statErr := os.Stat(filepath.Join(dir, ".gitignore")); statErr == nil {
		if _, addErr := wt.Add(".gitignore"); addErr != nil {
			return nil, fmt.Errorf("stage .gitignore: %w", addErr)
		}
	}
	_, err = wt.Commit("float: init", &git.CommitOptions{
		Author:            floatSignature(),
		AllowEmptyCommits: true,
	})
	if err != nil {
		return nil, fmt.Errorf("initial commit: %w", err)
	}
	return r, nil
}

// writeGitignore writes a .gitignore in dir if it does not already exist.
// config.toml is excluded to prevent passphrase hashes from being committed.
func writeGitignore(dir string) error {
	path := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}
	return os.WriteFile(path, []byte("config.toml\nfloat.key\n"), 0600)
}

// Commit stages all changes and creates a new commit with msg.
// If the working tree is clean, Commit is a no-op (returns nil).
func (r *Repo) Commit(_ context.Context, msg string) error {
	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("gitsnap: commit: worktree: %w", err)
	}
	status, err := wt.Status()
	if err != nil {
		return fmt.Errorf("gitsnap: commit: status: %w", err)
	}
	if status.IsClean() {
		return nil
	}
	if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return fmt.Errorf("gitsnap: commit: add: %w", err)
	}
	if _, err := wt.Commit(msg, &git.CommitOptions{Author: floatSignature()}); err != nil {
		return fmt.Errorf("gitsnap: commit: %w", err)
	}
	return nil
}

// List returns up to limit recent snapshots, newest first.
// If limit is 0, it defaults to 50.
func (r *Repo) List(_ context.Context, limit int) ([]Snapshot, error) {
	if limit <= 0 {
		limit = 50
	}
	iter, err := r.repo.Log(&git.LogOptions{Order: git.LogOrderCommitterTime})
	if err != nil {
		return nil, fmt.Errorf("gitsnap: list: %w", err)
	}
	defer iter.Close()

	var snaps []Snapshot
	for len(snaps) < limit {
		c, err := iter.Next()
		if errors.Is(err, io.EOF) || c == nil {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("gitsnap: list: iterate: %w", err)
		}
		snaps = append(snaps, Snapshot{
			Hash:      c.Hash.String(),
			Message:   c.Message,
			Timestamp: c.Author.When,
		})
	}
	return snaps, nil
}

// Restore performs a hard reset to the commit identified by hash.
// This is intentionally destructive: all commits after hash are discarded.
func (r *Repo) Restore(_ context.Context, hash string) error {
	h := plumbing.NewHash(hash)
	if h.IsZero() {
		return fmt.Errorf("gitsnap: restore: invalid hash %q", hash)
	}
	if _, err := r.repo.CommitObject(h); err != nil {
		return fmt.Errorf("gitsnap: restore: commit %q not found: %w", hash, err)
	}
	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("gitsnap: restore: worktree: %w", err)
	}
	if err := wt.Reset(&git.ResetOptions{
		Commit: h,
		Mode:   git.HardReset,
	}); err != nil {
		return fmt.Errorf("gitsnap: restore: reset: %w", err)
	}
	return nil
}

// RecoverUncommitted commits any dirty working tree under the message
// "float: recovery snapshot". Called on startup to ensure nothing is lost
// if floatd was killed mid-write.
func (r *Repo) RecoverUncommitted(ctx context.Context) error {
	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("gitsnap: recover: worktree: %w", err)
	}
	status, err := wt.Status()
	if err != nil {
		return fmt.Errorf("gitsnap: recover: status: %w", err)
	}
	if status.IsClean() {
		return nil
	}
	return r.Commit(ctx, "float: recovery snapshot (uncommitted changes at startup)")
}

// floatSignature returns the author/committer identity used for all float commits.
func floatSignature() *object.Signature {
	return &object.Signature{
		Name:  "float",
		Email: "float@localhost",
		When:  time.Now(),
	}
}
