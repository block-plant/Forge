package database

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Snapshot represents a point-in-time consistent view of the entire database.
// Used for backup and fast recovery (avoids replaying the entire WAL).
type Snapshot struct {
	Version     int                           `json:"version"` // snapshot format version
	Timestamp   int64                         `json:"timestamp"`
	Sequence    uint64                        `json:"sequence"` // WAL sequence at snapshot time
	Collections map[string][]*Document        `json:"collections"`
	Indexes     map[string][]string           `json:"indexes"` // collection → indexed fields
}

// SnapshotManager handles creating and loading snapshots.
type SnapshotManager struct {
	dir string
}

// NewSnapshotManager creates a new snapshot manager.
func NewSnapshotManager(dir string) *SnapshotManager {
	os.MkdirAll(dir, 0755)
	return &SnapshotManager{dir: dir}
}

// Save creates a snapshot of the current database state.
func (sm *SnapshotManager) Save(store *MemoryStore, indexes *IndexManager, walSeq uint64) error {
	snap := &Snapshot{
		Version:     1,
		Timestamp:   time.Now().UnixMilli(),
		Sequence:    walSeq,
		Collections: make(map[string][]*Document),
		Indexes:     make(map[string][]string),
	}

	// Capture all collections
	store.mu.RLock()
	for name, tree := range store.collections {
		docs := tree.All()
		snap.Collections[name] = docs
	}
	store.mu.RUnlock()

	// Capture index definitions
	indexes.mu.RLock()
	for col, fieldMap := range indexes.indexes {
		fields := make([]string, 0, len(fieldMap))
		for field := range fieldMap {
			fields = append(fields, field)
		}
		snap.Indexes[col] = fields
	}
	indexes.mu.RUnlock()

	// Serialize
	data, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("snapshot: failed to serialize: %w", err)
	}

	// Write to temp file, then rename (atomic)
	tmpPath := filepath.Join(sm.dir, "snapshot.tmp")
	finalPath := filepath.Join(sm.dir, "snapshot.json")

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("snapshot: failed to write: %w", err)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		return fmt.Errorf("snapshot: failed to rename: %w", err)
	}

	return nil
}

// Load reads the latest snapshot from disk.
// Returns nil if no snapshot exists.
func (sm *SnapshotManager) Load() (*Snapshot, error) {
	path := filepath.Join(sm.dir, "snapshot.json")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No snapshot yet
		}
		return nil, fmt.Errorf("snapshot: failed to read: %w", err)
	}

	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("snapshot: failed to parse: %w", err)
	}

	return &snap, nil
}

// Restore loads a snapshot into the memory store and index manager.
func (sm *SnapshotManager) Restore(snap *Snapshot, store *MemoryStore, indexes *IndexManager) error {
	// Restore collections
	for name, docs := range snap.Collections {
		tree := store.GetCollection(name)
		for _, doc := range docs {
			tree.Put(doc)
		}
	}

	// Restore index definitions and rebuild
	for col, fields := range snap.Indexes {
		for _, field := range fields {
			idx := indexes.CreateIndex(col, field)
			// Rebuild index from loaded documents
			tree := store.GetCollection(col)
			tree.ForEach(func(doc *Document) bool {
				idx.Add(doc)
				return true
			})
		}
	}

	return nil
}

// Exists checks if a snapshot file exists.
func (sm *SnapshotManager) Exists() bool {
	path := filepath.Join(sm.dir, "snapshot.json")
	_, err := os.Stat(path)
	return err == nil
}
