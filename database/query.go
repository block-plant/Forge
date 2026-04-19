package database

import (
	"fmt"
	"sort"
	"strings"
)

// QueryOp represents a comparison operator in a where clause.
type QueryOp string

const (
	OpEq            QueryOp = "=="
	OpNeq           QueryOp = "!="
	OpGt            QueryOp = ">"
	OpGte           QueryOp = ">="
	OpLt            QueryOp = "<"
	OpLte           QueryOp = "<="
	OpIn            QueryOp = "in"
	OpArrayContains QueryOp = "array-contains"
)

// WhereClause represents a single filter condition.
type WhereClause struct {
	Field    string      `json:"field"`
	Operator QueryOp     `json:"op"`
	Value    interface{} `json:"value"`
}

// OrderByClause represents a sort directive.
type OrderByClause struct {
	Field     string `json:"field"`
	Direction string `json:"direction"` // "asc" or "desc"
}

// Query represents a database query.
type Query struct {
	Collection string          `json:"collection"`
	Where      []WhereClause   `json:"where,omitempty"`
	OrderBy    []OrderByClause `json:"order_by,omitempty"`
	Limit      int             `json:"limit,omitempty"`
	Offset     int             `json:"offset,omitempty"`
	StartAfter string          `json:"start_after,omitempty"` // doc ID for pagination
}

// QueryResult holds the result of a query execution.
type QueryResult struct {
	Documents []*Document `json:"documents"`
	Count     int         `json:"count"`
}

// QueryExecutor runs queries against the in-memory store.
type QueryExecutor struct {
	store   *MemoryStore
	indexes *IndexManager
}

// NewQueryExecutor creates a new query executor.
func NewQueryExecutor(store *MemoryStore, indexes *IndexManager) *QueryExecutor {
	return &QueryExecutor{
		store:   store,
		indexes: indexes,
	}
}

// Execute runs a query and returns matching documents.
func (qe *QueryExecutor) Execute(q *Query) (*QueryResult, error) {
	tree := qe.store.GetCollection(q.Collection)
	if tree == nil {
		return &QueryResult{Documents: []*Document{}, Count: 0}, nil
	}

	// Get all documents from the collection
	allDocs := tree.All()

	// Apply where clauses (filter)
	filtered := qe.applyFilters(allDocs, q.Where)

	// Apply ordering
	if len(q.OrderBy) > 0 {
		qe.applyOrdering(filtered, q.OrderBy)
	}

	// Apply pagination (startAfter)
	if q.StartAfter != "" {
		filtered = qe.applyStartAfter(filtered, q.StartAfter)
	}

	// Apply offset
	if q.Offset > 0 && q.Offset < len(filtered) {
		filtered = filtered[q.Offset:]
	} else if q.Offset >= len(filtered) {
		filtered = nil
	}

	// Apply limit
	if q.Limit > 0 && q.Limit < len(filtered) {
		filtered = filtered[:q.Limit]
	}

	// Clone documents to prevent mutation
	results := make([]*Document, len(filtered))
	for i, doc := range filtered {
		results[i] = doc.Clone()
	}

	return &QueryResult{
		Documents: results,
		Count:     len(results),
	}, nil
}

// applyFilters filters documents by all where clauses.
func (qe *QueryExecutor) applyFilters(docs []*Document, clauses []WhereClause) []*Document {
	if len(clauses) == 0 {
		return docs
	}

	result := make([]*Document, 0, len(docs))
	for _, doc := range docs {
		if qe.matchesAllClauses(doc, clauses) {
			result = append(result, doc)
		}
	}
	return result
}

// matchesAllClauses checks if a document matches all where clauses.
func (qe *QueryExecutor) matchesAllClauses(doc *Document, clauses []WhereClause) bool {
	for _, clause := range clauses {
		if !matchesClause(doc, clause) {
			return false
		}
	}
	return true
}

// matchesClause checks if a document matches a single where clause.
func matchesClause(doc *Document, clause WhereClause) bool {
	fieldValue, exists := doc.Data[clause.Field]

	switch clause.Operator {
	case OpEq:
		return exists && compareValues(fieldValue, clause.Value) == 0
	case OpNeq:
		return !exists || compareValues(fieldValue, clause.Value) != 0
	case OpGt:
		return exists && compareValues(fieldValue, clause.Value) > 0
	case OpGte:
		return exists && compareValues(fieldValue, clause.Value) >= 0
	case OpLt:
		return exists && compareValues(fieldValue, clause.Value) < 0
	case OpLte:
		return exists && compareValues(fieldValue, clause.Value) <= 0
	case OpIn:
		return matchesIn(fieldValue, clause.Value)
	case OpArrayContains:
		return matchesArrayContains(fieldValue, clause.Value)
	default:
		return false
	}
}

// compareValues compares two values for ordering.
// Returns -1, 0, or 1. Different types are compared by type name.
func compareValues(a, b interface{}) int {
	// Handle nil
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	// Compare same types
	switch av := a.(type) {
	case float64:
		if bv, ok := b.(float64); ok {
			if av < bv {
				return -1
			}
			if av > bv {
				return 1
			}
			return 0
		}
	case string:
		if bv, ok := b.(string); ok {
			return strings.Compare(av, bv)
		}
	case bool:
		if bv, ok := b.(bool); ok {
			if av == bv {
				return 0
			}
			if !av && bv {
				return -1
			}
			return 1
		}
	}

	// Fallback: compare string representations
	as := fmt.Sprintf("%v", a)
	bs := fmt.Sprintf("%v", b)
	return strings.Compare(as, bs)
}

// matchesIn checks if the field value is in the list of values.
func matchesIn(fieldValue, listValue interface{}) bool {
	list, ok := listValue.([]interface{})
	if !ok {
		return false
	}

	for _, item := range list {
		if compareValues(fieldValue, item) == 0 {
			return true
		}
	}
	return false
}

// matchesArrayContains checks if the field (an array) contains the value.
func matchesArrayContains(fieldValue, searchValue interface{}) bool {
	arr, ok := fieldValue.([]interface{})
	if !ok {
		return false
	}

	for _, item := range arr {
		if compareValues(item, searchValue) == 0 {
			return true
		}
	}
	return false
}

// applyOrdering sorts documents by the given order-by clauses.
func (qe *QueryExecutor) applyOrdering(docs []*Document, orderBy []OrderByClause) {
	sort.SliceStable(docs, func(i, j int) bool {
		for _, ob := range orderBy {
			vi, _ := docs[i].Data[ob.Field]
			vj, _ := docs[j].Data[ob.Field]

			cmp := compareValues(vi, vj)
			if cmp == 0 {
				continue
			}

			if ob.Direction == "desc" {
				return cmp > 0
			}
			return cmp < 0
		}
		return false // equal on all sort keys
	})
}

// applyStartAfter removes documents up to and including the startAfter doc ID.
func (qe *QueryExecutor) applyStartAfter(docs []*Document, startAfterID string) []*Document {
	for i, doc := range docs {
		if doc.ID == startAfterID {
			if i+1 < len(docs) {
				return docs[i+1:]
			}
			return nil
		}
	}
	return docs // startAfter ID not found — return all
}

// ParseQuery builds a Query from a raw request map (used by HTTP handlers).
func ParseQuery(data map[string]interface{}) (*Query, error) {
	q := &Query{}

	if col, ok := data["collection"].(string); ok {
		q.Collection = col
	} else {
		return nil, fmt.Errorf("query: missing 'collection' field")
	}

	// Parse where clauses
	if wheres, ok := data["where"].([]interface{}); ok {
		for _, w := range wheres {
			clause, err := parseWhereClause(w)
			if err != nil {
				return nil, err
			}
			q.Where = append(q.Where, *clause)
		}
	}

	// Parse orderBy
	if orders, ok := data["order_by"].([]interface{}); ok {
		for _, o := range orders {
			ob, err := parseOrderByClause(o)
			if err != nil {
				return nil, err
			}
			q.OrderBy = append(q.OrderBy, *ob)
		}
	}

	// Parse limit
	if limit, ok := data["limit"].(float64); ok {
		q.Limit = int(limit)
	}

	// Parse offset
	if offset, ok := data["offset"].(float64); ok {
		q.Offset = int(offset)
	}

	// Parse startAfter
	if sa, ok := data["start_after"].(string); ok {
		q.StartAfter = sa
	}

	return q, nil
}

// parseWhereClause parses a single where clause from JSON.
// Expected format: [field, operator, value] or {"field": "...", "op": "...", "value": ...}
func parseWhereClause(raw interface{}) (*WhereClause, error) {
	switch v := raw.(type) {
	case []interface{}:
		if len(v) != 3 {
			return nil, fmt.Errorf("query: where clause must have 3 elements [field, op, value]")
		}
		field, ok := v[0].(string)
		if !ok {
			return nil, fmt.Errorf("query: where field must be a string")
		}
		op, ok := v[1].(string)
		if !ok {
			return nil, fmt.Errorf("query: where operator must be a string")
		}
		return &WhereClause{
			Field:    field,
			Operator: QueryOp(op),
			Value:    v[2],
		}, nil

	case map[string]interface{}:
		field, _ := v["field"].(string)
		op, _ := v["op"].(string)
		value := v["value"]
		if field == "" || op == "" {
			return nil, fmt.Errorf("query: where clause must have 'field' and 'op'")
		}
		return &WhereClause{
			Field:    field,
			Operator: QueryOp(op),
			Value:    value,
		}, nil

	default:
		return nil, fmt.Errorf("query: invalid where clause format")
	}
}

// parseOrderByClause parses an orderBy clause from JSON.
func parseOrderByClause(raw interface{}) (*OrderByClause, error) {
	switch v := raw.(type) {
	case []interface{}:
		if len(v) < 1 {
			return nil, fmt.Errorf("query: orderBy must have at least a field name")
		}
		field, ok := v[0].(string)
		if !ok {
			return nil, fmt.Errorf("query: orderBy field must be a string")
		}
		dir := "asc"
		if len(v) >= 2 {
			if d, ok := v[1].(string); ok {
				dir = d
			}
		}
		return &OrderByClause{Field: field, Direction: dir}, nil

	case map[string]interface{}:
		field, _ := v["field"].(string)
		dir, _ := v["direction"].(string)
		if field == "" {
			return nil, fmt.Errorf("query: orderBy must have a 'field'")
		}
		if dir == "" {
			dir = "asc"
		}
		return &OrderByClause{Field: field, Direction: dir}, nil

	case string:
		return &OrderByClause{Field: v, Direction: "asc"}, nil

	default:
		return nil, fmt.Errorf("query: invalid orderBy format")
	}
}
