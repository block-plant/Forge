package rules

import (
	"fmt"
)

// Validator performs semantic validation on a parsed RuleSet.
// It checks for undefined variables, invalid operations, type errors,
// and structural issues that the parser alone cannot catch.
type Validator struct {
	errors []ValidationError
}

// ValidationError represents a semantic validation error.
type ValidationError struct {
	Message  string
	Line     int
	Column   int
	Severity string // "error" or "warning"
}

// Error implements the error interface.
func (e ValidationError) Error() string {
	return fmt.Sprintf("[%s] line %d, col %d: %s", e.Severity, e.Line, e.Column, e.Message)
}

// validOperations are the allowed operation names in allow statements.
var validOperations = map[string]bool{
	"read":   true,
	"write":  true,
	"get":    true,
	"list":   true,
	"create": true,
	"update": true,
	"delete": true,
}

// validServices are the recognized service names.
var validServices = map[string]bool{
	"forge.database": true,
	"forge.storage":  true,
}

// builtinVars are variables always available in rules scope.
var builtinVars = map[string]bool{
	"request":  true,
	"resource": true,
	"true":     true,
	"false":    true,
	"null":     true,
}

// Validate performs semantic validation on a rule set.
// Returns a list of validation errors (empty if valid).
func Validate(ruleSet *RuleSet) []ValidationError {
	v := &Validator{
		errors: make([]ValidationError, 0),
	}

	if ruleSet == nil {
		v.addError(0, 0, "rule set is nil")
		return v.errors
	}

	// Validate version
	if ruleSet.Version != "" && ruleSet.Version != "2" && ruleSet.Version != "1" {
		v.addWarning(ruleSet.Line, ruleSet.Col,
			fmt.Sprintf("unknown rules_version %q, expected '2'", ruleSet.Version))
	}

	// Validate each service block
	for _, svc := range ruleSet.Services {
		v.validateServiceBlock(svc)
	}

	return v.errors
}

// validateServiceBlock validates a service block.
func (v *Validator) validateServiceBlock(svc *ServiceBlock) {
	if !validServices[svc.Name] {
		v.addWarning(svc.Line, svc.Col,
			fmt.Sprintf("unknown service %q, expected 'forge.database' or 'forge.storage'", svc.Name))
	}

	if len(svc.Matches) == 0 {
		v.addWarning(svc.Line, svc.Col,
			fmt.Sprintf("service %q has no match blocks", svc.Name))
	}

	for _, match := range svc.Matches {
		// Build the set of variables captured by this match's path
		capturedVars := make(map[string]bool)
		v.validateMatchBlock(match, capturedVars)
	}
}

// validateMatchBlock validates a match block and its contents.
func (v *Validator) validateMatchBlock(match *MatchBlock, parentVars map[string]bool) {
	// Collect variables from path segments
	localVars := make(map[string]bool)
	for k := range parentVars {
		localVars[k] = true
	}

	for _, seg := range match.PathSegments {
		if seg.IsVariable {
			if seg.VariableName == "" {
				v.addError(match.Line, match.Col, "empty variable name in path segment")
			} else {
				localVars[seg.VariableName] = true
			}
		}
	}

	// Add built-in variables
	for k := range builtinVars {
		localVars[k] = true
	}

	// Validate allow statements
	for _, rule := range match.Rules {
		v.validateAllowStatement(rule, localVars)
	}

	// Validate let statements
	for _, let := range match.Lets {
		v.validateExpr(let.Value, localVars)
		// Add the let-bound variable to scope
		localVars[let.Name] = true
	}

	// Validate functions
	for _, fn := range match.Functions {
		v.validateFunctionDecl(fn, localVars)
	}

	// Validate nested match blocks
	for _, nested := range match.NestedMatches {
		v.validateMatchBlock(nested, localVars)
	}

	// Check for unreachable rules
	v.checkUnreachableRules(match)
}

// validateAllowStatement validates an allow statement.
func (v *Validator) validateAllowStatement(stmt *AllowStatement, vars map[string]bool) {
	if len(stmt.Operations) == 0 {
		v.addError(stmt.Line, stmt.Col, "allow statement has no operations")
		return
	}

	for _, op := range stmt.Operations {
		if !validOperations[op] {
			v.addError(stmt.Line, stmt.Col,
				fmt.Sprintf("unknown operation %q, expected: read, write, get, list, create, update, delete", op))
		}
	}

	// Validate condition expression
	if stmt.Condition != nil {
		v.validateExpr(stmt.Condition, vars)
	}
}

// validateFunctionDecl validates a function declaration.
func (v *Validator) validateFunctionDecl(fn *FunctionDecl, parentVars map[string]bool) {
	if fn.Name == "" {
		v.addError(fn.Line, fn.Col, "function has no name")
	}

	if fn.Body == nil {
		v.addError(fn.Line, fn.Col,
			fmt.Sprintf("function %q has no return expression", fn.Name))
		return
	}

	// Build function scope with parent vars + params
	fnVars := make(map[string]bool)
	for k := range parentVars {
		fnVars[k] = true
	}
	for _, param := range fn.Params {
		fnVars[param] = true
	}

	v.validateExpr(fn.Body, fnVars)
}

// validateExpr recursively validates an expression for undefined variables.
func (v *Validator) validateExpr(expr Expr, vars map[string]bool) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *IdentifierExpr:
		if !vars[e.Name] && !isBuiltinFunction(e.Name) {
			v.addWarning(e.Line, e.Col,
				fmt.Sprintf("possibly undefined variable %q", e.Name))
		}

	case *BinaryExpr:
		v.validateExpr(e.Left, vars)
		v.validateExpr(e.Right, vars)

	case *UnaryExpr:
		v.validateExpr(e.Operand, vars)

	case *MemberExpr:
		v.validateExpr(e.Object, vars)

	case *CallExpr:
		v.validateExpr(e.Callee, vars)
		for _, arg := range e.Args {
			v.validateExpr(arg, vars)
		}

	case *IndexExpr:
		v.validateExpr(e.Object, vars)
		v.validateExpr(e.Index, vars)

	case *ArrayLit:
		for _, elem := range e.Elements {
			v.validateExpr(elem, vars)
		}

	case *MapLit:
		for _, val := range e.Values {
			v.validateExpr(val, vars)
		}

	case *BoolLit, *NumberLit, *StringLit, *NullLit:
		// Literals always valid
	}
}

// checkUnreachableRules detects rules that can never fire.
func (v *Validator) checkUnreachableRules(match *MatchBlock) {
	// Track which operations have unconditional allows
	unconditional := make(map[string]bool)

	for _, rule := range match.Rules {
		for _, op := range rule.Operations {
			if unconditional[op] {
				v.addWarning(rule.Line, rule.Col,
					fmt.Sprintf("unreachable rule: 'allow %s' — already unconditionally allowed above", op))
			}
		}

		// If this rule has no condition (always true), mark ops as unconditional
		if rule.Condition == nil {
			for _, op := range rule.Operations {
				unconditional[op] = true
			}
		}

		// Check for "if true" which is also unconditional
		if boolLit, ok := rule.Condition.(*BoolLit); ok && boolLit.Value {
			for _, op := range rule.Operations {
				unconditional[op] = true
			}
		}
	}
}

// isBuiltinFunction checks if a name is a known built-in function.
func isBuiltinFunction(name string) bool {
	builtins := map[string]bool{
		"exists":   true,
		"debug":    true,
		"int":      true,
		"float":    true,
		"string":   true,
		"abs":      true,
		"ceil":     true,
		"floor":    true,
		"now":      true,
		"duration": true,
	}
	return builtins[name]
}

// ── Helper Methods ──

func (v *Validator) addError(line, col int, msg string) {
	v.errors = append(v.errors, ValidationError{
		Message:  msg,
		Line:     line,
		Column:   col,
		Severity: "error",
	})
}

func (v *Validator) addWarning(line, col int, msg string) {
	v.errors = append(v.errors, ValidationError{
		Message:  msg,
		Line:     line,
		Column:   col,
		Severity: "warning",
	})
}

// ValidateSource is a convenience function that tokenizes, parses, and validates
// a rules source string in one step.
func ValidateSource(source string) ([]ValidationError, error) {
	ruleSet, errs := ParseRules(source)
	if len(errs) > 0 {
		// Convert parse errors to validation errors
		var result []ValidationError
		for _, err := range errs {
			result = append(result, ValidationError{
				Message:  err.Error(),
				Severity: "error",
			})
		}
		return result, nil
	}

	return Validate(ruleSet), nil
}
