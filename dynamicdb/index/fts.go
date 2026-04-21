package index

import (
	"strings"

	"github.com/ayushkunwarsingh/forge/dynamicdb/mvcc"
)

// FTS manages inverted indexing algorithms for full-text search.
type FTS struct {
	db *mvcc.DB
}

// tokenize splits a string into lowercase standardized word chunks.
func tokenize(text string) []string {
	text = strings.ToLower(text)
	text = strings.ReplaceAll(text, ".", "")
	text = strings.ReplaceAll(text, ",", "")
	return strings.Fields(text)
}

// IndexText tokenizes the text block and binds every token to the document.
func (f *FTS) IndexText(txn *mvcc.Transaction, collection, docID, text string) error {
	tokens := tokenize(text)
	
	for _, token := range tokens {
		// Key format: `FTS_[Word]_[Collection]_[DocID]`
		key := secondaryKey(collection, "FTS", token, docID)
		if err := txn.Put(key, []byte{1}); err != nil {
			return err
		}
	}
	return nil
}

// Search calculates the intersection of sets spanning multiple tokens.
func (f *FTS) Search(collection, query string) []string {
	// 1. Tokenize query
	// 2. Map intersection lists based on `FTS_[token]_[collection]` prefix scans.
	return []string{}
}
