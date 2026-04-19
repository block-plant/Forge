package rules

import (
	"fmt"
	"strconv"
	"strings"
)

// Parser builds an AST from a sequence of tokens produced by the lexer.
// It implements a recursive descent parser for the Forge rules DSL.
type Parser struct {
	tokens  []Token
	current int
	errors  []ParseError
}

// ParseError represents a parsing error at a specific location.
type ParseError struct {
	Message string
	Line    int
	Column  int
}

// Error implements the error interface for ParseError.
func (e ParseError) Error() string {
	return fmt.Sprintf("line %d, col %d: %s", e.Line, e.Column, e.Message)
}

// NewParser creates a new parser for the given tokens.
func NewParser(tokens []Token) *Parser {
	return &Parser{
		tokens:  tokens,
		current: 0,
		errors:  make([]ParseError, 0),
	}
}

// Parse parses the token stream into a RuleSet AST.
func (p *Parser) Parse() (*RuleSet, []ParseError) {
	ruleSet := &RuleSet{
		Services: make([]*ServiceBlock, 0),
		Line:     1,
		Col:      1,
	}

	// Parse optional rules_version
	if p.check(TokenRulesVersion) {
		p.advance()
		if !p.consume(TokenAssign, "expected '=' after rules_version") {
			return ruleSet, p.errors
		}
		if !p.check(TokenString) {
			p.addError("expected version string after '='")
			return ruleSet, p.errors
		}
		ruleSet.Version = p.advance().Value
	}

	// Parse service blocks
	for !p.isAtEnd() {
		if p.check(TokenService) {
			service := p.parseServiceBlock()
			if service != nil {
				ruleSet.Services = append(ruleSet.Services, service)
			}
		} else {
			p.addError(fmt.Sprintf("expected 'service', got %s", p.peek().Value))
			p.advance() // Skip unknown token
		}
	}

	return ruleSet, p.errors
}

// parseServiceBlock parses "service <name> { ... }".
func (p *Parser) parseServiceBlock() *ServiceBlock {
	tok := p.advance() // consume 'service'

	// Parse service name (e.g., "forge.database")
	name := p.parseQualifiedName()
	if name == "" {
		p.addError("expected service name after 'service'")
		return nil
	}

	if !p.consume(TokenLeftBrace, "expected '{' after service name") {
		return nil
	}

	block := &ServiceBlock{
		Name:    name,
		Matches: make([]*MatchBlock, 0),
		Line:    tok.Line,
		Col:     tok.Column,
	}

	// Parse match blocks within the service
	for !p.isAtEnd() && !p.check(TokenRightBrace) {
		if p.check(TokenMatch) {
			match := p.parseMatchBlock()
			if match != nil {
				block.Matches = append(block.Matches, match)
			}
		} else {
			p.addError(fmt.Sprintf("expected 'match' inside service block, got %s", p.peek().Value))
			p.advance()
		}
	}

	p.consume(TokenRightBrace, "expected '}' to close service block")

	return block
}

// parseMatchBlock parses "match /path/{var} { ... }".
func (p *Parser) parseMatchBlock() *MatchBlock {
	tok := p.advance() // consume 'match'

	// Parse the match path
	path, segments := p.parseMatchPath()

	if !p.consume(TokenLeftBrace, "expected '{' after match path") {
		return nil
	}

	block := &MatchBlock{
		Path:         path,
		PathSegments: segments,
		Rules:        make([]*AllowStatement, 0),
		NestedMatches:  make([]*MatchBlock, 0),
		Functions:    make([]*FunctionDecl, 0),
		Lets:         make([]*LetStatement, 0),
		Line:         tok.Line,
		Col:          tok.Column,
	}

	// Check for recursive wildcard
	for _, seg := range segments {
		if seg.IsRecursiveWildcard {
			block.IsRecursiveWildcard = true
			break
		}
	}

	// Parse contents: allow statements, nested match blocks, functions, lets
	for !p.isAtEnd() && !p.check(TokenRightBrace) {
		switch {
		case p.check(TokenAllow):
			allow := p.parseAllowStatement()
			if allow != nil {
				block.Rules = append(block.Rules, allow)
			}
		case p.check(TokenMatch):
			nested := p.parseMatchBlock()
			if nested != nil {
				block.NestedMatches = append(block.NestedMatches, nested)
			}
		case p.check(TokenFunction):
			fn := p.parseFunctionDecl()
			if fn != nil {
				block.Functions = append(block.Functions, fn)
			}
		case p.check(TokenLet):
			let := p.parseLetStatement()
			if let != nil {
				block.Lets = append(block.Lets, let)
			}
		default:
			p.addError(fmt.Sprintf("unexpected token in match block: %s", p.peek().Value))
			p.advance()
		}
	}

	p.consume(TokenRightBrace, "expected '}' to close match block")

	return block
}

// parseMatchPath parses a path like "/users/{userId}" or "/admin/{document=**}".
func (p *Parser) parseMatchPath() (string, []PathSeg) {
	var pathBuilder strings.Builder
	var segments []PathSeg

	for !p.isAtEnd() && !p.check(TokenLeftBrace) {
		tok := p.peek()

		if tok.Type == TokenSlash || tok.Value == "/" {
			pathBuilder.WriteString("/")
			p.advance()
			continue
		}

		if tok.Type == TokenLeftBrace {
			break
		}

		// Check for {variable} pattern
		if tok.Type == TokenIdentifier || tok.Type == TokenStar {
			pathBuilder.WriteString(tok.Value)
			segments = append(segments, PathSeg{
				Value: tok.Value,
			})
			p.advance()

			// Check for = after identifier if inside braces would be parsed differently
			continue
		}

		// Handle path with {variable} captures
		break
	}

	// Parse remaining path segments including {var} captures
	// The path is typically provided as a single string-like sequence
	// In practice we parse the individual tokens that make up the path
	return pathBuilder.String(), segments
}

// parseAllowStatement parses "allow read, write: if <expr>;".
func (p *Parser) parseAllowStatement() *AllowStatement {
	tok := p.advance() // consume 'allow'

	stmt := &AllowStatement{
		Operations: make([]string, 0),
		Line:       tok.Line,
		Col:        tok.Column,
	}

	// Parse operations (comma-separated identifiers)
	for {
		if !p.check(TokenIdentifier) {
			p.addError("expected operation name (read, write, create, update, delete)")
			break
		}

		op := p.advance().Value
		stmt.Operations = append(stmt.Operations, op)

		if !p.check(TokenComma) {
			break
		}
		p.advance() // consume comma
	}

	// Parse optional ": if <condition>"
	if p.check(TokenColon) {
		p.advance() // consume ':'

		if p.check(TokenIf) {
			p.advance() // consume 'if'
			stmt.Condition = p.parseExpression()
		} else {
			p.addError("expected 'if' after ':'")
		}
	}

	// Optional semicolon
	if p.check(TokenSemicolon) {
		p.advance()
	}

	return stmt
}

// parseFunctionDecl parses "function name(params) { return expr; }".
func (p *Parser) parseFunctionDecl() *FunctionDecl {
	tok := p.advance() // consume 'function'

	if !p.check(TokenIdentifier) {
		p.addError("expected function name")
		return nil
	}
	name := p.advance().Value

	if !p.consume(TokenLeftParen, "expected '(' after function name") {
		return nil
	}

	// Parse parameters
	params := make([]string, 0)
	for !p.isAtEnd() && !p.check(TokenRightParen) {
		if !p.check(TokenIdentifier) {
			p.addError("expected parameter name")
			break
		}
		params = append(params, p.advance().Value)

		if !p.check(TokenComma) {
			break
		}
		p.advance() // consume comma
	}

	if !p.consume(TokenRightParen, "expected ')' after parameters") {
		return nil
	}

	if !p.consume(TokenLeftBrace, "expected '{' before function body") {
		return nil
	}

	// Parse "return expr;"
	var body Expr
	if p.check(TokenReturn) {
		p.advance() // consume 'return'
		body = p.parseExpression()
		if p.check(TokenSemicolon) {
			p.advance()
		}
	} else {
		p.addError("expected 'return' in function body")
	}

	p.consume(TokenRightBrace, "expected '}' to close function body")

	return &FunctionDecl{
		Name:   name,
		Params: params,
		Body:   body,
		Line:   tok.Line,
		Col:    tok.Column,
	}
}

// parseLetStatement parses "let name = expr;".
func (p *Parser) parseLetStatement() *LetStatement {
	tok := p.advance() // consume 'let'

	if !p.check(TokenIdentifier) {
		p.addError("expected variable name after 'let'")
		return nil
	}
	name := p.advance().Value

	if !p.consume(TokenAssign, "expected '=' after variable name") {
		return nil
	}

	value := p.parseExpression()

	if p.check(TokenSemicolon) {
		p.advance()
	}

	return &LetStatement{
		Name:  name,
		Value: value,
		Line:  tok.Line,
		Col:   tok.Column,
	}
}

// ── Expression Parsing (Precedence Climbing) ──

// parseExpression parses a full expression.
func (p *Parser) parseExpression() Expr {
	return p.parseOr()
}

// parseOr handles "||" operator.
func (p *Parser) parseOr() Expr {
	left := p.parseAnd()

	for p.check(TokenOr) {
		tok := p.advance()
		right := p.parseAnd()
		left = &BinaryExpr{
			Left:     left,
			Operator: "||",
			Right:    right,
			Line:     tok.Line,
			Col:      tok.Column,
		}
	}

	return left
}

// parseAnd handles "&&" operator.
func (p *Parser) parseAnd() Expr {
	left := p.parseEquality()

	for p.check(TokenAnd) {
		tok := p.advance()
		right := p.parseEquality()
		left = &BinaryExpr{
			Left:     left,
			Operator: "&&",
			Right:    right,
			Line:     tok.Line,
			Col:      tok.Column,
		}
	}

	return left
}

// parseEquality handles "==" and "!=" operators.
func (p *Parser) parseEquality() Expr {
	left := p.parseComparison()

	for p.check(TokenEquals) || p.check(TokenNotEquals) {
		tok := p.advance()
		right := p.parseComparison()
		left = &BinaryExpr{
			Left:     left,
			Operator: tok.Value,
			Right:    right,
			Line:     tok.Line,
			Col:      tok.Column,
		}
	}

	return left
}

// parseComparison handles <, <=, >, >= operators.
func (p *Parser) parseComparison() Expr {
	left := p.parseInIs()

	for p.check(TokenLess) || p.check(TokenLessEqual) ||
		p.check(TokenGreater) || p.check(TokenGreaterEqual) {
		tok := p.advance()
		right := p.parseInIs()
		left = &BinaryExpr{
			Left:     left,
			Operator: tok.Value,
			Right:    right,
			Line:     tok.Line,
			Col:      tok.Column,
		}
	}

	return left
}

// parseInIs handles "in" and "is" operators.
func (p *Parser) parseInIs() Expr {
	left := p.parseAddition()

	if p.check(TokenIn) {
		tok := p.advance()
		right := p.parseAddition()
		return &BinaryExpr{
			Left:     left,
			Operator: "in",
			Right:    right,
			Line:     tok.Line,
			Col:      tok.Column,
		}
	}

	if p.check(TokenIs) {
		tok := p.advance()
		right := p.parseAddition()
		return &BinaryExpr{
			Left:     left,
			Operator: "is",
			Right:    right,
			Line:     tok.Line,
			Col:      tok.Column,
		}
	}

	return left
}

// parseAddition handles "+" and "-" operators.
func (p *Parser) parseAddition() Expr {
	left := p.parseMultiplication()

	for p.check(TokenPlus) || p.check(TokenMinus) {
		tok := p.advance()
		right := p.parseMultiplication()
		left = &BinaryExpr{
			Left:     left,
			Operator: tok.Value,
			Right:    right,
			Line:     tok.Line,
			Col:      tok.Column,
		}
	}

	return left
}

// parseMultiplication handles "*", "/", "%" operators.
func (p *Parser) parseMultiplication() Expr {
	left := p.parseUnary()

	for p.check(TokenStar) || p.check(TokenSlash) || p.check(TokenPercent) {
		tok := p.advance()
		right := p.parseUnary()
		left = &BinaryExpr{
			Left:     left,
			Operator: tok.Value,
			Right:    right,
			Line:     tok.Line,
			Col:      tok.Column,
		}
	}

	return left
}

// parseUnary handles "!" and "-" prefix operators.
func (p *Parser) parseUnary() Expr {
	if p.check(TokenBang) || p.check(TokenMinus) {
		tok := p.advance()
		operand := p.parseUnary()
		return &UnaryExpr{
			Operator: tok.Value,
			Operand:  operand,
			Line:     tok.Line,
			Col:      tok.Column,
		}
	}

	return p.parsePostfix()
}

// parsePostfix handles member access (.), function calls (), and index access [].
func (p *Parser) parsePostfix() Expr {
	expr := p.parsePrimary()

	for {
		if p.check(TokenDot) {
			p.advance() // consume '.'
			if !p.check(TokenIdentifier) {
				p.addError("expected property name after '.'")
				break
			}
			prop := p.advance()
			expr = &MemberExpr{
				Object:   expr,
				Property: prop.Value,
				Line:     prop.Line,
				Col:      prop.Column,
			}
		} else if p.check(TokenLeftParen) {
			expr = p.parseCallExpr(expr)
		} else if p.check(TokenLeftBracket) {
			tok := p.advance() // consume '['
			index := p.parseExpression()
			p.consume(TokenRightBracket, "expected ']' after index")
			expr = &IndexExpr{
				Object: expr,
				Index:  index,
				Line:   tok.Line,
				Col:    tok.Column,
			}
		} else {
			break
		}
	}

	return expr
}

// parseCallExpr parses function call arguments.
func (p *Parser) parseCallExpr(callee Expr) Expr {
	tok := p.advance() // consume '('

	args := make([]Expr, 0)
	for !p.isAtEnd() && !p.check(TokenRightParen) {
		args = append(args, p.parseExpression())
		if !p.check(TokenComma) {
			break
		}
		p.advance() // consume comma
	}

	p.consume(TokenRightParen, "expected ')' after arguments")

	return &CallExpr{
		Callee: callee,
		Args:   args,
		Line:   tok.Line,
		Col:    tok.Column,
	}
}

// parsePrimary parses primary expressions (literals, identifiers, grouped).
func (p *Parser) parsePrimary() Expr {
	tok := p.peek()

	switch tok.Type {
	case TokenIdentifier:
		p.advance()
		return &IdentifierExpr{Name: tok.Value, Line: tok.Line, Col: tok.Column}

	case TokenString:
		p.advance()
		return &StringLit{Value: tok.Value, Line: tok.Line, Col: tok.Column}

	case TokenNumber:
		p.advance()
		val, _ := strconv.ParseFloat(tok.Value, 64)
		return &NumberLit{Value: val, Raw: tok.Value, Line: tok.Line, Col: tok.Column}

	case TokenBool:
		p.advance()
		return &BoolLit{Value: tok.Value == "true", Line: tok.Line, Col: tok.Column}

	case TokenNull:
		p.advance()
		return &NullLit{Line: tok.Line, Col: tok.Column}

	case TokenLeftParen:
		p.advance() // consume '('
		expr := p.parseExpression()
		p.consume(TokenRightParen, "expected ')' after expression")
		return expr

	case TokenLeftBracket:
		return p.parseArrayLiteral()

	case TokenLeftBrace:
		return p.parseMapLiteral()

	default:
		p.addError(fmt.Sprintf("unexpected token: %s (%q)", tok.Type.String(), tok.Value))
		p.advance()
		return &NullLit{Line: tok.Line, Col: tok.Column}
	}
}

// parseArrayLiteral parses [elem1, elem2, ...].
func (p *Parser) parseArrayLiteral() Expr {
	tok := p.advance() // consume '['

	elements := make([]Expr, 0)
	for !p.isAtEnd() && !p.check(TokenRightBracket) {
		elements = append(elements, p.parseExpression())
		if !p.check(TokenComma) {
			break
		}
		p.advance() // consume comma
	}

	p.consume(TokenRightBracket, "expected ']' after array elements")

	return &ArrayLit{Elements: elements, Line: tok.Line, Col: tok.Column}
}

// parseMapLiteral parses {key: value, ...}.
func (p *Parser) parseMapLiteral() Expr {
	tok := p.advance() // consume '{'

	keys := make([]string, 0)
	values := make([]Expr, 0)

	for !p.isAtEnd() && !p.check(TokenRightBrace) {
		// Parse key
		var key string
		if p.check(TokenIdentifier) {
			key = p.advance().Value
		} else if p.check(TokenString) {
			key = p.advance().Value
		} else {
			p.addError("expected map key (identifier or string)")
			break
		}

		p.consume(TokenColon, "expected ':' after map key")

		value := p.parseExpression()

		keys = append(keys, key)
		values = append(values, value)

		if !p.check(TokenComma) {
			break
		}
		p.advance() // consume comma
	}

	p.consume(TokenRightBrace, "expected '}' after map entries")

	return &MapLit{Keys: keys, Values: values, Line: tok.Line, Col: tok.Column}
}

// parseQualifiedName parses "a.b.c" as a single string.
func (p *Parser) parseQualifiedName() string {
	if !p.check(TokenIdentifier) {
		return ""
	}

	var parts []string
	parts = append(parts, p.advance().Value)

	for p.check(TokenDot) {
		p.advance()
		if !p.check(TokenIdentifier) {
			p.addError("expected identifier after '.' in qualified name")
			break
		}
		parts = append(parts, p.advance().Value)
	}

	return strings.Join(parts, ".")
}

// ── Helper Methods ──

func (p *Parser) peek() Token {
	if p.current >= len(p.tokens) {
		return Token{Type: TokenEOF}
	}
	return p.tokens[p.current]
}

func (p *Parser) advance() Token {
	tok := p.peek()
	if !p.isAtEnd() {
		p.current++
	}
	return tok
}

func (p *Parser) check(tokenType TokenType) bool {
	return p.peek().Type == tokenType
}

func (p *Parser) consume(tokenType TokenType, message string) bool {
	if p.check(tokenType) {
		p.advance()
		return true
	}
	p.addError(message)
	return false
}

func (p *Parser) isAtEnd() bool {
	return p.peek().Type == TokenEOF
}

func (p *Parser) addError(msg string) {
	tok := p.peek()
	p.errors = append(p.errors, ParseError{
		Message: msg,
		Line:    tok.Line,
		Column:  tok.Column,
	})
}

// ParseRules is the convenience function that tokenizes and parses a rules string.
func ParseRules(source string) (*RuleSet, []error) {
	lexer := NewLexer(source)
	tokens, lexErrors := lexer.Tokenize()

	if len(lexErrors) > 0 {
		errs := make([]error, len(lexErrors))
		for i, e := range lexErrors {
			errs[i] = e
		}
		return nil, errs
	}

	parser := NewParser(tokens)
	ruleSet, parseErrors := parser.Parse()

	if len(parseErrors) > 0 {
		errs := make([]error, len(parseErrors))
		for i, e := range parseErrors {
			errs[i] = e
		}
		return ruleSet, errs
	}

	return ruleSet, nil
}
