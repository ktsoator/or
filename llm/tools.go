package llm

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/invopop/jsonschema"
)

// NewTool creates a tool definition whose parameters are generated from T.
// T must be a struct or pointer to a struct. Fields without json omitempty are
// required, and jsonschema tags can add descriptions, enums, and constraints.
func NewTool[T any](name, description string) (ToolDefinition, error) {
	if strings.TrimSpace(name) == "" {
		return ToolDefinition{}, fmt.Errorf("tool name is empty")
	}

	typeOf := reflect.TypeOf((*T)(nil)).Elem()
	for typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}
	if typeOf.Kind() != reflect.Struct {
		return ToolDefinition{}, fmt.Errorf("tool arguments type %s is not a struct", typeOf)
	}

	reflector := jsonschema.Reflector{
		Anonymous:      true,
		DoNotReference: true,
	}
	schema := reflector.ReflectFromType(typeOf)
	// Tool providers expect the parameter schema itself, not a standalone JSON
	// Schema document. DoNotReference already removes $ref and $defs; clear the
	// remaining document metadata for compatibility with stricter providers.
	schema.Version = ""
	schema.ID = ""
	schema.Ref = ""
	schema.Definitions = nil

	parameters, err := json.Marshal(schema)
	if err != nil {
		return ToolDefinition{}, fmt.Errorf("encode schema for tool %q: %w", name, err)
	}
	return ToolDefinition{
		Name:        name,
		Description: description,
		Parameters:  parameters,
	}, nil
}

// MustTool is NewTool for statically declared tools. It panics when the tool
// name or argument type cannot produce a valid definition.
func MustTool[T any](name, description string) ToolDefinition {
	tool, err := NewTool[T](name, description)
	if err != nil {
		panic(err)
	}
	return tool
}

// DecodeToolCall validates and coerces call arguments with tool's schema, then
// decodes them into T. The original ToolCall is not modified.
func DecodeToolCall[T any](tool ToolDefinition, call ToolCall) (T, error) {
	var result T
	if call.Name != tool.Name {
		return result, fmt.Errorf("tool call name %q does not match definition %q", call.Name, tool.Name)
	}

	arguments, err := ValidateToolArguments(tool, call)
	if err != nil {
		return result, err
	}
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return result, fmt.Errorf("encode validated arguments for tool %q: %w", tool.Name, err)
	}
	if err := json.Unmarshal(encoded, &result); err != nil {
		return result, fmt.Errorf("decode arguments for tool %q: %w", tool.Name, err)
	}
	return result, nil
}
