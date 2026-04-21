package mvcc

import (
	"encoding/binary"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ayushkunwarsingh/forge/dynamicdb/lsm"
)

var (
	ErrWriteConflict = errors.New("mvcc: write conflict")
	ErrTxnClosed     = errors.New("mvcc: transaction closed")
)

// DB wraps the LSM engine to provide Multi-Version Concurrency Control (MVCC).
type DB struct {
	lsmEngine *lsm.Engine
	oracle    *Oracle
	
	// Track active write transactions to detect conflicts
	mu         sync.Mutex
	activeTxns map[uint64]*Transaction
}

// Oracle dispenses globally increasing timestamps.
type Oracle struct {
	nextTS uint64
}

// Open initializes the MVCC layer over an LSM engine.
func Open(lsmDir string) (*DB, error) {
	engine, err := lsm.Open(lsmDir)
	if err != nil {
		return nil, err
	}

	return &DB{
		lsmEngine: engine,
		oracle:    &Oracle{nextTS: uint64(time.Now().UnixNano())},
		activeTxns: make(map[uint64]*Transaction),
	}, nil
}

// newTS returns a strictly increasing timestamp.
func (o *Oracle) newTS() uint64 {
	return atomic.AddUint64(&o.nextTS, 1)
}

// BeginContext starts a new MVCC transaction.
func (db *DB) BeginContext(writable bool) *Transaction {
	readTS := db.oracle.newTS()
	
	txn := &Transaction{
		db:       db,
		readTS:   readTS,
		writable: writable,
		writes:   make(map[string][]byte),
	}

	if writable {
		db.mu.Lock()
		db.activeTxns[readTS] = txn
		db.mu.Unlock()
	}

	return txn
}

// Close gracefully shutdowns the database.
func (db *DB) Close() error {
	return db.lsmEngine.Close()
}

// EncodeKey encodes a user key and a timestamp into an LSM key.
// We subtract the timestamp from MaxUint64 so higher timestamps sort first in the LSM byte array!
func EncodeKey(key []byte, ts uint64) []byte {
	kLen := len(key)
	encoded := make([]byte, kLen+8)
	copy(encoded, key)
	// Subtract to reverse sort order
	revTS := math.MaxUint64 - ts
	binary.BigEndian.PutUint64(encoded[kLen:], revTS)
	return encoded
}

// DecodeKey extracts the user key and timestamp.
func DecodeKey(encoded []byte) ([]byte, uint64) {
	kLen := len(encoded) - 8
	key := make([]byte, kLen)
	copy(key, encoded[:kLen])
	
	revTS := binary.BigEndian.Uint64(encoded[kLen:])
	ts := math.MaxUint64 - revTS
	return key, ts
}
