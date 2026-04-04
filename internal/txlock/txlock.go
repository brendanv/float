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
//  2. Snapshot all *.journal files in dataDir (in-memory)
//  3. Execute fn (caller writes files)
//  4. Run hledger check to validate the journal
//  5. On check failure: revert all journal files from snapshot, return error
//  6. On success: bump generation counter
func (l *TxLock) Do(ctx context.Context, fn func() error) error {
	logger := slogctx.FromContext(ctx)
	l.mu.Lock()
	defer l.mu.Unlock()

	snap, err := snapshotJournalFiles(l.dataDir)
	if err != nil {
		return fmt.Errorf("txlock: snapshot: %w", err)
	}
	logger.Debug("txlock: snapshotted journal files", "file_count", len(snap))

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
		if snapErr := l.snap.Commit(ctx, "float: write"); snapErr != nil {
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

// snapshotJournalFiles records the content of every *.journal file under dataDir.
// The returned map is keyed by absolute path.
func snapshotJournalFiles(dataDir string) (map[string][]byte, error) {
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
	return snap, err
}

// revertFromSnapshot restores all journal files to their pre-write state:
//  1. Restore every file in the snapshot (handles modified and deleted files)
//  2. Delete any *.journal files that fn created (not present in snapshot)
func revertFromSnapshot(dataDir string, snap map[string][]byte) error {
	for path, content := range snap {
		if err := os.WriteFile(path, content, 0644); err != nil {
			return fmt.Errorf("revert: restore %s: %w", path, err)
		}
	}
	return filepath.WalkDir(dataDir, func(path string, d fs.DirEntry, err error) error {
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
	})
}
