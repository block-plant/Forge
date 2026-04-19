package database

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// WAL operation types
const (
	OpSet    = "SET"
	OpUpdate = "UPDATE"
	OpDelete = "DELETE"
)

// WALEntry represents a single entry in the Write-Ahead Log.
type WALEntry struct {
	Timestamp  int64                  `json:"ts"`
	Operation  string                 `json:"op"`
	Collection string                 `json:"col"`
	DocumentID string                 `json:"id"`
	Data       map[string]interface{} `json:"data,omitempty"`
	Sequence   uint64                 `json:"seq"`
}

// WAL is the Write-Ahead Log that ensures durability.
// Every write is logged to disk BEFORE being applied to memory.
// On crash recovery, the WAL is replayed to rebuild the in-memory state.
type WAL struct {
	mu       sync.Mutex
	dir      string
	file     *os.File
	writer   *bufio.Writer
	sequence uint64
	size     int64 // current file size
	maxSize  int64 // max size before rotation (default: 64MB)
	segment  int   // current segment number
}

// NewWAL creates a new Write-Ahead Log in the given directory.
func NewWAL(dir string) (*WAL, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("wal: failed to create directory: %w", err)
	}

	w := &WAL{
		dir:     dir,
		maxSize: 64 * 1024 * 1024, // 64MB per segment
		segment: 0,
	}

	// Find the latest segment
	w.segment = w.findLatestSegment()

	// Open the current segment for appending
	if err := w.openSegment(); err != nil {
		return nil, err
	}

	return w, nil
}

// Append writes a WAL entry to disk. This MUST complete before the
// corresponding write is applied to memory.
func (w *WAL) Append(entry *WALEntry) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.sequence++
	entry.Sequence = w.sequence
	entry.Timestamp = time.Now().UnixMilli()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("wal: failed to marshal entry: %w", err)
	}

	// Write entry as a single line (newline delimited JSON)
	n, err := w.writer.Write(data)
	if err != nil {
		return fmt.Errorf("wal: failed to write entry: %w", err)
	}
	w.writer.WriteByte('\n')
	w.size += int64(n + 1)

	// Flush to ensure durability
	if err := w.writer.Flush(); err != nil {
		return fmt.Errorf("wal: failed to flush: %w", err)
	}

	// Sync to disk for true durability
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("wal: failed to sync: %w", err)
	}

	// Rotate if segment is too large
	if w.size >= w.maxSize {
		if err := w.rotate(); err != nil {
			return fmt.Errorf("wal: failed to rotate: %w", err)
		}
	}

	return nil
}

// Replay reads all WAL entries and calls the handler for each.
// Used during startup to rebuild the in-memory state.
func (w *WAL) Replay(handler func(entry *WALEntry) error) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	count := 0

	// Find all segment files
	segments := w.findAllSegments()

	for _, segFile := range segments {
		n, err := w.replaySegment(segFile, handler)
		if err != nil {
			return count, fmt.Errorf("wal: failed to replay segment %s: %w", segFile, err)
		}
		count += n
	}

	return count, nil
}

// replaySegment reads and replays a single segment file.
func (w *WAL) replaySegment(path string, handler func(entry *WALEntry) error) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB max line
	count := 0

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry WALEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			// Skip corrupted entries
			continue
		}

		if err := handler(&entry); err != nil {
			return count, err
		}

		// Track the highest sequence number
		if entry.Sequence > w.sequence {
			w.sequence = entry.Sequence
		}

		count++
	}

	return count, scanner.Err()
}

// Truncate removes all WAL segments (called after successful snapshot).
func (w *WAL) Truncate() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Close current file
	if w.file != nil {
		w.writer.Flush()
		w.file.Close()
	}

	// Remove all segments
	segments := w.findAllSegments()
	for _, seg := range segments {
		os.Remove(seg)
	}

	// Reset and open new segment
	w.segment = 0
	w.size = 0
	return w.openSegment()
}

// Close flushes and closes the WAL.
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.writer != nil {
		w.writer.Flush()
	}
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// Sequence returns the current sequence number.
func (w *WAL) Sequence() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.sequence
}

// ---- Internal ----

// openSegment opens the current segment file for appending.
func (w *WAL) openSegment() error {
	path := w.segmentPath(w.segment)

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("wal: failed to open segment %s: %w", path, err)
	}

	info, err := file.Stat()
	if err == nil {
		w.size = info.Size()
	}

	w.file = file
	w.writer = bufio.NewWriterSize(file, 32*1024) // 32KB buffer
	return nil
}

// rotate closes the current segment and opens a new one.
func (w *WAL) rotate() error {
	if w.writer != nil {
		w.writer.Flush()
	}
	if w.file != nil {
		w.file.Sync()
		w.file.Close()
	}

	w.segment++
	w.size = 0
	return w.openSegment()
}

// segmentPath returns the file path for a segment number.
func (w *WAL) segmentPath(n int) string {
	return filepath.Join(w.dir, fmt.Sprintf("%06d.wal", n))
}

// findLatestSegment finds the highest segment number.
func (w *WAL) findLatestSegment() int {
	segments := w.findAllSegments()
	if len(segments) == 0 {
		return 0
	}
	// The last file is the latest
	return len(segments) - 1
}

// findAllSegments returns all segment file paths in order.
func (w *WAL) findAllSegments() []string {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return nil
	}

	var segments []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".wal" {
			segments = append(segments, filepath.Join(w.dir, entry.Name()))
		}
	}
	return segments
}
