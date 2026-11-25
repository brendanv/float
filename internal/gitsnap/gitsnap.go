package gitsnap

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type Repo struct {
	dir  string
	repo *git.Repository
}

const maxCommitMessageLen = 180

type Snapshot struct {
	Hash      string
	Message   string
	Timestamp time.Time
}

type ChangeType int

const (
	ChangeAdded ChangeType = iota
	ChangeDeleted
	ChangeModified
	ChangeRenamed
)

type FileDiff struct {
	Path     string
	OldPath  string
	Change   ChangeType
	IsBinary bool
	Patch    string
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
	// Self-heal .gitignore in existing repos: entries added in newer float
	// versions (e.g. SSH key material) must take effect without re-init,
	// otherwise the next commit would capture secrets permanently.
	if err := ensureGitignore(dir); err != nil {
		return nil, fmt.Errorf("gitsnap: ensure gitignore: %w", err)
	}
	return &Repo{dir: dir, repo: r}, nil
}

func initRepo(dir string) (*git.Repository, error) {
	if err := ensureGitignore(dir); err != nil {
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

// gitignoreEntries are local-only files that must never be committed to the
// snapshot repo: user config, key material, and derived AI suggestions
// (advisory, keyed by transaction code; not worth a snapshot per import).
var gitignoreEntries = []string{"config.toml", "float.key", "ssh_host_key", "ssh_known_hosts", "account-suggestions.json"}

// ensureGitignore creates .gitignore or appends any required entries missing
// from an existing one, preserving user-added lines.
func ensureGitignore(dir string) error {
	path := filepath.Join(dir, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	existing := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		existing[strings.TrimSpace(line)] = true
	}
	var missing []string
	for _, entry := range gitignoreEntries {
		if !existing[entry] {
			missing = append(missing, entry)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	content := string(data)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += strings.Join(missing, "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0600)
}

func (r *Repo) Commit(ctx context.Context, msg string) error {
	_, err := r.CommitHash(ctx, msg)
	return err
}

// CommitHash is Commit that also returns the new commit's hash, or "" when
// the worktree was clean and nothing was committed.
func (r *Repo) CommitHash(_ context.Context, msg string) (string, error) {
	wt, err := r.repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("gitsnap: commit: worktree: %w", err)
	}
	status, err := wt.Status()
	if err != nil {
		return "", fmt.Errorf("gitsnap: commit: status: %w", err)
	}
	if status.IsClean() {
		return "", nil
	}
	if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return "", fmt.Errorf("gitsnap: commit: add: %w", err)
	}
	h, err := wt.Commit(limitCommitMessage(msg), &git.CommitOptions{Author: floatSignature()})
	if err != nil {
		return "", fmt.Errorf("gitsnap: commit: %w", err)
	}
	return h.String(), nil
}

// Tracks reports whether snapshots capture the file at absolute path p, i.e.
// whether restoring or undoing a snapshot brings it back. Local config and
// key material (config.toml, …) are never tracked.
func (r *Repo) Tracks(p string) bool {
	rel, err := filepath.Rel(r.dir, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return !skipRestorePath(filepath.ToSlash(rel))
}

func limitCommitMessage(msg string) string {
	runes := []rune(msg)
	if len(runes) <= maxCommitMessageLen {
		return msg
	}
	if maxCommitMessageLen <= 3 {
		return string(runes[:maxCommitMessageLen])
	}
	return string(runes[:maxCommitMessageLen-3]) + "..."
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

var preservedFiles = []string{"config.toml", "float.key", "ssh_host_key", "ssh_known_hosts", "account-suggestions.json"}

// ErrHeadMoved is returned by ApplyRestore when HEAD is no longer the commit
// the plan was computed against (another write committed in between).
var ErrHeadMoved = errors.New("gitsnap: HEAD moved since the restore was planned")

// ErrDirty is returned by ApplyRestore when the worktree has uncommitted
// changes, which a forward restore would otherwise fold into its commit.
var ErrDirty = errors.New("gitsnap: worktree has uncommitted changes")

// RestorePlan describes a forward restore: the files that must be rewritten
// to make the worktree match Target, computed against Head.
type RestorePlan struct {
	Head    string // HEAD when the plan was made
	Target  string // commit whose tree is restored
	Message string // Target's commit message
	// Paths are the absolute paths ApplyRestore may create, modify or delete.
	// Callers pass them to txlock so a failed restore is reverted exactly.
	Paths   []string
	changes object.Changes
}

// skipRestorePath reports whether a forward restore must leave path alone:
// local secrets/config (never tracked going forward) and .gitignore itself,
// so an old tree cannot drop newer ignore entries.
func skipRestorePath(path string) bool {
	if path == ".gitignore" {
		return true
	}
	for _, name := range preservedFiles {
		if path == name {
			return true
		}
	}
	return false
}

// Head returns the hash of the current HEAD commit.
func (r *Repo) Head() (string, error) {
	ref, err := r.repo.Head()
	if err != nil {
		return "", fmt.Errorf("gitsnap: head: %w", err)
	}
	return ref.Hash().String(), nil
}

func (r *Repo) commitByHash(op, hash string) (*object.Commit, error) {
	h := plumbing.NewHash(hash)
	if h.IsZero() || h.String() != strings.ToLower(hash) {
		return nil, fmt.Errorf("gitsnap: %s: invalid hash %q", op, hash)
	}
	c, err := r.repo.CommitObject(h)
	if err != nil {
		return nil, fmt.Errorf("gitsnap: %s: commit %q not found: %w", op, hash, err)
	}
	return c, nil
}

// PlanRestore computes the file changes that turn HEAD's tree into hash's
// tree. It reads only git objects, so it is safe to call outside txlock;
// ApplyRestore re-checks HEAD before writing.
func (r *Repo) PlanRestore(ctx context.Context, hash string) (*RestorePlan, error) {
	target, err := r.commitByHash("plan restore", hash)
	if err != nil {
		return nil, err
	}
	headRef, err := r.repo.Head()
	if err != nil {
		return nil, fmt.Errorf("gitsnap: plan restore: head: %w", err)
	}
	head, err := r.repo.CommitObject(headRef.Hash())
	if err != nil {
		return nil, fmt.Errorf("gitsnap: plan restore: head commit: %w", err)
	}
	headTree, err := head.Tree()
	if err != nil {
		return nil, fmt.Errorf("gitsnap: plan restore: head tree: %w", err)
	}
	targetTree, err := target.Tree()
	if err != nil {
		return nil, fmt.Errorf("gitsnap: plan restore: target tree: %w", err)
	}
	all, err := object.DiffTreeWithOptions(ctx, headTree, targetTree, &object.DiffTreeOptions{})
	if err != nil {
		return nil, fmt.Errorf("gitsnap: plan restore: tree diff: %w", err)
	}
	plan := &RestorePlan{Head: head.Hash.String(), Target: target.Hash.String(), Message: target.Message}
	seen := map[string]bool{}
	for _, ch := range all {
		keep := false
		for _, name := range []string{ch.From.Name, ch.To.Name} {
			if name == "" || skipRestorePath(name) {
				continue
			}
			keep = true
			if !seen[name] {
				seen[name] = true
				plan.Paths = append(plan.Paths, filepath.Join(r.dir, filepath.FromSlash(name)))
			}
		}
		if keep {
			plan.changes = append(plan.changes, ch)
		}
	}
	return plan, nil
}

// ApplyRestore rewrites the worktree so every tracked file matches the plan's
// target tree. HEAD and the index are left alone, so the next Commit records
// the restore as a new snapshot on top of the existing history. It refuses
// with ErrHeadMoved if HEAD changed since PlanRestore, and with ErrDirty if
// the worktree has uncommitted changes. Call it inside txlock.
func (r *Repo) ApplyRestore(ctx context.Context, plan *RestorePlan) error {
	head, err := r.Head()
	if err != nil {
		return err
	}
	if head != plan.Head {
		return ErrHeadMoved
	}
	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("gitsnap: restore: worktree: %w", err)
	}
	status, err := wt.Status()
	if err != nil {
		return fmt.Errorf("gitsnap: restore: status: %w", err)
	}
	if !status.IsClean() {
		return ErrDirty
	}
	for _, ch := range plan.changes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if ch.From.Name != "" && ch.From.Name != ch.To.Name && !skipRestorePath(ch.From.Name) {
			p := filepath.Join(r.dir, filepath.FromSlash(ch.From.Name))
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("gitsnap: restore: remove %s: %w", ch.From.Name, err)
			}
		}
		if ch.To.Name == "" || skipRestorePath(ch.To.Name) {
			continue
		}
		if err := r.writeEntry(ch.To); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repo) writeEntry(e object.ChangeEntry) error {
	blob, err := r.repo.BlobObject(e.TreeEntry.Hash)
	if err != nil {
		return fmt.Errorf("gitsnap: restore: blob %s: %w", e.Name, err)
	}
	rd, err := blob.Reader()
	if err != nil {
		return fmt.Errorf("gitsnap: restore: read %s: %w", e.Name, err)
	}
	defer func() { _ = rd.Close() }()
	data, err := io.ReadAll(rd)
	if err != nil {
		return fmt.Errorf("gitsnap: restore: read %s: %w", e.Name, err)
	}
	p := filepath.Join(r.dir, filepath.FromSlash(e.Name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("gitsnap: restore: mkdir for %s: %w", e.Name, err)
	}
	perm := os.FileMode(0o644)
	if e.TreeEntry.Mode == filemode.Executable {
		perm = 0o755
	}
	// Remove first so a mode change (and a file replacing a symlink) applies.
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("gitsnap: restore: replace %s: %w", e.Name, err)
	}
	if err := os.WriteFile(p, data, perm); err != nil {
		return fmt.Errorf("gitsnap: restore: write %s: %w", e.Name, err)
	}
	return nil
}

// ErrNotUndoable is returned by PlanUndo for a snapshot with no parent.
var ErrNotUndoable = errors.New("gitsnap: snapshot has nothing before it to go back to")

// PlanUndo plans undoing snapshot hash, which must be HEAD: a forward restore
// of its parent, so the undo is itself a new snapshot (and can be undone).
// It returns ErrHeadMoved when hash is no longer HEAD, i.e. a later snapshot
// would be lost too. The returned message is hash's own commit message.
func (r *Repo) PlanUndo(ctx context.Context, hash string) (plan *RestorePlan, undone string, err error) {
	c, err := r.commitByHash("undo", hash)
	if err != nil {
		return nil, "", err
	}
	head, err := r.Head()
	if err != nil {
		return nil, "", err
	}
	if head != c.Hash.String() {
		return nil, "", ErrHeadMoved
	}
	if c.NumParents() == 0 {
		return nil, "", ErrNotUndoable
	}
	plan, err = r.PlanRestore(ctx, c.ParentHashes[0].String())
	if err != nil {
		return nil, "", err
	}
	if plan.Head != c.Hash.String() {
		return nil, "", ErrHeadMoved
	}
	return plan, c.Message, nil
}

// Restore is PlanRestore + ApplyRestore + Commit for callers that do not go
// through txlock (floatctl): the worktree is set to hash's tree and recorded
// as a new snapshot with msg, keeping all later snapshots in history.
func (r *Repo) Restore(ctx context.Context, hash, msg string) error {
	plan, err := r.PlanRestore(ctx, hash)
	if err != nil {
		return err
	}
	if err := r.ApplyRestore(ctx, plan); err != nil {
		return err
	}
	return r.Commit(ctx, msg)
}

// ResetHard hard-resets the branch and worktree to the given commit, discarding
// every later snapshot from history, while preserving secret/local files.
func (r *Repo) ResetHard(_ context.Context, hash string) error {
	c, err := r.commitByHash("reset", hash)
	if err != nil {
		return err
	}
	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("gitsnap: reset: worktree: %w", err)
	}

	saved := map[string][]byte{}
	for _, name := range preservedFiles {
		data, err := os.ReadFile(filepath.Join(r.dir, name))
		if err == nil {
			saved[name] = data
		}
	}

	if err := wt.Reset(&git.ResetOptions{
		Commit: c.Hash,
		Mode:   git.HardReset,
	}); err != nil {
		return fmt.Errorf("gitsnap: reset: %w", err)
	}

	for name, data := range saved {
		if err := os.WriteFile(filepath.Join(r.dir, name), data, 0600); err != nil {
			return fmt.Errorf("gitsnap: reset: restore %s: %w", name, err)
		}
	}
	return nil
}

func (r *Repo) Diff(ctx context.Context, hash string) ([]FileDiff, error) {
	h := plumbing.NewHash(hash)
	if h.IsZero() {
		return nil, fmt.Errorf("gitsnap: diff: invalid hash %q", hash)
	}
	commit, err := r.repo.CommitObject(h)
	if err != nil {
		return nil, fmt.Errorf("gitsnap: diff: commit %q not found: %w", hash, err)
	}

	var oldTree *object.Tree
	parents := commit.Parents()
	parent, err := parents.Next()
	if err == nil {
		oldTree, err = parent.Tree()
		if err != nil {
			return nil, fmt.Errorf("gitsnap: diff: parent tree: %w", err)
		}
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("gitsnap: diff: parents: %w", err)
	}
	parents.Close()

	newTree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("gitsnap: diff: tree: %w", err)
	}

	changes, err := object.DiffTreeWithOptions(ctx, oldTree, newTree, &object.DiffTreeOptions{DetectRenames: true})
	if err != nil {
		return nil, fmt.Errorf("gitsnap: diff: tree diff: %w", err)
	}
	patch, err := changes.Patch()
	if err != nil {
		return nil, fmt.Errorf("gitsnap: diff: patch: %w", err)
	}

	filePatches := patch.FilePatches()
	out := make([]FileDiff, 0, len(filePatches))
	for _, fp := range filePatches {
		from, to := fp.Files()
		fd := FileDiff{IsBinary: fp.IsBinary()}
		switch {
		case from == nil && to != nil:
			fd.Change = ChangeAdded
			fd.Path = to.Path()
		case from != nil && to == nil:
			fd.Change = ChangeDeleted
			fd.Path = from.Path()
		case from != nil && to != nil && from.Path() != to.Path():
			fd.Change = ChangeRenamed
			fd.OldPath = from.Path()
			fd.Path = to.Path()
		case from != nil && to != nil:
			fd.Change = ChangeModified
			fd.Path = to.Path()
		default:
			continue
		}

		if !fd.IsBinary {
			var buf bytes.Buffer
			enc := diff.NewUnifiedEncoder(&buf, diff.DefaultContextLines)
			if err := enc.Encode(singleFilePatch{fp: fp, message: patch.Message()}); err != nil {
				return nil, fmt.Errorf("gitsnap: diff: encode %q: %w", fd.Path, err)
			}
			fd.Patch = buf.String()
		}
		out = append(out, fd)
	}
	return out, nil
}

// singleFilePatch wraps one diff.FilePatch so we can render its unified diff
// independently of the parent *object.Patch.
type singleFilePatch struct {
	fp      diff.FilePatch
	message string
}

func (s singleFilePatch) FilePatches() []diff.FilePatch { return []diff.FilePatch{s.fp} }
func (s singleFilePatch) Message() string               { return s.message }

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
