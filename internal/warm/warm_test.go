package warm_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/brendanv/float/internal/warm"
)

func TestWarmer_Start_RunsAllEntriesInOrder(t *testing.T) {
	var gen atomic.Uint64
	var order []string
	done := make(chan struct{})

	entries := []warm.Entry{
		{Name: "a", Load: func(ctx context.Context) error { order = append(order, "a"); return nil }},
		{Name: "b", Load: func(ctx context.Context) error { order = append(order, "b"); close(done); return nil }},
	}
	w := warm.New(gen.Load, func() []warm.Entry { return entries }, 10*time.Millisecond)
	w.Start(t.Context())

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for warm pass")
	}
	if len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Errorf("entries ran in order %v, want [a b]", order)
	}
}

func TestWarmer_AbortsWhenGenerationMoves(t *testing.T) {
	var gen atomic.Uint64
	var secondRan atomic.Bool
	unblock := make(chan struct{})

	entries := []warm.Entry{
		{Name: "first", Load: func(ctx context.Context) error {
			gen.Add(1) // simulate a write landing mid-pass
			close(unblock)
			return nil
		}},
		{Name: "second", Load: func(ctx context.Context) error {
			secondRan.Store(true)
			return nil
		}},
	}
	w := warm.New(gen.Load, func() []warm.Entry { return entries }, 10*time.Millisecond)
	w.Start(t.Context())

	select {
	case <-unblock:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first entry")
	}
	// Give the (aborted) pass a moment to reach the second entry if it were
	// going to run it.
	time.Sleep(50 * time.Millisecond)
	if secondRan.Load() {
		t.Error("second entry ran after generation moved; pass should have aborted")
	}
}

func TestWarmer_TriggerDebouncesBursts(t *testing.T) {
	var gen atomic.Uint64
	var runs atomic.Int32
	entries := []warm.Entry{
		{Name: "only", Load: func(ctx context.Context) error { runs.Add(1); return nil }},
	}
	w := warm.New(gen.Load, func() []warm.Entry { return entries }, 100*time.Millisecond)

	// A burst of triggers within the debounce window should collapse to one pass.
	for i := 0; i < 5; i++ {
		w.Trigger(gen.Add(1))
		time.Sleep(10 * time.Millisecond)
	}

	time.Sleep(300 * time.Millisecond)
	if got := runs.Load(); got != 1 {
		t.Errorf("warm pass ran %d times, want 1", got)
	}
}

func TestWarmer_EntriesFnCalledFreshEachPass(t *testing.T) {
	var gen atomic.Uint64
	var callCount atomic.Int32
	w := warm.New(gen.Load, func() []warm.Entry {
		callCount.Add(1)
		return []warm.Entry{{Name: "e", Load: func(ctx context.Context) error { return nil }}}
	}, 10*time.Millisecond)

	w.Start(t.Context())
	time.Sleep(50 * time.Millisecond)
	w.Trigger(gen.Add(1))
	time.Sleep(200 * time.Millisecond)

	if got := callCount.Load(); got < 2 {
		t.Errorf("entriesFn called %d times, want at least 2 (one per pass)", got)
	}
}
