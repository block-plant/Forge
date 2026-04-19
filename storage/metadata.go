package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ayushkunwarsingh/forge/logger"
)

// MetadataStore manages file metadata on disk and in memory.
// Each file's metadata is persisted as a JSON file mirroring the file path.
type MetadataStore struct {
	dir string
	log *logger.Logger

	// files is the in-memory index: path → *FileInfo
	files map[string]*FileInfo
	mu    sync.RWMutex
}

// NewMetadataStore creates a new metadata store rooted at the given directory.
func NewMetadataStore(dir string, log *logger.Logger) (*MetadataStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("metadata: failed to create directory: %w", err)
	}

	return &MetadataStore{
		dir:   dir,
		log:   log,
		files: make(map[string]*FileInfo),
	}, nil
}

// LoadAll loads all metadata files from disk into memory.
// Returns the count of files loaded and any error encountered.
func (ms *MetadataStore) LoadAll() (int, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	count := 0
	var lastErr error

	err := filepath.Walk(ms.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			lastErr = err
			return nil // Continue walking
		}
		if info.IsDir() {
			return nil
		}

		// Only load .meta files
		if !strings.HasSuffix(path, ".meta") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			lastErr = err
			return nil
		}

		var fileInfo FileInfo
		if err := json.Unmarshal(data, &fileInfo); err != nil {
			lastErr = err
			return nil
		}

		ms.files[fileInfo.Path] = &fileInfo
		count++
		return nil
	})

	if err != nil {
		lastErr = err
	}

	return count, lastErr
}

// Get retrieves metadata for a file path.
func (ms *MetadataStore) Get(path string) (*FileInfo, bool) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	info, ok := ms.files[path]
	if !ok {
		return nil, false
	}

	// Return a copy to prevent mutation
	copy := *info
	if info.CustomMetadata != nil {
		copy.CustomMetadata = make(map[string]string, len(info.CustomMetadata))
		for k, v := range info.CustomMetadata {
			copy.CustomMetadata[k] = v
		}
	}
	return &copy, true
}

// Put stores metadata both in memory and on disk.
func (ms *MetadataStore) Put(path string, info *FileInfo) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	// Persist to disk
	metaPath := ms.metaPathFor(path)
	dir := filepath.Dir(metaPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("metadata: failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("metadata: failed to marshal: %w", err)
	}

	// Write atomically
	tmpPath := metaPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("metadata: failed to write: %w", err)
	}
	if err := os.Rename(tmpPath, metaPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("metadata: failed to rename: %w", err)
	}

	// Store in memory
	copy := *info
	ms.files[path] = &copy

	return nil
}

// Delete removes metadata for a file path.
func (ms *MetadataStore) Delete(path string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	delete(ms.files, path)

	// Remove from disk
	metaPath := ms.metaPathFor(path)
	if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("metadata: failed to delete: %w", err)
	}

	// Clean up empty parent directories
	ms.cleanEmptyDirs(filepath.Dir(metaPath))

	return nil
}

// List returns all files whose path starts with the given prefix.
// If prefix is empty, all files are returned.
func (ms *MetadataStore) List(prefix string) []*FileInfo {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	var results []*FileInfo

	for path, info := range ms.files {
		if prefix == "" || strings.HasPrefix(path, prefix) {
			copy := *info
			results = append(results, &copy)
		}
	}

	// Sort by path for deterministic output
	sortFileInfos(results)

	return results
}

// HasBlobReference checks if any file still references the given blob hash.
func (ms *MetadataStore) HasBlobReference(hash string) bool {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	for _, info := range ms.files {
		if info.Hash == hash {
			return true
		}
	}
	return false
}

// Count returns the total number of tracked files.
func (ms *MetadataStore) Count() int {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return len(ms.files)
}

// TotalSize returns the sum of all tracked file sizes.
func (ms *MetadataStore) TotalSize() int64 {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	var total int64
	for _, info := range ms.files {
		total += info.Size
	}
	return total
}

// metaPathFor returns the disk path for a metadata file.
// Mirrors the file path structure: "images/photo.jpg" → "<dir>/images/photo.jpg.meta"
func (ms *MetadataStore) metaPathFor(path string) string {
	return filepath.Join(ms.dir, path+".meta")
}

// cleanEmptyDirs removes empty parent directories up to the metadata root.
func (ms *MetadataStore) cleanEmptyDirs(dir string) {
	for dir != ms.dir && dir != "." && dir != "/" {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		os.Remove(dir)
		dir = filepath.Dir(dir)
	}
}

// sortFileInfos sorts a slice of FileInfo by path using insertion sort.
// Simple and efficient for typical storage listings.
func sortFileInfos(infos []*FileInfo) {
	for i := 1; i < len(infos); i++ {
		key := infos[i]
		j := i - 1
		for j >= 0 && infos[j].Path > key.Path {
			infos[j+1] = infos[j]
			j--
		}
		infos[j+1] = key
	}
}
