package rules

import (
	"fmt"
	"strings"
	"time"
)

// Evaluator evaluates parsed rules against a request context.
// It walks the AST and determines whether a request should be allowed or denied.
type Evaluator struct {
	// ruleSet is the parsed rules AST.
	ruleSet *RuleSet
	// builtins is the registry of built-in functions.
	builtins *BuiltinRegistry
}

// RequestContext contains all the information available during rule evaluation.
type RequestContext struct {
	// Auth contains the authenticated user's information (nil if unauthenticated).
	Auth *AuthContext `json:"auth"`
	// Method is the operation being performed: "get", "list", "create", "update", "delete".
	Method string `json:"method"`
	// Path is the resource path being accessed (e.g., "users/uid123").
	Path string `json:"path"`
	// Resource is the incoming data being written (for create/update operations).
	Resource map[string]interface{} `json:"resource"`
	// ExistingData is the current data at the path (for update/delete operations).
	ExistingData map[string]interface{} `json:"existing_data"`
	// Timestamp is the current server time.
	Timestamp time.Time `json:"timestamp"`
}

// AuthContext represents the authenticated user's information.
type AuthContext struct {
	// UID is the user's unique identifier.
	UID string `json:"uid"`
	// Email is the user's email address.
	Email string `json:"email"`
	// Token contains the JWT custom claims.
	Token map[string]interface{} `json:"token"`
}

// EvalResult is the outcome of evaluating a request against the rules.
type EvalResult struct {
	// Allowed is true if the request is permitted.
	Allowed bool `json:"allowed"`
	// MatchedRule describes which rule matched (for debugging).
	MatchedRule string `json:"matched_rule,omitempty"`
	// Reason explains why the request was allowed or denied.
	Reason string `json:"reason,omitempty"`
}

// NewEvaluator creates a new evaluator for the given rule set.
func NewEvaluator(ruleSet *RuleSet) *Evaluator {
	return &Evaluator{
		ruleSet:  ruleSet,
		builtins: DefaultBuiltins(),
	}
}

// Evaluate checks whether the given request is allowed by the rules.
// Service name should be "forge.database" or "forge.storage".
func (ev *Evaluator) Evaluate(serviceName string, ctx *RequestContext) *EvalResult {
	if ev.ruleSet == nil {
		return &EvalResult{Allowed: false, Reason: "no rules defined"}
	}

	// Find the matching service block
	var serviceBlock *ServiceBlock
	for _, svc := range ev.ruleSet.Services {
		if svc.Name == serviceName {
			serviceBlock = svc
			break
		}
	}

	if serviceBlock == nil {
		return &EvalResult{Allowed: false, Reason: fmt.Sprintf("no rules for service %q", serviceName)}
	}

	// Resolve the path into segments
	pathSegments := splitRulePath(ctx.Path)

	// Build the evaluation scope
	scope := newScope(nil)
	scope.set("request", ev.buildRequestObject(ctx))
	scope.set("resource", ev.buildResourceObject(ctx))

	// Try to match the path against match blocks
	result := ev.evaluateMatchBlocks(serviceBlock.Matches, pathSegments, 0, ctx, scope)
	if result != nil {
		return result
	}

	// Default deny
	return &EvalResult{Allowed: false, Reason: "no matching rule found"}
}

// evaluateMatchBlocks tries each match block against the remaining path.
func (ev *Evaluator) evaluateMatchBlocks(matches []*MatchBlock, pathSegments []string, depth int, ctx *RequestContext, parentScope *scope) *EvalResult {
	for _, match := range matches {
		childScope := newScope(parentScope)

		// Try to match the path segments against this block's pattern
		consumed, ok := ev.matchPath(match, pathSegments[depth:], childScope)
		if !ok {
			continue
		}

		newDepth := depth + consumed

		// Register let bindings
		for _, let := range match.Lets {
			val := ev.evalExpr(let.Value, childScope)
			childScope.set(let.Name, val)
		}

		// Register functions
		for _, fn := range match.Functions {
			childScope.setFunc(fn.Name, fn)
		}

		// If we've consumed the entire path, evaluate allow rules
		if newDepth >= len(pathSegments) || match.IsRecursiveWildcard {
			for _, rule := range match.Rules {
				if ev.operationMatches(rule.Operations, ctx.Method) {
					if rule.Condition == nil {
						return &EvalResult{
							Allowed:     true,
							MatchedRule: fmt.Sprintf("allow %s (unconditional)", strings.Join(rule.Operations, ", ")),
						}
					}

					result := ev.evalExpr(rule.Condition, childScope)
					if toBool(result) {
						return &EvalResult{
							Allowed:     true,
							MatchedRule: fmt.Sprintf("allow %s: if %s", strings.Join(rule.Operations, ", "), ExprString(rule.Condition)),
						}
					}
				}
			}
		}

		// Try nested match blocks
		if len(match.NestedMatches) > 0 {
			result := ev.evaluateMatchBlocks(match.NestedMatches, pathSegments, newDepth, ctx, childScope)
			if result != nil {
				return result
			}
		}
	}

	return nil
}

// matchPath tries to match path segments against a match block's pattern.
// Returns the number of path segments consumed and whether the match succeeded.
func (ev *Evaluator) matchPath(match *MatchBlock, segments []string, s *scope) (int, bool) {
	pattern := match.PathSegments

	if len(pattern) == 0 {
		// Empty pattern matches everything (root match)
		return 0, true
	}

	if match.IsRecursiveWildcard {
		// Recursive wildcard matches any remaining path
		if len(segments) == 0 {
			return 0, false
		}
		// Capture all remaining segments
		for _, seg := range pattern {
			if seg.IsRecursiveWildcard {
				s.set(seg.VariableName, strings.Join(segments, "/"))
				return len(segments), true
			}
		}
		return len(segments), true
	}

	if len(segments) < len(pattern) {
		return 0, false
	}

	for i, pat := range pattern {
		if pat.IsVariable {
			// Variable segment — capture the value
			s.set(pat.VariableName, segments[i])
		} else {
			// Static segment — must match exactly
			if segments[i] != pat.Value {
				return 0, false
			}
		}
	}

	return len(pattern), true
}

// operationMatches checks if the request method matches any of the allowed operations.
func (ev *Evaluator) operationMatches(operations []string, method string) bool {
	for _, op := range operations {
		switch op {
		case "read":
			if method == "get" || method == "list" || method == "read" {
				return true
			}
		case "write":
			if method == "create" || method == "update" || method == "delete" || method == "write" {
				return true
			}
		default:
			if op == method {
				return true
			}
		}
	}
	return false
}

// ── Expression Evaluation ──

// evalExpr evaluates an expression and returns its value.
func (ev *Evaluator) evalExpr(expr Expr, s *scope) interface{} {
	if expr == nil {
		return nil
	}

	switch e := expr.(type) {
	case *BoolLit:
		return e.Value
	case *NumberLit:
		return e.Value
	case *StringLit:
		return e.Value
	case *NullLit:
		return nil

	case *IdentifierExpr:
		val, ok := s.get(e.Name)
		if !ok {
			return nil
		}
		return val

	case *BinaryExpr:
		return ev.evalBinary(e, s)

	case *UnaryExpr:
		return ev.evalUnary(e, s)

	case *MemberExpr:
		obj := ev.evalExpr(e.Object, s)
		return getMember(obj, e.Property)

	case *CallExpr:
		return ev.evalCall(e, s)

	case *IndexExpr:
		obj := ev.evalExpr(e.Object, s)
		idx := ev.evalExpr(e.Index, s)
		return getIndex(obj, idx)

	case *ArrayLit:
		elements := make([]interface{}, len(e.Elements))
		for i, elem := range e.Elements {
			elements[i] = ev.evalExpr(elem, s)
		}
		return elements

	case *MapLit:
		m := make(map[string]interface{})
		for i := range e.Keys {
			m[e.Keys[i]] = ev.evalExpr(e.Values[i], s)
		}
		return m

	default:
		return nil
	}
}

// evalBinary evaluates a binary expression.
func (ev *Evaluator) evalBinary(e *BinaryExpr, s *scope) interface{} {
	// Short-circuit for logical operators
	if e.Operator == "&&" {
		left := ev.evalExpr(e.Left, s)
		if !toBool(left) {
			return false
		}
		return toBool(ev.evalExpr(e.Right, s))
	}

	if e.Operator == "||" {
		left := ev.evalExpr(e.Left, s)
		if toBool(left) {
			return true
		}
		return toBool(ev.evalExpr(e.Right, s))
	}

	left := ev.evalExpr(e.Left, s)
	right := ev.evalExpr(e.Right, s)

	switch e.Operator {
	case "==":
		return equals(left, right)
	case "!=":
		return !equals(left, right)
	case "<":
		return compareNum(left, right) < 0
	case "<=":
		return compareNum(left, right) <= 0
	case ">":
		return compareNum(left, right) > 0
	case ">=":
		return compareNum(left, right) >= 0
	case "+":
		return add(left, right)
	case "-":
		return subtract(left, right)
	case "*":
		return multiply(left, right)
	case "/":
		return divide(left, right)
	case "%":
		return modulo(left, right)
	case "in":
		return inCollection(left, right)
	case "is":
		return isType(left, right)
	default:
		return nil
	}
}

// evalUnary evaluates a unary expression.
func (ev *Evaluator) evalUnary(e *UnaryExpr, s *scope) interface{} {
	operand := ev.evalExpr(e.Operand, s)

	switch e.Operator {
	case "!":
		return !toBool(operand)
	case "-":
		return -toNumber(operand)
	default:
		return nil
	}
}

// evalCall evaluates a function call expression.
func (ev *Evaluator) evalCall(e *CallExpr, s *scope) interface{} {
	// Evaluate arguments
	args := make([]interface{}, len(e.Args))
	for i, arg := range e.Args {
		args[i] = ev.evalExpr(arg, s)
	}

	// Check for member function call (e.g., data.hasAll(), list.size())
	if member, ok := e.Callee.(*MemberExpr); ok {
		obj := ev.evalExpr(member.Object, s)
		return ev.builtins.CallMethod(obj, member.Property, args)
	}

	// Check for built-in function
	if ident, ok := e.Callee.(*IdentifierExpr); ok {
		// Check user-defined functions first
		if fn, ok := s.getFunc(ident.Name); ok {
			return ev.callUserFunc(fn, args, s)
		}
		// Then built-ins
		return ev.builtins.Call(ident.Name, args)
	}

	return nil
}

// callUserFunc executes a user-defined function.
func (ev *Evaluator) callUserFunc(fn *FunctionDecl, args []interface{}, parentScope *scope) interface{} {
	fnScope := newScope(parentScope)

	// Bind parameters
	for i, param := range fn.Params {
		if i < len(args) {
			fnScope.set(param, args[i])
		} else {
			fnScope.set(param, nil)
		}
	}

	return ev.evalExpr(fn.Body, fnScope)
}

// buildRequestObject creates the "request" object available in rules.
func (ev *Evaluator) buildRequestObject(ctx *RequestContext) map[string]interface{} {
	req := map[string]interface{}{
		"method": ctx.Method,
		"path":   ctx.Path,
		"time":   ctx.Timestamp.Unix(),
	}

	if ctx.Auth != nil {
		authMap := map[string]interface{}{
			"uid":   ctx.Auth.UID,
			"email": ctx.Auth.Email,
		}
		if ctx.Auth.Token != nil {
			authMap["token"] = ctx.Auth.Token
		}
		req["auth"] = authMap
	} else {
		req["auth"] = nil
	}

	if ctx.Resource != nil {
		req["resource"] = map[string]interface{}{
			"data": ctx.Resource,
		}
	}

	return req
}

// buildResourceObject creates the "resource" object (existing data).
func (ev *Evaluator) buildResourceObject(ctx *RequestContext) map[string]interface{} {
	if ctx.ExistingData != nil {
		return map[string]interface{}{
			"data": ctx.ExistingData,
		}
	}
	return nil
}

// ── Scope (variable environment) ──

type scope struct {
	parent *scope
	vars   map[string]interface{}
	funcs  map[string]*FunctionDecl
}

func newScope(parent *scope) *scope {
	return &scope{
		parent: parent,
		vars:   make(map[string]interface{}),
		funcs:  make(map[string]*FunctionDecl),
	}
}

func (s *scope) set(name string, value interface{}) {
	s.vars[name] = value
}

func (s *scope) get(name string) (interface{}, bool) {
	if val, ok := s.vars[name]; ok {
		return val, true
	}
	if s.parent != nil {
		return s.parent.get(name)
	}
	return nil, false
}

func (s *scope) setFunc(name string, fn *FunctionDecl) {
	s.funcs[name] = fn
}

func (s *scope) getFunc(name string) (*FunctionDecl, bool) {
	if fn, ok := s.funcs[name]; ok {
		return fn, true
	}
	if s.parent != nil {
		return s.parent.getFunc(name)
	}
	return nil, false
}

// ── Value Helpers ──

// toBool converts a value to boolean.
func toBool(v interface{}) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val != ""
	case float64:
		return val != 0
	case int:
		return val != 0
	case map[string]interface{}:
		return true
	case []interface{}:
		return len(val) > 0
	default:
		return true
	}
}

// toNumber converts a value to float64.
func toNumber(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case bool:
		if val {
			return 1
		}
		return 0
	default:
		return 0
	}
}

// equals checks deep equality between two values.
func equals(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Numeric comparison (int == float64)
	aNum, aIsNum := toNumericValue(a)
	bNum, bIsNum := toNumericValue(b)
	if aIsNum && bIsNum {
		return aNum == bNum
	}

	// String comparison
	aStr, aIsStr := a.(string)
	bStr, bIsStr := b.(string)
	if aIsStr && bIsStr {
		return aStr == bStr
	}

	// Bool comparison
	aBool, aIsBool := a.(bool)
	bBool, bIsBool := b.(bool)
	if aIsBool && bIsBool {
		return aBool == bBool
	}

	return false
}

func toNumericValue(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	default:
		return 0, false
	}
}

// compareNum compares two numeric values.
func compareNum(a, b interface{}) int {
	aNum := toNumber(a)
	bNum := toNumber(b)
	if aNum < bNum {
		return -1
	}
	if aNum > bNum {
		return 1
	}
	return 0
}

// add adds two values (numeric or string concatenation).
func add(a, b interface{}) interface{} {
	aStr, aIsStr := a.(string)
	bStr, bIsStr := b.(string)
	if aIsStr || bIsStr {
		if !aIsStr {
			aStr = fmt.Sprintf("%v", a)
		}
		if !bIsStr {
			bStr = fmt.Sprintf("%v", b)
		}
		return aStr + bStr
	}
	return toNumber(a) + toNumber(b)
}

func subtract(a, b interface{}) interface{} { return toNumber(a) - toNumber(b) }
func multiply(a, b interface{}) interface{} { return toNumber(a) * toNumber(b) }

func divide(a, b interface{}) interface{} {
	bNum := toNumber(b)
	if bNum == 0 {
		return nil // Division by zero
	}
	return toNumber(a) / bNum
}

func modulo(a, b interface{}) interface{} {
	bNum := int(toNumber(b))
	if bNum == 0 {
		return nil
	}
	return float64(int(toNumber(a)) % bNum)
}

// inCollection checks if a value is in a collection (array or map keys).
func inCollection(value, collection interface{}) bool {
	switch coll := collection.(type) {
	case []interface{}:
		for _, item := range coll {
			if equals(value, item) {
				return true
			}
		}
	case map[string]interface{}:
		key, ok := value.(string)
		if !ok {
			return false
		}
		_, exists := coll[key]
		return exists
	}
	return false
}

// isType checks if a value is of a given type name.
func isType(value, typeName interface{}) bool {
	name, ok := typeName.(string)
	if !ok {
		return false
	}

	switch name {
	case "string":
		_, ok := value.(string)
		return ok
	case "number", "int", "float":
		_, ok := toNumericValue(value)
		return ok
	case "bool":
		_, ok := value.(bool)
		return ok
	case "list":
		_, ok := value.([]interface{})
		return ok
	case "map":
		_, ok := value.(map[string]interface{})
		return ok
	case "null":
		return value == nil
	default:
		return false
	}
}

// getMember accesses a property on an object.
func getMember(obj interface{}, property string) interface{} {
	if obj == nil {
		return nil
	}

	switch o := obj.(type) {
	case map[string]interface{}:
		return o[property]
	case string:
		switch property {
		case "size", "length":
			return float64(len(o))
		}
	case []interface{}:
		switch property {
		case "size", "length":
			return float64(len(o))
		}
	}
	return nil
}

// getIndex accesses an element by index.
func getIndex(obj, index interface{}) interface{} {
	if obj == nil {
		return nil
	}

	switch o := obj.(type) {
	case []interface{}:
		idx := int(toNumber(index))
		if idx >= 0 && idx < len(o) {
			return o[idx]
		}
	case map[string]interface{}:
		key, ok := index.(string)
		if ok {
			return o[key]
		}
	}
	return nil
}

// splitRulePath splits a resource path into segments.
func splitRulePath(path string) []string {
	var segments []string
	for _, seg := range strings.Split(path, "/") {
		if seg != "" {
			segments = append(segments, seg)
		}
	}
	return segments
}
