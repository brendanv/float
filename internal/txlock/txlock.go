package txlock

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/brendanv/float/internal/gitsnap"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/slogctx"
)

// ErrNoChanges is a sentinel a caller's fn can return from DoWith/Do to signal
// that it made no modifications to any snapshotted file. DoWith treats this as
// success but skips hledger check, generation bump, and gitsnap commit — the
// mutation is a genuine no-op, not a failure.
var ErrNoChanges = errors.New("txlock: no changes made")

// TxLock serializes all journal mutations and enforces the write protocol:
// snapshot → write → hledger check → (revert on failure | bump generation on success).
type TxLock struct {
	mu      sync.Mutex
	dataDir string
	client  *hledger.Client
	gen     atomic.Uint64
	snap    *gitsnap.Repo
}

// New creates a TxLock for the given data directory and hledger client.
func New(dataDir string, client *hledger.Client) *TxLock {
	return &TxLock{dataDir: dataDir, client: client}
}

// Generation returns the current generation counter value.
// The cache reads this to detect invalidation after writes.
func (l *TxLock) Generation() uint64 {
	return l.gen.Load()
}

// Do executes the write protocol for journal-only mutations.
// It is equivalent to DoWith with no extra paths declared.
func (l *TxLock) Do(ctx context.Context, msg string, fn func() error) error {
	return l.DoWith(ctx, msg, nil, fn)
}

// DoWith executes the write protocol:
//  1. Acquire mutex
//  2. Snapshot all *.journal files in dataDir and any explicitly declared extra paths
//  3. Execute fn (caller writes files)
//  4. Run hledger check to validate the journal
//  5. On fn error or check failure: revert all snapshotted files, delete any
//     new journal files or new extra files fn created, return error
//  6. On success: bump generation counter, optionally gitsnap commit
//
// extraPaths contains absolute paths to non-journal files that fn may create,
// modify, or delete. On failure they are reverted along with the journal files.
// Paths that do not exist before fn are removed on revert if fn created them.
func (l *TxLock) DoWith(ctx context.Context, msg string, extraPaths []string, fn func() error) error {
	logger := slogctx.FromContext(ctx)
	l.mu.Lock()
	defer l.mu.Unlock()

	snap, err := snapshotFiles(l.dataDir, extraPaths)
	if err != nil {
		return fmt.Errorf("txlock: snapshot: %w", err)
	}
	logger.Debug("txlock: snapshotted files", "file_count", len(snap.present)+len(snap.absentExtras))

	if err := fn(); err != nil {
		if errors.Is(err, ErrNoChanges) {
			logger.Debug("txlock: fn reported no changes; skipping check/bump/commit")
			return nil
		}
		logger.Debug("txlock: reverting snapshot after write failure", "error", err)
		revertErr := snap.revert(l.dataDir)
		// Bump the generation even though the write failed: a concurrent read
		// may have observed the intermediate file state and would otherwise
		// cache its result under the unchanged generation, serving poisoned
		// data until the next successful write.
		l.gen.Add(1)
		if revertErr != nil {
			return fmt.Errorf("txlock: fn failed (%w) and revert also failed: %v", err, revertErr)
		}
		return err
	}

	if err := l.client.Check(ctx); err != nil {
		logger.Debug("txlock: reverting snapshot after check failure", "error", err)
		revertErr := snap.revert(l.dataDir)
		// See the comment on the fn-failure path: invalidate caches that may
		// have loaded the intermediate (now reverted) state.
		l.gen.Add(1)
		if revertErr != nil {
			return fmt.Errorf("txlock: check failed (%w) and revert also failed: %v", err, revertErr)
		}
		return err
	}

	gen := l.gen.Add(1)
	logger.Info("txlock: write committed", "generation", gen)
	if l.snap != nil {
		if snapErr := l.snap.Commit(ctx, msg); snapErr != nil {
			logger.Warn("txlock: gitsnap commit failed", "error", snapErr)
		}
	}
	return nil
}

// DoConfig serializes a mutation to non-journal files (config.toml,
// rules.json, templates.json, bank profile rules files under rules/). It
// snapshots and reverts the declared paths on failure but skips `hledger
// check` and does NOT bump the generation counter — none of these mutations
// can change any hledger query result. Gitsnap still commits on success.
//
// paths must not include any *.journal file; DoConfig refuses those so a
// misclassified call site fails loudly instead of silently skipping
// validation and cache invalidation for a journal-affecting write — use
// DoWith for those.
func (l *TxLock) DoConfig(ctx context.Context, msg string, paths []string, fn func() error) error {
	logger := slogctx.FromContext(ctx)
	for _, p := range paths {
		if strings.HasSuffix(p, ".journal") {
			return fmt.Errorf("txlock: DoConfig: refusing journal path %s (use DoWith)", p)
		}
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	snap, err := snapshotConfigPaths(paths)
	if err != nil {
		return fmt.Errorf("txlock: snapshot: %w", err)
	}
	logger.Debug("txlock: snapshotted config files", "file_count", len(snap.present)+len(snap.absentExtras))

	if err := fn(); err != nil {
		if errors.Is(err, ErrNoChanges) {
			logger.Debug("txlock: fn reported no changes; skipping commit")
			return nil
		}
		logger.Debug("txlock: reverting config snapshot after write failure", "error", err)
		if revertErr := snap.revert(); revertErr != nil {
			return fmt.Errorf("txlock: fn failed (%w) and revert also failed: %v", err, revertErr)
		}
		return err
	}

	logger.Info("txlock: config write committed")
	if l.snap != nil {
		if snapErr := l.snap.Commit(ctx, msg); snapErr != nil {
			logger.Warn("txlock: gitsnap commit failed", "error", snapErr)
		}
	}
	return nil
}

func (l *TxLock) SetSnap(snap *gitsnap.Repo) {
	l.snap = snap
}

func (l *TxLock) BumpGeneration() uint64 {
	return l.gen.Add(1)
}

// configSnapshot captures pre-write state for a fixed list of non-journal
// paths, used by DoConfig. Unlike txSnapshot it does not walk dataDir for
// *.journal files, since DoConfig mutations never touch those.
type configSnapshot struct {
	present      map[string][]byte
	absentExtras []string
}

func snapshotConfigPaths(paths []string) (*configSnapshot, error) {
	s := &configSnapshot{present: make(map[string][]byte)}
	for _, p := range paths {
		content, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			s.absentExtras = append(s.absentExtras, p)
		} else if err != nil {
			return nil, fmt.Errorf("snapshot: read %s: %w", p, err)
		} else {
			s.present[p] = content
		}
	}
	return s, nil
}

func (s *configSnapshot) revert() error {
	for path, content := range s.present {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("revert: recreate dir for %s: %w", path, err)
		}
		if err := os.WriteFile(path, content, 0644); err != nil {
			return fmt.Errorf("revert: restore %s: %w", path, err)
		}
	}
	for _, p := range s.absentExtras {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("revert: delete new file %s: %w", p, err)
		}
	}
	return nil
}

// txSnapshot captures pre-write file state for reverting on failure.
type txSnapshot struct {
	// present maps absolute path → file content for files that existed before fn.
	// Covers *.journal files (discovered by walk) plus extra paths that existed.
	present map[string][]byte
	// absentExtras lists declared extra paths that did not exist before fn.
	// Any that fn creates are removed on revert.
	absentExtras []string
}

// snapshotFiles builds a txSnapshot covering all *.journal files under dataDir
// and any explicitly listed extra paths.
func snapshotFiles(dataDir string, extraPaths []string) (*txSnapshot, error) {
	s := &txSnapshot{present: make(map[string][]byte)}
	err := filepath.WalkDir(dataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".journal") {
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return fmt.Errorf("snapshot: read %s: %w", path, readErr)
			}
			s.present[path] = content
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, p := range extraPaths {
		if _, already := s.present[p]; already {
			continue // already captured (e.g. a .journal path passed explicitly)
		}
		content, readErr := os.ReadFile(p)
		if os.IsNotExist(readErr) {
			s.absentExtras = append(s.absentExtras, p)
		} else if readErr != nil {
			return nil, fmt.Errorf("snapshot: read %s: %w", p, readErr)
		} else {
			s.present[p] = content
		}
	}
	return s, nil
}

// revert restores all snapshotted files and removes any new files fn created:
//  1. Restore every file in present (handles modified and deleted files)
//  2. Delete any *.journal files fn created (not in present)
//  3. Delete any declared-absent extra files fn created
func (s *txSnapshot) revert(dataDir string) error {
	for path, content := range s.present {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("revert: recreate dir for %s: %w", path, err)
		}
		if err := os.WriteFile(path, content, 0644); err != nil {
			return fmt.Errorf("revert: restore %s: %w", path, err)
		}
	}
	if err := filepath.WalkDir(dataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".journal") {
			if _, existed := s.present[path]; !existed {
				if err := os.Remove(path); err != nil {
					return fmt.Errorf("revert: delete new journal %s: %w", path, err)
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}
	for _, p := range s.absentExtras {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("revert: delete new extra file %s: %w", p, err)
		}
	}
	return nil
}
