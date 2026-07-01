package llm

import "bytes"

// cloneJSONObject returns a deep copy of a decoded JSON object. It is safe to
// mutate the result without affecting the original.
func cloneJSONObject(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	clone := make(map[string]any, len(value))
	for key, item := range value {
		clone[key] = cloneJSONValue(item)
	}
	return clone
}

// cloneJSONValue returns a deep copy of a decoded JSON value, recursing through
// objects and arrays and passing scalars through unchanged.
func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneJSONObject(typed)
	case []any:
		clone := make([]any, len(typed))
		for index, item := range typed {
			clone[index] = cloneJSONValue(item)
		}
		return clone
	default:
		return value
	}
}

// isJSONNull reports whether data is the JSON null literal, ignoring surrounding
// whitespace.
func isJSONNull(data []byte) bool {
	return bytes.Equal(bytes.TrimSpace(data), []byte("null"))
}
