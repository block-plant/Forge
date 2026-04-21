package database

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// CompositeKey is a multi-field index key formed by concatenating sorted field values.
type CompositeKey string

// CompositeIndex is a secondary index spanning multiple fields of a collection.
// It answers equality + range queries that touch all indexed fields together.
type CompositeIndex struct {
	mu         sync.RWMutex
	Collection string
	Fields     []string            // ordered list of fields
	entries    map[CompositeKey][]string // composite key → document IDs
}

// NewCompositeIndex creates a new composite index for the given collection and fields.
// Fields must be provided in the same order they will be queried.
func NewCompositeIndex(collection string, fields []string) *CompositeIndex {
	return &CompositeIndex{
		Collection: collection,
		Fields:     fields,
		entries:    make(map[CompositeKey][]string),
	}
}

// makeKey builds a composite key from a document's data for the indexed fields.
// Returns ("", false) if any field is missing.
func (ci *CompositeIndex) makeKey(doc *Document) (CompositeKey, bool) {
	parts := make([]string, 0, len(ci.Fields))
	for _, field := range ci.Fields {
		val, ok := doc.Data[field]
		if !ok {
			return "", false
		}
		parts = append(parts, valueToIndexKey(val))
	}
	return CompositeKey(strings.Join(parts, "\x00")), true
}

// Add inserts a document into this composite index.
func (ci *CompositeIndex) Add(doc *Document) {
	key, ok := ci.makeKey(doc)
	if !ok {
		return
	}

	ci.mu.Lock()
	defer ci.mu.Unlock()

	ids := ci.entries[key]
	for _, id := range ids {
		if id == doc.ID {
			return // already indexed
		}
	}
	ci.entries[key] = append(ids, doc.ID)
}

// Remove deletes a document from this composite index.
func (ci *CompositeIndex) Remove(doc *Document) {
	key, ok := ci.makeKey(doc)
	if !ok {
		return
	}

	ci.mu.Lock()
	defer ci.mu.Unlock()

	ids := ci.entries[key]
	for i, id := range ids {
		if id == doc.ID {
			ci.entries[key] = append(ids[:i], ids[i+1:]...)
			if len(ci.entries[key]) == 0 {
				delete(ci.entries, key)
			}
			return
		}
	}
}

// Update re-indexes a document after an update.
func (ci *CompositeIndex) Update(old, new *Document) {
	ci.Remove(old)
	ci.Add(new)
}

// LookupExact returns document IDs whose fields exactly match all provided values.
// values must be in the same order as ci.Fields.
func (ci *CompositeIndex) LookupExact(values []interface{}) []string {
	if len(values) != len(ci.Fields) {
		return nil
	}

	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = valueToIndexKey(v)
	}
	key := CompositeKey(strings.Join(parts, "\x00"))

	ci.mu.RLock()
	defer ci.mu.RUnlock()

	ids, ok := ci.entries[key]
	if !ok {
		return nil
	}
	result := make([]string, len(ids))
	copy(result, ids)
	return result
}

// Size returns number of distinct composite keys in the index.
func (ci *CompositeIndex) Size() int {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return len(ci.entries)
}

// Name returns a human-readable name for this index, e.g. "posts[uid,createdAt]".
func (ci *CompositeIndex) Name() string {
	return fmt.Sprintf("%s[%s]", ci.Collection, strings.Join(ci.Fields, ","))
}

// ---- CompositeIndexManager ----

// CompositeIndexManager holds all composite indexes across all collections.
type CompositeIndexManager struct {
	mu      sync.RWMutex
	indexes map[string][]*CompositeIndex // collection → list of composite indexes
}

// NewCompositeIndexManager creates a new manager.
func NewCompositeIndexManager() *CompositeIndexManager {
	return &CompositeIndexManager{
		indexes: make(map[string][]*CompositeIndex),
	}
}

// Register adds a new composite index definition.
// Silently ignores duplicates (same collection + same field set).
func (m *CompositeIndexManager) Register(collection string, fields []string) *CompositeIndex {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for exact duplicate
	for _, existing := range m.indexes[collection] {
		if fieldsEqual(existing.Fields, fields) {
			return existing
		}
	}

	idx := NewCompositeIndex(collection, fields)
	m.indexes[collection] = append(m.indexes[collection], idx)
	return idx
}

// IndexDocument inserts a document into all composite indexes for its collection.
func (m *CompositeIndexManager) IndexDocument(collection string, doc *Document) {
	m.mu.RLock()
	idxList := m.indexes[collection]
	m.mu.RUnlock()

	for _, idx := range idxList {
		idx.Add(doc)
	}
}

// UnindexDocument removes a document from all composite indexes for its collection.
func (m *CompositeIndexManager) UnindexDocument(collection string, doc *Document) {
	m.mu.RLock()
	idxList := m.indexes[collection]
	m.mu.RUnlock()

	for _, idx := range idxList {
		idx.Remove(doc)
	}
}

// UpdateDocument updates all composite indexes for a changed document.
func (m *CompositeIndexManager) UpdateDocument(collection string, old, new *Document) {
	m.mu.RLock()
	idxList := m.indexes[collection]
	m.mu.RUnlock()

	for _, idx := range idxList {
		idx.Update(old, new)
	}
}

// FindIndex returns the best composite index that can satisfy all requested fields, or nil.
func (m *CompositeIndexManager) FindIndex(collection string, fields []string) *CompositeIndex {
	m.mu.RLock()
	idxList := m.indexes[collection]
	m.mu.RUnlock()

	// Build a sortable lookup set for fast subset check
	want := make(map[string]bool, len(fields))
	for _, f := range fields {
		want[f] = true
	}

	// Prefer the index whose fields are a subset of what we want, with most coverage
	var best *CompositeIndex
	for _, idx := range idxList {
		if len(idx.Fields) > len(fields) {
			continue // index needs more specificity than the query provides
		}
		allCovered := true
		for _, f := range idx.Fields {
			if !want[f] {
				allCovered = false
				break
			}
		}
		if allCovered && (best == nil || len(idx.Fields) > len(best.Fields)) {
			best = idx
		}
	}
	return best
}

// List returns all composite indexes for a collection, sorted by name.
func (m *CompositeIndexManager) List(collection string) []*CompositeIndex {
	m.mu.RLock()
	defer m.mu.RUnlock()
	idxList := m.indexes[collection]
	result := make([]*CompositeIndex, len(idxList))
	copy(result, idxList)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name() < result[j].Name()
	})
	return result
}

// ListAll returns all composite indexes, grouped by collection.
func (m *CompositeIndexManager) ListAll() map[string][]*CompositeIndex {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string][]*CompositeIndex, len(m.indexes))
	for col, list := range m.indexes {
		cp := make([]*CompositeIndex, len(list))
		copy(cp, list)
		out[col] = cp
	}
	return out
}

// fieldsEqual returns true if two field slices have the same elements in the same order.
func fieldsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
