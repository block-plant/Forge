package mvcc

import (
	"bytes"

	"github.com/ayushkunwarsingh/forge/dynamicdb/lsm"
)

// ScanResult holds a decoded user-space key-value from a prefix scan.
type ScanResult struct {
	Key   []byte
	Value []byte
}

// Scan returns all live (non-tombstoned) entries whose user-level key starts
// with the given prefix. Because the LSM stores MVCC-encoded keys
// (userKey + reversedTimestamp), we must decode each key and deduplicate by
// the user-key, keeping only the newest version visible to the caller.
func (db *DB) Scan(prefix []byte) ([]ScanResult, error) {
	raw, err := db.lsmEngine.Scan(prefix)
	if err != nil {
		return nil, err
	}

	// Deduplicate: MVCC keys encode (userKey || reverseTS). The LSM scan
	// returns sorted keys, so for the same userKey, earlier entries in the
	// slice correspond to higher timestamps (newest first because reverseTS
	// sorts ascending for higher real timestamps).
	//
	// However, since we also collect from MemTable → Immutable → SSTable in
	// that order and the LSM Scan already deduplicates by raw key, we need
	// to strip the 8-byte timestamp suffix and deduplicate again at the
	// user-key level.
	seen := make(map[string]ScanResult)

	for _, entry := range raw {
		rawKey := entry.Key
		// Skip keys that are too short to contain a timestamp suffix
		if len(rawKey) < 8 {
			continue
		}

		userKey, _ := DecodeKey(rawKey)

		// Only include if user key actually has our prefix
		if !bytes.HasPrefix(userKey, prefix) {
			continue
		}

		k := string(userKey)
		if _, exists := seen[k]; !exists {
			val := make([]byte, len(entry.Value))
			copy(val, entry.Value)
			seen[k] = ScanResult{Key: userKey, Value: val}
		}
	}

	results := make([]ScanResult, 0, len(seen))
	for _, sr := range seen {
		results = append(results, sr)
	}

	return results, nil
}

// ScanRaw returns all live entries from the LSM engine whose raw (MVCC-encoded)
// keys begin with the given prefix. No timestamp decoding is performed.
// This is useful for internal operations like counting or deleting by prefix.
func (db *DB) ScanRaw(prefix []byte) ([]lsm.ScanResult, error) {
	return db.lsmEngine.Scan(prefix)
}
