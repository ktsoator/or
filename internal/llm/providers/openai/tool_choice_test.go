package openai

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ktsoator/or/internal/llm"
	oai "github.com/openai/openai-go/v3"
)

func TestApplyToolChoice(t *testing.T) {
	tests := []struct {
		name   string
		choice llm.OpenAIToolChoice
		want   string
	}{
		{name: "auto", choice: llm.OpenAIToolChoiceAuto, want: `"auto"`},
		{name: "none", choice: llm.OpenAIToolChoiceNone, want: `"none"`},
		{name: "required", choice: llm.OpenAIToolChoiceRequired, want: `"required"`},
		{name: "named", choice: llm.OpenAIToolChoiceFunction{Name: "weather"}, want: `{"type":"function","function":{"name":"weather"}}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := oai.ChatCompletionNewParams{}
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
