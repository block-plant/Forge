package lsm

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	maxMemTableSize = 4 * 1024 * 1024 // 4MB
)

// Engine represents the top-level LSM Tree manager.
type Engine struct {
	mu           sync.RWMutex
	dir          string
	
	activeMem    *MemTable
	wal          *WAL
	
	immutable    []*MemTable
	
	// sstables stores references to disk files, organized by level.
	// For Phase 1 we use a flattened L0 listing.
	sstables     []*SSTableReader 
	
	flushCh      chan *MemTable
	closeCh      chan struct{}
	wg           sync.WaitGroup
}

// Open creates or loads an LSM Engine from disk.
func Open(dir string) (*Engine, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	walPath := filepath.Join(dir, "active.wal")
	wal, err := NewWAL(walPath)
	if err != nil {
		return nil, err
	}

	e := &Engine{
		dir:       dir,
		activeMem: NewMemTable(),
		wal:       wal,
		flushCh:   make(chan *MemTable, 16),
		closeCh:   make(chan struct{}),
	}

	// Replay WAL
	var totalRecovered int
	err = wal.Replay(func(key, value []byte, eType EntryType) error {
		e.activeMem.Put(key, value, eType)
		totalRecovered++
		return nil
	})

	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("lsm: failed to replay wal: %v", err)
	}

	// Load existing SSTables
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, entry := range entries {
			if filepath.Ext(entry.Name()) == ".sst" {
				reader, err := OpenSSTable(filepath.Join(dir, entry.Name()))
				if err == nil {
					e.sstables = append(e.sstables, reader)
				}
			}
		}
	}

	// Start background flusher
	e.wg.Add(1)
	go e.flusher()

	return e, nil
}

// Put inserts or updates a key-value pair.
func (e *Engine) Put(key, value []byte) error {
	return e.write(key, value, TypePut)
}

// Delete logically removes a key via tombstone.
func (e *Engine) Delete(key []byte) error {
	return e.write(key, nil, TypeDelete)
}

func (e *Engine) write(key, value []byte, eType EntryType) error {
	e.mu.Lock()

	// Rotate MemTable if full
	if e.activeMem.Size() >= maxMemTableSize {
		frozen := e.activeMem
		e.immutable = append(e.immutable, frozen)
		
		e.activeMem = NewMemTable()
		// Start a new WAL and truncate
		e.wal.Truncate() // In a strict MVCC we would rotate WAL files; for Phase 1 we truncate after queueing 
		
		// Queue flush (non-blocking mechanism needed in prod, but simplified here)
		e.flushCh <- frozen
	}
	
	// Mutate under read lock so WAL write doesn't block concurrently completely?
	// Simplified: blocking on WAL append.
	if err := e.wal.Write(key, value, eType); err != nil {
		e.mu.Unlock()
		return err
	}

	e.activeMem.Put(key, value, eType)
	e.mu.Unlock()
	return nil
}

// Get retrieves the value. It looks in MemTable, Immutable, then SSTables.
func (e *Engine) Get(key []byte) ([]byte, bool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// 1. Active MemTable
	val, ok, eType := e.activeMem.Get(key)
	if ok {
		if eType == TypeDelete {
			return nil, false, nil // Tombstone
		}
		return duplicate(val), true, nil
	}

	// 2. Immutable MemTables (LIFO order, newest first)
	for i := len(e.immutable) - 1; i >= 0; i-- {
		val, ok, eType := e.immutable[i].Get(key)
		if ok {
			if eType == TypeDelete {
				return nil, false, nil
			}
			return duplicate(val), true, nil
		}
	}

	// 3. SSTables (Newest first, assuming append order)
	// For production L0 is unordered and needs checking fully, L1+ uses bloom filters optimally.
	for i := len(e.sstables) - 1; i >= 0; i-- {
		val, ok, eType, err := e.sstables[i].Get(key)
		if err != nil {
			return nil, false, err
		}
		if ok {
			if eType == TypeDelete {
				return nil, false, nil
			}
			return duplicate(val), true, nil
		}
	}

	return nil, false, nil
}

// flusher runs in the background and writes Immutable MemTables to SSTables.
func (e *Engine) flusher() {
	defer e.wg.Done()
	
	for {
		select {
		case mem := <-e.flushCh:
			filename := fmt.Sprintf("%d.sst", time.Now().UnixNano())
			path := filepath.Join(e.dir, filename)
			
			builder, err := NewSSTableBuilder(path)
			if err != nil {
				continue // Need robust logging here
			}
			
			// MemTable Iterate returns elements in sorted order naturally
			mem.Iterate(func(k, v []byte, eType EntryType) bool {
				builder.Add(k, v, eType)
				return true
			})
			
			if err := builder.Finish(); err == nil {
				reader, _ := OpenSSTable(path)
				
				e.mu.Lock()
				if reader != nil {
					e.sstables = append(e.sstables, reader)
				}
				// Remove flushed memtable from immutable queue
				if len(e.immutable) > 0 {
					e.immutable = e.immutable[1:]
				}
				e.mu.Unlock()
			}
			
		case <-e.closeCh:
			return
		}
	}
}

// Close explicitly shuts down the LSM Engine, waiting for background flushes.
func (e *Engine) Close() error {
	close(e.closeCh)
	e.wg.Wait()
	
	e.mu.Lock()
	defer e.mu.Unlock()
	
	for _, reader := range e.sstables {
		reader.Close()
	}
	return e.wal.Close()
}

func duplicate(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}
