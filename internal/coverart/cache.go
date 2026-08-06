package coverart

import (
	"sync"
	"time"
)

// trackCache is a small, bounded, TTL-based cache keyed by a normalized
// "artist\x00title" string. It exists to avoid hammering the iTunes API when
// the same track airs again within the TTL.
type trackCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	max     int
	entries map[string]trackEntry
}

type trackEntry struct {
	url     string
	expires time.Time
}

func newTrackCache(ttl time.Duration) *trackCache {
	return &trackCache{
		ttl:     ttl,
		max:     1000,
		entries: make(map[string]trackEntry),
	}
}

func (c *trackCache) get(key string) (string, bool) {
	if key == "" {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return "", false
	}
	if time.Now().After(entry.expires) {
		delete(c.entries, key)
		return "", false
	}
	return entry.url, true
}

func (c *trackCache) set(key, url string) {
	if key == "" || url == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.max {
		// Simple bound: when full, drop everything. Resolution is cheap and
		// the cache is only an anti-hammering guard.
		c.entries = make(map[string]trackEntry)
	}
	c.entries[key] = trackEntry{
		url:     url,
		expires: time.Now().Add(c.ttl),
	}
}
