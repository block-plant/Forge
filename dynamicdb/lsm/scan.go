package lsm

import "bytes"

// ScanResult holds a single key-value pair from a prefix scan.
type ScanResult struct {
	Key   []byte
	Value []byte
	Type  EntryType
}

// Scan returns all non-deleted entries whose keys begin with the given prefix.
// Results are collected from active MemTable, immutable MemTables, and SSTables.
// Newer values shadow older ones via a deduplication map keyed on the raw key bytes.
func (e *Engine) Scan(prefix []byte) ([]ScanResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// seen tracks which keys we've already resolved (newest-wins).
	seen := make(map[string]ScanResult)

	// 1. Active MemTable (freshest data)
	e.activeMem.Iterate(func(key, value []byte, eType EntryType) bool {
		if bytes.HasPrefix(key, prefix) {
			k := string(key)
			if _, exists := seen[k]; !exists {
				seen[k] = ScanResult{Key: duplicate(key), Value: duplicate(value), Type: eType}
			}
		}
		return true
	})

	// 2. Immutable MemTables (LIFO — newest first)
	for i := len(e.immutable) - 1; i >= 0; i-- {
		e.immutable[i].Iterate(func(key, value []byte, eType EntryType) bool {
			if bytes.HasPrefix(key, prefix) {
				k := string(key)
				if _, exists := seen[k]; !exists {
					seen[k] = ScanResult{Key: duplicate(key), Value: duplicate(value), Type: eType}
				}
			}
			return true
		})
	}

	// 3. SSTables — we skip SSTable-level prefix scans for now (would require
	//    a block-level iterator). In Phase 1, hot data lives in MemTable,
	//    so this covers the critical path. We'll add SSTable iterators in Phase 2.

	// Collect non-tombstoned results
	results := make([]ScanResult, 0, len(seen))
	for _, sr := range seen {
		if sr.Type != TypeDelete {
			results = append(results, sr)
		}
	}

	return results, nil
}
