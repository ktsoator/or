package openai

import (
	"encoding/json"
	"testing"

	"github.com/ktsoator/or/internal/llm"
)

// toolUseHistory is a multi-turn tool-use transcript whose assistant turn
// carries a reasoning block (whose signature names the source field) ahead of
// the tool call, then the tool result and a final user turn. The assistant turn
// is tagged with the target model so TransformMessages keeps the reasoning.
func toolUseHistory(model llm.Model, signature, thinking string) llm.Context {
	content := []llm.AssistantContent{}
	if thinking != "" {
		content = append(content, &llm.ThinkingContent{Thinking: thinking, ThinkingSignature: signature})
	}
	content = append(content, &llm.ToolCall{ID: "call_1", Name: "weather", Arguments: map[string]any{"city": "Paris"}})

	return llm.Context{
		Messages: []llm.Message{
			&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "weather in Paris?"}}},
			&llm.AssistantMessage{
				Provider:   model.Provider,
				Protocol:   model.Protocol,
				Model:      model.ID,
				StopReason: llm.StopReasonToolUse,
				Content:    content,
			},
			&llm.ToolResultMessage{
				ToolCallID: "call_1",
				ToolName:   "weather",
				Content:    []llm.ToolResultContent{&llm.TextContent{Text: "sunny"}},
			},
			&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "thanks"}}},
		},
	}
}

func openAIReplayModel() llm.Model {
	return llm.Model{
		ID:        "test-model",
		Protocol:  llm.ProtocolOpenAICompletions,
		Provider:  "test",
		Reasoning: true,
		Input:     []llm.ModelInput{llm.Text},
	}
}

// assistantWire marshals the converted transcript and returns the first assistant
// message as a decoded JSON object so tests can assert on the wire fields,
// including the non-standard reasoning fields written via SetExtraFields.
func assistantWire(t *testing.T, input llm.Context, model llm.Model, compat resolvedCompat) map[string]any {
	t.Helper()
	messages, err := convertMessages(input, model, compat)
	if err != nil {
		t.Fatalf("convertMessages() error = %v", err)
	}
	raw, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal messages: %v", err)
	}
	for _, message := range decoded {
		if message["role"] == "assistant" {
			return message
		}
	}
	t.Fatalf("no assistant message in %s", raw)
	return nil
}

// A reasoning block is replayed under the field its signature recorded, sitting
// alongside the tool call in the same assistant turn.
func TestConvertMessagesReplaysReasoningUnderSourceField(t *testing.T) {
	model := openAIReplayModel()
	assistant := assistantWire(t, toolUseHistory(model, "reasoning_content", "plan"), model, resolvedCompat{})

	if got := assistant["reasoning_content"]; got != "plan" {
		t.Fatalf("reasoning_content = %#v, want plan", got)
	}
	calls, ok := assistant["tool_calls"].([]any)
	if !ok || len(calls) != 1 {
		t.Fatalf("tool_calls = %#v, want one call", assistant["tool_calls"])
	}
}

// When the source field is "reasoning", it is replayed under "reasoning" (not
// rewritten), so a provider that streams and accepts the same field round-trips.
func TestConvertMessagesReplaysReasoningFieldVerbatim(t *testing.T) {
	model := openAIReplayModel()
	assistant := assistantWire(t, toolUseHistory(model, "reasoning", "plan"), model, resolvedCompat{})

	if got := assistant["reasoning"]; got != "plan" {
		t.Fatalf("reasoning = %#v, want plan", got)
	}
	if _, present := assistant["reasoning_content"]; present {
		t.Fatalf("reasoning_content must not be set when source field is reasoning: %#v", assistant)
	}
}

// With requiresReasoningContentOnAssistantMessages, a reasoning-capable model
// gets an empty reasoning_content injected even on a turn that carried no
// reasoning, so the provider does not reject the replayed assistant message.
func TestConvertMessagesInjectsEmptyReasoningContentWhenRequired(t *testing.T) {
	model := openAIReplayModel()
	compat := resolvedCompat{requiresReasoningContentOnAssistantMessages: true}
	assistant := assistantWire(t, toolUseHistory(model, "", ""), model, compat)

	value, present := assistant["reasoning_content"]
	if !present || value != "" {
		t.Fatalf("reasoning_content = %#v (present=%v), want empty string", value, present)
	}
}
