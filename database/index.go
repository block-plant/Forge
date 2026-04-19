package database

import (
	"fmt"
	"sort"
	"sync"
)

// Index is a secondary B-tree index on a specific field across documents in a collection.
// It maps field values to sets of document IDs for fast lookup.
type Index struct {
	mu         sync.RWMutex
	Field      string
	Collection string
	entries    map[string][]string // field value (as string) → document IDs
}

// NewIndex creates a new secondary index for the given field.
func NewIndex(collection, field string) *Index {
	return &Index{
		Field:      field,
		Collection: collection,
		entries:    make(map[string][]string),
	}
}

// Add indexes a document by extracting the field value and mapping it to the doc ID.
func (idx *Index) Add(doc *Document) {
	val, ok := doc.Data[idx.Field]
	if !ok {
		return
	}

	key := valueToIndexKey(val)

	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Avoid duplicates
	ids := idx.entries[key]
	for _, id := range ids {
		if id == doc.ID {
			return
		}
	}
	idx.entries[key] = append(ids, doc.ID)
}

// Remove removes a document from the index.
func (idx *Index) Remove(doc *Document) {
	val, ok := doc.Data[idx.Field]
	if !ok {
		return
	}

	key := valueToIndexKey(val)

	idx.mu.Lock()
	defer idx.mu.Unlock()

	ids := idx.entries[key]
	for i, id := range ids {
		if id == doc.ID {
			idx.entries[key] = append(ids[:i], ids[i+1:]...)
			if len(idx.entries[key]) == 0 {
				delete(idx.entries, key)
			}
			return
		}
	}
}

// Update re-indexes a document (removes old, adds new).
func (idx *Index) Update(oldDoc, newDoc *Document) {
	idx.Remove(oldDoc)
	idx.Add(newDoc)
}

// Lookup returns document IDs matching the exact field value.
func (idx *Index) Lookup(value interface{}) []string {
	key := valueToIndexKey(value)

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	ids, ok := idx.entries[key]
	if !ok {
		return nil
	}

	// Return a copy to avoid race conditions
	result := make([]string, len(ids))
	copy(result, ids)
	return result
}

// Size returns the number of indexed values.
func (idx *Index) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.entries)
}

// IndexManager manages all secondary indexes for all collections.
type IndexManager struct {
	mu      sync.RWMutex
	indexes map[string]map[string]*Index // collection → field → Index
}

// NewIndexManager creates a new index manager.
func NewIndexManager() *IndexManager {
	return &IndexManager{
		indexes: make(map[string]map[string]*Index),
	}
}

// CreateIndex creates a new secondary index on a field for a collection.
// If the index already exists, it returns the existing one.
func (im *IndexManager) CreateIndex(collection, field string) *Index {
	im.mu.Lock()
	defer im.mu.Unlock()

	if _, ok := im.indexes[collection]; !ok {
		im.indexes[collection] = make(map[string]*Index)
	}

	if idx, ok := im.indexes[collection][field]; ok {
		return idx
	}

	idx := NewIndex(collection, field)
	im.indexes[collection][field] = idx
	return idx
}

// GetIndex returns an index for a collection/field, or nil if it doesn't exist.
func (im *IndexManager) GetIndex(collection, field string) *Index {
	im.mu.RLock()
	defer im.mu.RUnlock()

	colIndexes, ok := im.indexes[collection]
	if !ok {
		return nil
	}
	return colIndexes[field]
}

// IndexDocument updates all indexes for a collection when a document is added.
func (im *IndexManager) IndexDocument(collection string, doc *Document) {
	im.mu.RLock()
	colIndexes, ok := im.indexes[collection]
	im.mu.RUnlock()

	if !ok {
		return
	}

	for _, idx := range colIndexes {
		idx.Add(doc)
	}
}

// UnindexDocument removes a document from all indexes.
func (im *IndexManager) UnindexDocument(collection string, doc *Document) {
	im.mu.RLock()
	colIndexes, ok := im.indexes[collection]
	im.mu.RUnlock()

	if !ok {
		return
	}

	for _, idx := range colIndexes {
		idx.Remove(doc)
	}
}

// UpdateDocument updates all indexes when a document changes.
func (im *IndexManager) UpdateDocument(collection string, oldDoc, newDoc *Document) {
	im.mu.RLock()
	colIndexes, ok := im.indexes[collection]
	im.mu.RUnlock()

	if !ok {
		return
	}

	for _, idx := range colIndexes {
		idx.Update(oldDoc, newDoc)
	}
}

// ListIndexes returns all indexed fields for a collection.
func (im *IndexManager) ListIndexes(collection string) []string {
	im.mu.RLock()
	defer im.mu.RUnlock()

	colIndexes, ok := im.indexes[collection]
	if !ok {
		return nil
	}

	fields := make([]string, 0, len(colIndexes))
	for field := range colIndexes {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

// valueToIndexKey converts any value to a string key for indexing.
func valueToIndexKey(v interface{}) string {
	if v == nil {
		return "__null__"
	}

	switch val := v.(type) {
	case string:
		return "s:" + val
	case float64:
		return "n:" + formatFloat(val)
	case bool:
		if val {
			return "b:true"
		}
		return "b:false"
	default:
		return "o:" + fmt.Sprintf("%v", v)
	}
}

// formatFloat formats a float64 as a string preserving sort order.
func formatFloat(f float64) string {
	return fmt.Sprintf("%020.6f", f)
}
