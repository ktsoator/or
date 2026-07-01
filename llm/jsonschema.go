package llm

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

func parseSchema(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}
	if schema == nil {
		return nil, fmt.Errorf("schema must be a JSON object")
	}
	return schema, nil
}

// schemaTypes returns the declared JSON types of a schema. "type" may be a
// single string or an array of strings.
func schemaTypes(schema map[string]any) []string {
	switch typed := schema["type"].(type) {
	case string:
		return []string{typed}
	case []any:
		types := make([]string, 0, len(typed))
		for _, value := range typed {
			if name, ok := value.(string); ok {
				types = append(types, name)
			}
		}
		return types
	}
	return nil
}

// matchesJSONType reports whether value already satisfies a JSON type. JSON
// numbers decode to float64, so integers are float64 with no fractional part.
func matchesJSONType(value any, jsonType string) bool {
	switch jsonType {
	case "number":
		_, ok := value.(float64)
		return ok
	case "integer":
		number, ok := value.(float64)
		return ok && number == math.Trunc(number)
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "null":
		return value == nil
	case "array":
		_, ok := value.([]any)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	}
	return false
}

// coercePrimitiveByType nudges a primitive toward the requested JSON type. It
// returns the value unchanged when no safe conversion applies.
func coercePrimitiveByType(value any, jsonType string) any {
	switch jsonType {
	case "number":
		if value == nil {
			return float64(0)
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			if parsed, err := strconv.ParseFloat(text, 64); err == nil {
				return parsed
			}
		}
		if flag, ok := value.(bool); ok {
			return boolToFloat(flag)
		}
		return value
	case "integer":
		if value == nil {
			return float64(0)
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			if parsed, err := strconv.ParseFloat(text, 64); err == nil && parsed == math.Trunc(parsed) {
				return parsed
			}
		}
		if flag, ok := value.(bool); ok {
			return boolToFloat(flag)
		}
		return value
	case "boolean":
		if value == nil {
			return false
		}
		if text, ok := value.(string); ok {
			if text == "true" {
				return true
			}
			if text == "false" {
				return false
			}
		}
		if number, ok := value.(float64); ok {
			if number == 1 {
				return true
			}
			if number == 0 {
				return false
			}
		}
		return value
	case "string":
		if value == nil {
			return ""
		}
		switch typed := value.(type) {
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64)
		case bool:
			return strconv.FormatBool(typed)
		}
		return value
	case "null":
		if value == "" || value == float64(0) || value == false {
			return nil
		}
		return value
	}
	return value
}

func boolToFloat(flag bool) float64 {
	if flag {
		return 1
	}
	return 0
}

// coerceWithJSONSchema recursively coerces a value toward a schema: apply
// allOf/anyOf/oneOf, then primitive type coercion, then descend into object
// properties and array items.
func coerceWithJSONSchema(value any, schema map[string]any) any {
	next := value

	for _, nested := range schemaList(schema["allOf"]) {
		next = coerceWithJSONSchema(next, nested)
	}
	if union := schemaList(schema["anyOf"]); len(union) > 0 {
		next = coerceWithUnionSchema(next, union)
	}
	if union := schemaList(schema["oneOf"]); len(union) > 0 {
		next = coerceWithUnionSchema(next, union)
	}

	types := schemaTypes(schema)
	matchesUnionMember := false
	if len(types) > 1 {
		for _, jsonType := range types {
			if matchesJSONType(next, jsonType) {
				matchesUnionMember = true
				break
			}
		}
	}
	// Primitive coercion only applies to scalars; composites (maps, slices) are
	// never changed by coercePrimitiveByType, and comparing them with != would
	// panic on the uncomparable map type.
	if len(types) > 0 && !matchesUnionMember && !isComposite(next) {
		for _, jsonType := range types {
			candidate := coercePrimitiveByType(next, jsonType)
			if candidate != next {
				next = candidate
				break
			}
		}
	}

	if containsString(types, "object") {
		if object, ok := next.(map[string]any); ok {
			applySchemaObjectCoercion(object, schema)
		}
	}
	if containsString(types, "array") {
		if array, ok := next.([]any); ok {
			applySchemaArrayCoercion(array, schema)
		}
	}

	return next
}

func applySchemaObjectCoercion(value map[string]any, schema map[string]any) {
	properties := schemaMap(schema["properties"])
	for key, propertySchema := range properties {
		if _, present := value[key]; !present {
			continue
		}
		value[key] = coerceWithJSONSchema(value[key], propertySchema)
	}

	if additional, ok := schema["additionalProperties"].(map[string]any); ok {
		for key, propertyValue := range value {
			if _, defined := properties[key]; defined {
				continue
			}
			value[key] = coerceWithJSONSchema(propertyValue, additional)
		}
	}
}

func applySchemaArrayCoercion(value []any, schema map[string]any) {
	switch items := schema["items"].(type) {
	case []any:
		for index := range value {
			if index >= len(items) {
				continue
			}
			if itemSchema, ok := items[index].(map[string]any); ok {
				value[index] = coerceWithJSONSchema(value[index], itemSchema)
			}
		}
	case map[string]any:
		for index := range value {
			value[index] = coerceWithJSONSchema(value[index], items)
		}
	}
}

// coerceWithUnionSchema tries each branch and returns the first coerced value
// that validates, falling back to the original value when none do.
func coerceWithUnionSchema(value any, schemas []map[string]any) any {
	for _, schema := range schemas {
		candidate := coerceWithJSONSchema(cloneJSONValue(value), schema)
		if len(validateValue(candidate, schema, "")) == 0 {
			return candidate
		}
	}
	return value
}

// validateValue checks a value against the JSON Schema features commonly used
// by tool definitions and returns one message per problem.
func validateValue(value any, schema map[string]any, path string) []string {
	var problems []string

	types := schemaTypes(schema)
	if len(types) > 0 {
		matched := false
		for _, jsonType := range types {
			if matchesJSONType(value, jsonType) {
				matched = true
				break
			}
		}
		if !matched {
			problems = append(problems, formatProblem(path, fmt.Sprintf("expected %s", strings.Join(types, " or "))))
			// Type mismatch makes deeper checks meaningless.
			return problems
		}
	}

	for _, nested := range schemaList(schema["allOf"]) {
		problems = append(problems, validateValue(value, nested, path)...)
	}
	if variants := schemaList(schema["anyOf"]); len(variants) > 0 {
		matched := false
		for _, nested := range variants {
			if len(validateValue(value, nested, path)) == 0 {
				matched = true
				break
			}
		}
		if !matched {
			problems = append(problems, formatProblem(path, "value does not match any anyOf schema"))
		}
	}
	if variants := schemaList(schema["oneOf"]); len(variants) > 0 {
		matches := 0
		for _, nested := range variants {
			if len(validateValue(value, nested, path)) == 0 {
				matches++
			}
		}
		if matches != 1 {
			problems = append(problems, formatProblem(path, fmt.Sprintf("value must match exactly one oneOf schema (matched %d)", matches)))
		}
	}
	if nested, ok := schema["not"].(map[string]any); ok && len(validateValue(value, nested, path)) == 0 {
		problems = append(problems, formatProblem(path, "value matches a forbidden schema"))
	}

	if constant, present := schema["const"]; present && !reflect.DeepEqual(constant, value) {
		problems = append(problems, formatProblem(path, "value does not match const"))
	}

	if enum, ok := schema["enum"].([]any); ok && len(enum) > 0 {
		if !containsValue(enum, value) {
			problems = append(problems, formatProblem(path, "value is not one of the allowed options"))
		}
	}

	if object, ok := value.(map[string]any); ok {
		for _, required := range stringList(schema["required"]) {
			if _, present := object[required]; !present {
				problems = append(problems, formatProblem(joinPath(path, required), "required property missing"))
			}
		}
		for key, propertySchema := range schemaMap(schema["properties"]) {
			if propertyValue, present := object[key]; present {
				problems = append(problems, validateValue(propertyValue, propertySchema, joinPath(path, key))...)
			}
		}
		properties := schemaMap(schema["properties"])
		for key, propertyValue := range object {
			if _, defined := properties[key]; defined {
				continue
			}
			switch additional := schema["additionalProperties"].(type) {
			case bool:
				if !additional {
					problems = append(problems, formatProblem(joinPath(path, key), "additional property is not allowed"))
				}
			case map[string]any:
				problems = append(problems, validateValue(propertyValue, additional, joinPath(path, key))...)
			}
		}
		if minimum, ok := schemaNumber(schema["minProperties"]); ok && float64(len(object)) < minimum {
			problems = append(problems, formatProblem(path, fmt.Sprintf("must contain at least %v properties", minimum)))
		}
		if maximum, ok := schemaNumber(schema["maxProperties"]); ok && float64(len(object)) > maximum {
			problems = append(problems, formatProblem(path, fmt.Sprintf("must contain at most %v properties", maximum)))
		}
	}

	if array, ok := value.([]any); ok {
		switch items := schema["items"].(type) {
		case map[string]any:
			for index, item := range array {
				problems = append(problems, validateValue(item, items, indexPath(path, index))...)
			}
		case []any:
			for index, item := range array {
				if index >= len(items) {
					break
				}
				if itemSchema, ok := items[index].(map[string]any); ok {
					problems = append(problems, validateValue(item, itemSchema, indexPath(path, index))...)
				}
			}
		}
		if minimum, ok := schemaNumber(schema["minItems"]); ok && float64(len(array)) < minimum {
			problems = append(problems, formatProblem(path, fmt.Sprintf("must contain at least %v items", minimum)))
		}
		if maximum, ok := schemaNumber(schema["maxItems"]); ok && float64(len(array)) > maximum {
			problems = append(problems, formatProblem(path, fmt.Sprintf("must contain at most %v items", maximum)))
		}
		if unique, _ := schema["uniqueItems"].(bool); unique {
			for i := range array {
				for j := 0; j < i; j++ {
					if reflect.DeepEqual(array[i], array[j]) {
						problems = append(problems, formatProblem(indexPath(path, i), "item must be unique"))
						break
					}
				}
			}
		}
	}

	if text, ok := value.(string); ok {
		length := float64(utf8.RuneCountInString(text))
		if minimum, ok := schemaNumber(schema["minLength"]); ok && length < minimum {
			problems = append(problems, formatProblem(path, fmt.Sprintf("length must be at least %v", minimum)))
		}
		if maximum, ok := schemaNumber(schema["maxLength"]); ok && length > maximum {
			problems = append(problems, formatProblem(path, fmt.Sprintf("length must be at most %v", maximum)))
		}
		if pattern, ok := schema["pattern"].(string); ok {
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				problems = append(problems, formatProblem(path, "schema contains an invalid pattern"))
			} else if !compiled.MatchString(text) {
				problems = append(problems, formatProblem(path, "value does not match pattern"))
			}
		}
	}

	if number, ok := value.(float64); ok {
		if minimum, ok := schemaNumber(schema["minimum"]); ok && number < minimum {
			problems = append(problems, formatProblem(path, fmt.Sprintf("must be at least %v", minimum)))
		}
		if maximum, ok := schemaNumber(schema["maximum"]); ok && number > maximum {
			problems = append(problems, formatProblem(path, fmt.Sprintf("must be at most %v", maximum)))
		}
		if minimum, ok := schemaNumber(schema["exclusiveMinimum"]); ok && number <= minimum {
			problems = append(problems, formatProblem(path, fmt.Sprintf("must be greater than %v", minimum)))
		}
		if maximum, ok := schemaNumber(schema["exclusiveMaximum"]); ok && number >= maximum {
			problems = append(problems, formatProblem(path, fmt.Sprintf("must be less than %v", maximum)))
		}
	}

	return problems
}

func indexPath(path string, index int) string {
	return fmt.Sprintf("%s[%d]", path, index)
}

func schemaNumber(value any) (float64, bool) {
	number, ok := value.(float64)
	return number, ok
}

func formatProblem(path, message string) string {
	if path == "" {
		path = "root"
	}
	return fmt.Sprintf("  - %s: %s", path, message)
}

func joinPath(base, key string) string {
	if base == "" {
		return key
	}
	return base + "." + key
}

// schemaList interprets a schema keyword whose value is an array of schemas
// (allOf/anyOf/oneOf).
func schemaList(value any) []map[string]any {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	schemas := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		if schema, ok := entry.(map[string]any); ok {
			schemas = append(schemas, schema)
		}
	}
	return schemas
}

// schemaMap interprets a keyword whose value maps names to sub-schemas
// (properties).
func schemaMap(value any) map[string]map[string]any {
	raw, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	result := make(map[string]map[string]any, len(raw))
	for key, entry := range raw {
		if schema, ok := entry.(map[string]any); ok {
			result[key] = schema
		}
	}
	return result
}

func stringList(value any) []string {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, entry := range raw {
		if name, ok := entry.(string); ok {
			result = append(result, name)
		}
	}
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsValue(values []any, target any) bool {
	for _, value := range values {
		if reflect.DeepEqual(value, target) {
			return true
		}
	}
	return false
}

func isComposite(value any) bool {
	switch value.(type) {
	case map[string]any, []any:
		return true
	}
	return false
}
