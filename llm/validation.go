package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ValidateToolCall finds the tool named by the call and validates its arguments.
// It is a utility callers may invoke before dispatching a tool, not something the
// library calls itself. It returns the coerced arguments.
func ValidateToolCall(tools []ToolDefinition, toolCall ToolCall) (map[string]any, error) {
	for _, tool := range tools {
		if tool.Name == toolCall.Name {
			return ValidateToolArguments(tool, toolCall)
		}
	}
	return nil, fmt.Errorf("tool %q not found", toolCall.Name)
}

// ValidateToolArguments coerces a tool call's arguments toward the tool's JSON
// Schema (forgiving common model mistakes such as "3" for a number), then
// validates them. It returns the coerced arguments, or a detailed error naming
// the failing fields. The original toolCall.Arguments are left unchanged.
//
// The coercion is a best-effort pass toward the schema. Validation covers the
// JSON Schema features normally emitted for tool definitions, including
// composition keywords and object, array, string, and numeric constraints.
func ValidateToolArguments(tool ToolDefinition, toolCall ToolCall) (map[string]any, error) {
	schema, err := parseSchema(tool.Parameters)
	if err != nil {
		return nil, fmt.Errorf("invalid schema for tool %q: %w", tool.Name, err)
	}
	if schema == nil {
		// No schema to validate against; return arguments unchanged.
		return toolCall.Arguments, nil
	}

	coerced := coerceWithJSONSchema(cloneJSONValue(toolCall.Arguments), schema)
	arguments, _ := coerced.(map[string]any)
	if arguments == nil {
		arguments = map[string]any{}
	}

	if problems := validateValue(coerced, schema, ""); len(problems) > 0 {
		received, _ := json.MarshalIndent(toolCall.Arguments, "", "  ")
		return nil, fmt.Errorf(
			"validation failed for tool %q:\n%s\n\nReceived arguments:\n%s",
			toolCall.Name,
			strings.Join(problems, "\n"),
			received,
		)
	}
	return arguments, nil
}
