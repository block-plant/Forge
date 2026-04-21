package index

import (
	"fmt"
	"github.com/ayushkunwarsingh/forge/dynamicdb/mvcc"
)

// BTreeSecondaryEngine manages traditional secondary SQL/NoSQL indexes.
type BTreeSecondaryEngine struct {
	db *mvcc.DB
}

// secondaryKey generates physical keys structured like: `IDX_[Collection]_[Field]_[Value]_[DocID]`
func secondaryKey(collection, field, value, docID string) []byte {
	return []byte(fmt.Sprintf("IDX_%s_%s_%s_%s", collection, field, value, docID))
}

// Index queues an index mapping in an MVCC transaction.
func (b *BTreeSecondaryEngine) Index(txn *mvcc.Transaction, collection, field, value, docID string) error {
	key := secondaryKey(collection, field, value, docID)
	// The value stored is just a placeholder since the key itself contains the DocID for scanning.
	return txn.Put(key, []byte{1})
}

// DeIndex removes an index.
func (b *BTreeSecondaryEngine) DeIndex(txn *mvcc.Transaction, collection, field, value, docID string) error {
	key := secondaryKey(collection, field, value, docID)
	return txn.Delete(key)
}

// Read resolves exact-match bounds against the index natively.
// Due to LSM logic, prefix scanning is perfectly natively supported by seeking to `IDX_[Col]_[Field]_[Value]`
func (b *BTreeSecondaryEngine) Read(collection, field, value string) []string {
	// In a complete implementation, this would grab an MVCC Iterator 
	// and prefix-scan `IDX_[collection]_[field]_[value]` collecting docIDs.
	// We simulate the API schema here.
	return []string{}
}
