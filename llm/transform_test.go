package llm

import "testing"

func TestTransformMessagesDowngradesImagesWithoutMutatingHistory(t *testing.T) {
	user := &UserMessage{Content: []UserContent{
		&TextContent{Text: "before"},
		&ImageContent{Data: "one", MIMEType: "image/png"},
		&ImageContent{Data: "two", MIMEType: "image/png"},
		&TextContent{Text: "after"},
	}}
	toolResult := &ToolResultMessage{
		ToolCallID: "call_1",
		ToolName:   "look",
		Content: []ToolResultContent{
			&ImageContent{Data: "tool", MIMEType: "image/png"},
		},
	}

	transformed := TransformMessages([]Message{user, toolResult}, Model{Input: []ModelInput{Text}}, nil)
	gotUser := transformed[0].(*UserMessage)
	if len(gotUser.Content) != 3 {
		t.Fatalf("user content length = %d, want 3", len(gotUser.Content))
	}
	placeholder, ok := gotUser.Content[1].(*TextContent)
	if !ok || placeholder.Text != nonVisionUserImagePlaceholder {
		t.Fatalf("user placeholder = %#v", gotUser.Content[1])
	}
	gotResult := transformed[1].(*ToolResultMessage)
	resultPlaceholder, ok := gotResult.Content[0].(*TextContent)
	if !ok || resultPlaceholder.Text != nonVisionToolImagePlaceholder {
		t.Fatalf("tool placeholder = %#v", gotResult.Content[0])
	}
	if len(user.Content) != 4 {
		t.Fatal("canonical user history was mutated")
	}
	if _, ok := toolResult.Content[0].(*ImageContent); !ok {
		t.Fatal("canonical tool result was mutated")
	}
}

func TestTransformMessagesCleansCrossModelSignaturesAndRemapsToolResult(t *testing.T) {
	assistant := &AssistantMessage{
		Protocol: ProtocolOpenAICompletions,
		Provider: "source",
		Model:    "source-model",
		Content: []AssistantContent{
			&ThinkingContent{Thinking: "reasoning", ThinkingSignature: "thinking-sig"},
			&ThinkingContent{Thinking: "secret", ThinkingSignature: "redacted-sig", Redacted: true},
			&TextContent{Text: "answer", TextSignature: "text-sig"},
			&ToolCall{ID: "source|call", Name: "lookup", Arguments: map[string]any{}, ThoughtSignature: "thought-sig"},
		},
		StopReason: StopReasonToolUse,
	}
	result := &ToolResultMessage{ToolCallID: "source|call", ToolName: "lookup"}
	target := Model{ID: "target-model", Provider: "target", Protocol: ProtocolAnthropicMessages, Input: []ModelInput{Text}}

	transformed := TransformMessages([]Message{assistant, result}, target, func(string) string { return "normalized_call" })
	gotAssistant := transformed[0].(*AssistantMessage)
	if len(gotAssistant.Content) != 3 {
		t.Fatalf("assistant content length = %d, want 3", len(gotAssistant.Content))
	}
	thinkingAsText, ok := gotAssistant.Content[0].(*TextContent)
	if !ok || thinkingAsText.Text != "reasoning" || thinkingAsText.TextSignature != "" {
		t.Fatalf("downgraded thinking = %#v", gotAssistant.Content[0])
	}
	text, ok := gotAssistant.Content[1].(*TextContent)
	if !ok || text.TextSignature != "" {
		t.Fatalf("cross-model text = %#v", gotAssistant.Content[1])
	}
	call, ok := gotAssistant.Content[2].(*ToolCall)
	if !ok || call.ID != "normalized_call" || call.ThoughtSignature != "" {
		t.Fatalf("cross-model tool call = %#v", gotAssistant.Content[2])
	}
	gotResult := transformed[1].(*ToolResultMessage)
	if gotResult.ToolCallID != "normalized_call" {
		t.Fatalf("tool result id = %q", gotResult.ToolCallID)
	}
	if assistant.Content[3].(*ToolCall).ID != "source|call" {
		t.Fatal("canonical assistant history was mutated")
	}
}

func TestTransformMessagesSynthesizesOnlyMissingToolResults(t *testing.T) {
	assistant := &AssistantMessage{
		Content: []AssistantContent{
			&ToolCall{ID: "call_a", Name: "first", Arguments: map[string]any{}},
			&ToolCall{ID: "call_b", Name: "second", Arguments: map[string]any{}},
		},
		StopReason: StopReasonToolUse,
	}
	existing := &ToolResultMessage{ToolCallID: "call_b", ToolName: "second", Content: []ToolResultContent{&TextContent{Text: "ok"}}}
	user := &UserMessage{Content: []UserContent{&TextContent{Text: "continue"}}}

	transformed := TransformMessages([]Message{assistant, existing, user}, Model{Input: []ModelInput{Text}}, nil)
	if len(transformed) != 4 {
		t.Fatalf("message count = %d, want 4", len(transformed))
	}
	synthetic, ok := transformed[2].(*ToolResultMessage)
	if !ok {
		t.Fatalf("synthetic message type = %T", transformed[2])
	}
	if synthetic.ToolCallID != "call_a" || synthetic.ToolName != "first" || !synthetic.IsError {
		t.Fatalf("synthetic result = %#v", synthetic)
	}
	if text := synthetic.Content[0].(*TextContent).Text; text != orphanedToolResultText {
		t.Fatalf("synthetic result text = %q", text)
	}
	keptResult, ok := transformed[1].(*ToolResultMessage)
	if !ok || keptResult.ToolCallID != "call_b" {
		t.Fatalf("existing result position = %#v", transformed[1])
	}
	if _, ok := transformed[3].(*UserMessage); !ok {
		t.Fatalf("user message position = %T", transformed[3])
	}
}

func TestTransformMessagesDropsIncompleteAssistantTurns(t *testing.T) {
	failed := &AssistantMessage{
		Content:    []AssistantContent{&ThinkingContent{Thinking: "partial"}},
		StopReason: StopReasonError,
	}
	user := &UserMessage{Content: []UserContent{&TextContent{Text: "retry"}}}

	transformed := TransformMessages([]Message{failed, user}, Model{Input: []ModelInput{Text}}, nil)
	gotUser, ok := transformed[0].(*UserMessage)
	if len(transformed) != 1 || !ok || gotUser.Content[0].(*TextContent).Text != "retry" {
		t.Fatalf("transformed messages = %#v", transformed)
	}
}
