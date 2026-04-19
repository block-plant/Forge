package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ayushkunwarsingh/forge/config"
	"github.com/ayushkunwarsingh/forge/logger"
)

// ChunkManager handles chunked (resumable) file uploads.
// Each upload session tracks which chunks have been received
// and assembles the final file when all chunks are present.
type ChunkManager struct {
	cfg     *config.Config
	log     *logger.Logger
	uploads map[string]*ChunkUpload
	mu      sync.Mutex
}

// ChunkUpload represents an in-progress chunked upload session.
type ChunkUpload struct {
	// ID is the unique upload session identifier.
	ID string `json:"id"`
	// Path is the destination file path.
	Path string `json:"path"`
	// TotalSize is the expected total file size in bytes.
	TotalSize int64 `json:"total_size"`
	// ChunkSize is the size of each chunk in bytes.
	ChunkSize int `json:"chunk_size"`
	// TotalChunks is the expected number of chunks.
	TotalChunks int `json:"total_chunks"`
	// ContentType is the file's MIME type.
	ContentType string `json:"content_type"`
	// UploaderUID is the UID of the uploading user.
	UploaderUID string `json:"uploader_uid"`
	// CreatedAt is when the upload session was created.
	CreatedAt time.Time `json:"created_at"`
	// ExpiresAt is when the upload session expires.
	ExpiresAt time.Time `json:"expires_at"`

	// chunks stores received chunk data indexed by chunk number.
	chunks map[int][]byte
	// received tracks which chunks have been received.
	received map[int]bool
}

// ChunkUploadStatus is the response after receiving a chunk.
type ChunkUploadStatus struct {
	// UploadID is the upload session ID.
	UploadID string `json:"upload_id"`
	// ChunkIndex is the index of the received chunk.
	ChunkIndex int `json:"chunk_index"`
	// ReceivedChunks is how many chunks have been received.
	ReceivedChunks int `json:"received_chunks"`
	// TotalChunks is the total expected chunks.
	TotalChunks int `json:"total_chunks"`
	// Complete is true when all chunks have been received.
	Complete bool `json:"complete"`
}

// NewChunkManager creates a new chunk manager.
func NewChunkManager(cfg *config.Config, log *logger.Logger) *ChunkManager {
	cm := &ChunkManager{
		cfg:     cfg,
		log:     log,
		uploads: make(map[string]*ChunkUpload),
	}

	// Start cleanup goroutine to expire stale uploads
	go cm.cleanupLoop()

	return cm
}

// InitUpload starts a new chunked upload session.
func (cm *ChunkManager) InitUpload(path string, totalSize int64, contentType string, uploaderUID string) (*ChunkUpload, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if totalSize <= 0 {
		return nil, fmt.Errorf("chunker: total size must be positive")
	}

	if totalSize > cm.cfg.Storage.MaxFileSize {
		return nil, fmt.Errorf("chunker: file size %d exceeds maximum %d", totalSize, cm.cfg.Storage.MaxFileSize)
	}

	chunkSize := cm.cfg.Storage.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 256 * 1024 // 256KB default
	}

	totalChunks := int(totalSize / int64(chunkSize))
	if totalSize%int64(chunkSize) != 0 {
		totalChunks++
	}

	id := generateUploadID()
	now := time.Now().UTC()

	upload := &ChunkUpload{
		ID:          id,
		Path:        path,
		TotalSize:   totalSize,
		ChunkSize:   chunkSize,
		TotalChunks: totalChunks,
		ContentType: contentType,
		UploaderUID: uploaderUID,
		CreatedAt:   now,
		ExpiresAt:   now.Add(1 * time.Hour), // 1 hour to complete upload
		chunks:      make(map[int][]byte),
		received:    make(map[int]bool),
	}

	cm.uploads[id] = upload

	cm.log.Info("Chunked upload initiated", logger.Fields{
		"upload_id":    id,
		"path":         path,
		"total_size":   totalSize,
		"total_chunks": totalChunks,
		"chunk_size":   chunkSize,
	})

	return upload, nil
}

// AddChunk receives a chunk for an upload session.
func (cm *ChunkManager) AddChunk(uploadID string, index int, data []byte) (*ChunkUploadStatus, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	upload, ok := cm.uploads[uploadID]
	if !ok {
		return nil, fmt.Errorf("chunker: upload session not found: %s", uploadID)
	}

	// Check expiration
	if time.Now().UTC().After(upload.ExpiresAt) {
		delete(cm.uploads, uploadID)
		return nil, fmt.Errorf("chunker: upload session expired: %s", uploadID)
	}

	// Validate chunk index
	if index < 0 || index >= upload.TotalChunks {
		return nil, fmt.Errorf("chunker: invalid chunk index %d (expected 0-%d)", index, upload.TotalChunks-1)
	}

	// Store chunk
	chunkCopy := make([]byte, len(data))
	copy(chunkCopy, data)
	upload.chunks[index] = chunkCopy
	upload.received[index] = true

	status := &ChunkUploadStatus{
		UploadID:       uploadID,
		ChunkIndex:     index,
		ReceivedChunks: len(upload.received),
		TotalChunks:    upload.TotalChunks,
		Complete:       len(upload.received) == upload.TotalChunks,
	}

	return status, nil
}

// Assemble combines all received chunks into the final file data.
// Returns the assembled data or an error if chunks are missing.
func (cm *ChunkManager) Assemble(uploadID string) ([]byte, *ChunkUpload, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	upload, ok := cm.uploads[uploadID]
	if !ok {
		return nil, nil, fmt.Errorf("chunker: upload session not found: %s", uploadID)
	}

	// Verify all chunks are present
	if len(upload.received) != upload.TotalChunks {
		missing := make([]int, 0)
		for i := 0; i < upload.TotalChunks; i++ {
			if !upload.received[i] {
				missing = append(missing, i)
			}
		}
		return nil, nil, fmt.Errorf("chunker: missing chunks: %v", missing)
	}

	// Assemble in order
	data := make([]byte, 0, upload.TotalSize)
	for i := 0; i < upload.TotalChunks; i++ {
		data = append(data, upload.chunks[i]...)
	}

	// Verify assembled size matches expected
	if int64(len(data)) != upload.TotalSize {
		return nil, nil, fmt.Errorf("chunker: assembled size %d doesn't match expected %d", len(data), upload.TotalSize)
	}

	// Clean up the upload session
	uploadCopy := *upload
	delete(cm.uploads, uploadID)

	cm.log.Info("Chunked upload assembled", logger.Fields{
		"upload_id": uploadID,
		"path":      upload.Path,
		"size":      len(data),
	})

	return data, &uploadCopy, nil
}

// GetUpload returns the status of an upload session.
func (cm *ChunkManager) GetUpload(uploadID string) (*ChunkUpload, bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	upload, ok := cm.uploads[uploadID]
	if !ok {
		return nil, false
	}

	return upload, true
}

// CancelUpload removes an upload session and frees its resources.
func (cm *ChunkManager) CancelUpload(uploadID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, ok := cm.uploads[uploadID]; !ok {
		return fmt.Errorf("chunker: upload session not found: %s", uploadID)
	}

	delete(cm.uploads, uploadID)

	cm.log.Info("Chunked upload cancelled", logger.Fields{
		"upload_id": uploadID,
	})

	return nil
}

// ActiveCount returns the number of active upload sessions.
func (cm *ChunkManager) ActiveCount() int {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return len(cm.uploads)
}

// cleanupLoop periodically removes expired upload sessions.
func (cm *ChunkManager) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		cm.mu.Lock()
		now := time.Now().UTC()
		for id, upload := range cm.uploads {
			if now.After(upload.ExpiresAt) {
				delete(cm.uploads, id)
				cm.log.Info("Expired chunked upload cleaned up", logger.Fields{
					"upload_id": id,
					"path":      upload.Path,
				})
			}
		}
		cm.mu.Unlock()
	}
}

// generateUploadID creates a unique upload session ID.
func generateUploadID() string {
	// Use timestamp + random bytes for uniqueness
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))

	// Mix in some entropy from the system
	buf := make([]byte, 16)
	f, err := os.Open("/dev/urandom")
	if err == nil {
		f.Read(buf)
		f.Close()
	}
	h.Write(buf)

	hash := h.Sum(nil)
	return "upload_" + hex.EncodeToString(hash[:12])
}
