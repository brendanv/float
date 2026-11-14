package gitsnap

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// preservedFiles are local-only files a restore never creates, overwrites, or
// deletes, even if an old snapshot happens to contain them.
var preservedFiles = []string{"config.toml", "float.key", "ssh_host_key", "ssh_known_hosts"}

// ErrStaleRestorePlan is returned by ApplyRestore when HEAD moved after the
// plan was computed, so the plan's path set may no longer cover every file
// the restore would touch. Callers should recompute the plan and retry.
var ErrStaleRestorePlan = errors.New("gitsnap: restore plan is stale (HEAD moved)")

// RestorePlan describes how to make the worktree match a target commit's tree
// without moving HEAD. Build one with PlanRestore and apply it with
// ApplyRestore.
type RestorePlan struct {
	dir  string
	head plumbing.Hash
	// writes are target-tree files to create or overwrite.
	writes []*object.File
	// removes are slash paths tracked at HEAD but absent from the target tree.
	removes []string
}

// Paths returns the sorted absolute paths of every file the restore may
// create, modify, or delete: the union of files tracked at HEAD and files in
// the target tree, excluding preserved local/secret files and gitignored
// paths. Callers snapshot these before applying (txlock.DoWith extraPaths) so
// a failed validation can fully revert the restore.
func (p *RestorePlan) Paths() []string {
	out := make([]string, 0, len(p.writes)+len(p.removes))
	for _, f := range p.writes {
		out = append(out, filepath.Join(p.dir, filepath.FromSlash(f.Name)))
	}
	for _, name := range p.removes {
		out = append(out, filepath.Join(p.dir, filepath.FromSlash(name)))
	}
	sort.Strings(out)
	return out
}

// PlanRestore computes the file changes needed to make the worktree match the
// tree of commit hash, relative to the files tracked at the current HEAD.
// Untracked files are left alone.
func (r *Repo) PlanRestore(_ context.Context, hash string) (*RestorePlan, error) {
	h := plumbing.NewHash(hash)
	if h.IsZero() {
		return nil, fmt.Errorf("gitsnap: restore: invalid hash %q", hash)
	}
	target, err := r.repo.CommitObject(h)
	if err != nil {
		return nil, fmt.Errorf("gitsnap: restore: commit %q not found: %w", hash, err)
	}
	targetTree, err := target.Tree()
	if err != nil {
		return nil, fmt.Errorf("gitsnap: restore: target tree: %w", err)
	}
	headRef, err := r.repo.Head()
	if err != nil {
		return nil, fmt.Errorf("gitsnap: restore: head: %w", err)
	}
	headCommit, err := r.repo.CommitObject(headRef.Hash())
	if err != nil {
		return nil, fmt.Errorf("gitsnap: restore: head commit: %w", err)
	}
	headTree, err := headCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("gitsnap: restore: head tree: %w", err)
	}
	skip, err := r.restoreSkipper()
	if err != nil {
		return nil, err
	}

	plan := &RestorePlan{dir: r.dir, head: headRef.Hash()}
	inTarget := map[string]bool{}
	err = targetTree.Files().ForEach(func(f *object.File) error {
		if skip(f.Name) {
			return nil
		}
		if f.Mode != filemode.Regular && f.Mode != filemode.Deprecated && f.Mode != filemode.Executable {
			return fmt.Errorf("unsupported file mode %s for %q", f.Mode, f.Name)
		}
		inTarget[f.Name] = true
		plan.writes = append(plan.writes, f)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("gitsnap: restore: walk target tree: %w", err)
	}
	err = headTree.Files().ForEach(func(f *object.File) error {
		if !inTarget[f.Name] && !skip(f.Name) {
			plan.removes = append(plan.removes, f.Name)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("gitsnap: restore: walk head tree: %w", err)
	}
	return plan, nil
}

// restoreSkipper returns a predicate reporting whether a slash path must be
// left untouched by a restore: preserved local/secret files and anything the
// current .gitignore (or info/exclude) ignores.
func (r *Repo) restoreSkipper() (func(string) bool, error) {
	wt, err := r.repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("gitsnap: restore: worktree: %w", err)
	}
	patterns, err := gitignore.ReadPatterns(wt.Filesystem, nil)
	if err != nil {
		return nil, fmt.Errorf("gitsnap: restore: read gitignore: %w", err)
	}
	matcher := gitignore.NewMatcher(append(patterns, wt.Excludes...))
	return func(name string) bool {
		if slices.Contains(preservedFiles, name) {
			return true
		}
		return matcher.Match(strings.Split(name, "/"), false)
	}, nil
}

// ApplyRestore writes the plan's target tree into the worktree: it creates
// missing files, overwrites files whose content differs, and deletes files
// tracked at HEAD that the target lacks. HEAD and the index are not moved, so
// the next Commit records the restore as a new snapshot on top of the
// existing history, and the restore can itself be undone by restoring a newer
// snapshot. It returns ErrStaleRestorePlan if HEAD moved since the plan was
// computed.
func (r *Repo) ApplyRestore(_ context.Context, plan *RestorePlan) error {
	headRef, err := r.repo.Head()
	if err != nil {
		return fmt.Errorf("gitsnap: restore: head: %w", err)
	}
	if headRef.Hash() != plan.head {
		return ErrStaleRestorePlan
	}
	for _, name := range plan.removes {
		path := filepath.Join(r.dir, filepath.FromSlash(name))
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("gitsnap: restore: remove %s: %w", name, err)
		}
		r.removeEmptyParents(path)
	}
	for _, f := range plan.writes {
		if err := r.writeTreeFile(f); err != nil {
			return fmt.Errorf("gitsnap: restore: write %s: %w", f.Name, err)
		}
	}
	// An older snapshot's .gitignore may predate entries added since; re-heal
	// it so the post-restore commit never captures secrets.
	if err := ensureGitignore(r.dir); err != nil {
		return fmt.Errorf("gitsnap: restore: ensure gitignore: %w", err)
	}
	return nil
}

// Restore makes the worktree match the tree of commit hash without moving
// HEAD; see PlanRestore and ApplyRestore.
func (r *Repo) Restore(ctx context.Context, hash string) error {
	plan, err := r.PlanRestore(ctx, hash)
	if err != nil {
		return err
	}
	return r.ApplyRestore(ctx, plan)
}

func (r *Repo) writeTreeFile(f *object.File) error {
	rd, err := f.Reader()
	if err != nil {
		return err
	}
	data, err := io.ReadAll(rd)
	_ = rd.Close()
	if err != nil {
		return err
	}
	path := filepath.Join(r.dir, filepath.FromSlash(f.Name))
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, data) {
		return nil
	}
	perm := os.FileMode(0o644)
	if f.Mode == filemode.Executable {
		perm = 0o755
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}

// removeEmptyParents deletes directories left empty by a removal, stopping at
// the first non-empty one or the repo root.
func (r *Repo) removeEmptyParents(path string) {
	prefix := r.dir + string(filepath.Separator)
	for d := filepath.Dir(path); strings.HasPrefix(d, prefix); d = filepath.Dir(d) {
		if os.Remove(d) != nil {
			return
		}
	}
}
