package realtime

import (
	"sync"

	"github.com/ayushkunwarsingh/forge/dynamicdb/mvcc"
)

// StreamHub orchestrates change-data-capture broadcasting dynamically.
type StreamHub struct {
	mu          sync.RWMutex
	listeners   map[string][]chan *mvcc.Transaction
}

func NewStreamHub() *StreamHub {
	return &StreamHub{
		listeners: make(map[string][]chan *mvcc.Transaction),
	}
}

// Subscribe binds a client channel to a specific namespace or subset query context.
func (h *StreamHub) Subscribe(collection string) <-chan *mvcc.Transaction {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan *mvcc.Transaction, 16)
	h.listeners[collection] = append(h.listeners[collection], ch)
	return ch
}

// Publish notifies clients of committed MVCC mutations asynchronously.
func (h *StreamHub) Publish(collection string, txn *mvcc.Transaction) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, ch := range h.listeners[collection] {
		select {
		case ch <- txn:
		default:
			// Non-blocking drop if client is saturated, alternatively queue logic
		}
	}
}
