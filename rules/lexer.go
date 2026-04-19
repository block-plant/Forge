// Package rules implements the Forge security rules engine.
// It provides a DSL parser, AST representation, and evaluator
// for declarative access control on database and storage resources.
// All built from scratch with zero external dependencies.
package rules

import (
	"fmt"
	"strings"
	"unicode"
)

// TokenType identifies the kind of a lexer token.
type TokenType int

const (
	// Literal tokens
	TokenEOF        TokenType = iota // end of input
	TokenIdentifier                  // variable or keyword name
	TokenString                      // "string literal"
	TokenNumber                      // 42, 3.14
	TokenBool                        // true, false
	TokenNull                        // null

	// Delimiters
	TokenLeftParen    // (
	TokenRightParen   // )
	TokenLeftBrace    // {
	TokenRightBrace   // }
	TokenLeftBracket  // [
	TokenRightBracket // ]
	TokenSemicolon    // ;
	TokenComma        // ,
	TokenDot          // .
	TokenColon        // :

	// Operators
	TokenEquals        // ==
	TokenNotEquals     // !=
	TokenLess          // <
	TokenLessEqual     // <=
	TokenGreater       // >
	TokenGreaterEqual  // >=
	TokenAssign        // =
	TokenBang          // !
	TokenAnd           // &&
	TokenOr            // ||
	TokenPlus          // +
	TokenMinus         // -
	TokenStar          // *
	TokenSlash         // /
	TokenPercent       // %

	// Keywords
	TokenRulesVersion // rules_version
	TokenService      // service
	TokenMatch        // match
	TokenAllow        // allow
	TokenIf           // if
	TokenIn           // in
	TokenIs           // is
	TokenReturn       // return
	TokenFunction     // function
	TokenLet          // let
)

// Token is a single lexical token produced by the tokenizer.
type Token struct {
	// Type is the token category.
	Type TokenType
	// Value is the literal value of the token.
	Value string
	// Line is the 1-based line number where the token starts.
	Line int
	// Column is the 1-based column number where the token starts.
	Column int
}

// String returns a human-readable representation of a token.
func (t Token) String() string {
	return fmt.Sprintf("Token(%s, %q, %d:%d)", t.Type.String(), t.Value, t.Line, t.Column)
}

// String returns the name of a TokenType.
func (tt TokenType) String() string {
	switch tt {
	case TokenEOF:
		return "EOF"
	case TokenIdentifier:
		return "Identifier"
	case TokenString:
		return "String"
	case TokenNumber:
		return "Number"
	case TokenBool:
		return "Bool"
	case TokenNull:
		return "Null"
	case TokenLeftParen:
		return "("
	case TokenRightParen:
		return ")"
	case TokenLeftBrace:
		return "{"
	case TokenRightBrace:
		return "}"
	case TokenLeftBracket:
		return "["
	case TokenRightBracket:
		return "]"
	case TokenSemicolon:
		return ";"
	case TokenComma:
		return ","
	case TokenDot:
		return "."
	case TokenColon:
		return ":"
	case TokenEquals:
		return "=="
	case TokenNotEquals:
		return "!="
	case TokenLess:
		return "<"
	case TokenLessEqual:
		return "<="
	case TokenGreater:
		return ">"
	case TokenGreaterEqual:
		return ">="
	case TokenAssign:
		return "="
	case TokenBang:
		return "!"
	case TokenAnd:
		return "&&"
	case TokenOr:
		return "||"
	case TokenPlus:
		return "+"
	case TokenMinus:
		return "-"
	case TokenStar:
		return "*"
	case TokenSlash:
		return "/"
	case TokenPercent:
		return "%"
	case TokenRulesVersion:
		return "rules_version"
	case TokenService:
		return "service"
	case TokenMatch:
		return "match"
	case TokenAllow:
		return "allow"
	case TokenIf:
		return "if"
	case TokenIn:
		return "in"
	case TokenIs:
		return "is"
	case TokenReturn:
		return "return"
	case TokenFunction:
		return "function"
	case TokenLet:
		return "let"
	default:
		return fmt.Sprintf("Unknown(%d)", int(tt))
	}
}

// keywords maps keyword strings to their token types.
var keywords = map[string]TokenType{
	"rules_version": TokenRulesVersion,
	"service":       TokenService,
	"match":         TokenMatch,
	"allow":         TokenAllow,
	"if":            TokenIf,
	"in":            TokenIn,
	"is":            TokenIs,
	"return":        TokenReturn,
	"function":      TokenFunction,
	"let":           TokenLet,
	"true":          TokenBool,
	"false":         TokenBool,
	"null":          TokenNull,
}

// Lexer tokenizes a rules DSL source string into a sequence of tokens.
type Lexer struct {
	source  string
	tokens  []Token
	start   int // start of current token
	current int // current position
	line    int
	column  int
	errors  []LexError
}

// LexError represents a tokenization error.
type LexError struct {
	Message string
	Line    int
	Column  int
}

// Error implements the error interface for LexError.
func (e LexError) Error() string {
	return fmt.Sprintf("line %d, col %d: %s", e.Line, e.Column, e.Message)
}

// NewLexer creates a new lexer for the given source.
func NewLexer(source string) *Lexer {
	return &Lexer{
		source: source,
		tokens: make([]Token, 0, 128),
		line:   1,
		column: 1,
	}
}

// Tokenize scans the entire source and returns all tokens.
func (l *Lexer) Tokenize() ([]Token, []LexError) {
	for !l.isAtEnd() {
		l.start = l.current
		l.scanToken()
	}

	l.tokens = append(l.tokens, Token{
		Type:   TokenEOF,
		Value:  "",
		Line:   l.line,
		Column: l.column,
	})

	return l.tokens, l.errors
}

// scanToken scans a single token.
func (l *Lexer) scanToken() {
	ch := l.advance()

	switch ch {
	case '(':
		l.addToken(TokenLeftParen, "(")
	case ')':
		l.addToken(TokenRightParen, ")")
	case '{':
		l.addToken(TokenLeftBrace, "{")
	case '}':
		l.addToken(TokenRightBrace, "}")
	case '[':
		l.addToken(TokenLeftBracket, "[")
	case ']':
		l.addToken(TokenRightBracket, "]")
	case ';':
		l.addToken(TokenSemicolon, ";")
	case ',':
		l.addToken(TokenComma, ",")
	case '.':
		l.addToken(TokenDot, ".")
	case ':':
		l.addToken(TokenColon, ":")
	case '+':
		l.addToken(TokenPlus, "+")
	case '-':
		l.addToken(TokenMinus, "-")
	case '*':
		if l.peek() == '*' {
			// ** wildcard — treat as identifier
			l.advance()
			l.addToken(TokenIdentifier, "**")
		} else {
			l.addToken(TokenStar, "*")
		}
	case '%':
		l.addToken(TokenPercent, "%")

	case '=':
		if l.match('=') {
			l.addToken(TokenEquals, "==")
		} else {
			l.addToken(TokenAssign, "=")
		}
	case '!':
		if l.match('=') {
			l.addToken(TokenNotEquals, "!=")
		} else {
			l.addToken(TokenBang, "!")
		}
	case '<':
		if l.match('=') {
			l.addToken(TokenLessEqual, "<=")
		} else {
			l.addToken(TokenLess, "<")
		}
	case '>':
		if l.match('=') {
			l.addToken(TokenGreaterEqual, ">=")
		} else {
			l.addToken(TokenGreater, ">")
		}
	case '&':
		if l.match('&') {
			l.addToken(TokenAnd, "&&")
		} else {
			l.addError("unexpected '&', did you mean '&&'?")
		}
	case '|':
		if l.match('|') {
			l.addToken(TokenOr, "||")
		} else {
			l.addError("unexpected '|', did you mean '||'?")
		}

	case '/':
		if l.match('/') {
			// Line comment
			for !l.isAtEnd() && l.peek() != '\n' {
				l.advance()
			}
		} else if l.match('*') {
			// Block comment
			l.blockComment()
		} else {
			l.addToken(TokenSlash, "/")
		}

	case '"':
		l.scanString('"')
	case '\'':
		l.scanString('\'')

	case '\n':
		l.line++
		l.column = 1
	case ' ', '\t', '\r':
		// Skip whitespace

	default:
		if isDigit(ch) {
			l.scanNumber()
		} else if isIdentStart(ch) {
			l.scanIdentifier()
		} else {
			l.addError(fmt.Sprintf("unexpected character '%c'", ch))
		}
	}
}

// scanString scans a string literal delimited by the given quote character.
func (l *Lexer) scanString(quote byte) {
	var sb strings.Builder

	for !l.isAtEnd() && l.peek() != quote {
		if l.peek() == '\n' {
			l.line++
			l.column = 1
		}
		if l.peek() == '\\' {
			l.advance() // consume backslash
			if l.isAtEnd() {
				l.addError("unterminated string escape")
				return
			}
			escaped := l.advance()
			switch escaped {
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case 'r':
				sb.WriteByte('\r')
			case '\\':
				sb.WriteByte('\\')
			case '\'':
				sb.WriteByte('\'')
			case '"':
				sb.WriteByte('"')
			default:
				sb.WriteByte('\\')
				sb.WriteByte(byte(escaped))
			}
		} else {
			sb.WriteByte(byte(l.advance()))
		}
	}

	if l.isAtEnd() {
		l.addError("unterminated string")
		return
	}

	l.advance() // consume closing quote
	l.addToken(TokenString, sb.String())
}

// scanNumber scans an integer or floating-point number.
func (l *Lexer) scanNumber() {
	for !l.isAtEnd() && isDigit(l.peek()) {
		l.advance()
	}

	// Look for fractional part
	if !l.isAtEnd() && l.peek() == '.' {
		// Check next char is a digit (not a method call like 123.toString)
		if l.current+1 < len(l.source) && isDigit(l.source[l.current+1]) {
			l.advance() // consume '.'
			for !l.isAtEnd() && isDigit(l.peek()) {
				l.advance()
			}
		}
	}

	l.addToken(TokenNumber, l.source[l.start:l.current])
}

// scanIdentifier scans an identifier or keyword.
func (l *Lexer) scanIdentifier() {
	for !l.isAtEnd() && isIdentPart(l.peek()) {
		l.advance()
	}

	text := l.source[l.start:l.current]

	// Check for keywords
	if tokenType, ok := keywords[text]; ok {
		l.addToken(tokenType, text)
	} else {
		l.addToken(TokenIdentifier, text)
	}
}

// blockComment scans a /* ... */ block comment.
func (l *Lexer) blockComment() {
	depth := 1
	for !l.isAtEnd() && depth > 0 {
		if l.peek() == '/' && l.current+1 < len(l.source) && l.source[l.current+1] == '*' {
			depth++
			l.advance()
			l.advance()
		} else if l.peek() == '*' && l.current+1 < len(l.source) && l.source[l.current+1] == '/' {
			depth--
			l.advance()
			l.advance()
		} else {
			if l.peek() == '\n' {
				l.line++
				l.column = 1
			}
			l.advance()
		}
	}

	if depth > 0 {
		l.addError("unterminated block comment")
	}
}

// ── Helper Methods ──

func (l *Lexer) advance() byte {
	ch := l.source[l.current]
	l.current++
	l.column++
	return ch
}

func (l *Lexer) peek() byte {
	if l.isAtEnd() {
		return 0
	}
	return l.source[l.current]
}

func (l *Lexer) match(expected byte) bool {
	if l.isAtEnd() || l.source[l.current] != expected {
		return false
	}
	l.current++
	l.column++
	return true
}

func (l *Lexer) isAtEnd() bool {
	return l.current >= len(l.source)
}

func (l *Lexer) addToken(tokenType TokenType, value string) {
	l.tokens = append(l.tokens, Token{
		Type:   tokenType,
		Value:  value,
		Line:   l.line,
		Column: l.column - len(value),
	})
}

func (l *Lexer) addError(msg string) {
	l.errors = append(l.errors, LexError{
		Message: msg,
		Line:    l.line,
		Column:  l.column,
	})
}

// isDigit returns true if the rune is 0-9.
func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

// isIdentStart returns true if the rune can start an identifier.
func isIdentStart(ch byte) bool {
	return unicode.IsLetter(rune(ch)) || ch == '_'
}

// isIdentPart returns true if the rune can be part of an identifier.
func isIdentPart(ch byte) bool {
	return unicode.IsLetter(rune(ch)) || unicode.IsDigit(rune(ch)) || ch == '_'
}
