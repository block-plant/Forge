package lsm

import (
	"bytes"
	"math/rand"
	"sync"
	"time"
)

// EntryType defines the mutation type of a log record.
type EntryType byte

const (
	// TypePut indicates a new key-value insertion or update.
	TypePut EntryType = 1
	// TypeDelete indicates a tombstone marker for a deleted key.
	TypeDelete EntryType = 2
)

// Entry represents a single record in the LSM tree.
type Entry struct {
	Key   []byte
	Value []byte
	Type  EntryType
}

const (
	defaultMaxHeight = 16
	probability      = 0.5
)

// skipNode is a node within the MemTable skip list.
type skipNode struct {
	key       []byte
	value     []byte
	entryType EntryType
	next      []*skipNode
}

// MemTable is an in-memory sparse SkipList that caches recent database writes
// before they are flushed to Immutable SSTables on disk.
type MemTable struct {
	mu        sync.RWMutex
	head      *skipNode
	maxHeight int
	size      int64 // Tracks approximate byte size of contents for flush threshold
	randSource *rand.Rand
}

// NewMemTable initializes a new empty SkipList optimized for concurrent operations.
func NewMemTable() *MemTable {
	return &MemTable{
		head:      &skipNode{next: make([]*skipNode, defaultMaxHeight)},
		maxHeight: defaultMaxHeight,
		size:      0,
		randSource: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// randomHeight generates a geometric probability height for a new node.
func (m *MemTable) randomHeight() int {
	height := 1
	for height < m.maxHeight && m.randSource.Float32() < probability {
		height++
	}
	return height
}

// Put inserts or updates a key in the MemTable.
// If the key exists, it updates the value and returns the old byte size.
func (m *MemTable) Put(key, value []byte, eType EntryType) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	update := make([]*skipNode, m.maxHeight)
	current := m.head

	// Traverse the skip list from highest level to lowest
	for i := m.maxHeight - 1; i >= 0; i-- {
		for current.next[i] != nil && bytes.Compare(current.next[i].key, key) < 0 {
			current = current.next[i]
		}
		update[i] = current
	}

	current = current.next[0]

	// Key exactly matches: it's an update
	if current != nil && bytes.Equal(current.key, key) {
		oldSize := int64(len(current.value))
		current.value = make([]byte, len(value))
		copy(current.value, value)
		current.entryType = eType
		
		delta := int64(len(value)) - oldSize
		m.size += delta
		return delta
	}

	// Insert new node
	level := m.randomHeight()
	newNode := &skipNode{
		key:       make([]byte, len(key)),
		value:     make([]byte, len(value)),
		entryType: eType,
		next:      make([]*skipNode, level),
	}
	
	// Copy exactly to avoid slice memory leaks from larger backing arrays
	copy(newNode.key, key)
	copy(newNode.value, value)

	for i := 0; i < level; i++ {
		newNode.next[i] = update[i].next[i]
		update[i].next[i] = newNode
	}

	// Calculate added byte footprint (Key + Value + Type overhead + pointers)
	addedBytes := int64(len(key) + len(value) + 1 + (level * 8))
	m.size += addedBytes
	return addedBytes
}

// Get retrieves an entry from the MemTable.
// Returns the value and true if found. Note: A tombstone returns (nil, true, TypeDelete).
func (m *MemTable) Get(key []byte) ([]byte, bool, EntryType) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	current := m.head
	for i := m.maxHeight - 1; i >= 0; i-- {
		for current.next[i] != nil && bytes.Compare(current.next[i].key, key) < 0 {
			current = current.next[i]
		}
	}

	current = current.next[0]

	if current != nil && bytes.Equal(current.key, key) {
		return current.value, true, current.entryType
	}

	return nil, false, 0
}

// Size returns the approximate memory footprint of the MemTable.
func (m *MemTable) Size() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.size
}

// Iterator returns a functional iterator ascending through all elements.
func (m *MemTable) Iterate(fn func(key, value []byte, eType EntryType) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	current := m.head.next[0]
	for current != nil {
		if !fn(current.key, current.value, current.entryType) {
			break
		}
		current = current.next[0]
	}
}
