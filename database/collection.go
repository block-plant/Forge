package database

import (
	"fmt"
	"sync"
)

// Collection provides a high-level, Firestore-like interface over the
// underlying B-tree storage. It wraps the Engine methods with a fluent,
// chainable API for building queries and performing CRUD operations.
type Collection struct {
	engine *Engine
	name   string
}

// CollectionRef returns a Collection handle for the given name.
func (e *Engine) Collection(name string) *Collection {
	return &Collection{engine: e, name: name}
}

// Doc returns a DocumentRef pointing at a specific document in this collection.
func (c *Collection) Doc(id string) *DocumentRef {
	return &DocumentRef{collection: c, id: id}
}

// Add creates a new document with an auto-generated ID.
func (c *Collection) Add(data map[string]interface{}) (*Document, error) {
	return c.engine.SetDocument(c.name, "", data)
}

// Get returns all documents in this collection (with optional limit/offset).
func (c *Collection) Get(limit, offset int) ([]*Document, int) {
	return c.engine.ListDocuments(c.name, limit, offset)
}

// Where starts building a query chain on this collection.
func (c *Collection) Where(field, op string, value interface{}) *CollectionQuery {
	q := &CollectionQuery{
		collection: c,
		query: &Query{
			Collection: c.name,
			Where:      []WhereClause{{Field: field, Operator: QueryOp(op), Value: value}},
		},
	}
	return q
}

// Count returns the number of documents in the collection.
func (c *Collection) Count() int {
	return c.engine.CollectionSize(c.name)
}

// Delete removes the entire collection.
func (c *Collection) Delete() bool {
	return c.engine.DeleteCollection(c.name)
}

// OnSnapshot registers a real-time listener for changes in this collection.
func (c *Collection) OnSnapshot(callback func(event *ChangeEvent)) {
	c.engine.OnChange(func(event *ChangeEvent) {
		if event.Collection == c.name {
			callback(event)
		}
	})
}

// ---- DocumentRef ----

// DocumentRef points at a single document within a collection.
type DocumentRef struct {
	collection *Collection
	id         string
}

// Set creates or overwrites the document.
func (r *DocumentRef) Set(data map[string]interface{}) (*Document, error) {
	return r.collection.engine.SetDocument(r.collection.name, r.id, data)
}

// Get retrieves the document.
func (r *DocumentRef) Get() (*Document, error) {
	doc := r.collection.engine.GetDocument(r.collection.name, r.id)
	if doc == nil {
		return nil, fmt.Errorf("document %s/%s not found", r.collection.name, r.id)
	}
	return doc, nil
}

// Update merges fields into the existing document.
func (r *DocumentRef) Update(data map[string]interface{}) (*Document, error) {
	return r.collection.engine.UpdateDocument(r.collection.name, r.id, data)
}

// Delete removes the document.
func (r *DocumentRef) Delete() error {
	return r.collection.engine.DeleteDocument(r.collection.name, r.id)
}

// OnSnapshot registers a real-time listener for changes to this specific document.
func (r *DocumentRef) OnSnapshot(callback func(event *ChangeEvent)) {
	r.collection.engine.OnChange(func(event *ChangeEvent) {
		if event.Collection == r.collection.name && event.DocumentID == r.id {
			callback(event)
		}
	})
}

// ---- CollectionQuery (chainable) ----

// CollectionQuery is a fluent query builder for a collection.
type CollectionQuery struct {
	collection *Collection
	query      *Query
	mu         sync.Mutex
}

// Where adds another filter to the query chain.
func (cq *CollectionQuery) Where(field, op string, value interface{}) *CollectionQuery {
	cq.mu.Lock()
	defer cq.mu.Unlock()
	cq.query.Where = append(cq.query.Where, WhereClause{Field: field, Operator: QueryOp(op), Value: value})
	return cq
}

// OrderBy sets sort order on the query.
func (cq *CollectionQuery) OrderBy(field, direction string) *CollectionQuery {
	cq.mu.Lock()
	defer cq.mu.Unlock()
	cq.query.OrderBy = append(cq.query.OrderBy, OrderByClause{Field: field, Direction: direction})
	return cq
}

// Limit sets a maximum number of results.
func (cq *CollectionQuery) Limit(n int) *CollectionQuery {
	cq.mu.Lock()
	defer cq.mu.Unlock()
	cq.query.Limit = n
	return cq
}

// Execute runs the query and returns results.
func (cq *CollectionQuery) Execute() (*QueryResult, error) {
	return cq.collection.engine.ExecuteQuery(cq.query)
}
