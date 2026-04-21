package database

import "fmt"

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

// ParseQuery parses a JSON request body into a Query object.
func ParseQuery(data map[string]interface{}) (*Query, error) {
	q := &Query{}

	// Collection (required)
	if col, ok := data["collection"].(string); ok {
		q.Collection = col
	} else {
		return nil, fmt.Errorf("missing or invalid 'collection'")
	}

	// Where clauses
	if whereRaw, ok := data["where"].([]interface{}); ok {
		for _, item := range whereRaw {
			clause, cOk := item.(map[string]interface{})
			if !cOk {
				continue
			}
			field, _ := clause["field"].(string)
			op, _ := clause["op"].(string)
			value := clause["value"]

			if field != "" && op != "" {
				q.Where = append(q.Where, WhereClause{
					Field:    field,
					Operator: QueryOp(op),
					Value:    value,
				})
			}
		}
	}

	// OrderBy clauses
	if orderRaw, ok := data["order_by"].([]interface{}); ok {
		for _, item := range orderRaw {
			clause, cOk := item.(map[string]interface{})
			if !cOk {
				continue
			}
			field, _ := clause["field"].(string)
			direction, _ := clause["direction"].(string)
			if direction == "" {
				direction = "asc"
			}
			if field != "" {
				q.OrderBy = append(q.OrderBy, OrderByClause{
					Field:     field,
					Direction: direction,
				})
			}
		}
	}

	// Limit
	if limit, ok := data["limit"].(float64); ok {
		q.Limit = int(limit)
	}

	// Offset
	if offset, ok := data["offset"].(float64); ok {
		q.Offset = int(offset)
	}

	// StartAfter
	if startAfter, ok := data["start_after"].(string); ok {
		q.StartAfter = startAfter
	}

	return q, nil
}
