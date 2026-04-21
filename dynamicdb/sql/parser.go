package sql

import (
	"errors"
	"strings"
)

// TokenType classifies our bare-bones SQL vocabulary.
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenIdent
	TokenString
	TokenSelect
	TokenInsert
	TokenUpdate
	TokenDelete
	TokenFrom
	TokenWhere
	TokenInto
	TokenValues
	TokenEq
)

// SQLStatement represents the AST abstraction spanning different operations.
type SQLStatement interface {
	statement()
}

type SelectStatement struct {
	Table string
	WhereField string
	WhereValue string
}
func (s *SelectStatement) statement() {}

type InsertStatement struct {
	Table string
	ID    string
	Data  map[string]interface{}
}
func (s *InsertStatement) statement() {}

// Parse processes raw SQL strings into our AST statements.
// For Phase 4 MVP, this is a highly simplified regex/split string parser.
func Parse(query string) (SQLStatement, error) {
	q := strings.TrimSpace(query)
	
	// Normalize spacing simply
	q = strings.ReplaceAll(q, "  ", " ")
	tokens := strings.Split(q, " ")
	
	if len(tokens) == 0 {
		return nil, errors.New("sql: empty query")
	}

	cmd := strings.ToUpper(tokens[0])
	
	switch cmd {
	case "SELECT":
		// ex: SELECT * FROM users WHERE id = '123'
		if len(tokens) < 8 {
			return nil, errors.New("sql: syntax error in SELECT")
		}
		if strings.ToUpper(tokens[2]) != "FROM" {
			return nil, errors.New("sql: expected FROM")
		}
		
		stmt := &SelectStatement{Table: tokens[3]}
		
		if strings.ToUpper(tokens[4]) == "WHERE" {
			stmt.WhereField = tokens[5]
			// simple value stripping -> '123' to 123
			stmt.WhereValue = strings.Trim(tokens[7], "'\"")
		}
		return stmt, nil
		
	case "INSERT":
		// ex: INSERT INTO users VALUES ('123', '{"name":"ayush"}')
		if len(tokens) < 6 {
			return nil, errors.New("sql: syntax error in INSERT")
		}
		stmt := &InsertStatement{
			Table: tokens[2],
			ID:    strings.Trim(tokens[4], "'\",()"), // parse naive 1st tuple value as ID
			Data:  map[string]interface{}{"raw": strings.Trim(tokens[5], "'\",()")}, // raw mock inject
		}
		return stmt, nil
	}

	return nil, errors.New("sql: unsupported command")
}
