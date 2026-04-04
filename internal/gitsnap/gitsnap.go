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

type Repo struct {
	dir  string
	repo *git.Repository
}

type Snapshot struct {
	Hash      string
	Message   string
	Timestamp time.Time
}

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

func initRepo(dir string) (*git.Repository, error) {
	if err := writeGitignore(dir); err != nil {
		return nil, err
	}
	r, err := git.PlainInit(dir, false)
	if err != nil {
		return nil, fmt.Errorf("plain init: %w", err)
	}
	wt, err := r.Worktree()
	if err != nil {
		return nil, fmt.Errorf("worktree: %w", err)
	}
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

func writeGitignore(dir string) error {
	path := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte("config.toml\nfloat.key\n"), 0600)
}

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

func floatSignature() *object.Signature {
	return &object.Signature{
		Name:  "float",
		Email: "float@localhost",
		When:  time.Now(),
	}
}
