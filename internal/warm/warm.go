// Package warm proactively populates the query cache so that steady-state
// browsing hits warm entries instead of paying for a full-journal hledger
// parse on the first request after startup or after a write.
package warm

import (
	"context"
	"sync"
	"time"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/slogctx"
)

// Entry is one warm-able dataset. Load should call the same cached* helper
// the RPC handlers use, so a warm load and a racing user request share a
// single hledger invocation via the cache's singleflight.
type Entry struct {
	Name string
	Load func(ctx context.Context) error
}

// Warmer runs a fixed, ordered set of Entries, one at a time, aborting the
// pass early if the generation moves again mid-pass (a newer write is on the
// way, and its own pass will supersede this one).
type Warmer struct {
	gen       func() uint64
	entriesFn func() []Entry
	debounce  time.Duration

	runMu sync.Mutex

	timerMu sync.Mutex
	timer   *time.Timer
}

// New returns a Warmer. gen reads the current cache generation; entriesFn is
// called fresh at the start of every pass (so a dynamic entry set, such as a
// recently-used-accounts LRU, reflects the latest state rather than whatever
// it was at Warmer construction time); debounce coalesces bursts of writes
// (imports, apply-rules) into a single warm pass after the burst settles.
func New(gen func() uint64, entriesFn func() []Entry, debounce time.Duration) *Warmer {
	return &Warmer{gen: gen, entriesFn: entriesFn, debounce: debounce}
}

// Start runs one warm pass immediately in the background. Intended for
// startup, called after the server begins listening so warming never delays
// boot.
func (w *Warmer) Start(ctx context.Context) {
	go w.run(ctx)
}

// Trigger schedules a debounced warm pass. It is meant to be registered as a
// txlock.TxLock.OnCommit hook: each call resets the debounce timer, so a
// burst of writes results in one pass after the last one settles.
func (w *Warmer) Trigger(_ uint64) {
	w.timerMu.Lock()
	defer w.timerMu.Unlock()
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(w.debounce, func() {
		w.run(context.Background())
	})
}

// run executes entries in order, one at a time, stopping if the generation
// advances past the one observed at the start of the pass. Concurrent calls
// to run (e.g. Start racing a debounced Trigger) are serialized: only one
// pass executes at a time, matching the concurrency-1 requirement for warm
// loads.
func (w *Warmer) run(ctx context.Context) {
	w.runMu.Lock()
	defer w.runMu.Unlock()

	// Warm loads never queue ahead of an interactive request for an hledger
	// concurrency slot; see hledger.WithLowPriority.
	ctx = hledger.WithLowPriority(ctx)
	logger := slogctx.FromContext(ctx)
	startGen := w.gen()
	entries := w.entriesFn()
	for _, e := range entries {
		if w.gen() != startGen {
			logger.Debug("warm: aborting pass, generation moved", "entry", e.Name)
			return
		}
		if err := e.Load(ctx); err != nil {
			logger.Warn("warm: entry failed", "entry", e.Name, "error", err)
		}
	}
	logger.Debug("warm: pass complete", "generation", startGen, "entries", len(entries))
}
