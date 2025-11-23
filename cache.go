package fusefs

import (
	"container/list"
	"sync"
	"time"
)

// lruCache implements a thread-safe LRU cache with TTL support.
//
// The cache evicts entries in two scenarios:
// 1. When the cache size exceeds maxSize (LRU eviction)
// 2. When entries exceed their TTL (lazy expiration on access)
//
// This implementation uses a doubly-linked list for O(1) LRU operations
// and a map for O(1) lookups.
type lruCache struct {
	mu       sync.RWMutex
	maxSize  int
	ttl      time.Duration
	items    map[string]*list.Element
	lruList  *list.List
	hits     uint64
	misses   uint64
	evictions uint64
}

// lruEntry represents a single cache entry
type lruEntry struct {
	key       string
	value     interface{}
	timestamp time.Time
}

// newLRUCache creates a new LRU cache with the specified maximum size and TTL.
// If maxSize is 0, the cache has unlimited size (not recommended).
// If ttl is 0, entries never expire based on time.
func newLRUCache(maxSize int, ttl time.Duration) *lruCache {
	return &lruCache{
		maxSize: maxSize,
		ttl:     ttl,
		items:   make(map[string]*list.Element),
		lruList: list.New(),
	}
}

// Get retrieves a value from the cache.
// Returns (value, true) if found and not expired, (nil, false) otherwise.
func (c *lruCache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, exists := c.items[key]
	if !exists {
		c.misses++
		return nil, false
	}

	entry := elem.Value.(*lruEntry)

	// Check if entry has expired
	if c.ttl > 0 && time.Since(entry.timestamp) > c.ttl {
		c.remove(key, elem)
		c.misses++
		return nil, false
	}

	// Move to front (most recently used)
	c.lruList.MoveToFront(elem)
	c.hits++
	return entry.value, true
}

// Put adds or updates a value in the cache.
func (c *lruCache) Put(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If key already exists, update it
	if elem, exists := c.items[key]; exists {
		entry := elem.Value.(*lruEntry)
		entry.value = value
		entry.timestamp = time.Now()
		c.lruList.MoveToFront(elem)
		return
	}

	// Add new entry
	entry := &lruEntry{
		key:       key,
		value:     value,
		timestamp: time.Now(),
	}
	elem := c.lruList.PushFront(entry)
	c.items[key] = elem

	// Evict if over capacity
	if c.maxSize > 0 && c.lruList.Len() > c.maxSize {
		c.evictOldest()
	}
}

// Delete removes a key from the cache.
func (c *lruCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, exists := c.items[key]; exists {
		c.remove(key, elem)
	}
}

// Clear removes all entries from the cache.
func (c *lruCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.lruList = list.New()
}

// Len returns the current number of entries in the cache.
func (c *lruCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lruList.Len()
}

// Stats returns cache statistics.
func (c *lruCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	return CacheStats{
		Size:      c.lruList.Len(),
		MaxSize:   c.maxSize,
		Hits:      c.hits,
		Misses:    c.misses,
		Evictions: c.evictions,
		HitRate:   hitRate,
	}
}

// evictOldest removes the least recently used entry (assumes lock is held)
func (c *lruCache) evictOldest() {
	elem := c.lruList.Back()
	if elem == nil {
		return
	}

	entry := elem.Value.(*lruEntry)
	c.remove(entry.key, elem)
	c.evictions++
}

// remove deletes an entry from the cache (assumes lock is held)
func (c *lruCache) remove(key string, elem *list.Element) {
	c.lruList.Remove(elem)
	delete(c.items, key)
}

// CacheStats contains cache performance statistics
type CacheStats struct {
	Size      int     // Current number of entries
	MaxSize   int     // Maximum number of entries
	Hits      uint64  // Number of cache hits
	Misses    uint64  // Number of cache misses
	Evictions uint64  // Number of evictions
	HitRate   float64 // Hit rate (hits / (hits + misses))
}
