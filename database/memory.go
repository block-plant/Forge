package database

import (
	"fmt"
	"sync"
)

// BTree is an in-memory B-tree for storing documents indexed by ID.
// It provides O(log n) insert, delete, and lookup operations.
// Thread-safe for concurrent access via RWMutex.
type BTree struct {
	mu    sync.RWMutex
	root  *btreeNode
	order int // maximum number of children per node
	size  int
}

// btreeNode is a single node in the B-tree.
type btreeNode struct {
	keys     []string
	values   []*Document
	children []*btreeNode
	leaf     bool
}

// NewBTree creates a new B-tree with the given order.
// Order determines the max number of children per node (min keys = order/2 - 1).
func NewBTree(order int) *BTree {
	if order < 4 {
		order = 4
	}
	return &BTree{
		root: &btreeNode{
			leaf: true,
		},
		order: order,
	}
}

// Size returns the number of documents in the tree.
func (bt *BTree) Size() int {
	bt.mu.RLock()
	defer bt.mu.RUnlock()
	return bt.size
}

// Get retrieves a document by its ID. Returns nil if not found.
func (bt *BTree) Get(id string) *Document {
	bt.mu.RLock()
	defer bt.mu.RUnlock()
	return bt.search(bt.root, id)
}

// Put inserts or updates a document. Returns true if it was an update.
func (bt *BTree) Put(doc *Document) bool {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	// Check if key already exists (update case)
	if existing := bt.search(bt.root, doc.ID); existing != nil {
		// Update in-place
		bt.update(bt.root, doc)
		return true
	}

	// Insert new key
	if len(bt.root.keys) == bt.order-1 {
		// Root is full — split it
		oldRoot := bt.root
		newRoot := &btreeNode{leaf: false}
		newRoot.children = append(newRoot.children, oldRoot)
		bt.splitChild(newRoot, 0)
		bt.root = newRoot
	}

	bt.insertNonFull(bt.root, doc)
	bt.size++
	return false
}

// Delete removes a document by ID. Returns the removed document or nil.
func (bt *BTree) Delete(id string) *Document {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	doc := bt.search(bt.root, id)
	if doc == nil {
		return nil
	}

	bt.deleteKey(bt.root, id)
	bt.size--

	// If root has no keys and has a child, make the child the new root
	if len(bt.root.keys) == 0 && !bt.root.leaf {
		bt.root = bt.root.children[0]
	}

	return doc
}

// ForEach iterates over all documents in sorted order by ID.
// The callback receives each document. Return false to stop iteration.
func (bt *BTree) ForEach(fn func(*Document) bool) {
	bt.mu.RLock()
	defer bt.mu.RUnlock()
	bt.inorder(bt.root, fn)
}

// All returns all documents in sorted order.
func (bt *BTree) All() []*Document {
	bt.mu.RLock()
	defer bt.mu.RUnlock()

	docs := make([]*Document, 0, bt.size)
	bt.inorder(bt.root, func(d *Document) bool {
		docs = append(docs, d)
		return true
	})
	return docs
}

// Range returns documents with IDs in [startID, endID].
func (bt *BTree) Range(startID, endID string) []*Document {
	bt.mu.RLock()
	defer bt.mu.RUnlock()

	var docs []*Document
	bt.inorder(bt.root, func(d *Document) bool {
		if d.ID >= startID && d.ID <= endID {
			docs = append(docs, d)
		}
		return d.ID <= endID // stop after endID
	})
	return docs
}

// ---- Internal B-tree operations ----

// search finds a document by key in the subtree rooted at node.
func (bt *BTree) search(node *btreeNode, key string) *Document {
	if node == nil {
		return nil
	}

	i := 0
	for i < len(node.keys) && key > node.keys[i] {
		i++
	}

	if i < len(node.keys) && key == node.keys[i] {
		return node.values[i]
	}

	if node.leaf {
		return nil
	}

	return bt.search(node.children[i], key)
}

// update replaces a document value for an existing key.
func (bt *BTree) update(node *btreeNode, doc *Document) {
	if node == nil {
		return
	}

	i := 0
	for i < len(node.keys) && doc.ID > node.keys[i] {
		i++
	}

	if i < len(node.keys) && doc.ID == node.keys[i] {
		node.values[i] = doc
		return
	}

	if !node.leaf {
		bt.update(node.children[i], doc)
	}
}

// insertNonFull inserts a document into a node that is guaranteed not to be full.
func (bt *BTree) insertNonFull(node *btreeNode, doc *Document) {
	i := len(node.keys) - 1

	if node.leaf {
		// Insert into leaf
		node.keys = append(node.keys, "")
		node.values = append(node.values, nil)
		for i >= 0 && doc.ID < node.keys[i] {
			node.keys[i+1] = node.keys[i]
			node.values[i+1] = node.values[i]
			i--
		}
		node.keys[i+1] = doc.ID
		node.values[i+1] = doc
	} else {
		// Find child to recurse into
		for i >= 0 && doc.ID < node.keys[i] {
			i--
		}
		i++

		if len(node.children[i].keys) == bt.order-1 {
			bt.splitChild(node, i)
			if doc.ID > node.keys[i] {
				i++
			}
		}
		bt.insertNonFull(node.children[i], doc)
	}
}

// splitChild splits the i-th child of parent.
func (bt *BTree) splitChild(parent *btreeNode, i int) {
	child := parent.children[i]
	mid := (bt.order - 1) / 2

	// Create new right sibling
	sibling := &btreeNode{
		leaf:   child.leaf,
		keys:   make([]string, len(child.keys[mid+1:])),
		values: make([]*Document, len(child.values[mid+1:])),
	}
	copy(sibling.keys, child.keys[mid+1:])
	copy(sibling.values, child.values[mid+1:])

	if !child.leaf {
		sibling.children = make([]*btreeNode, len(child.children[mid+1:]))
		copy(sibling.children, child.children[mid+1:])
	}

	// Promote the middle key to parent
	midKey := child.keys[mid]
	midValue := child.values[mid]

	// Truncate child
	child.keys = child.keys[:mid]
	child.values = child.values[:mid]
	if !child.leaf {
		child.children = child.children[:mid+1]
	}

	// Insert into parent
	parent.keys = append(parent.keys, "")
	parent.values = append(parent.values, nil)
	parent.children = append(parent.children, nil)

	for j := len(parent.keys) - 1; j > i; j-- {
		parent.keys[j] = parent.keys[j-1]
		parent.values[j] = parent.values[j-1]
	}
	for j := len(parent.children) - 1; j > i+1; j-- {
		parent.children[j] = parent.children[j-1]
	}

	parent.keys[i] = midKey
	parent.values[i] = midValue
	parent.children[i+1] = sibling
}

// deleteKey removes a key from the subtree rooted at node.
func (bt *BTree) deleteKey(node *btreeNode, key string) {
	i := 0
	for i < len(node.keys) && key > node.keys[i] {
		i++
	}

	if node.leaf {
		// Case 1: Key is in leaf node
		if i < len(node.keys) && node.keys[i] == key {
			node.keys = append(node.keys[:i], node.keys[i+1:]...)
			node.values = append(node.values[:i], node.values[i+1:]...)
		}
		return
	}

	if i < len(node.keys) && node.keys[i] == key {
		// Case 2: Key is in internal node
		bt.deleteInternalKey(node, i)
	} else {
		// Case 3: Key is in a child
		bt.deleteFromChild(node, i, key)
	}
}

// deleteInternalKey handles deletion when the key is in an internal node.
func (bt *BTree) deleteInternalKey(node *btreeNode, i int) {
	minKeys := (bt.order - 1) / 2

	if len(node.children[i].keys) > minKeys {
		// Use predecessor
		pred := bt.predecessor(node.children[i])
		node.keys[i] = pred.ID
		node.values[i] = pred
		bt.deleteKey(node.children[i], pred.ID)
	} else if len(node.children[i+1].keys) > minKeys {
		// Use successor
		succ := bt.successor(node.children[i+1])
		node.keys[i] = succ.ID
		node.values[i] = succ
		bt.deleteKey(node.children[i+1], succ.ID)
	} else {
		// Merge children
		bt.merge(node, i)
		bt.deleteKey(node.children[i], node.keys[i])
	}
}

// deleteFromChild handles deletion when the key is in a child subtree.
func (bt *BTree) deleteFromChild(node *btreeNode, i int, key string) {
	minKeys := (bt.order - 1) / 2

	if i < len(node.children) && len(node.children[i].keys) <= minKeys {
		// Ensure child has enough keys
		if i > 0 && len(node.children[i-1].keys) > minKeys {
			bt.borrowFromPrev(node, i)
		} else if i < len(node.children)-1 && len(node.children[i+1].keys) > minKeys {
			bt.borrowFromNext(node, i)
		} else {
			if i < len(node.keys) {
				bt.merge(node, i)
			} else {
				bt.merge(node, i-1)
				i--
			}
		}
	}

	if i < len(node.children) {
		bt.deleteKey(node.children[i], key)
	}
}

// predecessor finds the largest key in the left subtree.
func (bt *BTree) predecessor(node *btreeNode) *Document {
	for !node.leaf {
		node = node.children[len(node.children)-1]
	}
	return node.values[len(node.values)-1]
}

// successor finds the smallest key in the right subtree.
func (bt *BTree) successor(node *btreeNode) *Document {
	for !node.leaf {
		node = node.children[0]
	}
	return node.values[0]
}

// borrowFromPrev borrows a key from the left sibling.
func (bt *BTree) borrowFromPrev(parent *btreeNode, i int) {
	child := parent.children[i]
	sibling := parent.children[i-1]

	// Shift child keys right
	child.keys = append([]string{parent.keys[i-1]}, child.keys...)
	child.values = append([]*Document{parent.values[i-1]}, child.values...)

	// Move sibling's last key to parent
	parent.keys[i-1] = sibling.keys[len(sibling.keys)-1]
	parent.values[i-1] = sibling.values[len(sibling.values)-1]

	// Move sibling's last child
	if !sibling.leaf {
		child.children = append([]*btreeNode{sibling.children[len(sibling.children)-1]}, child.children...)
		sibling.children = sibling.children[:len(sibling.children)-1]
	}

	sibling.keys = sibling.keys[:len(sibling.keys)-1]
	sibling.values = sibling.values[:len(sibling.values)-1]
}

// borrowFromNext borrows a key from the right sibling.
func (bt *BTree) borrowFromNext(parent *btreeNode, i int) {
	child := parent.children[i]
	sibling := parent.children[i+1]

	// Move parent key to child
	child.keys = append(child.keys, parent.keys[i])
	child.values = append(child.values, parent.values[i])

	// Move sibling's first key to parent
	parent.keys[i] = sibling.keys[0]
	parent.values[i] = sibling.values[0]

	// Move sibling's first child
	if !sibling.leaf {
		child.children = append(child.children, sibling.children[0])
		sibling.children = sibling.children[1:]
	}

	sibling.keys = sibling.keys[1:]
	sibling.values = sibling.values[1:]
}

// merge merges children[i+1] into children[i] with keys[i] as median.
func (bt *BTree) merge(parent *btreeNode, i int) {
	if i >= len(parent.keys) || i+1 >= len(parent.children) {
		return
	}

	left := parent.children[i]
	right := parent.children[i+1]

	// Pull down the parent key
	left.keys = append(left.keys, parent.keys[i])
	left.values = append(left.values, parent.values[i])

	// Append right's keys and children
	left.keys = append(left.keys, right.keys...)
	left.values = append(left.values, right.values...)
	if !left.leaf {
		left.children = append(left.children, right.children...)
	}

	// Remove from parent
	parent.keys = append(parent.keys[:i], parent.keys[i+1:]...)
	parent.values = append(parent.values[:i], parent.values[i+1:]...)
	parent.children = append(parent.children[:i+1], parent.children[i+2:]...)
}

// inorder traverses the tree in sorted order.
func (bt *BTree) inorder(node *btreeNode, fn func(*Document) bool) bool {
	if node == nil {
		return true
	}

	for i := 0; i < len(node.keys); i++ {
		if !node.leaf {
			if !bt.inorder(node.children[i], fn) {
				return false
			}
		}
		if !fn(node.values[i]) {
			return false
		}
	}

	if !node.leaf && len(node.children) > len(node.keys) {
		return bt.inorder(node.children[len(node.keys)], fn)
	}

	return true
}

// MemoryStore wraps a BTree to provide a named collection store.
type MemoryStore struct {
	mu          sync.RWMutex
	collections map[string]*BTree
	order       int
}

// NewMemoryStore creates a new in-memory store with B-trees of the given order.
func NewMemoryStore(order int) *MemoryStore {
	if order < 4 {
		order = 128
	}
	return &MemoryStore{
		collections: make(map[string]*BTree),
		order:       order,
	}
}

// GetCollection returns the B-tree for a collection, creating it if needed.
func (ms *MemoryStore) GetCollection(name string) *BTree {
	ms.mu.RLock()
	tree, ok := ms.collections[name]
	ms.mu.RUnlock()
	if ok {
		return tree
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	// Double-check after acquiring write lock
	if tree, ok = ms.collections[name]; ok {
		return tree
	}

	tree = NewBTree(ms.order)
	ms.collections[name] = tree
	return tree
}

// HasCollection checks if a collection exists.
func (ms *MemoryStore) HasCollection(name string) bool {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	_, ok := ms.collections[name]
	return ok
}

// DeleteCollection removes an entire collection.
func (ms *MemoryStore) DeleteCollection(name string) bool {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	_, ok := ms.collections[name]
	if ok {
		delete(ms.collections, name)
	}
	return ok
}

// ListCollections returns the names of all collections.
func (ms *MemoryStore) ListCollections() []string {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	names := make([]string, 0, len(ms.collections))
	for name := range ms.collections {
		names = append(names, name)
	}
	return names
}

// Stats returns storage statistics.
func (ms *MemoryStore) Stats() map[string]interface{} {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	stats := make(map[string]interface{})
	totalDocs := 0
	collStats := make(map[string]int, len(ms.collections))

	for name, tree := range ms.collections {
		size := tree.Size()
		collStats[name] = size
		totalDocs += size
	}

	stats["total_collections"] = len(ms.collections)
	stats["total_documents"] = totalDocs
	stats["collections"] = collStats
	return stats
}

// String returns a debug summary.
func (ms *MemoryStore) String() string {
	stats := ms.Stats()
	return fmt.Sprintf("MemoryStore{collections=%d, documents=%d}",
		stats["total_collections"], stats["total_documents"])
}
