// Package storage implements the Forge file storage engine.
// It provides content-addressed file storage, chunked uploads,
// streaming downloads, access control, and metadata management.
// All built from scratch with zero external dependencies.
package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ayushkunwarsingh/forge/config"
	"github.com/ayushkunwarsingh/forge/logger"
)

// Engine is the core storage service that coordinates blobs, metadata,
// chunked uploads, and access control.
type Engine struct {
	cfg *config.Config
	log *logger.Logger

	// blobs is the content-addressed object store.
	blobs *BlobStore
	// meta is the file metadata manager.
	meta *MetadataStore
	// chunks manages in-progress chunked uploads.
	chunks *ChunkManager
	// access provides access control and signed URLs.
	access *AccessController

	// objectsDir is the path to the content-addressed blob directory.
	objectsDir string
	// metadataDir is the path to the metadata directory.
	metadataDir string

	mu sync.RWMutex
}

// FileInfo represents a stored file's metadata and location.
type FileInfo struct {
	// Path is the user-facing file path (e.g., "images/photo.jpg").
	Path string `json:"path"`
	// Hash is the SHA256 content hash (content-addressed key).
	Hash string `json:"hash"`
	// Size is the file size in bytes.
	Size int64 `json:"size"`
	// ContentType is the MIME type of the file.
	ContentType string `json:"content_type"`
	// CreatedAt is the upload timestamp.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is the last modification timestamp.
	UpdatedAt time.Time `json:"updated_at"`
	// UploaderUID is the UID of the user who uploaded the file.
	UploaderUID string `json:"uploader_uid,omitempty"`
	// CustomMetadata is user-defined key-value metadata.
	CustomMetadata map[string]string `json:"custom_metadata,omitempty"`
}

// NewEngine creates a new storage engine with the given configuration.
func NewEngine(cfg *config.Config, log *logger.Logger) (*Engine, error) {
	objectsDir := cfg.ResolveDataPath("storage", "objects")
	metadataDir := cfg.ResolveDataPath("storage", "metadata")

	// Ensure directories exist
	for _, dir := range []string{objectsDir, metadataDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("storage: failed to create directory %q: %w", dir, err)
		}
	}

	blobs, err := NewBlobStore(objectsDir, log)
	if err != nil {
		return nil, fmt.Errorf("storage: failed to initialize blob store: %w", err)
	}

	meta, err := NewMetadataStore(metadataDir, log)
	if err != nil {
		return nil, fmt.Errorf("storage: failed to initialize metadata store: %w", err)
	}

	chunks := NewChunkManager(cfg, log)

	// Use a secret key derived from the data directory for signed URLs
	secretKey := deriveSecret(cfg.DataDir)
	access := NewAccessController(secretKey, log)

	engine := &Engine{
		cfg:         cfg,
		log:         log,
		blobs:       blobs,
		meta:        meta,
		chunks:      chunks,
		access:      access,
		objectsDir:  objectsDir,
		metadataDir: metadataDir,
	}

	// Load existing metadata into memory
	count, err := meta.LoadAll()
	if err != nil {
		log.Warn("Storage: partial metadata load", logger.Fields{"error": err.Error()})
	}

	log.Info("Storage engine initialized", logger.Fields{
		"objects_dir":  objectsDir,
		"metadata_dir": metadataDir,
		"files_loaded": count,
		"max_file_size": cfg.Storage.MaxFileSize,
		"chunk_size":    cfg.Storage.ChunkSize,
	})

	return engine, nil
}

// Upload stores a file's content and records its metadata.
// Returns the created FileInfo.
func (e *Engine) Upload(path string, data []byte, contentType string, uploaderUID string, customMeta map[string]string) (*FileInfo, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Validate file size
	if int64(len(data)) > e.cfg.Storage.MaxFileSize {
		return nil, fmt.Errorf("storage: file size %d exceeds maximum %d bytes", len(data), e.cfg.Storage.MaxFileSize)
	}

	// Normalize path
	path = normalizePath(path)
	if path == "" {
		return nil, fmt.Errorf("storage: empty file path")
	}

	// Auto-detect content type if not provided
	if contentType == "" {
		contentType = DetectMIME(path, data)
	}

	// Store the blob (content-addressed)
	hash, err := e.blobs.Put(data)
	if err != nil {
		return nil, fmt.Errorf("storage: failed to store blob: %w", err)
	}

	now := time.Now().UTC()
	info := &FileInfo{
		Path:           path,
		Hash:           hash,
		Size:           int64(len(data)),
		ContentType:    contentType,
		CreatedAt:      now,
		UpdatedAt:      now,
		UploaderUID:    uploaderUID,
		CustomMetadata: customMeta,
	}

	// Check if a file already exists at this path — update metadata
	if existing, exists := e.meta.Get(path); exists {
		info.CreatedAt = existing.CreatedAt
	}

	// Store metadata
	if err := e.meta.Put(path, info); err != nil {
		return nil, fmt.Errorf("storage: failed to store metadata: %w", err)
	}

	e.log.Info("File uploaded", logger.Fields{
		"path": path,
		"hash": hash[:12],
		"size": info.Size,
		"type": contentType,
	})

	return info, nil
}

// Download retrieves a file's content by path.
// Returns the content, file info, and any error.
func (e *Engine) Download(path string) ([]byte, *FileInfo, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	path = normalizePath(path)

	info, exists := e.meta.Get(path)
	if !exists {
		return nil, nil, fmt.Errorf("storage: file not found: %s", path)
	}

	data, err := e.blobs.Get(info.Hash)
	if err != nil {
		return nil, nil, fmt.Errorf("storage: failed to read blob: %w", err)
	}

	return data, info, nil
}

// Delete removes a file by path. The blob may remain if other files reference it.
func (e *Engine) Delete(path string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	path = normalizePath(path)

	info, exists := e.meta.Get(path)
	if !exists {
		return fmt.Errorf("storage: file not found: %s", path)
	}

	// Remove metadata
	if err := e.meta.Delete(path); err != nil {
		return fmt.Errorf("storage: failed to delete metadata: %w", err)
	}

	// Check if any other file references the same blob
	if !e.meta.HasBlobReference(info.Hash) {
		// No other references — safe to delete the blob
		if err := e.blobs.Delete(info.Hash); err != nil {
			e.log.Warn("Storage: failed to delete orphan blob", logger.Fields{
				"hash":  info.Hash[:12],
				"error": err.Error(),
			})
		}
	}

	e.log.Info("File deleted", logger.Fields{
		"path": path,
		"hash": info.Hash[:12],
	})

	return nil
}

// GetMetadata returns the metadata for a file without reading its content.
func (e *Engine) GetMetadata(path string) (*FileInfo, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	path = normalizePath(path)
	info, exists := e.meta.Get(path)
	if !exists {
		return nil, fmt.Errorf("storage: file not found: %s", path)
	}

	return info, nil
}

// UpdateMetadata updates the custom metadata for a file.
func (e *Engine) UpdateMetadata(path string, customMeta map[string]string) (*FileInfo, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	path = normalizePath(path)
	info, exists := e.meta.Get(path)
	if !exists {
		return nil, fmt.Errorf("storage: file not found: %s", path)
	}

	info.CustomMetadata = customMeta
	info.UpdatedAt = time.Now().UTC()

	if err := e.meta.Put(path, info); err != nil {
		return nil, fmt.Errorf("storage: failed to update metadata: %w", err)
	}

	return info, nil
}

// List returns all files under a given prefix/directory.
func (e *Engine) List(prefix string) []*FileInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()

	prefix = normalizePath(prefix)
	return e.meta.List(prefix)
}

// GetBlobPath returns the disk path for a blob hash (used for streaming).
func (e *Engine) GetBlobPath(hash string) string {
	return e.blobs.PathFor(hash)
}

// Blobs returns the underlying blob store.
func (e *Engine) Blobs() *BlobStore {
	return e.blobs
}

// Chunks returns the chunk manager for chunked uploads.
func (e *Engine) Chunks() *ChunkManager {
	return e.chunks
}

// Access returns the access controller for signed URLs.
func (e *Engine) Access() *AccessController {
	return e.access
}

// Stats returns storage statistics.
func (e *Engine) Stats() map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()

	totalFiles := e.meta.Count()
	totalSize := e.meta.TotalSize()
	blobCount := e.blobs.Count()

	return map[string]interface{}{
		"total_files":   totalFiles,
		"total_size":    totalSize,
		"unique_blobs":  blobCount,
		"active_chunks": e.chunks.ActiveCount(),
	}
}

// normalizePath cleans up a file path — removes leading/trailing slashes,
// prevents directory traversal.
func normalizePath(path string) string {
	// Remove leading slashes
	for len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}
	// Remove trailing slashes
	for len(path) > 0 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}

	// Prevent directory traversal
	cleaned := filepath.Clean(path)
	if cleaned == "." || cleaned == ".." {
		return ""
	}

	// Reject any path that tries to go up
	segments := splitPathSegments(cleaned)
	result := make([]string, 0, len(segments))
	for _, s := range segments {
		if s == ".." || s == "." {
			continue
		}
		result = append(result, s)
	}

	return joinPathSegments(result)
}

// splitPathSegments splits a file path by "/" separator.
func splitPathSegments(path string) []string {
	var segments []string
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			if i > start {
				segments = append(segments, path[start:i])
			}
			start = i + 1
		}
	}
	if start < len(path) {
		segments = append(segments, path[start:])
	}
	return segments
}

// joinPathSegments joins segments with "/".
func joinPathSegments(segments []string) string {
	if len(segments) == 0 {
		return ""
	}
	result := segments[0]
	for i := 1; i < len(segments); i++ {
		result += "/" + segments[i]
	}
	return result
}

// deriveSecret creates a deterministic secret key from the data directory path.
// In production, this should be loaded from a config file or env var.
func deriveSecret(seed string) []byte {
	h := sha256.New()
	h.Write([]byte("forge-storage-secret-v1:"))
	h.Write([]byte(seed))
	return h.Sum(nil)
}

// MarshalFileInfo serializes a FileInfo to JSON bytes.
func MarshalFileInfo(info *FileInfo) ([]byte, error) {
	return json.Marshal(info)
}

// UnmarshalFileInfo deserializes JSON bytes into a FileInfo.
func UnmarshalFileInfo(data []byte) (*FileInfo, error) {
	var info FileInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// ContentHash computes the SHA256 hash of data and returns it as a hex string.
func ContentHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
