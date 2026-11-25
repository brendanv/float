package txlock

import (
	"context"
	"sync"

	"github.com/brendanv/float/internal/slogctx"
)

// Receipt records the snapshot made by the writes performed under a context,
// so a caller (the RPC layer) can offer to undo them. Attach one with
// WithReceipt before the writes; txlock fills it in after each commit.
type Receipt struct {
	mu       sync.Mutex
	writes   int
	snapshot string
	undoable bool
}

type receiptKey struct{}

// WithReceipt returns a context that records txlock writes into the returned
// Receipt.
func WithReceipt(ctx context.Context) (context.Context, *Receipt) {
	r := &Receipt{}
	return context.WithValue(ctx, receiptKey{}, r), r
}

func receiptFrom(ctx context.Context) *Receipt {
	r, _ := ctx.Value(receiptKey{}).(*Receipt)
	return r
}

func (r *Receipt) record(snapshot string, undoable bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writes++
	r.snapshot = snapshot
	r.undoable = undoable && snapshot != ""
}

// Undoable returns the snapshot that undoing the recorded writes would revert.
// It is set only when exactly one write committed under the context, its
// snapshot commit succeeded, and every file it declared is covered by
// snapshots (a write touching config.toml can't be undone from git).
func (r *Receipt) Undoable() (snapshot string, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.writes != 1 || !r.undoable {
		return "", false
	}
	return r.snapshot, true
}

// commitSnap commits a successful write to the snapshot repo (when one is
// configured) and records it on the context's Receipt, if any. paths are the
// non-journal files the write declared.
func (l *TxLock) commitSnap(ctx context.Context, msg string, paths []string) {
	var hash string
	if l.snap != nil {
		h, err := l.snap.CommitHash(ctx, msg)
		if err != nil {
			slogctx.FromContext(ctx).Warn("txlock: gitsnap commit failed", "error", err)
		}
		hash = h
	}
	r := receiptFrom(ctx)
	if r == nil {
		return
	}
	undoable := hash != ""
	for _, p := range paths {
		if undoable && !l.snap.Tracks(p) {
			undoable = false
		}
	}
	r.record(hash, undoable)
}
