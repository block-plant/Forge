// Package hosting — cdn.go implements an in-memory CDN caching layer
// for static site hosting. It acts as a high-performance read-through
// cache with Gzip compression and ETag validation.
package hosting

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// CDN is the in-memory content delivery layer that sits in front of
// the disk-based hosting server. It caches compressed responses and
// handles conditional requests (ETag/If-None-Match).
type CDN struct {
	mu       sync.RWMutex
	entries  map[string]*CDNEntry
	maxItems int
	maxBytes int64
	usedBytes int64
}

// CDNEntry is a single cached response.
type CDNEntry struct {
	Key          string    `json:"key"`
	ContentType  string    `json:"content_type"`
	RawBody      []byte    `json:"-"`
	GzipBody     []byte    `json:"-"`
	ETag         string    `json:"etag"`
	LastModified time.Time `json:"last_modified"`
	HitCount     int64     `json:"hit_count"`
	CreatedAt    time.Time `json:"created_at"`
	Size         int64     `json:"size"`
}

// NewCDN creates a new CDN cache layer.
func NewCDN(maxItems int, maxBytes int64) *CDN {
	return &CDN{
		entries:  make(map[string]*CDNEntry),
		maxItems: maxItems,
		maxBytes: maxBytes,
	}
}

// Get retrieves a cached entry. Returns nil on cache miss.
func (c *CDN) Get(key string) *CDNEntry {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return nil
	}

	// Atomically increment hit count
	c.mu.Lock()
	entry.HitCount++
	c.mu.Unlock()

	return entry
}

// Put adds or updates a cache entry. Automatically compresses with Gzip.
func (c *CDN) Put(key, contentType string, body []byte, modTime time.Time) *CDNEntry {
	// Compute ETag from content hash
	hash := sha256.Sum256(body)
	etag := fmt.Sprintf(`"%x"`, hash[:8])

	// Gzip compress
	var gzipBuf bytes.Buffer
	gz, _ := gzip.NewWriterLevel(&gzipBuf, gzip.BestCompression)
	gz.Write(body)
	gz.Close()
	gzipBody := gzipBuf.Bytes()

	entry := &CDNEntry{
		Key:          key,
		ContentType:  contentType,
		RawBody:      body,
		GzipBody:     gzipBody,
		ETag:         etag,
		LastModified: modTime,
		CreatedAt:    time.Now(),
		Size:         int64(len(body)),
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict if at capacity
	if len(c.entries) >= c.maxItems {
		c.evictLRU()
	}

	// Check byte budget
	if c.usedBytes+entry.Size > c.maxBytes {
		c.evictLRU()
	}

	// Remove old entry size if replacing
	if old, exists := c.entries[key]; exists {
		c.usedBytes -= old.Size
	}

	c.entries[key] = entry
	c.usedBytes += entry.Size

	return entry
}

// Invalidate removes a specific entry from the cache.
func (c *CDN) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.entries[key]; ok {
		c.usedBytes -= entry.Size
		delete(c.entries, key)
	}
}

// InvalidatePrefix removes all entries whose key starts with prefix.
func (c *CDN) InvalidatePrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, entry := range c.entries {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			c.usedBytes -= entry.Size
			delete(c.entries, key)
		}
	}
}

// Flush clears the entire cache.
func (c *CDN) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*CDNEntry)
	c.usedBytes = 0
}

// Stats returns CDN cache statistics.
func (c *CDN) Stats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	totalHits := int64(0)
	for _, e := range c.entries {
		totalHits += e.HitCount
	}

	return map[string]interface{}{
		"entries":    len(c.entries),
		"max_items":  c.maxItems,
		"used_bytes": c.usedBytes,
		"max_bytes":  c.maxBytes,
		"total_hits": totalHits,
	}
}

// evictLRU removes the least-recently-hit entry. Must be called with lock held.
func (c *CDN) evictLRU() {
	var lowestKey string
	var lowestHits int64 = 1<<63 - 1
	var lowestTime time.Time

	for key, entry := range c.entries {
		if entry.HitCount < lowestHits ||
			(entry.HitCount == lowestHits && entry.CreatedAt.Before(lowestTime)) {
			lowestKey = key
			lowestHits = entry.HitCount
			lowestTime = entry.CreatedAt
		}
	}

	if lowestKey != "" {
		c.usedBytes -= c.entries[lowestKey].Size
		delete(c.entries, lowestKey)
	}
}
