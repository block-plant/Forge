// Package database implements the Forge document database engine.
// It provides a Firestore-like document store with collections, queries,
// indexes, transactions, and real-time change notifications.
//
// The underlying storage is powered by DynamicDB — an LSM-Tree engine with
// MVCC concurrency control, SkipList-based MemTables, and binary WAL durability.
package database

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ayushkunwarsingh/forge/config"
	"github.com/ayushkunwarsingh/forge/dynamicdb/mvcc"
	"github.com/ayushkunwarsingh/forge/logger"
)

// Engine is the core database engine powered by DynamicDB's MVCC layer.
type Engine struct {
	mu         sync.RWMutex
	db         *mvcc.DB
	indexes    *IndexManager
	log        *logger.Logger
	cfg        *config.Config
	dataDir    string
	listeners  []ChangeListener
	listenerMu sync.RWMutex
}

// ChangeEvent represents a database change notification.
type ChangeEvent struct {
	Type       string                 `json:"type"`       // "set", "update", "delete"
	Collection string                 `json:"collection"`
	DocumentID string                 `json:"document_id"`
	Data       map[string]interface{} `json:"data,omitempty"`
	Timestamp  int64                  `json:"timestamp"`
}

// ChangeListener is called when a document changes.
type ChangeListener func(event *ChangeEvent)

// NewEngine creates and initializes the DynamicDB-backed engine.
func NewEngine(cfg *config.Config, log *logger.Logger) (*Engine, error) {
	dataDir := cfg.ResolveDataPath("dynamicdb")

	mvccDB, err := mvcc.Open(dataDir)
	if err != nil {
		return nil, fmt.Errorf("database: failed to start DynamicDB: %w", err)
	}

	engine := &Engine{
		db:      mvccDB,
		indexes: NewIndexManager(),
		log:     log.WithField("service", "dynamicdb"),
		cfg:     cfg,
		dataDir: dataDir,
	}

	log.Info("DynamicDB engine initialized", logger.Fields{
		"path":    dataDir,
		"backend": "LSM/MVCC",
	})

	return engine, nil
}

// ---- Internal Key Encoding ----

// formatKey builds the MVCC user-key for a document: DOC/<collection>/<docID>
func formatKey(collection, docID string) []byte {
	return []byte(fmt.Sprintf("DOC/%s/%s", collection, docID))
}

// collectionPrefix returns the key prefix for all documents in a collection.
func collectionPrefix(collection string) []byte {
	return []byte(fmt.Sprintf("DOC/%s/", collection))
}

// allDocsPrefix is the prefix shared by every document key.
var allDocsPrefix = []byte("DOC/")

// ---- Document Operations ----

// SetDocument creates or overwrites a document with the given data.
func (e *Engine) SetDocument(collection, docID string, data map[string]interface{}) (*Document, error) {
	if docID == "" {
		docID = generateID()
	}

	// Check if we're updating an existing document
	existing := e.GetDocument(collection, docID)

	doc := NewDocument(docID, data)
	if existing != nil {
		doc.CreatedAt = existing.CreatedAt
		doc.Version = existing.Version + 1
		e.indexes.UpdateDocument(collection, existing, doc)
	} else {
		e.indexes.IndexDocument(collection, doc)
	}

	key := formatKey(collection, docID)

	payload, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("database: failed to marshal document: %w", err)
	}

	txn := e.db.BeginContext(true)
	txn.Put(key, payload)
	if err := txn.Commit(); err != nil {
		return nil, fmt.Errorf("database: commit failed: %w", err)
	}

	e.notifyChange("set", collection, docID, data)
	return doc.Clone(), nil
}

// GetDocument retrieves a document by collection and ID.
func (e *Engine) GetDocument(collection, docID string) *Document {
	key := formatKey(collection, docID)

	txn := e.db.BeginContext(false)
	val, err := txn.Get(key)
	txn.Commit()

	if err != nil || val == nil {
		return nil
	}

	var doc Document
	if err := json.Unmarshal(val, &doc); err != nil {
		return nil
	}
	return &doc
}

// UpdateDocument merges fields into an existing document.
func (e *Engine) UpdateDocument(collection, docID string, data map[string]interface{}) (*Document, error) {
	existing := e.GetDocument(collection, docID)
	if existing == nil {
		return nil, fmt.Errorf("document %s/%s not found", collection, docID)
	}

	oldDoc := existing.Clone()
	existing.Update(data)

	e.indexes.UpdateDocument(collection, oldDoc, existing)

	payload, err := json.Marshal(existing)
	if err != nil {
		return nil, fmt.Errorf("database: failed to marshal document: %w", err)
	}

	key := formatKey(collection, docID)
	txn := e.db.BeginContext(true)
	txn.Put(key, payload)

	if err := txn.Commit(); err != nil {
		return nil, fmt.Errorf("database: commit failed: %w", err)
	}

	e.notifyChange("update", collection, docID, data)
	return existing.Clone(), nil
}

// DeleteDocument removes a document by ID.
func (e *Engine) DeleteDocument(collection, docID string) error {
	existing := e.GetDocument(collection, docID)
	if existing == nil {
		return fmt.Errorf("document %s/%s not found", collection, docID)
	}

	e.indexes.UnindexDocument(collection, existing)

	key := formatKey(collection, docID)
	txn := e.db.BeginContext(true)
	txn.Delete(key)

	if err := txn.Commit(); err != nil {
		return fmt.Errorf("database: commit failed: %w", err)
	}

	e.notifyChange("delete", collection, docID, nil)
	return nil
}

// ListDocuments returns documents in a collection with pagination support.
func (e *Engine) ListDocuments(collection string, limit, offset int) ([]*Document, int) {
	prefix := collectionPrefix(collection)

	results, err := e.db.Scan(prefix)
	if err != nil {
		e.log.Error("ListDocuments scan failed", logger.Fields{"error": err.Error()})
		return []*Document{}, 0
	}

	// Decode all documents from the scan results
	allDocs := make([]*Document, 0, len(results))
	for _, sr := range results {
		var doc Document
		if err := json.Unmarshal(sr.Value, &doc); err != nil {
			continue
		}
		allDocs = append(allDocs, &doc)
	}

	// Sort by creation time (ascending) for stable ordering
	sort.Slice(allDocs, func(i, j int) bool {
		return allDocs[i].CreatedAt < allDocs[j].CreatedAt
	})

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

	// Clone all docs for safe return
	result := make([]*Document, len(allDocs))
	for i, doc := range allDocs {
		result[i] = doc.Clone()
	}

	return result, total
}

// ---- Query ----

// ExecuteQuery runs a query with where-clause filtering, ordering, and pagination.
func (e *Engine) ExecuteQuery(q *Query) (*QueryResult, error) {
	prefix := collectionPrefix(q.Collection)

	results, err := e.db.Scan(prefix)
	if err != nil {
		return nil, fmt.Errorf("database: scan failed: %w", err)
	}

	// Decode documents
	allDocs := make([]*Document, 0, len(results))
	for _, sr := range results {
		var doc Document
		if err := json.Unmarshal(sr.Value, &doc); err != nil {
			continue
		}
		allDocs = append(allDocs, &doc)
	}

	// Apply where-clause filters
	filtered := applyFilters(allDocs, q.Where)

	// Apply ordering
	if len(q.OrderBy) > 0 {
		applyOrdering(filtered, q.OrderBy)
	}

	// Apply StartAfter pagination
	if q.StartAfter != "" {
		idx := -1
		for i, doc := range filtered {
			if doc.ID == q.StartAfter {
				idx = i
				break
			}
		}
		if idx >= 0 && idx+1 < len(filtered) {
			filtered = filtered[idx+1:]
		} else {
			filtered = nil
		}
	}

	// Apply offset
	if q.Offset > 0 && q.Offset < len(filtered) {
		filtered = filtered[q.Offset:]
	} else if q.Offset >= len(filtered) {
		filtered = nil
	}

	// Apply limit
	if q.Limit > 0 && q.Limit < len(filtered) {
		filtered = filtered[:q.Limit]
	}

	// Clone results
	docs := make([]*Document, len(filtered))
	for i, doc := range filtered {
		docs[i] = doc.Clone()
	}

	return &QueryResult{
		Documents: docs,
		Count:     len(docs),
	}, nil
}

// applyFilters returns only documents that match all where clauses.
func applyFilters(docs []*Document, clauses []WhereClause) []*Document {
	if len(clauses) == 0 {
		return docs
	}

	result := make([]*Document, 0)
	for _, doc := range docs {
		if matchesAllClauses(doc, clauses) {
			result = append(result, doc)
		}
	}
	return result
}

// matchesAllClauses checks if a document satisfies every filter clause.
func matchesAllClauses(doc *Document, clauses []WhereClause) bool {
	for _, c := range clauses {
		val, ok := doc.Data[c.Field]
		if !ok {
			return false
		}
		if !matchClause(val, c.Operator, c.Value) {
			return false
		}
	}
	return true
}

// matchClause evaluates a single field comparison.
func matchClause(docVal interface{}, op QueryOp, filterVal interface{}) bool {
	switch op {
	case OpEq:
		return fmt.Sprintf("%v", docVal) == fmt.Sprintf("%v", filterVal)
	case OpNeq:
		return fmt.Sprintf("%v", docVal) != fmt.Sprintf("%v", filterVal)
	case OpGt, OpGte, OpLt, OpLte:
		return compareNumeric(docVal, filterVal, op)
	case OpIn:
		return matchIn(docVal, filterVal)
	case OpArrayContains:
		return matchArrayContains(docVal, filterVal)
	default:
		return false
	}
}

// compareNumeric handles >, >=, <, <= for float64 values.
func compareNumeric(a, b interface{}, op QueryOp) bool {
	af, aok := toFloat64(a)
	bf, bok := toFloat64(b)
	if !aok || !bok {
		return false
	}
	switch op {
	case OpGt:
		return af > bf
	case OpGte:
		return af >= bf
	case OpLt:
		return af < bf
	case OpLte:
		return af <= bf
	default:
		return false
	}
}

// toFloat64 coerces a value to float64.
func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

// matchIn checks if docVal is one of the values in filterVal (expected: []interface{}).
func matchIn(docVal, filterVal interface{}) bool {
	list, ok := filterVal.([]interface{})
	if !ok {
		return false
	}
	ds := fmt.Sprintf("%v", docVal)
	for _, item := range list {
		if fmt.Sprintf("%v", item) == ds {
			return true
		}
	}
	return false
}

// matchArrayContains checks if docVal (expected: []interface{}) contains filterVal.
func matchArrayContains(docVal, filterVal interface{}) bool {
	arr, ok := docVal.([]interface{})
	if !ok {
		return false
	}
	fs := fmt.Sprintf("%v", filterVal)
	for _, item := range arr {
		if fmt.Sprintf("%v", item) == fs {
			return true
		}
	}
	return false
}

// applyOrdering sorts docs by the given OrderBy clauses.
func applyOrdering(docs []*Document, orders []OrderByClause) {
	sort.SliceStable(docs, func(i, j int) bool {
		for _, o := range orders {
			vi, _ := docs[i].Data[o.Field]
			vj, _ := docs[j].Data[o.Field]

			cmp := compareValues(vi, vj)
			if cmp == 0 {
				continue
			}
			if o.Direction == "desc" {
				return cmp > 0
			}
			return cmp < 0
		}
		return false
	})
}

// compareValues compares two arbitrary values for ordering.
func compareValues(a, b interface{}) int {
	af, aok := toFloat64(a)
	bf, bok := toFloat64(b)
	if aok && bok {
		if af < bf {
			return -1
		}
		if af > bf {
			return 1
		}
		return 0
	}
	// Fall back to string comparison
	as := fmt.Sprintf("%v", a)
	bs := fmt.Sprintf("%v", b)
	return strings.Compare(as, bs)
}

// ---- Collection Management ----

// ListCollections returns the names of all collections by scanning all document keys.
func (e *Engine) ListCollections() []string {
	results, err := e.db.Scan(allDocsPrefix)
	if err != nil {
		return []string{}
	}

	// Extract unique collection names from keys like DOC/<collection>/<id>
	collSet := make(map[string]bool)
	for _, sr := range results {
		userKey := string(sr.Key)
		if !strings.HasPrefix(userKey, "DOC/") {
			continue
		}
		rest := userKey[4:] // strip "DOC/"
		slashIdx := strings.Index(rest, "/")
		if slashIdx > 0 {
			collSet[rest[:slashIdx]] = true
		}
	}

	collections := make([]string, 0, len(collSet))
	for name := range collSet {
		collections = append(collections, name)
	}
	sort.Strings(collections)
	return collections
}

// DeleteCollection removes all documents in a collection.
func (e *Engine) DeleteCollection(name string) bool {
	prefix := collectionPrefix(name)

	results, err := e.db.Scan(prefix)
	if err != nil || len(results) == 0 {
		return false
	}

	txn := e.db.BeginContext(true)
	for _, sr := range results {
		txn.Delete(sr.Key)
	}
	if err := txn.Commit(); err != nil {
		e.log.Error("DeleteCollection commit failed", logger.Fields{"error": err.Error()})
		return false
	}

	return true
}

// CollectionSize returns the number of documents in a collection.
func (e *Engine) CollectionSize(collection string) int {
	prefix := collectionPrefix(collection)

	results, err := e.db.Scan(prefix)
	if err != nil {
		return 0
	}
	return len(results)
}

// ---- Index Management ----

// CreateIndex creates a secondary index on a field for a collection.
func (e *Engine) CreateIndex(collection, field string) {
	idx := e.indexes.CreateIndex(collection, field)

	// Build index from existing documents
	docs, _ := e.ListDocuments(collection, 0, 0)
	for _, doc := range docs {
		idx.Add(doc)
	}

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

// BeginTransaction starts a new optimistic transaction.
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

// CreateSnapshot is a no-op — DynamicDB's binary WAL provides inherent durability.
// SSTable flushes act as natural compaction checkpoints.
func (e *Engine) CreateSnapshot() error {
	return nil
}

// Stats returns database statistics.
func (e *Engine) Stats() map[string]interface{} {
	collections := e.ListCollections()
	totalDocs := 0
	for _, col := range collections {
		totalDocs += e.CollectionSize(col)
	}

	return map[string]interface{}{
		"backend":     "DynamicDB/LSM+MVCC",
		"status":      "operational",
		"collections": len(collections),
		"documents":   totalDocs,
	}
}

// Close shuts down the database engine gracefully.
func (e *Engine) Close() error {
	return e.db.Close()
}

// ---- ID Generation ----

// generateID creates a unique document ID using nanosecond timestamp.
func generateID() string {
	return fmt.Sprintf("%016x", time.Now().UnixNano())
}
