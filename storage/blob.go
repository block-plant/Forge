package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ayushkunwarsingh/forge/logger"
)

// BlobStore is a content-addressed object store.
// Files are stored under their SHA256 hash, split into two-character prefix
// directories for filesystem scalability (e.g., ab/cd/abcdef1234...).
type BlobStore struct {
	dir string
	log *logger.Logger
	mu  sync.RWMutex

	// count tracks the number of blobs on disk.
	count int
}

// NewBlobStore creates a new blob store rooted at the given directory.
func NewBlobStore(dir string, log *logger.Logger) (*BlobStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("blob: failed to create directory: %w", err)
	}

	bs := &BlobStore{
		dir: dir,
		log: log,
	}

	// Count existing blobs
	bs.count = bs.countBlobs()

	return bs, nil
}

// Put stores data and returns its SHA256 hex hash.
// If the blob already exists (deduplication), it's a no-op.
func (bs *BlobStore) Put(data []byte) (string, error) {
	hash := computeHash(data)

	bs.mu.Lock()
	defer bs.mu.Unlock()

	blobPath := bs.pathFor(hash)

	// Check if blob already exists (deduplication)
	if _, err := os.Stat(blobPath); err == nil {
		return hash, nil
	}

	// Create parent directories
	dir := filepath.Dir(blobPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("blob: failed to create directory: %w", err)
	}

	// Write atomically: write to temp file, then rename
	tmpPath := blobPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return "", fmt.Errorf("blob: failed to write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, blobPath); err != nil {
		// Clean up temp file on failure
		os.Remove(tmpPath)
		return "", fmt.Errorf("blob: failed to rename temp file: %w", err)
	}

	bs.count++

	return hash, nil
}

// Get retrieves blob data by its hash.
func (bs *BlobStore) Get(hash string) ([]byte, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	blobPath := bs.pathFor(hash)

	data, err := os.ReadFile(blobPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("blob: not found: %s", hash)
		}
		return nil, fmt.Errorf("blob: failed to read: %w", err)
	}

	// Verify integrity
	if computeHash(data) != hash {
		return nil, fmt.Errorf("blob: integrity check failed for %s", hash)
	}

	return data, nil
}

// Delete removes a blob by its hash.
func (bs *BlobStore) Delete(hash string) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	blobPath := bs.pathFor(hash)

	if err := os.Remove(blobPath); err != nil {
		if os.IsNotExist(err) {
			return nil // Already gone
		}
		return fmt.Errorf("blob: failed to delete: %w", err)
	}

	bs.count--
	if bs.count < 0 {
		bs.count = 0
	}

	// Try to clean up empty parent directories
	bs.cleanEmptyDirs(filepath.Dir(blobPath))

	return nil
}

// Exists checks if a blob exists on disk.
func (bs *BlobStore) Exists(hash string) bool {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	blobPath := bs.pathFor(hash)
	_, err := os.Stat(blobPath)
	return err == nil
}

// Size returns the size of a blob in bytes.
func (bs *BlobStore) Size(hash string) (int64, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	blobPath := bs.pathFor(hash)
	stat, err := os.Stat(blobPath)
	if err != nil {
		return 0, fmt.Errorf("blob: not found: %s", hash)
	}

	return stat.Size(), nil
}

// PathFor returns the filesystem path for a given hash.
// Exported version for streaming access.
func (bs *BlobStore) PathFor(hash string) string {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return bs.pathFor(hash)
}

// Count returns the number of blobs currently stored.
func (bs *BlobStore) Count() int {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return bs.count
}

// pathFor returns the filesystem path for a given hash.
// Uses two levels of directory sharding: ab/cd/abcdef1234...
func (bs *BlobStore) pathFor(hash string) string {
	if len(hash) < 4 {
		return filepath.Join(bs.dir, hash)
	}
	return filepath.Join(bs.dir, hash[:2], hash[2:4], hash)
}

// cleanEmptyDirs removes empty parent directories up to the blob root.
func (bs *BlobStore) cleanEmptyDirs(dir string) {
	for dir != bs.dir && dir != "." && dir != "/" {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		os.Remove(dir)
		dir = filepath.Dir(dir)
	}
}

// countBlobs counts all blob files on disk by walking the directory.
func (bs *BlobStore) countBlobs() int {
	count := 0
	filepath.Walk(bs.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if !info.IsDir() {
			count++
		}
		return nil
	})
	return count
}

// computeHash computes SHA256 of data and returns hex string.
func computeHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
