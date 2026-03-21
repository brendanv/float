package cache_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/brendanv/float/internal/cache"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/txlock"
)

// fakeGen returns a gen counter and the corresponding func() uint64.
func fakeGen() (*atomic.Uint64, func() uint64) {
	var g atomic.Uint64
	return &g, g.Load
}

func TestCache_GetAndStore(t *testing.T) {
	gen, genFn := fakeGen()
	c := cache.New[string](genFn)

	calls := 0
	load := func(ctx context.Context) (string, error) {
		calls++
		return "hello", nil
	}

	// First call: miss, load called.
	v, err := c.Get(t.Context(), "key", load)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "hello" {
		t.Errorf("v = %q, want %q", v, "hello")
	}
	if calls != 1 {
		t.Errorf("load calls = %d, want 1", calls)
	}

	// Second call same generation: hit, load not called.
	v, err = c.Get(t.Context(), "key", load)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "hello" {
		t.Errorf("v = %q, want %q", v, "hello")
	}
	if calls != 1 {
		t.Errorf("load calls = %d, want 1 (cache should have hit)", calls)
	}

	// After generation bump: miss again, load called.
	gen.Add(1)
	v, err = c.Get(t.Context(), "key", load)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "hello" {
		t.Errorf("v = %q, want %q", v, "hello")
	}
	if calls != 2 {
		t.Errorf("load calls = %d, want 2 after generation bump", calls)
	}
}

func TestCache_LoadErrorNotCached(t *testing.T) {
	_, genFn := fakeGen()
	c := cache.New[string](genFn)

	calls := 0
	load := func(ctx context.Context) (string, error) {
		calls++
		if calls == 1 {
			return "", errors.New("transient error")
		}
		return "ok", nil
	}

	// First call: error.
	_, err := c.Get(t.Context(), "key", load)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Error was not cached; next call retries.
	v, err := c.Get(t.Context(), "key", load)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "ok" {
		t.Errorf("v = %q, want %q", v, "ok")
	}
	if calls != 2 {
		t.Errorf("load calls = %d, want 2", calls)
	}
}

func TestCache_GenerationInvalidation(t *testing.T) {
	gen, genFn := fakeGen()
	c := cache.New[string](genFn)

	tests := []struct {
		name     string
		bumpGen  bool
		wantCall bool
	}{
		{"first call is miss", false, true},
		{"second call same gen is hit", false, false},
		{"after bump is miss", true, true},
		{"after bump second call is hit", false, false},
	}

	calls := 0
	load := func(ctx context.Context) (string, error) {
		calls++
		return "v", nil
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.bumpGen {
				gen.Add(1)
			}
			prevCalls := calls
			_, err := c.Get(t.Context(), "key", load)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			called := calls > prevCalls
			if called != tt.wantCall {
				t.Errorf("load called = %v, want %v", called, tt.wantCall)
			}
		})
	}
}

func TestCache_Singleflight(t *testing.T) {
	_, genFn := fakeGen()
	c := cache.New[string](genFn)

	var loadCalls atomic.Int32
	load := func(ctx context.Context) (string, error) {
		loadCalls.Add(1)
		time.Sleep(20 * time.Millisecond) // simulate slow hledger call
		return "shared", nil
	}

	n := 10
	results := make([]string, n)
	errs := make([]error, n)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			results[i], errs[i] = c.Get(t.Context(), "key", load)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
		if results[i] != "shared" {
			t.Errorf("goroutine %d: result = %q, want %q", i, results[i], "shared")
		}
	}

	// Singleflight should collapse concurrent misses; load called at most a small number of times.
	if n := loadCalls.Load(); n > 2 {
		t.Errorf("load called %d times, want ≤2 (singleflight should deduplicate)", n)
	}
}

// Integration tests — require a real hledger binary.

func setupDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "accounts.journal"), []byte("account assets:checking\naccount income:salary\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte("include accounts.journal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCache_InvalidationAfterWrite(t *testing.T) {
	dir := setupDataDir(t)
	hl, err := hledger.New("hledger", filepath.Join(dir, "main.journal"))
	if err != nil {
		t.Skipf("hledger unavailable: %v", err)
	}
	lock := txlock.New(dir, hl)

	c := cache.New[any](lock.Generation)

	// Seed one transaction outside txlock (initial data, no generation bump needed).
	tx1 := journal.TransactionInput{
		Date:        time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		Description: "INITIAL DEPOSIT",
		Postings: []journal.PostingInput{
			{Account: "assets:checking", Amount: "$1000.00"},
			{Account: "income:salary"},
		},
	}
	if _, err := journal.AppendTransaction(t.Context(), hl, dir, tx1); err != nil {
		t.Fatalf("AppendTransaction: %v", err)
	}

	calls := 0
	load := func(ctx context.Context) (any, error) {
		calls++
		return hl.Transactions(ctx)
	}

	// First query: miss, populates cache.
	v1, err := c.Get(t.Context(), "transactions:", load)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	txns1 := v1.([]hledger.Transaction)
	if len(txns1) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(txns1))
	}
	if calls != 1 {
		t.Errorf("load calls = %d, want 1", calls)
	}

	// Second query: hit, load not called.
	_, err = c.Get(t.Context(), "transactions:", load)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if calls != 1 {
		t.Errorf("load calls = %d, want 1 (cache hit)", calls)
	}

	// Write a new transaction via txlock, bumping the generation.
	tx2 := journal.TransactionInput{
		Date:        time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
		Description: "GROCERY STORE",
		Postings: []journal.PostingInput{
			{Account: "assets:checking", Amount: "$-50.00"},
			{Account: "income:salary"},
		},
	}
	if err := lock.Do(t.Context(), func() error {
		_, err := journal.AppendTransaction(t.Context(), hl, dir, tx2)
		return err
	}); err != nil {
		t.Fatalf("txlock.Do: %v", err)
	}

	// Third query: cache is stale, load must be called and result must reflect the write.
	v3, err := c.Get(t.Context(), "transactions:", load)
	if err != nil {
		t.Fatalf("third Get: %v", err)
	}
	txns3 := v3.([]hledger.Transaction)
	if len(txns3) != 2 {
		t.Fatalf("expected 2 transactions after write, got %d", len(txns3))
	}
	if calls != 2 {
		t.Errorf("load calls = %d, want 2 (cache invalidated after write)", calls)
	}
}

// Benchmarks

func BenchmarkCache_Hit(b *testing.B) {
	_, genFn := fakeGen()
	c := cache.New[string](genFn)

	// Pre-populate the cache.
	c.Get(context.Background(), "key", func(ctx context.Context) (string, error) { //nolint:errcheck
		return "value", nil
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Get(context.Background(), "key", func(ctx context.Context) (string, error) { //nolint:errcheck
				return "value", nil
			})
		}
	})
}

func BenchmarkCache_Miss_WithHledger(b *testing.B) {
	dir := b.TempDir()
	mainJ := filepath.Join(dir, "main.journal")
	if err := os.WriteFile(filepath.Join(dir, "accounts.journal"), []byte("account assets:checking\naccount income:salary\n"), 0o644); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(mainJ, []byte("include accounts.journal\n\n2026-01-05 PAYROLL\n  assets:checking  $1000\n  income:salary\n"), 0o644); err != nil {
		b.Fatal(err)
	}

	hl, err := hledger.New("hledger", mainJ)
	if err != nil {
		b.Skipf("hledger unavailable: %v", err)
	}

	_, genFn := fakeGen()
	c := cache.New[any](genFn)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Unique key per iteration forces a cache miss and a real hledger call.
		key := fmt.Sprintf("transactions:bench%d", i)
		c.Get(context.Background(), key, func(ctx context.Context) (any, error) { //nolint:errcheck
			return hl.Transactions(ctx)
		})
	}
}

func BenchmarkCache_ConcurrentReads(b *testing.B) {
	_, genFn := fakeGen()
	c := cache.New[string](genFn)

	// Pre-populate.
	c.Get(context.Background(), "hot", func(ctx context.Context) (string, error) { //nolint:errcheck
		return "value", nil
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Get(context.Background(), "hot", func(ctx context.Context) (string, error) { //nolint:errcheck
				return "value", nil
			})
		}
	})
}
