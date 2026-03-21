package cache

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/singleflight"
)

// Cache is a generation-aware cache keyed by string.
// Entries are grouped by generation; when the generation advances, stale
// tiers are dropped on the next write. The cache grows without bound within
// a generation — query keys are few in practice.
// Cache is safe for concurrent use.
type Cache[T any] struct {
	mu    sync.RWMutex
	gen   func() uint64
	tiers map[uint64]map[string]T
	sf    singleflight.Group
}

// New returns a Cache whose entries are invalidated when gen() advances.
// gen must return the current generation counter (e.g. txlock.TxLock.Generation).
func New[T any](gen func() uint64) *Cache[T] {
	return &Cache[T]{
		gen:   gen,
		tiers: make(map[uint64]map[string]T),
	}
}

// Get returns the cached value for key at the current generation, calling load
// on a miss. Concurrent calls with the same key at the same generation share a
// single load invocation via singleflight.
func (c *Cache[T]) Get(ctx context.Context, key string, load func(context.Context) (T, error)) (T, error) {
	currentGen := c.gen()
	if val, ok := c.lookup(key, currentGen); ok {
		return val, nil
	}
	// Miss: deduplicate concurrent loads for the same key+generation.
	// The singleflight key is generation-scoped so callers at different
	// generations never share a result.
	sfKey := fmt.Sprintf("%s@gen%d", key, currentGen)
	v, err, _ := c.sf.Do(sfKey, func() (any, error) {
		loaded, err := load(ctx)
		if err != nil {
			return nil, err
		}
		c.store(key, loaded, currentGen)
		return loaded, nil
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return v.(T), nil
}

func (c *Cache[T]) lookup(key string, gen uint64) (T, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if tier, ok := c.tiers[gen]; ok {
		val, ok := tier[key]
		return val, ok
	}
	var zero T
	return zero, false
}

func (c *Cache[T]) store(key string, val T, currentGen uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	latestGen := c.gen()
	// Drop stale tiers from previous generations.
	for g := range c.tiers {
		if g != latestGen {
			delete(c.tiers, g)
		}
	}
	// Only store if the generation hasn't advanced past what we loaded for.
	// If it has, return the value to this caller but don't pollute the cache.
	if latestGen == currentGen {
		if c.tiers[currentGen] == nil {
			c.tiers[currentGen] = make(map[string]T)
		}
		c.tiers[currentGen][key] = val
	}
}
