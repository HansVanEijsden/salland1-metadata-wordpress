package cache

import (
	"sync"
)

type Cache struct {
	mu      sync.RWMutex
	data    interface{}
	excerpt string
	valid   bool
	hasData bool
}

func New() *Cache {
	return &Cache{
		hasData: false,
		valid:   false,
	}
}

func (c *Cache) Set(data interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = data
	c.hasData = true
	c.valid = true
}

func (c *Cache) Get() (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.hasData || !c.valid {
		return nil, false
	}
	return c.data, true
}

// SetExcerpt stores the current programme excerpt (may be empty).
func (c *Cache) SetExcerpt(excerpt string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.excerpt = excerpt
}

// GetExcerpt returns the current programme excerpt (may be empty).
func (c *Cache) GetExcerpt() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.excerpt
}

func (c *Cache) IsValid() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hasData && c.valid
}

func (c *Cache) MarkInvalid() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.valid = false
}
