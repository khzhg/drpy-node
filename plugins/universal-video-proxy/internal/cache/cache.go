package cache

import (
	"sync"
	"time"
	lru "github.com/hashicorp/golang-lru/v2"
)

type CacheEntry struct {
	Data      []byte
	ExpiresAt time.Time
}

type Cache struct {
	lru     *lru.Cache[string, *CacheEntry]
	ttl     time.Duration
	enabled bool
	mu      sync.RWMutex
}

func New(maxEntries int, ttlSeconds int, enabled bool) *Cache {
	if !enabled || maxEntries <= 0 {
		return &Cache{enabled: false}
	}

	cache, _ := lru.New[string, *CacheEntry](maxEntries)
	return &Cache{
		lru:     cache,
		ttl:     time.Duration(ttlSeconds) * time.Second,
		enabled: enabled,
	}
}

func (c *Cache) Get(key string) ([]byte, bool) {
	if !c.enabled {
		return nil, false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.lru.Get(key)
	if !exists || time.Now().After(entry.ExpiresAt) {
		if exists {
			c.lru.Remove(key)
		}
		return nil, false
	}

	return entry.Data, true
}

func (c *Cache) Set(key string, data []byte) {
	if !c.enabled {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry := &CacheEntry{
		Data:      data,
		ExpiresAt: time.Now().Add(c.ttl),
	}
	c.lru.Add(key, entry)
}

func (c *Cache) IsEnabled() bool {
	return c.enabled
}