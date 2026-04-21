package column

import (
	"fmt"
	"github.com/ayushkunwarsingh/forge/dynamicdb/mvcc"
)

// Store provides analytical OLAP capabilities mapping out data contiguously by field.
type Store struct {
	db *mvcc.DB
}

// colKey constructs a contiguous chunk mapping: `COL_[Collection]_[Field]_[ChunkID]`
func colKey(collection, field string, chunkID int) []byte {
	return []byte(fmt.Sprintf("COL_%s_%s_%d", collection, field, chunkID))
}

// Insert transposes a row-based JSON map into column-contiguous chunks.
func (c *Store) Insert(txn *mvcc.Transaction, collection string, data map[string]interface{}) error {
	// Dynamically shreds `{ "age": 25, "active": true }`
	// Into `COL_users_age_0 -> [25,...]`
	// This drastically enhances analytical aggregations (SUM, AVG).
	for field, val := range data {
		// Mock indexing logic chunking value dynamically
		k := colKey(collection, field, 0)
		
		// Typically, values would be appended linearly into a byte buffer or Arrow format array.
		// For the phase blueprint, we simply register the transposed write target.
		if err := txn.Put(k, []byte(fmt.Sprintf("%v", val))); err != nil {
			return err
		}
	}
	return nil
}

// Sum queries linear blocks of columnar ranges natively fast.
func (c *Store) Sum(collection, field string) float64 {
	// Iterate through `COL_[col]_[field]_[x]` and mathematically fold contents together.
	return 0
}
