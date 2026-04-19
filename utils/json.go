package utils

import (
	"encoding/json"
	"fmt"
	"io"
)

// JSONMarshal serializes a value to a JSON byte slice.
// Returns an error if marshaling fails.
func JSONMarshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// JSONMarshalPretty serializes a value to an indented JSON byte slice.
// Useful for debugging and human-readable output.
func JSONMarshalPretty(v interface{}) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

// JSONUnmarshal deserializes a JSON byte slice into the target value.
func JSONUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// JSONDecode reads JSON from a reader and decodes it into the target value.
func JSONDecode(r io.Reader, v interface{}) error {
	decoder := json.NewDecoder(r)
	return decoder.Decode(v)
}

// JSONEncode writes the JSON encoding of v to the writer.
func JSONEncode(w io.Writer, v interface{}) error {
	encoder := json.NewEncoder(w)
	return encoder.Encode(v)
}

// ToMap converts a struct or other value to a map[string]interface{} via JSON round-trip.
// This is useful for merging or inspecting structured data generically.
func ToMap(v interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal value to JSON: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to map: %w", err)
	}

	return result, nil
}

// GetStringField safely extracts a string field from a map.
// Returns the value and true if found and is a string, empty string and false otherwise.
func GetStringField(m map[string]interface{}, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// GetFloat64Field safely extracts a float64 field from a map.
// JSON numbers are decoded as float64 by default in Go.
func GetFloat64Field(m map[string]interface{}, key string) (float64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	f, ok := v.(float64)
	return f, ok
}

// GetBoolField safely extracts a bool field from a map.
func GetBoolField(m map[string]interface{}, key string) (bool, bool) {
	v, ok := m[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// GetMapField safely extracts a nested map field.
func GetMapField(m map[string]interface{}, key string) (map[string]interface{}, bool) {
	v, ok := m[key]
	if !ok {
		return nil, false
	}
	nested, ok := v.(map[string]interface{})
	return nested, ok
}

// GetSliceField safely extracts a slice field from a map.
func GetSliceField(m map[string]interface{}, key string) ([]interface{}, bool) {
	v, ok := m[key]
	if !ok {
		return nil, false
	}
	slice, ok := v.([]interface{})
	return slice, ok
}
