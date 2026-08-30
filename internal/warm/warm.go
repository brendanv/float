// Package warm proactively populates the query cache so that steady-state
// browsing hits warm entries instead of paying for a full-journal hledger
// parse on the first request after startup or after a write.
package warm

import (
	"context"
	"sync"
	"time"

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
	wait      func(ctx context.Context) error

	runMu sync.Mutex

	timerMu sync.Mutex
	timer   *time.Timer
}

// New returns a Warmer. gen reads the current cache generation; entriesFn is
// called fresh at the start of every pass (so a dynamic entry set, such as a
// recently-used-accounts LRU, reflects the latest state rather than whatever
// it was at Warmer construction time); debounce coalesces bursts of writes
// (imports, apply-rules) into a single warm pass after the burst settles.
// wait, typically hledger.Client.WaitUncontended, is called before each entry
// to wait for an idle moment on the hledger concurrency semaphore without
// taking a slot or joining its FIFO queue — so a warm load only *starts* once
// nothing interactive is waiting, but runs at normal priority once started.
// nil skips this politeness wait (used in tests with no real hledger client).
func New(gen func() uint64, entriesFn func() []Entry, debounce time.Duration, wait func(ctx context.Context) error) *Warmer {
	return &Warmer{gen: gen, entriesFn: entriesFn, debounce: debounce, wait: wait}
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

	logger := slogctx.FromContext(ctx)
	startGen := w.gen()
	entries := w.entriesFn()
	for _, e := range entries {
		if w.gen() != startGen {
			logger.Debug("warm: aborting pass, generation moved", "entry", e.Name)
			return
		}
		// Wait for an idle moment before starting this load, so it never
		// queues ahead of interactive requests to acquire a concurrency
		// slot. Once started, it runs at normal priority: any interactive
		// request that joins it via singleflight is never stuck behind
		// other queued low-priority work.
		if w.wait != nil {
			if err := w.wait(ctx); err != nil {
				logger.Debug("warm: aborting pass, wait failed", "entry", e.Name, "error", err)
				return
			}
		}
		if err := e.Load(ctx); err != nil {
			logger.Warn("warm: entry failed", "entry", e.Name, "error", err)
		}
	}
	logger.Debug("warm: pass complete", "generation", startGen, "entries", len(entries))
}
