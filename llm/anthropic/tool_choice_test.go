package anthropic

import (
	"encoding/json"
	"reflect"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/ktsoator/or/llm"
)

func TestApplyToolChoice(t *testing.T) {
	tests := []struct {
		name   string
		choice llm.AnthropicToolChoice
		want   string
	}{
		{name: "auto", choice: llm.AnthropicToolChoiceAuto, want: `{"type":"auto"}`},
		{name: "any", choice: llm.AnthropicToolChoiceAny, want: `{"type":"any"}`},
		{name: "none", choice: llm.AnthropicToolChoiceNone, want: `{"type":"none"}`},
		{name: "named", choice: llm.AnthropicToolChoiceTool{Name: "weather"}, want: `{"type":"tool","name":"weather"}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := sdk.MessageNewParams{}
			applyToolChoice(&params, test.choice)
			assertToolChoiceJSON(t, params.ToolChoice, test.want)
		})
	}
}

func assertToolChoiceJSON(t *testing.T, value any, want string) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal tool choice: %v", err)
	}
	var gotValue, wantValue any
	if err := json.Unmarshal(encoded, &gotValue); err != nil {
		t.Fatalf("decode actual tool choice: %v", err)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("decode expected tool choice: %v", err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("tool choice = %s, want %s", encoded, want)
	}
}
