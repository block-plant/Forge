package hosting

import (
	"sync"
	"strings"
)

// FileCache is an in-memory LRU file cache for the hosting server.
// It caches file content to avoid repeated disk reads for hot files.
type FileCache struct {
	maxEntries    int
	maxFileSize   int64

	// entries maps cache key → *cacheEntry
	entries map[string]*cacheEntry
	// order tracks access order for LRU eviction (doubly linked list).
	head *cacheEntry
	tail *cacheEntry
	mu   sync.Mutex
}

// cacheEntry is a single cached file with LRU pointers.
type cacheEntry struct {
	key   string
	file  *ServedFile
	prev  *cacheEntry
	next  *cacheEntry
}

// NewFileCache creates a new LRU file cache.
func NewFileCache(maxEntries int, maxFileSize int64) *FileCache {
	if maxEntries <= 0 {
		maxEntries = 1000
	}
	if maxFileSize <= 0 {
		maxFileSize = 1024 * 1024 // 1MB default
	}

	return &FileCache{
		maxEntries:  maxEntries,
		maxFileSize: maxFileSize,
		entries:     make(map[string]*cacheEntry, maxEntries),
	}
}

// Get retrieves a file from the cache, or nil if not cached.
// Moves the entry to the front (most recently used).
func (c *FileCache) Get(key string) *ServedFile {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil
	}

	// Move to front (most recently used)
	c.moveToFront(entry)

	return entry.file
}

// Set adds a file to the cache. If the cache is full, evicts the
// least recently used entry.
func (c *FileCache) Set(key string, file *ServedFile) {
	if file == nil {
		return
	}

	// Don't cache files larger than the max size
	if file.Size > c.maxFileSize {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Update existing entry
	if entry, ok := c.entries[key]; ok {
		entry.file = file
		c.moveToFront(entry)
		return
	}

	// Evict if at capacity
	for len(c.entries) >= c.maxEntries {
		c.evictLRU()
	}

	// Add new entry at front
	entry := &cacheEntry{
		key:  key,
		file: file,
	}
	c.entries[key] = entry
	c.addToFront(entry)
}

// Invalidate removes all entries for a given site (prefix match on key).
func (c *FileCache) Invalidate(sitePrefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	prefix := sitePrefix + ":"
	for key, entry := range c.entries {
		if strings.HasPrefix(key, prefix) {
			c.removeEntry(entry)
			delete(c.entries, key)
		}
	}
}

// Clear removes all entries from the cache.
func (c *FileCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*cacheEntry, c.maxEntries)
	c.head = nil
	c.tail = nil
}

// Size returns the number of cached entries.
func (c *FileCache) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// ── Doubly Linked List Operations ──

// addToFront adds an entry to the front of the list.
func (c *FileCache) addToFront(entry *cacheEntry) {
	entry.prev = nil
	entry.next = c.head

	if c.head != nil {
		c.head.prev = entry
	}
	c.head = entry

	if c.tail == nil {
		c.tail = entry
	}
}

// removeEntry removes an entry from the linked list.
func (c *FileCache) removeEntry(entry *cacheEntry) {
	if entry.prev != nil {
		entry.prev.next = entry.next
	} else {
		c.head = entry.next
	}

	if entry.next != nil {
		entry.next.prev = entry.prev
	} else {
		c.tail = entry.prev
	}

	entry.prev = nil
	entry.next = nil
}

// moveToFront moves an existing entry to the front.
func (c *FileCache) moveToFront(entry *cacheEntry) {
	if entry == c.head {
		return // Already at front
	}
	c.removeEntry(entry)
	c.addToFront(entry)
}

// evictLRU removes the least recently used entry (tail).
func (c *FileCache) evictLRU() {
	if c.tail == nil {
		return
	}

	victim := c.tail
	c.removeEntry(victim)
	delete(c.entries, victim.key)
}
