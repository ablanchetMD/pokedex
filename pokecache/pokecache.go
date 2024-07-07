package pokecache

import (
	"errors"
	"time"
	"sync"
	

)

type cacheEntry struct {
	createdAt time.Time
	data      []byte
}

type Cache struct {
	entries map[string]cacheEntry
	mu 	sync.Mutex
}

func (c *Cache) Add(key string, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cacheEntry{
		createdAt: time.Now(),
		data:      data,
	}
	return nil
}

func (c *Cache) Get(key string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil, errors.New("key not found")
	}
	return entry.data, nil
}

func (c *Cache) ReapLoop() {
	for {
		time.Sleep(5 * time.Minute)
		c.Reap()
	}
}

func (c *Cache) Reap() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, entry := range c.entries {
		if time.Since(entry.createdAt) > 5*time.Minute {
			delete(c.entries, key)
		}
	}
}

func NewCache() *Cache {
	c := &Cache{
		entries: make(map[string]cacheEntry),
	}
	go c.ReapLoop()
	return c
}
