package txlock

import (
	"context"
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

// Do executes the write protocol:
//  1. Acquire mutex
//  2. Snapshot all *.journal files and known data files (config.toml, rules.json) in dataDir
//  3. Execute fn (caller writes files)
//  4. Run hledger check to validate the journal
//  5. On check failure: revert all snapshotted files, return error
//  6. On success: bump generation counter
func (l *TxLock) Do(ctx context.Context, msg string, fn func() error) error {
	logger := slogctx.FromContext(ctx)
	l.mu.Lock()
	defer l.mu.Unlock()

	snap, err := snapshotDataFiles(l.dataDir)
	if err != nil {
		return fmt.Errorf("txlock: snapshot: %w", err)
	}
	logger.Debug("txlock: snapshotted data files", "file_count", len(snap))

	if err := fn(); err != nil {
		logger.Debug("txlock: reverting snapshot after write failure", "error", err)
		if revertErr := revertFromSnapshot(l.dataDir, snap); revertErr != nil {
			return fmt.Errorf("txlock: fn failed (%w) and revert also failed: %v", err, revertErr)
		}
		return err
	}

	if err := l.client.Check(ctx); err != nil {
		logger.Debug("txlock: reverting snapshot after check failure", "error", err)
		if revertErr := revertFromSnapshot(l.dataDir, snap); revertErr != nil {
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

func (l *TxLock) SetSnap(snap *gitsnap.Repo) {
	l.snap = snap
}

func (l *TxLock) BumpGeneration() uint64 {
	return l.gen.Add(1)
}

// knownDataFiles are non-journal files in the data directory that txlock
// snapshots and reverts alongside journal files. Callers that mutate these
// files inside Do() get the same atomic rollback guarantee as journal files.
var knownDataFiles = []string{"config.toml", "rules.json"}

// snapshotDataFiles records the content of every *.journal file under dataDir
// and each file listed in knownDataFiles (if it exists). The returned map is
// keyed by absolute path. Absent knownDataFiles entries are omitted from the
// map so that revertFromSnapshot can detect if fn() created them and delete them.
func snapshotDataFiles(dataDir string) (map[string][]byte, error) {
	snap := make(map[string][]byte)
	err := filepath.WalkDir(dataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".journal") {
			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("snapshot: read %s: %w", path, err)
			}
			snap[path] = content
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, name := range knownDataFiles {
		path := filepath.Join(dataDir, name)
		content, err := os.ReadFile(path)
		if err == nil {
			snap[path] = content
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("snapshot: read %s: %w", path, err)
		}
		// File absent: leave it out of snap so revert knows to delete it if fn() creates it.
	}
	return snap, nil
}

// revertFromSnapshot restores all snapshotted files to their pre-write state:
//  1. Restore every file in the snapshot (handles modified and deleted files)
//  2. Delete any *.journal files that fn created (not present in snapshot)
//  3. Delete any knownDataFiles that fn created (not present in snapshot)
func revertFromSnapshot(dataDir string, snap map[string][]byte) error {
	for path, content := range snap {
		if err := os.WriteFile(path, content, 0644); err != nil {
			return fmt.Errorf("revert: restore %s: %w", path, err)
		}
	}
	// Delete new journal files created by fn.
	if err := filepath.WalkDir(dataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".journal") {
			if _, existed := snap[path]; !existed {
				if err := os.Remove(path); err != nil {
					return fmt.Errorf("revert: delete new file %s: %w", path, err)
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}
	// Delete known data files that fn created from scratch.
	for _, name := range knownDataFiles {
		path := filepath.Join(dataDir, name)
		if _, existed := snap[path]; !existed {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("revert: delete new file %s: %w", path, err)
			}
		}
	}
	return nil
}
