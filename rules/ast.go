package rules

import "fmt"

// NodeType identifies the kind of AST node.
type NodeType int

const (
	NodeRuleSet       NodeType = iota // top-level rules document
	NodeServiceBlock                  // service forge.database { ... }
	NodeMatchBlock                    // match /path/{var} { ... }
	NodeAllowStmt                     // allow read, write: if <expr>
	NodeFunctionDecl                  // function name(args) { return expr }
	NodeLetStmt                       // let var = expr;

	// Expressions
	NodeBinaryExpr    // left op right
	NodeUnaryExpr     // !expr, -expr
	NodeMemberExpr    // object.property
	NodeCallExpr      // func(args)
	NodeIndexExpr     // array[index]
	NodeIdentifier    // variable name
	NodeStringLiteral // "hello"
	NodeNumberLiteral // 42
	NodeBoolLiteral   // true/false
	NodeNullLiteral   // null
	NodeArrayLiteral  // [1, 2, 3]
	NodeMapLiteral    // {key: value}
	NodePathSegment   // /segment or /{variable} in match paths
	NodeWildcard      // {document=**}
)

// Node is the interface all AST nodes implement.
type Node interface {
	nodeType() NodeType
	Position() (int, int) // line, column
}

// ── Top-Level Nodes ──

// RuleSet is the root AST node representing a complete rules document.
type RuleSet struct {
	Version  string          // rules_version value
	Services []*ServiceBlock // service blocks
	Line     int
	Col      int
}

func (n *RuleSet) nodeType() NodeType     { return NodeRuleSet }
func (n *RuleSet) Position() (int, int)   { return n.Line, n.Col }

// ServiceBlock represents a "service forge.database { ... }" block.
type ServiceBlock struct {
	Name    string        // e.g., "forge.database" or "forge.storage"
	Matches []*MatchBlock // match blocks within this service
	Line    int
	Col     int
}

func (n *ServiceBlock) nodeType() NodeType   { return NodeServiceBlock }
func (n *ServiceBlock) Position() (int, int) { return n.Line, n.Col }

// MatchBlock represents a "match /path/{var} { ... }" block.
type MatchBlock struct {
	// Path is the match path pattern (e.g., "/users/{userId}")
	Path string
	// PathSegments are the individual parsed path segments.
	PathSegments []PathSeg
	// Rules are the allow statements within this match block.
	Rules []*AllowStatement
	// NestedMatches are nested match blocks.
	NestedMatches []*MatchBlock
	// Functions are function declarations within this match block.
	Functions []*FunctionDecl
	// Lets are let bindings within this match block.
	Lets []*LetStatement
	// IsRecursiveWildcard is true if the path ends with {name=**}
	IsRecursiveWildcard bool
	Line                int
	Col                 int
}

func (n *MatchBlock) nodeType() NodeType   { return NodeMatchBlock }
func (n *MatchBlock) Position() (int, int) { return n.Line, n.Col }

// PathSeg represents a single segment of a match path.
type PathSeg struct {
	// Value is the literal segment text.
	Value string
	// IsVariable is true if this is a {variable} capture.
	IsVariable bool
	// VariableName is the name of the captured variable (without braces).
	VariableName string
	// IsRecursiveWildcard is true for {name=**} segments.
	IsRecursiveWildcard bool
}

// AllowStatement represents "allow read, write: if <condition>".
type AllowStatement struct {
	// Operations allowed: "read", "write", "create", "update", "delete", "list", "get"
	Operations []string
	// Condition is the boolean expression after "if".
	// If nil, the allow is unconditional (syntax error in strict mode).
	Condition Expr
	Line      int
	Col       int
}

func (n *AllowStatement) nodeType() NodeType   { return NodeAllowStmt }
func (n *AllowStatement) Position() (int, int) { return n.Line, n.Col }

// FunctionDecl represents "function name(args) { return expr; }".
type FunctionDecl struct {
	Name   string
	Params []string
	Body   Expr // The return expression
	Line   int
	Col    int
}

func (n *FunctionDecl) nodeType() NodeType   { return NodeFunctionDecl }
func (n *FunctionDecl) Position() (int, int) { return n.Line, n.Col }

// LetStatement represents "let name = expr;".
type LetStatement struct {
	Name  string
	Value Expr
	Line  int
	Col   int
}

func (n *LetStatement) nodeType() NodeType   { return NodeLetStmt }
func (n *LetStatement) Position() (int, int) { return n.Line, n.Col }

// ── Expression Nodes ──

// Expr is the interface all expression nodes implement.
type Expr interface {
	Node
	exprNode()
}

// BinaryExpr represents "left op right".
type BinaryExpr struct {
	Left     Expr
	Operator string // "==", "!=", "<", "<=", ">", ">=", "&&", "||", "+", "-", "*", "/", "%", "in", "is"
	Right    Expr
	Line     int
	Col      int
}

func (n *BinaryExpr) nodeType() NodeType   { return NodeBinaryExpr }
func (n *BinaryExpr) Position() (int, int) { return n.Line, n.Col }
func (n *BinaryExpr) exprNode()            {}

// UnaryExpr represents "!expr" or "-expr".
type UnaryExpr struct {
	Operator string // "!" or "-"
	Operand  Expr
	Line     int
	Col      int
}

func (n *UnaryExpr) nodeType() NodeType   { return NodeUnaryExpr }
func (n *UnaryExpr) Position() (int, int) { return n.Line, n.Col }
func (n *UnaryExpr) exprNode()            {}

// MemberExpr represents "object.property".
type MemberExpr struct {
	Object   Expr
	Property string
	Line     int
	Col      int
}

func (n *MemberExpr) nodeType() NodeType   { return NodeMemberExpr }
func (n *MemberExpr) Position() (int, int) { return n.Line, n.Col }
func (n *MemberExpr) exprNode()            {}

// CallExpr represents "func(args)".
type CallExpr struct {
	Callee Expr
	Args   []Expr
	Line   int
	Col    int
}

func (n *CallExpr) nodeType() NodeType   { return NodeCallExpr }
func (n *CallExpr) Position() (int, int) { return n.Line, n.Col }
func (n *CallExpr) exprNode()            {}

// IndexExpr represents "expr[index]".
type IndexExpr struct {
	Object Expr
	Index  Expr
	Line   int
	Col    int
}

func (n *IndexExpr) nodeType() NodeType   { return NodeIndexExpr }
func (n *IndexExpr) Position() (int, int) { return n.Line, n.Col }
func (n *IndexExpr) exprNode()            {}

// IdentifierExpr represents a variable reference.
type IdentifierExpr struct {
	Name string
	Line int
	Col  int
}

func (n *IdentifierExpr) nodeType() NodeType   { return NodeIdentifier }
func (n *IdentifierExpr) Position() (int, int) { return n.Line, n.Col }
func (n *IdentifierExpr) exprNode()            {}

// StringLit represents a string literal.
type StringLit struct {
	Value string
	Line  int
	Col   int
}

func (n *StringLit) nodeType() NodeType   { return NodeStringLiteral }
func (n *StringLit) Position() (int, int) { return n.Line, n.Col }
func (n *StringLit) exprNode()            {}

// NumberLit represents a numeric literal.
type NumberLit struct {
	Value float64
	Raw   string // original text
	Line  int
	Col   int
}

func (n *NumberLit) nodeType() NodeType   { return NodeNumberLiteral }
func (n *NumberLit) Position() (int, int) { return n.Line, n.Col }
func (n *NumberLit) exprNode()            {}

// BoolLit represents a boolean literal.
type BoolLit struct {
	Value bool
	Line  int
	Col   int
}

func (n *BoolLit) nodeType() NodeType   { return NodeBoolLiteral }
func (n *BoolLit) Position() (int, int) { return n.Line, n.Col }
func (n *BoolLit) exprNode()            {}

// NullLit represents a null literal.
type NullLit struct {
	Line int
	Col  int
}

func (n *NullLit) nodeType() NodeType   { return NodeNullLiteral }
func (n *NullLit) Position() (int, int) { return n.Line, n.Col }
func (n *NullLit) exprNode()            {}

// ArrayLit represents [elem1, elem2, ...].
type ArrayLit struct {
	Elements []Expr
	Line     int
	Col      int
}

func (n *ArrayLit) nodeType() NodeType   { return NodeArrayLiteral }
func (n *ArrayLit) Position() (int, int) { return n.Line, n.Col }
func (n *ArrayLit) exprNode()            {}

// MapLit represents {key: value, ...}.
type MapLit struct {
	Keys   []string
	Values []Expr
	Line   int
	Col    int
}

func (n *MapLit) nodeType() NodeType   { return NodeMapLiteral }
func (n *MapLit) Position() (int, int) { return n.Line, n.Col }
func (n *MapLit) exprNode()            {}

// ── Pretty Printing ──

// String returns a human-readable representation of an expression.
func ExprString(e Expr) string {
	if e == nil {
		return "<nil>"
	}

	switch n := e.(type) {
	case *BinaryExpr:
		return fmt.Sprintf("(%s %s %s)", ExprString(n.Left), n.Operator, ExprString(n.Right))
	case *UnaryExpr:
		return fmt.Sprintf("(%s%s)", n.Operator, ExprString(n.Operand))
	case *MemberExpr:
		return fmt.Sprintf("%s.%s", ExprString(n.Object), n.Property)
	case *CallExpr:
		args := make([]string, len(n.Args))
		for i, arg := range n.Args {
			args[i] = ExprString(arg)
		}
		return fmt.Sprintf("%s(%s)", ExprString(n.Callee), joinStrings(args, ", "))
	case *IndexExpr:
		return fmt.Sprintf("%s[%s]", ExprString(n.Object), ExprString(n.Index))
	case *IdentifierExpr:
		return n.Name
	case *StringLit:
		return fmt.Sprintf("%q", n.Value)
	case *NumberLit:
		return n.Raw
	case *BoolLit:
		if n.Value {
			return "true"
		}
		return "false"
	case *NullLit:
		return "null"
	case *ArrayLit:
		elems := make([]string, len(n.Elements))
		for i, elem := range n.Elements {
			elems[i] = ExprString(elem)
		}
		return fmt.Sprintf("[%s]", joinStrings(elems, ", "))
	case *MapLit:
		pairs := make([]string, len(n.Keys))
		for i := range n.Keys {
			pairs[i] = fmt.Sprintf("%s: %s", n.Keys[i], ExprString(n.Values[i]))
		}
		return fmt.Sprintf("{%s}", joinStrings(pairs, ", "))
	default:
		return fmt.Sprintf("<%T>", e)
	}
}

// joinStrings joins strings with a separator (avoids strings pkg import).
func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}
