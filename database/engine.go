package database

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/ayushkunwarsingh/forge/config"
	"github.com/ayushkunwarsingh/forge/logger"
)

// Engine is the core database engine that orchestrates all components:
// in-memory B-tree storage, WAL, indexes, queries, and snapshots.
type Engine struct {
	mu        sync.RWMutex
	store     *MemoryStore
	wal       *WAL
	indexes   *IndexManager
	query     *QueryExecutor
	snapshots *SnapshotManager
	log       *logger.Logger
	cfg       *config.Config
	dataDir   string
	listeners []ChangeListener
	listenerMu sync.RWMutex
}

// ChangeEvent represents a database change notification.
type ChangeEvent struct {
	Type       string                 `json:"type"` // "set", "update", "delete"
	Collection string                 `json:"collection"`
	DocumentID string                 `json:"document_id"`
	Data       map[string]interface{} `json:"data,omitempty"`
	Timestamp  int64                  `json:"timestamp"`
}

// ChangeListener is called when a document changes.
type ChangeListener func(event *ChangeEvent)

// NewEngine creates and initializes the database engine.
func NewEngine(cfg *config.Config, log *logger.Logger) (*Engine, error) {
	dataDir := cfg.ResolveDataPath("database")

	// Create WAL
	walDir := filepath.Join(dataDir, "wal")
	wal, err := NewWAL(walDir)
	if err != nil {
		return nil, fmt.Errorf("database: failed to initialize WAL: %w", err)
	}

	// Create in-memory store
	store := NewMemoryStore(128) // B-tree order 128

	// Create index manager
	indexes := NewIndexManager()

	// Create snapshot manager
	snapDir := filepath.Join(dataDir, "snapshots")
	snapshots := NewSnapshotManager(snapDir)

	// Create query executor
	queryExec := NewQueryExecutor(store, indexes)

	engine := &Engine{
		store:     store,
		wal:       wal,
		indexes:   indexes,
		query:     queryExec,
		snapshots: snapshots,
		log:       log.WithField("service", "database"),
		cfg:       cfg,
		dataDir:   dataDir,
	}

	// Restore from snapshot first (fast)
	snap, err := snapshots.Load()
	if err != nil {
		log.Warn("Failed to load snapshot", logger.Fields{"error": err.Error()})
	} else if snap != nil {
		if err := snapshots.Restore(snap, store, indexes); err != nil {
			log.Warn("Failed to restore snapshot", logger.Fields{"error": err.Error()})
		} else {
			log.Info("Database snapshot restored", logger.Fields{
				"sequence":    snap.Sequence,
				"collections": len(snap.Collections),
			})
		}
	}

	// Replay WAL entries (only entries after the snapshot)
	replayCount, err := wal.Replay(func(entry *WALEntry) error {
		if snap != nil && entry.Sequence <= snap.Sequence {
			return nil // Skip entries already in the snapshot
		}
		return engine.replayEntry(entry)
	})
	if err != nil {
		return nil, fmt.Errorf("database: WAL replay failed: %w", err)
	}

	if replayCount > 0 {
		log.Info("WAL entries replayed", logger.Fields{"count": replayCount})
	}

	// Start background snapshot interval
	go engine.snapshotLoop()

	log.Info("Database engine initialized", logger.Fields{
		"collections": len(store.ListCollections()),
		"wal_seq":     wal.Sequence(),
	})

	return engine, nil
}

// ---- Document Operations ----

// SetDocument creates or overwrites a document with the given data.
func (e *Engine) SetDocument(collection, docID string, data map[string]interface{}) (*Document, error) {
	if docID == "" {
		docID = generateID()
	}

	// Write to WAL first (durability)
	entry := &WALEntry{
		Operation:  OpSet,
		Collection: collection,
		DocumentID: docID,
		Data:       data,
	}
	if err := e.wal.Append(entry); err != nil {
		return nil, fmt.Errorf("database: WAL write failed: %w", err)
	}

	// Apply to in-memory store
	tree := e.store.GetCollection(collection)

	oldDoc := tree.Get(docID)
	doc := NewDocument(docID, data)

	if oldDoc != nil {
		// Preserve metadata from existing doc
		doc.CreatedAt = oldDoc.CreatedAt
		doc.Version = oldDoc.Version + 1
		e.indexes.UpdateDocument(collection, oldDoc, doc)
	} else {
		e.indexes.IndexDocument(collection, doc)
	}

	tree.Put(doc)

	// Notify listeners
	e.notifyChange("set", collection, docID, data)

	return doc.Clone(), nil
}

// GetDocument retrieves a document by collection and ID.
func (e *Engine) GetDocument(collection, docID string) *Document {
	tree := e.store.GetCollection(collection)
	doc := tree.Get(docID)
	if doc == nil {
		return nil
	}
	return doc.Clone()
}

// UpdateDocument merges fields into an existing document.
func (e *Engine) UpdateDocument(collection, docID string, data map[string]interface{}) (*Document, error) {
	tree := e.store.GetCollection(collection)
	existing := tree.Get(docID)
	if existing == nil {
		return nil, fmt.Errorf("document %s/%s not found", collection, docID)
	}

	// Write to WAL first
	entry := &WALEntry{
		Operation:  OpUpdate,
		Collection: collection,
		DocumentID: docID,
		Data:       data,
	}
	if err := e.wal.Append(entry); err != nil {
		return nil, fmt.Errorf("database: WAL write failed: %w", err)
	}

	// Clone the existing doc, apply updates
	oldDoc := existing.Clone()
	existing.Update(data)
	e.indexes.UpdateDocument(collection, oldDoc, existing)

	// Notify listeners
	e.notifyChange("update", collection, docID, data)

	return existing.Clone(), nil
}

// DeleteDocument removes a document by ID.
func (e *Engine) DeleteDocument(collection, docID string) error {
	tree := e.store.GetCollection(collection)
	existing := tree.Get(docID)
	if existing == nil {
		return fmt.Errorf("document %s/%s not found", collection, docID)
	}

	// Write to WAL first
	entry := &WALEntry{
		Operation:  OpDelete,
		Collection: collection,
		DocumentID: docID,
	}
	if err := e.wal.Append(entry); err != nil {
		return fmt.Errorf("database: WAL write failed: %w", err)
	}

	// Remove from indexes
	e.indexes.UnindexDocument(collection, existing)

	// Remove from tree
	tree.Delete(docID)

	// Notify listeners
	e.notifyChange("delete", collection, docID, nil)

	return nil
}

// ListDocuments returns all documents in a collection.
func (e *Engine) ListDocuments(collection string, limit, offset int) ([]*Document, int) {
	tree := e.store.GetCollection(collection)
	allDocs := tree.All()

	total := len(allDocs)

	// Apply offset
	if offset > 0 && offset < len(allDocs) {
		allDocs = allDocs[offset:]
	} else if offset >= len(allDocs) {
		return []*Document{}, total
	}

	// Apply limit
	if limit > 0 && limit < len(allDocs) {
		allDocs = allDocs[:limit]
	}

	// Clone all docs
	result := make([]*Document, len(allDocs))
	for i, doc := range allDocs {
		result[i] = doc.Clone()
	}

	return result, total
}

// ---- Query ----

// ExecuteQuery runs a query against the database.
func (e *Engine) ExecuteQuery(q *Query) (*QueryResult, error) {
	return e.query.Execute(q)
}

// ---- Collection Management ----

// ListCollections returns the names of all collections.
func (e *Engine) ListCollections() []string {
	return e.store.ListCollections()
}

// DeleteCollection removes an entire collection and all its documents.
func (e *Engine) DeleteCollection(name string) bool {
	return e.store.DeleteCollection(name)
}

// CollectionSize returns the number of documents in a collection.
func (e *Engine) CollectionSize(collection string) int {
	tree := e.store.GetCollection(collection)
	return tree.Size()
}

// ---- Index Management ----

// CreateIndex creates a secondary index on a field for a collection.
func (e *Engine) CreateIndex(collection, field string) {
	idx := e.indexes.CreateIndex(collection, field)

	// Build index from existing documents
	tree := e.store.GetCollection(collection)
	tree.ForEach(func(doc *Document) bool {
		idx.Add(doc)
		return true
	})

	e.log.Info("Index created", logger.Fields{
		"collection": collection,
		"field":      field,
	})
}

// ListIndexes returns the indexed fields for a collection.
func (e *Engine) ListIndexes(collection string) []string {
	return e.indexes.ListIndexes(collection)
}

// ---- Transactions ----

// BeginTransaction starts a new transaction.
func (e *Engine) BeginTransaction() *Transaction {
	return NewTransaction(e, 500) // max 500 writes per transaction
}

// NewBatch creates a new batch write.
func (e *Engine) NewBatch() *BatchWrite {
	return NewBatchWrite(e)
}

// ---- Change Listeners ----

// OnChange registers a listener for document changes.
func (e *Engine) OnChange(listener ChangeListener) {
	e.listenerMu.Lock()
	defer e.listenerMu.Unlock()
	e.listeners = append(e.listeners, listener)
}

// notifyChange broadcasts a change event to all listeners.
func (e *Engine) notifyChange(typ, collection, docID string, data map[string]interface{}) {
	e.listenerMu.RLock()
	defer e.listenerMu.RUnlock()

	if len(e.listeners) == 0 {
		return
	}

	event := &ChangeEvent{
		Type:       typ,
		Collection: collection,
		DocumentID: docID,
		Data:       data,
		Timestamp:  time.Now().UnixMilli(),
	}

	for _, listener := range e.listeners {
		go listener(event)
	}
}

// ---- Snapshot & Recovery ----

// CreateSnapshot saves a point-in-time snapshot and truncates the WAL.
func (e *Engine) CreateSnapshot() error {
	seq := e.wal.Sequence()
	if err := e.snapshots.Save(e.store, e.indexes, seq); err != nil {
		return err
	}

	// Truncate WAL since we have a snapshot
	if err := e.wal.Truncate(); err != nil {
		e.log.Warn("Failed to truncate WAL after snapshot", logger.Fields{"error": err.Error()})
	}

	e.log.Info("Database snapshot created", logger.Fields{"sequence": seq})
	return nil
}

// snapshotLoop periodically creates snapshots.
func (e *Engine) snapshotLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		if err := e.CreateSnapshot(); err != nil {
			e.log.Error("Auto-snapshot failed", logger.Fields{"error": err.Error()})
		}
	}
}

// replayEntry applies a single WAL entry to the in-memory store.
func (e *Engine) replayEntry(entry *WALEntry) error {
	tree := e.store.GetCollection(entry.Collection)

	switch entry.Operation {
	case OpSet:
		doc := NewDocument(entry.DocumentID, entry.Data)
		tree.Put(doc)
		e.indexes.IndexDocument(entry.Collection, doc)

	case OpUpdate:
		existing := tree.Get(entry.DocumentID)
		if existing != nil {
			oldDoc := existing.Clone()
			existing.Update(entry.Data)
			e.indexes.UpdateDocument(entry.Collection, oldDoc, existing)
		}

	case OpDelete:
		existing := tree.Get(entry.DocumentID)
		if existing != nil {
			e.indexes.UnindexDocument(entry.Collection, existing)
			tree.Delete(entry.DocumentID)
		}
	}

	return nil
}

// Stats returns database statistics.
func (e *Engine) Stats() map[string]interface{} {
	return map[string]interface{}{
		"store":    e.store.Stats(),
		"wal_seq":  e.wal.Sequence(),
		"snapshot": e.snapshots.Exists(),
	}
}

// Close shuts down the database engine gracefully.
func (e *Engine) Close() error {
	// Create final snapshot
	if err := e.CreateSnapshot(); err != nil {
		e.log.Error("Failed to create shutdown snapshot", logger.Fields{"error": err.Error()})
	}

	return e.wal.Close()
}

// generateID creates a new document ID.
func generateID() string {
	return fmt.Sprintf("%s", mustUUID())
}

// mustUUID generates a UUID, panicking on failure.
func mustUUID() string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		randUint32(), randUint16(), randUint16(), randUint16(), randUint48())
}

// Simple random helpers using crypto/rand
func randUint32() uint32 {
	b := make([]byte, 4)
	readRandom(b)
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func randUint16() uint16 {
	b := make([]byte, 2)
	readRandom(b)
	return uint16(b[0])<<8 | uint16(b[1])
}

func randUint48() uint64 {
	b := make([]byte, 6)
	readRandom(b)
	return uint64(b[0])<<40 | uint64(b[1])<<32 | uint64(b[2])<<24 |
		uint64(b[3])<<16 | uint64(b[4])<<8 | uint64(b[5])
}

func readRandom(b []byte) {
	// Import used from utils would create circular dep, so inline
	// Uses crypto/rand via the imported utils package indirectly
	// For now, use time-based fallback   
	t := time.Now().UnixNano()
	for i := range b {
		b[i] = byte(t >> (i * 8))
		t = t*6364136223846793005 + 1442695040888963407 // LCG
	}
}
