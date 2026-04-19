// Package database implements the Forge NoSQL document database engine.
// It provides a Firestore-like document store with collections, queries,
// indexes, transactions, and real-time change notifications.
package database

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ayushkunwarsingh/forge/utils"
)

// Document represents a single document in a collection.
// Documents are JSON objects identified by a unique ID within their collection.
type Document struct {
	// ID is the unique identifier within the collection.
	ID string `json:"_id"`
	// Data holds the document's fields as a generic map.
	Data map[string]interface{} `json:"_data"`
	// Metadata
	CreatedAt int64 `json:"_created_at"`
	UpdatedAt int64 `json:"_updated_at"`
	// Version is incremented on each update (used for optimistic concurrency).
	Version int64 `json:"_version"`
}

// NewDocument creates a new document with the given ID and data.
// If id is empty, a UUID is generated.
func NewDocument(id string, data map[string]interface{}) *Document {
	if id == "" {
		id = utils.MustGenerateUUID()
	}
	now := time.Now().UnixMilli()

	return &Document{
		ID:        id,
		Data:      data,
		CreatedAt: now,
		UpdatedAt: now,
		Version:   1,
	}
}

// Clone creates a deep copy of the document.
func (d *Document) Clone() *Document {
	dataCopy := deepCopyMap(d.Data)
	return &Document{
		ID:        d.ID,
		Data:      dataCopy,
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
		Version:   d.Version,
	}
}

// Set overwrites the document data entirely.
func (d *Document) Set(data map[string]interface{}) {
	d.Data = data
	d.UpdatedAt = time.Now().UnixMilli()
	d.Version++
}

// Update merges the given fields into the document data.
func (d *Document) Update(fields map[string]interface{}) {
	if d.Data == nil {
		d.Data = make(map[string]interface{})
	}
	for k, v := range fields {
		if v == nil {
			delete(d.Data, k) // nil means delete the field
		} else {
			d.Data[k] = v
		}
	}
	d.UpdatedAt = time.Now().UnixMilli()
	d.Version++
}

// Get retrieves a field value from the document data.
func (d *Document) Get(field string) (interface{}, bool) {
	v, ok := d.Data[field]
	return v, ok
}

// GetString retrieves a string field.
func (d *Document) GetString(field string) string {
	v, ok := d.Data[field]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}

// GetFloat retrieves a numeric field (JSON numbers are float64).
func (d *Document) GetFloat(field string) float64 {
	v, ok := d.Data[field]
	if !ok {
		return 0
	}
	f, ok := v.(float64)
	if !ok {
		return 0
	}
	return f
}

// ToJSON serializes the document to a JSON map including metadata.
func (d *Document) ToJSON() map[string]interface{} {
	result := make(map[string]interface{}, len(d.Data)+4)
	for k, v := range d.Data {
		result[k] = v
	}
	result["_id"] = d.ID
	result["_created_at"] = d.CreatedAt
	result["_updated_at"] = d.UpdatedAt
	result["_version"] = d.Version
	return result
}

// ToClientJSON serializes the document for client consumption (clean format).
func (d *Document) ToClientJSON() map[string]interface{} {
	result := make(map[string]interface{}, len(d.Data)+1)
	for k, v := range d.Data {
		result[k] = v
	}
	result["_id"] = d.ID
	return result
}

// Marshal serializes the document to JSON bytes for storage.
func (d *Document) Marshal() ([]byte, error) {
	return json.Marshal(d)
}

// UnmarshalDocument deserializes a document from JSON bytes.
func UnmarshalDocument(data []byte) (*Document, error) {
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal document: %w", err)
	}
	return &doc, nil
}

// deepCopyMap creates a deep copy of a map.
func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case map[string]interface{}:
			result[k] = deepCopyMap(val)
		case []interface{}:
			result[k] = deepCopySlice(val)
		default:
			result[k] = v
		}
	}
	return result
}

// deepCopySlice creates a deep copy of a slice.
func deepCopySlice(s []interface{}) []interface{} {
	if s == nil {
		return nil
	}
	result := make([]interface{}, len(s))
	for i, v := range s {
		switch val := v.(type) {
		case map[string]interface{}:
			result[i] = deepCopyMap(val)
		case []interface{}:
			result[i] = deepCopySlice(val)
		default:
			result[i] = v
		}
	}
	return result
}
