package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateToolArgumentsCoercesAndValidatesConstraints(t *testing.T) {
	tool := ToolDefinition{
		Name: "schedule",
		Parameters: json.RawMessage(`{
			"type":"object",
			"required":["count","code"],
			"additionalProperties":false,
			"properties":{
				"count":{"type":"integer","minimum":1},
				"code":{"type":"string","pattern":"^[A-Z]{2}$"}
			}
		}`),
	}

	arguments, err := ValidateToolArguments(tool, ToolCall{
		Name:      tool.Name,
		Arguments: map[string]any{"count": "3", "code": "CN"},
	})
	if err != nil {
		t.Fatalf("ValidateToolArguments() error = %v", err)
	}
	if got := arguments["count"]; got != float64(3) {
		t.Fatalf("coerced count = %#v, want float64(3)", got)
	}

	_, err = ValidateToolArguments(tool, ToolCall{
		Name:      tool.Name,
		Arguments: map[string]any{"count": float64(0), "code": "china", "extra": true},
	})
	if err == nil {
		t.Fatal("ValidateToolArguments() accepted arguments that violate constraints")
	}
	for _, want := range []string{"count", "code", "extra"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("validation error %q does not mention %q", err, want)
		}
	}
}

func TestValidateToolArgumentsEnforcesComposition(t *testing.T) {
	tests := []struct {
		name        string
		valueSchema string
		value       any
	}{
		{name: "anyOf", valueSchema: `{"anyOf":[{"type":"string","pattern":"^ok$"},{"type":"integer","minimum":10}]}`, value: "no"},
		{name: "oneOf", valueSchema: `{"oneOf":[{"type":"number"},{"type":"integer"}]}`, value: float64(3)},
		{name: "allOf", valueSchema: `{"allOf":[{"type":"number","minimum":1},{"maximum":5}]}`, value: float64(8)},
		{name: "not", valueSchema: `{"not":{"const":"forbidden"}}`, value: "forbidden"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema := `{"type":"object","required":["value"],"properties":{"value":` + test.valueSchema + `}}`
			_, err := ValidateToolArguments(ToolDefinition{
				Name:       test.name,
				Parameters: json.RawMessage(schema),
			}, ToolCall{Name: test.name, Arguments: map[string]any{"value": test.value}})
			if err == nil {
				t.Fatal("ValidateToolArguments() accepted invalid composition")
			}
		})
	}
}

func TestValidateToolArgumentsRejectsMalformedSchema(t *testing.T) {
	_, err := ValidateToolArguments(
		ToolDefinition{Name: "broken", Parameters: json.RawMessage(`{"type":`)},
		ToolCall{Name: "broken", Arguments: map[string]any{}},
	)
	if err == nil || !strings.Contains(err.Error(), "invalid schema") {
		t.Fatalf("ValidateToolArguments() error = %v, want invalid schema error", err)
	}
}
