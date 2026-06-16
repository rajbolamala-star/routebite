package cache

import (
	"sync"
	"time"
)

// TTL is a tiny in-memory cache with per-entry expiry. Used to protect the
// Yelp + OSRM quotas — repeated identical requests are common when a driver
// re-runs the same search.
//
// RedisCache in this package is the shared JSON cache; TTL remains a small
// process-local byte cache for very hot restaurant searches.
type TTL struct {
	mu      sync.RWMutex
	entries map[string]entry
	ttl     time.Duration
}

type entry struct {
	value     []byte
	expiresAt time.Time
}

func New(ttl time.Duration) *TTL {
	return &TTL{
		entries: make(map[string]entry),
		ttl:     ttl,
	}
}

// Get returns the cached value and whether the key was a fresh hit.
func (c *TTL) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e.value, true
}

// Set stores a value with the configured TTL.
func (c *TTL) Set(key string, value []byte) {
	c.mu.Lock()
	c.entries[key] = entry{value: value, expiresAt: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}

// PurgeExpired removes expired entries. Call periodically.
func (c *TTL) PurgeExpired() {
	now := time.Now()
	c.mu.Lock()
	for k, e := range c.entries {
		if now.After(e.expiresAt) {
			delete(c.entries, k)
		}
	}
	c.mu.Unlock()
}
