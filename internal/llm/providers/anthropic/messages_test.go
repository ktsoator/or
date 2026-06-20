package anthropic

import (
	"testing"

	"github.com/ktsoator/or/internal/llm"
)

// toolUseHistory is a multi-turn tool-use transcript whose assistant turn
// carries a reasoning block (with the supplied signature) ahead of the tool
// call, followed by the tool result and a final user turn. The assistant turn
// is tagged with the target model so TransformMessages treats it as same-model
// and preserves reasoning.
func toolUseHistory(model llm.Model, signature string) llm.Context {
	return llm.Context{
		Messages: []llm.Message{
			&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "weather in Paris?"}}},
			&llm.AssistantMessage{
				Provider:   model.Provider,
				Protocol:   model.Protocol,
				Model:      model.ID,
				StopReason: llm.StopReasonToolUse,
				Content: []llm.AssistantContent{
					&llm.ThinkingContent{Thinking: "plan", ThinkingSignature: signature},
					&llm.ToolCall{ID: "toolu_1", Name: "weather", Arguments: map[string]any{"city": "Paris"}},
				},
			},
			&llm.ToolResultMessage{
				ToolCallID: "toolu_1",
				ToolName:   "weather",
				Content:    []llm.ToolResultContent{&llm.TextContent{Text: "sunny"}},
			},
			&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "thanks"}}},
		},
	}
}

func anthropicReplayModel() llm.Model {
	return llm.Model{
		ID:        "test-model",
		Protocol:  llm.ProtocolAnthropicMessages,
		Provider:  "test",
		Reasoning: true,
		Input:     []llm.ModelInput{llm.Text},
		MaxTokens: 128,
	}
}

// A signed reasoning block must be replayed as a thinking block that keeps its
// signature and sits before the tool_use block, the order the Messages API
// requires when extended thinking is enabled.
func TestConvertMessagesReplaysSignedThinkingBeforeToolUse(t *testing.T) {
	model := anthropicReplayModel()
	messages, err := convertMessages(toolUseHistory(model, "sig_1"), model, compat{})
	if err != nil {
		t.Fatalf("convertMessages() error = %v", err)
	}

	// user, assistant, tool result (user), user
	if len(messages) != 4 {
		t.Fatalf("message count = %d, want 4", len(messages))
	}
	blocks := messages[1].Content
	if len(blocks) != 2 {
		t.Fatalf("assistant block count = %d, want 2 (thinking, tool_use)", len(blocks))
	}
	thinking := blocks[0].OfThinking
	if thinking == nil {
		t.Fatalf("first assistant block = %#v, want thinking", blocks[0])
	}
	if thinking.Thinking != "plan" || thinking.Signature != "sig_1" {
		t.Fatalf("thinking block = %#v, want thinking=plan signature=sig_1", thinking)
	}
	if blocks[1].OfToolUse == nil || blocks[1].OfToolUse.ID != "toolu_1" {
		t.Fatalf("second assistant block = %#v, want tool_use toolu_1", blocks[1])
	}
}

// With allowEmptySignature opted in, an unsigned reasoning block is still
// replayed as a thinking block (empty signature) rather than downgraded.
func TestConvertMessagesEmptySignatureKeepsThinkingWhenAllowed(t *testing.T) {
	model := anthropicReplayModel()
	messages, err := convertMessages(toolUseHistory(model, ""), model, compat{allowEmptySignature: true})
	if err != nil {
		t.Fatalf("convertMessages() error = %v", err)
	}

	blocks := messages[1].Content
	thinking := blocks[0].OfThinking
	if thinking == nil || thinking.Thinking != "plan" || thinking.Signature != "" {
		t.Fatalf("first assistant block = %#v, want empty-signature thinking", blocks[0])
	}
	if blocks[1].OfToolUse == nil {
		t.Fatalf("second assistant block = %#v, want tool_use", blocks[1])
	}
}

// Interleaved thinking is validated per turn: every assistant turn's signed
// thinking block must survive replay, not just the most recent one. A two-round
// tool-use transcript must therefore emit all three signatures, each ahead of
// its turn's tool_use or text.
func TestConvertMessagesPreservesThinkingAcrossMultipleToolTurns(t *testing.T) {
	model := anthropicReplayModel()
	assistant := func(thinking, signature string, tail llm.AssistantContent) *llm.AssistantMessage {
		return &llm.AssistantMessage{
			Provider:   model.Provider,
			Protocol:   model.Protocol,
			Model:      model.ID,
			StopReason: llm.StopReasonToolUse,
			Content: []llm.AssistantContent{
				&llm.ThinkingContent{Thinking: thinking, ThinkingSignature: signature},
				tail,
			},
		}
	}
	toolResult := func(id, text string) *llm.ToolResultMessage {
		return &llm.ToolResultMessage{
			ToolCallID: id,
			ToolName:   "weather",
			Content:    []llm.ToolResultContent{&llm.TextContent{Text: text}},
		}
	}
	input := llm.Context{Messages: []llm.Message{
		&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "weather in Paris and London?"}}},
		assistant("checking Paris", "sig_1", &llm.ToolCall{ID: "call_1", Name: "weather", Arguments: map[string]any{"city": "Paris"}}),
		toolResult("call_1", "sunny"),
		assistant("now London", "sig_2", &llm.ToolCall{ID: "call_2", Name: "weather", Arguments: map[string]any{"city": "London"}}),
		toolResult("call_2", "rainy"),
		assistant("summarize", "sig_3", &llm.TextContent{Text: "Paris sunny, London rainy"}),
	}}

	messages, err := convertMessages(input, model, compat{})
	if err != nil {
		t.Fatalf("convertMessages() error = %v", err)
	}

	// user, assistant, tool result, assistant, tool result, assistant
	if len(messages) != 6 {
		t.Fatalf("message count = %d, want 6", len(messages))
	}
	wantSignatures := map[int]string{1: "sig_1", 3: "sig_2", 5: "sig_3"}
	for index, want := range wantSignatures {
		thinking := messages[index].Content[0].OfThinking
		if thinking == nil {
			t.Fatalf("message[%d] first block = %#v, want thinking", index, messages[index].Content[0])
		}
		if thinking.Signature != want {
			t.Fatalf("message[%d] signature = %q, want %q", index, thinking.Signature, want)
		}
	}
}

// Redacted reasoning is an opaque payload only the original model can read, so a
// same-model replay must forward it as a redacted_thinking block carrying that
// payload, not as text.
func TestConvertMessagesReplaysRedactedThinking(t *testing.T) {
	model := anthropicReplayModel()
	input := llm.Context{Messages: []llm.Message{
		&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "hi"}}},
		&llm.AssistantMessage{
			Provider: model.Provider,
			Protocol: model.Protocol,
			Model:    model.ID,
			Content: []llm.AssistantContent{
				&llm.ThinkingContent{Thinking: "[Reasoning redacted]", ThinkingSignature: "encrypted", Redacted: true},
				&llm.TextContent{Text: "answer"},
			},
		},
	}}

	messages, err := convertMessages(input, model, compat{})
	if err != nil {
		t.Fatalf("convertMessages() error = %v", err)
	}
	redacted := messages[1].Content[0].OfRedactedThinking
	if redacted == nil || redacted.Data != "encrypted" {
		t.Fatalf("first assistant block = %#v, want redacted_thinking data=encrypted", messages[1].Content[0])
	}
}

// Latent risk: without allowEmptySignature, an unsigned reasoning block in a
// tool-use turn is downgraded to a text block, so the thinking block the
// Messages API requires before tool_use disappears. This test pins the current
// behavior; if extended thinking is enabled on such a turn the provider rejects
// it, so the fix belongs at replay time, not here.
func TestConvertMessagesEmptySignatureDowngradesToTextInToolUseTurn(t *testing.T) {
	model := anthropicReplayModel()
	messages, err := convertMessages(toolUseHistory(model, ""), model, compat{})
	if err != nil {
		t.Fatalf("convertMessages() error = %v", err)
	}

	blocks := messages[1].Content
	if blocks[0].OfThinking != nil {
		t.Fatalf("first assistant block = %#v, want text (not thinking) under current behavior", blocks[0])
	}
	text := blocks[0].OfText
	if text == nil || text.Text != "plan" {
		t.Fatalf("first assistant block = %#v, want text=plan", blocks[0])
	}
	if blocks[1].OfToolUse == nil {
		t.Fatalf("second assistant block = %#v, want tool_use", blocks[1])
	}
}
