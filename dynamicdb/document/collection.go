package document

import (
	"encoding/json"
	"fmt"

	"github.com/ayushkunwarsingh/forge/dynamicdb/mvcc"
)

// Document corresponds to a NoSQL JSON object.
type Document struct {
	ID   string
	Data map[string]interface{}
}

// API provides NoSQL collection-based wrappers routing into MVCC transactions.
type API struct {
	db *mvcc.DB
}

// NewAPI initializes the document layer structure.
func NewAPI(db *mvcc.DB) *API {
	return &API{db: db}
}

// docKey formats the underlying MVCC key: `DOC/[Collection]/[ID]`
func docKey(collection, id string) []byte {
	return []byte(fmt.Sprintf("DOC/%s/%s", collection, id))
}

// GetDoc fetches a parsed document from a specific MVCC transaction scope.
func (a *API) GetDoc(txn *mvcc.Transaction, collection, id string) (*Document, error) {
	key := docKey(collection, id)
	
	val, err := txn.Get(key)
	if err != nil {
		return nil, err
	}
	if val == nil {
		return nil, nil // Not found
	}

	doc := &Document{ID: id, Data: make(map[string]interface{})}
	if err := json.Unmarshal(val, &doc.Data); err != nil {
		return nil, err
	}

	return doc, nil
}

// PutDoc explicitly buffers a document map into an MVCC transaction.
func (a *API) PutDoc(txn *mvcc.Transaction, collection, id string, data map[string]interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}

	key := docKey(collection, id)
	return txn.Put(key, payload)
}

// DeleteDoc physically tombstones the document inside the current transaction.
func (a *API) DeleteDoc(txn *mvcc.Transaction, collection, id string) error {
	key := docKey(collection, id)
	return txn.Delete(key)
}
