package analytics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Store is the persistent analytics data store.
// It uses an append-only log format (JSONL) for raw events
// and maintains a summary index for fast retrieval.
type Store struct {
	dataDir string
	mu      sync.Mutex

	// currentFile is the active log file for today.
	currentFile *os.File
	currentDate string

	// summaryIndex tracks total event counts per name.
	summaryIndex map[string]int64
	summaryMu    sync.RWMutex
}

// NewStore creates a new analytics store.
func NewStore(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("analytics store: failed to create data dir: %w", err)
	}

	s := &Store{
		dataDir:      dataDir,
		summaryIndex: make(map[string]int64),
	}

	// Open today's log file
	if err := s.rotateIfNeeded(); err != nil {
		return nil, err
	}

	return s, nil
}

// Append writes an event to the current day's log file.
func (s *Store) Append(event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Rotate file if date changed
	if err := s.rotateIfNeeded(); err != nil {
		return err
	}

	// Serialize
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("analytics store: marshal error: %w", err)
	}

	// Append to log
	if _, err := s.currentFile.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("analytics store: write error: %w", err)
	}

	// Update summary index
	s.summaryMu.Lock()
	s.summaryIndex[event.Name]++
	s.summaryMu.Unlock()

	return nil
}

// AppendBatch writes multiple events atomically.
func (s *Store) AppendBatch(events []Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.rotateIfNeeded(); err != nil {
		return err
	}

	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			continue
		}
		s.currentFile.Write(append(data, '\n'))

		s.summaryMu.Lock()
		s.summaryIndex[event.Name]++
		s.summaryMu.Unlock()
	}

	// Sync to disk
	return s.currentFile.Sync()
}

// ReadDay reads all events from a specific day's log file.
func (s *Store) ReadDay(date string) ([]Event, error) {
	path := filepath.Join(s.dataDir, date+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var events []Event
	lines := splitLines(data)
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			continue // skip corrupt lines
		}
		events = append(events, event)
	}

	return events, nil
}

// Summary returns total counts per event name.
func (s *Store) Summary() map[string]int64 {
	s.summaryMu.RLock()
	defer s.summaryMu.RUnlock()

	result := make(map[string]int64, len(s.summaryIndex))
	for k, v := range s.summaryIndex {
		result[k] = v
	}
	return result
}

// ListDays returns a list of dates that have log files.
func (s *Store) ListDays() []string {
	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		return nil
	}

	var days []string
	for _, entry := range entries {
		name := entry.Name()
		if len(name) > 6 && name[len(name)-6:] == ".jsonl" {
			days = append(days, name[:len(name)-6])
		}
	}
	return days
}

// PurgeOlderThan removes log files older than the given number of days.
func (s *Store) PurgeOlderThan(retentionDays int) (int, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays).Format("2006-01-02")
	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		return 0, err
	}

	removed := 0
	for _, entry := range entries {
		name := entry.Name()
		if len(name) > 6 && name[len(name)-6:] == ".jsonl" {
			date := name[:len(name)-6]
			if date < cutoff {
				os.Remove(filepath.Join(s.dataDir, name))
				removed++
			}
		}
	}
	return removed, nil
}

// Close closes the current log file.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentFile != nil {
		return s.currentFile.Close()
	}
	return nil
}

// rotateIfNeeded opens the current day's log file, closing the previous one if the date changed.
func (s *Store) rotateIfNeeded() error {
	today := time.Now().UTC().Format("2006-01-02")
	if today == s.currentDate && s.currentFile != nil {
		return nil
	}

	// Close old file
	if s.currentFile != nil {
		s.currentFile.Close()
	}

	// Open new file
	path := filepath.Join(s.dataDir, today+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("analytics store: failed to open log file: %w", err)
	}

	s.currentFile = f
	s.currentDate = today
	return nil
}

// splitLines splits byte data into lines.
func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
