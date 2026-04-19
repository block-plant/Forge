package rules

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// BuiltinRegistry holds all built-in functions and methods available in rules.
type BuiltinRegistry struct {
	// functions maps function names to implementations.
	functions map[string]BuiltinFunc
	// methods maps type+method names to implementations.
	methods map[string]BuiltinMethod
}

// BuiltinFunc is a built-in function callable from rules.
type BuiltinFunc func(args []interface{}) interface{}

// BuiltinMethod is a built-in method callable on objects.
type BuiltinMethod func(receiver interface{}, args []interface{}) interface{}

// DefaultBuiltins creates a registry with all standard built-in functions.
func DefaultBuiltins() *BuiltinRegistry {
	r := &BuiltinRegistry{
		functions: make(map[string]BuiltinFunc),
		methods:   make(map[string]BuiltinMethod),
	}

	r.registerFunctions()
	r.registerMethods()

	return r
}

// Call invokes a built-in function by name.
func (r *BuiltinRegistry) Call(name string, args []interface{}) interface{} {
	if fn, ok := r.functions[name]; ok {
		return fn(args)
	}
	return nil
}

// CallMethod invokes a method on a receiver object.
func (r *BuiltinRegistry) CallMethod(receiver interface{}, methodName string, args []interface{}) interface{} {
	// Try type-specific method
	typeName := typeOf(receiver)
	key := typeName + "." + methodName
	if method, ok := r.methods[key]; ok {
		return method(receiver, args)
	}

	// Try universal methods
	key = "*." + methodName
	if method, ok := r.methods[key]; ok {
		return method(receiver, args)
	}

	return nil
}

// registerFunctions adds all built-in functions.
func (r *BuiltinRegistry) registerFunctions() {
	// exists(value) — returns true if value is not null
	r.functions["exists"] = func(args []interface{}) interface{} {
		if len(args) == 0 {
			return false
		}
		return args[0] != nil
	}

	// debug(value) — returns the value (for debugging expressions)
	r.functions["debug"] = func(args []interface{}) interface{} {
		if len(args) == 0 {
			return nil
		}
		return args[0]
	}

	// int(value) — converts to integer
	r.functions["int"] = func(args []interface{}) interface{} {
		if len(args) == 0 {
			return 0.0
		}
		return math.Floor(toNumber(args[0]))
	}

	// float(value) — converts to float
	r.functions["float"] = func(args []interface{}) interface{} {
		if len(args) == 0 {
			return 0.0
		}
		return toNumber(args[0])
	}

	// string(value) — converts to string
	r.functions["string"] = func(args []interface{}) interface{} {
		if len(args) == 0 {
			return ""
		}
		return fmt.Sprintf("%v", args[0])
	}

	// math.abs(x) — available as abs()
	r.functions["abs"] = func(args []interface{}) interface{} {
		if len(args) == 0 {
			return 0.0
		}
		return math.Abs(toNumber(args[0]))
	}

	// math.ceil(x)
	r.functions["ceil"] = func(args []interface{}) interface{} {
		if len(args) == 0 {
			return 0.0
		}
		return math.Ceil(toNumber(args[0]))
	}

	// math.floor(x)
	r.functions["floor"] = func(args []interface{}) interface{} {
		if len(args) == 0 {
			return 0.0
		}
		return math.Floor(toNumber(args[0]))
	}

	// now() — returns current unix timestamp
	r.functions["now"] = func(args []interface{}) interface{} {
		return float64(time.Now().Unix())
	}

	// duration.value(unit) — helper for time calculations
	r.functions["duration"] = func(args []interface{}) interface{} {
		if len(args) < 2 {
			return 0.0
		}
		value := toNumber(args[0])
		unit, ok := args[1].(string)
		if !ok {
			return 0.0
		}
		switch unit {
		case "s", "seconds":
			return value
		case "m", "minutes":
			return value * 60
		case "h", "hours":
			return value * 3600
		case "d", "days":
			return value * 86400
		default:
			return value
		}
	}
}

// registerMethods adds all built-in methods on types.
func (r *BuiltinRegistry) registerMethods() {
	// ── String Methods ──

	// string.size() — returns string length
	r.methods["string.size"] = func(receiver interface{}, args []interface{}) interface{} {
		s, _ := receiver.(string)
		return float64(len(s))
	}

	// string.length() — alias for size()
	r.methods["string.length"] = r.methods["string.size"]

	// string.matches(regex) — basic pattern matching (not full regex)
	r.methods["string.matches"] = func(receiver interface{}, args []interface{}) interface{} {
		s, _ := receiver.(string)
		if len(args) == 0 {
			return false
		}
		pattern, _ := args[0].(string)
		return simpleMatch(s, pattern)
	}

	// string.trim() — remove leading/trailing whitespace
	r.methods["string.trim"] = func(receiver interface{}, args []interface{}) interface{} {
		s, _ := receiver.(string)
		return strings.TrimSpace(s)
	}

	// string.lower() — convert to lowercase
	r.methods["string.lower"] = func(receiver interface{}, args []interface{}) interface{} {
		s, _ := receiver.(string)
		return strings.ToLower(s)
	}

	// string.upper() — convert to uppercase
	r.methods["string.upper"] = func(receiver interface{}, args []interface{}) interface{} {
		s, _ := receiver.(string)
		return strings.ToUpper(s)
	}

	// string.split(sep) — split string into list
	r.methods["string.split"] = func(receiver interface{}, args []interface{}) interface{} {
		s, _ := receiver.(string)
		sep := ","
		if len(args) > 0 {
			sep, _ = args[0].(string)
		}
		parts := strings.Split(s, sep)
		result := make([]interface{}, len(parts))
		for i, p := range parts {
			result[i] = p
		}
		return result
	}

	// string.contains(substr) — check if string contains substring
	r.methods["string.contains"] = func(receiver interface{}, args []interface{}) interface{} {
		s, _ := receiver.(string)
		if len(args) == 0 {
			return false
		}
		sub, _ := args[0].(string)
		return strings.Contains(s, sub)
	}

	// string.startsWith(prefix)
	r.methods["string.startsWith"] = func(receiver interface{}, args []interface{}) interface{} {
		s, _ := receiver.(string)
		if len(args) == 0 {
			return false
		}
		prefix, _ := args[0].(string)
		return strings.HasPrefix(s, prefix)
	}

	// string.endsWith(suffix)
	r.methods["string.endsWith"] = func(receiver interface{}, args []interface{}) interface{} {
		s, _ := receiver.(string)
		if len(args) == 0 {
			return false
		}
		suffix, _ := args[0].(string)
		return strings.HasSuffix(s, suffix)
	}

	// ── List (Array) Methods ──

	// list.size() — returns array length
	r.methods["list.size"] = func(receiver interface{}, args []interface{}) interface{} {
		arr, ok := receiver.([]interface{})
		if !ok {
			return 0.0
		}
		return float64(len(arr))
	}

	// list.length() — alias for size()
	r.methods["list.length"] = r.methods["list.size"]

	// list.hasAny(other) — checks if any elements in 'other' exist in receiver
	r.methods["list.hasAny"] = func(receiver interface{}, args []interface{}) interface{} {
		arr, ok := receiver.([]interface{})
		if !ok || len(args) == 0 {
			return false
		}
		other, ok := args[0].([]interface{})
		if !ok {
			return false
		}
		for _, item := range other {
			for _, elem := range arr {
				if equals(elem, item) {
					return true
				}
			}
		}
		return false
	}

	// list.hasAll(other) — checks if all elements in 'other' exist in receiver
	r.methods["list.hasAll"] = func(receiver interface{}, args []interface{}) interface{} {
		arr, ok := receiver.([]interface{})
		if !ok || len(args) == 0 {
			return false
		}
		other, ok := args[0].([]interface{})
		if !ok {
			return false
		}
		for _, item := range other {
			found := false
			for _, elem := range arr {
				if equals(elem, item) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	}

	// list.contains(element) — checks if element exists in list
	r.methods["list.contains"] = func(receiver interface{}, args []interface{}) interface{} {
		arr, ok := receiver.([]interface{})
		if !ok || len(args) == 0 {
			return false
		}
		for _, elem := range arr {
			if equals(elem, args[0]) {
				return true
			}
		}
		return false
	}

	// list.join(sep) — joins elements with separator
	r.methods["list.join"] = func(receiver interface{}, args []interface{}) interface{} {
		arr, ok := receiver.([]interface{})
		if !ok {
			return ""
		}
		sep := ","
		if len(args) > 0 {
			if s, ok := args[0].(string); ok {
				sep = s
			}
		}
		parts := make([]string, len(arr))
		for i, elem := range arr {
			parts[i] = fmt.Sprintf("%v", elem)
		}
		return strings.Join(parts, sep)
	}

	// ── Map Methods ──

	// map.size() — returns number of keys
	r.methods["map.size"] = func(receiver interface{}, args []interface{}) interface{} {
		m, ok := receiver.(map[string]interface{})
		if !ok {
			return 0.0
		}
		return float64(len(m))
	}

	// map.keys() — returns list of keys
	r.methods["map.keys"] = func(receiver interface{}, args []interface{}) interface{} {
		m, ok := receiver.(map[string]interface{})
		if !ok {
			return []interface{}{}
		}
		keys := make([]interface{}, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		return keys
	}

	// map.values() — returns list of values
	r.methods["map.values"] = func(receiver interface{}, args []interface{}) interface{} {
		m, ok := receiver.(map[string]interface{})
		if !ok {
			return []interface{}{}
		}
		values := make([]interface{}, 0, len(m))
		for _, v := range m {
			values = append(values, v)
		}
		return values
	}

	// map.diff(other) — returns keys that are different
	r.methods["map.diff"] = func(receiver interface{}, args []interface{}) interface{} {
		m, ok := receiver.(map[string]interface{})
		if !ok || len(args) == 0 {
			return []interface{}{}
		}
		other, ok := args[0].(map[string]interface{})
		if !ok {
			return []interface{}{}
		}
		var diff []interface{}
		for k, v := range m {
			if ov, exists := other[k]; !exists || !equals(v, ov) {
				diff = append(diff, k)
			}
		}
		for k := range other {
			if _, exists := m[k]; !exists {
				diff = append(diff, k)
			}
		}
		return diff
	}
}

// typeOf returns a string type name for a value.
func typeOf(v interface{}) string {
	if v == nil {
		return "null"
	}
	switch v.(type) {
	case string:
		return "string"
	case float64, int:
		return "number"
	case bool:
		return "bool"
	case []interface{}:
		return "list"
	case map[string]interface{}:
		return "map"
	default:
		return "unknown"
	}
}

// simpleMatch performs basic pattern matching.
// Supports * as a glob wildcard. Not a full regex engine.
func simpleMatch(s, pattern string) bool {
	if pattern == "*" {
		return true
	}

	// Simple glob matching with *
	if strings.Contains(pattern, "*") {
		parts := strings.Split(pattern, "*")
		idx := 0
		for _, part := range parts {
			if part == "" {
				continue
			}
			pos := strings.Index(s[idx:], part)
			if pos < 0 {
				return false
			}
			idx += pos + len(part)
		}
		return true
	}

	// Exact match
	return s == pattern
}
