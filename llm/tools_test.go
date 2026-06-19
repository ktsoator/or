package llm_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ktsoator/or/llm"
)

type coordinates struct {
	Latitude  float64 `json:"latitude" jsonschema:"minimum=-90,maximum=90"`
	Longitude float64 `json:"longitude" jsonschema:"minimum=-180,maximum=180"`
}

type weatherArgs struct {
	City        string       `json:"city" jsonschema:"description=City name,minLength=1"`
	Units       string       `json:"units,omitempty" jsonschema:"enum=celsius,enum=fahrenheit"`
	Coordinates *coordinates `json:"coordinates,omitempty"`
	Days        int          `json:"days" jsonschema:"minimum=1,maximum=10"`
}

func TestNewToolGeneratesInlineProviderSchema(t *testing.T) {
	tool, err := llm.NewTool[weatherArgs]("get_weather", "Get a weather forecast")
	if err != nil {
		t.Fatalf("NewTool() error = %v", err)
	}
	if tool.Name != "get_weather" || tool.Description != "Get a weather forecast" {
		t.Fatalf("tool metadata = %#v", tool)
	}

	var schema map[string]any
	if err := json.Unmarshal(tool.Parameters, &schema); err != nil {
		t.Fatalf("decode generated schema: %v", err)
	}
	for _, forbidden := range []string{"$schema", "$id", "$ref", "$defs"} {
		if strings.Contains(string(tool.Parameters), `"`+forbidden+`"`) {
			t.Errorf("generated schema contains %s: %s", forbidden, tool.Parameters)
		}
	}
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("root schema = %s", tool.Parameters)
	}
	required := stringSet(schema["required"])
	if !required["city"] || !required["days"] || required["units"] || required["coordinates"] {
		t.Fatalf("required fields = %#v", schema["required"])
	}
	properties := schema["properties"].(map[string]any)
	units := properties["units"].(map[string]any)
	if got := stringSet(units["enum"]); !got["celsius"] || !got["fahrenheit"] {
		t.Fatalf("units enum = %#v", units["enum"])
	}
	coordinatesSchema := properties["coordinates"].(map[string]any)
	if _, hasRef := coordinatesSchema["$ref"]; hasRef {
		t.Fatalf("nested schema contains ref: %#v", coordinatesSchema)
	}
}

func TestDecodeToolCallValidatesCoercesAndDecodes(t *testing.T) {
	tool := llm.MustTool[weatherArgs]("get_weather", "Get a weather forecast")
	call := llm.ToolCall{
		Name: "get_weather",
		Arguments: map[string]any{
			"city":  "Paris",
			"units": "celsius",
			"days":  "3",
		},
	}

	arguments, err := llm.DecodeToolCall[weatherArgs](tool, call)
	if err != nil {
		t.Fatalf("DecodeToolCall() error = %v", err)
	}
	if arguments.City != "Paris" || arguments.Units != "celsius" || arguments.Days != 3 {
		t.Fatalf("decoded arguments = %#v", arguments)
	}
	if call.Arguments["days"] != "3" {
		t.Fatal("DecodeToolCall() mutated the original tool call")
	}
}

func TestDecodeToolCallRejectsInvalidArguments(t *testing.T) {
	tool := llm.MustTool[weatherArgs]("get_weather", "Get a weather forecast")
	tests := []llm.ToolCall{
		{Name: "other", Arguments: map[string]any{"city": "Paris", "days": float64(3)}},
		{Name: "get_weather", Arguments: map[string]any{"city": "", "days": float64(0)}},
		{Name: "get_weather", Arguments: map[string]any{"city": "Paris", "days": float64(3), "extra": true}},
	}
	for _, call := range tests {
		if _, err := llm.DecodeToolCall[weatherArgs](tool, call); err == nil {
			t.Fatalf("DecodeToolCall() accepted %#v", call)
		}
	}
}

func TestNewToolRejectsInvalidDefinition(t *testing.T) {
	if _, err := llm.NewTool[string]("not_struct", ""); err == nil {
		t.Fatal("NewTool() accepted a non-struct argument type")
	}
	if _, err := llm.NewTool[weatherArgs](" ", ""); err == nil {
		t.Fatal("NewTool() accepted an empty name")
	}
}

func stringSet(value any) map[string]bool {
	result := make(map[string]bool)
	for _, item := range value.([]any) {
		result[item.(string)] = true
	}
	return result
}
