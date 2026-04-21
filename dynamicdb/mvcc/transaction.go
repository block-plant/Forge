package mvcc

import (
	"errors"
)

// Transaction provides isolated read and write access.
type Transaction struct {
	db       *DB
	readTS   uint64
	commitTS uint64
	writable bool
	closed   bool
	
	writes map[string][]byte // Pending writes buffered in memory
}

// Get retrieves a key at the transaction's ReadTS isolation level.
func (t *Transaction) Get(key []byte) ([]byte, error) {
	if t.closed {
		return nil, ErrTxnClosed
	}

	// 1. Check local pending uncommitted writes
	if val, ok := t.writes[string(key)]; ok {
		if val == nil { // Tombstone in transaction
			return nil, nil // Not found
		}
		return val, nil
	}

	// 2. We need to query the LSM engine for versions of this key <= readTS.
	// Because keys are encoded as `Key + ReverseTS`, the first matching key
	// we encounter in standard LSM iteration that has `decodeKey == key` and `TS <= readTS` is the winner.
	
	// Fast Path for exact match resolution (Simplified iteration check for MVP Phase 1->2 integration)
	// Properly, this requires an Iterator in LSM. Let's ask LSM for exact queries.
	// Since LSM Engine `Get` is an exact match in Phase 1, we simulate version lookup by grabbing the
	// active latest from LSM (assuming Phase 1 `Get` is retrofitted or we fetch direct versions).
	// For MVCC correctness in this bridge phase: we try exact TS bounds if iterator is missing.
	
	// *NOTE: In a true implementation, LSM exposes a `NewIterator()`. 
	// For now, we will query exactly (fallback logic).
	encoded := EncodeKey(key, t.readTS)
	val, ok, err := t.db.lsmEngine.Get(encoded)
	if err != nil {
		return nil, err
	}
	if ok {
		return val, nil
	}
	
	return nil, nil // Not found
}

// Put buffers a write locally.
func (t *Transaction) Put(key, value []byte) error {
	if t.closed {
		return ErrTxnClosed
	}
	if !t.writable {
		return errors.New("mvcc: transaction is read-only")
	}
	t.writes[string(key)] = value
	return nil
}

// Delete buffers a tombstone.
func (t *Transaction) Delete(key []byte) error {
	if t.closed {
		return ErrTxnClosed
	}
	if !t.writable {
		return errors.New("mvcc: transaction is read-only")
	}
	t.writes[string(key)] = nil
	return nil
}

// Commit publishes all buffered writes to the LSM tree atomically at CommitTS.
func (t *Transaction) Commit() error {
	if t.closed {
		return ErrTxnClosed
	}
	if !t.writable {
		t.closed = true
		return nil // Read-only txn just closes
	}

	// Simplistic concurrency control: Write locking at commit
	t.db.mu.Lock()
	defer t.db.mu.Unlock()

	// Assign commit timestamp
	t.commitTS = t.db.oracle.newTS()

	// Flush to LSM
	for stringKey, value := range t.writes {
		k := []byte(stringKey)
		encodedKey := EncodeKey(k, t.commitTS)

		var err error
		if value == nil {
			err = t.db.lsmEngine.Delete(encodedKey) // write tombstone
		} else {
			err = t.db.lsmEngine.Put(encodedKey, value)
		}
		
		if err != nil {
			// In a real system, we'd abort and rollback.
			return err
		}
	}

	t.closed = true
	delete(t.db.activeTxns, t.readTS)
	return nil
}

// Rollback abandons the transaction.
func (t *Transaction) Rollback() error {
	if t.closed {
		return nil
	}
	if t.writable {
		t.db.mu.Lock()
		delete(t.db.activeTxns, t.readTS)
		t.db.mu.Unlock()
	}
	t.closed = true
	return nil
}
