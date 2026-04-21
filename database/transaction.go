package database

import (
	"fmt"
	"sync"
)

const (
	OpSet    = "SET"
	OpUpdate = "UPDATE"
	OpDelete = "DELETE"
)

// Transaction represents an atomic batch of read-modify-write operations.
// Uses optimistic concurrency control — reads snapshot versions, and
// the commit fails if any document was modified between read and write.
type Transaction struct {
	mu         sync.Mutex
	engine     *Engine
	reads      map[string]map[string]int64 // collection → docID → version at read time
	writes     []transactionWrite
	committed  bool
	aborted    bool
	maxWrites  int
}

// transactionWrite is a pending write in a transaction.
type transactionWrite struct {
	Operation  string
	Collection string
	DocumentID string
	Data       map[string]interface{}
}

// NewTransaction creates a new transaction.
func NewTransaction(engine *Engine, maxWrites int) *Transaction {
	return &Transaction{
		engine:    engine,
		reads:     make(map[string]map[string]int64),
		maxWrites: maxWrites,
	}
}

// Get reads a document within the transaction and records its version.
func (tx *Transaction) Get(collection, docID string) (*Document, error) {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.aborted {
		return nil, fmt.Errorf("transaction: already completed")
	}

	doc := tx.engine.GetDocument(collection, docID)

	// Record the version we read
	if _, ok := tx.reads[collection]; !ok {
		tx.reads[collection] = make(map[string]int64)
	}

	if doc != nil {
		tx.reads[collection][docID] = doc.Version
		return doc.Clone(), nil
	}

	tx.reads[collection][docID] = 0 // Document didn't exist
	return nil, nil
}

// Set schedules a document overwrite in the transaction.
func (tx *Transaction) Set(collection, docID string, data map[string]interface{}) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.aborted {
		return fmt.Errorf("transaction: already completed")
	}

	if len(tx.writes) >= tx.maxWrites {
		return fmt.Errorf("transaction: max writes exceeded (%d)", tx.maxWrites)
	}

	tx.writes = append(tx.writes, transactionWrite{
		Operation:  OpSet,
		Collection: collection,
		DocumentID: docID,
		Data:       data,
	})

	return nil
}

// Update schedules a document merge-update in the transaction.
func (tx *Transaction) Update(collection, docID string, data map[string]interface{}) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.aborted {
		return fmt.Errorf("transaction: already completed")
	}

	if len(tx.writes) >= tx.maxWrites {
		return fmt.Errorf("transaction: max writes exceeded (%d)", tx.maxWrites)
	}

	tx.writes = append(tx.writes, transactionWrite{
		Operation:  OpUpdate,
		Collection: collection,
		DocumentID: docID,
		Data:       data,
	})

	return nil
}

// Delete schedules a document deletion in the transaction.
func (tx *Transaction) Delete(collection, docID string) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.aborted {
		return fmt.Errorf("transaction: already completed")
	}

	if len(tx.writes) >= tx.maxWrites {
		return fmt.Errorf("transaction: max writes exceeded (%d)", tx.maxWrites)
	}

	tx.writes = append(tx.writes, transactionWrite{
		Operation:  OpDelete,
		Collection: collection,
		DocumentID: docID,
	})

	return nil
}

// Commit validates all reads and applies all writes atomically.
// Returns an error if any document was modified since reading (conflict).
func (tx *Transaction) Commit() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.aborted {
		return fmt.Errorf("transaction: already completed")
	}

	// Phase 1: Validate reads (optimistic concurrency check)
	for collection, docs := range tx.reads {
		for docID, readVersion := range docs {
			current := tx.engine.GetDocument(collection, docID)
			if current == nil && readVersion != 0 {
				tx.aborted = true
				return fmt.Errorf("transaction: conflict — document %s/%s was deleted", collection, docID)
			}
			if current != nil && current.Version != readVersion {
				tx.aborted = true
				return fmt.Errorf("transaction: conflict — document %s/%s was modified (v%d → v%d)",
					collection, docID, readVersion, current.Version)
			}
		}
	}

	// Phase 2: Apply all writes
	for _, w := range tx.writes {
		var err error
		switch w.Operation {
		case OpSet:
			_, err = tx.engine.SetDocument(w.Collection, w.DocumentID, w.Data)
		case OpUpdate:
			_, err = tx.engine.UpdateDocument(w.Collection, w.DocumentID, w.Data)
		case OpDelete:
			err = tx.engine.DeleteDocument(w.Collection, w.DocumentID)
		}
		if err != nil {
			tx.aborted = true
			return fmt.Errorf("transaction: write failed: %w", err)
		}
	}

	tx.committed = true
	return nil
}

// Abort cancels the transaction without applying any writes.
func (tx *Transaction) Abort() {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	tx.aborted = true
}

// BatchWrite performs a batch of write operations atomically (no read validation).
type BatchWrite struct {
	writes []transactionWrite
	engine *Engine
}

// NewBatchWrite creates a new batch write operation.
func NewBatchWrite(engine *Engine) *BatchWrite {
	return &BatchWrite{engine: engine}
}

// Set adds a set operation to the batch.
func (bw *BatchWrite) Set(collection, docID string, data map[string]interface{}) {
	bw.writes = append(bw.writes, transactionWrite{
		Operation:  OpSet,
		Collection: collection,
		DocumentID: docID,
		Data:       data,
	})
}

// Update adds an update operation to the batch.
func (bw *BatchWrite) Update(collection, docID string, data map[string]interface{}) {
	bw.writes = append(bw.writes, transactionWrite{
		Operation:  OpUpdate,
		Collection: collection,
		DocumentID: docID,
		Data:       data,
	})
}

// Delete adds a delete operation to the batch.
func (bw *BatchWrite) Delete(collection, docID string) {
	bw.writes = append(bw.writes, transactionWrite{
		Operation:  OpDelete,
		Collection: collection,
		DocumentID: docID,
	})
}

// Commit executes all operations in the batch.
func (bw *BatchWrite) Commit() error {
	for _, w := range bw.writes {
		var err error
		switch w.Operation {
		case OpSet:
			_, err = bw.engine.SetDocument(w.Collection, w.DocumentID, w.Data)
		case OpUpdate:
			_, err = bw.engine.UpdateDocument(w.Collection, w.DocumentID, w.Data)
		case OpDelete:
			err = bw.engine.DeleteDocument(w.Collection, w.DocumentID)
		}
		if err != nil {
			return fmt.Errorf("batch: write failed: %w", err)
		}
	}
	return nil
}
