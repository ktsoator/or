package openai

import (
	"encoding/json"
	"strings"
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

// A tool call carrying encrypted reasoning on its thought signature is replayed
// as a reasoning_details array entry, so the provider can continue the prior
// reasoning across the tool-use loop.
func TestConvertMessagesReplaysEncryptedReasoningDetails(t *testing.T) {
	model := openAIReplayModel()
	signature := `{"type":"reasoning.encrypted","id":"call_1","data":"ENC"}`
	input := llm.Context{Messages: []llm.Message{
		&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "weather in Paris?"}}},
		&llm.AssistantMessage{
			Provider:   model.Provider,
			Protocol:   model.Protocol,
			Model:      model.ID,
			StopReason: llm.StopReasonToolUse,
			Content: []llm.AssistantContent{
				&llm.ToolCall{ID: "call_1", Name: "weather", Arguments: map[string]any{"city": "Paris"}, ThoughtSignature: signature},
			},
		},
		&llm.ToolResultMessage{
			ToolCallID: "call_1",
			ToolName:   "weather",
			Content:    []llm.ToolResultContent{&llm.TextContent{Text: "sunny"}},
		},
		&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "thanks"}}},
	}}
	assistant := assistantWire(t, input, model, resolvedCompat{})

	details, ok := assistant["reasoning_details"].([]any)
	if !ok || len(details) != 1 {
		t.Fatalf("reasoning_details = %#v, want one entry", assistant["reasoning_details"])
	}
	entry, ok := details[0].(map[string]any)
	if !ok || entry["type"] != "reasoning.encrypted" || entry["data"] != "ENC" {
		t.Fatalf("reasoning_details[0] = %#v", details[0])
	}
}

// Crossing models, the thought signature has been cleared upstream, so no
// encrypted reasoning is replayed to a model that cannot decrypt it.
func TestConvertMessagesDropsEncryptedReasoningCrossModel(t *testing.T) {
	source := openAIReplayModel()
	signature := `{"type":"reasoning.encrypted","id":"call_1","data":"ENC"}`
	input := llm.Context{Messages: []llm.Message{
		&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "weather in Paris?"}}},
		&llm.AssistantMessage{
			Provider:   source.Provider,
			Protocol:   source.Protocol,
			Model:      source.ID,
			StopReason: llm.StopReasonToolUse,
			Content: []llm.AssistantContent{
				&llm.ToolCall{ID: "call_1", Name: "weather", Arguments: map[string]any{"city": "Paris"}, ThoughtSignature: signature},
			},
		},
		&llm.ToolResultMessage{
			ToolCallID: "call_1",
			ToolName:   "weather",
			Content:    []llm.ToolResultContent{&llm.TextContent{Text: "sunny"}},
		},
		&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "thanks"}}},
	}}
	// A different model id makes TransformMessages treat the turn as cross-model.
	target := source
	target.ID = "other-model"
	assistant := assistantWire(t, input, target, resolvedCompat{})

	if _, present := assistant["reasoning_details"]; present {
		t.Fatalf("reasoning_details must be absent cross-model: %#v", assistant)
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

// A reasoning-only model with requiresThinkingAsText turns the leading thinking
// block into a text content part instead of a reasoning field, for endpoints
// that reject reasoning fields on input.
func TestConvertMessagesRendersThinkingAsTextWhenRequired(t *testing.T) {
	model := openAIReplayModel()
	input := llm.Context{Messages: []llm.Message{
		&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "weather?"}}},
		&llm.AssistantMessage{
			Provider:   model.Provider,
			Protocol:   model.Protocol,
			Model:      model.ID,
			StopReason: llm.StopReasonStop,
			Content: []llm.AssistantContent{
				&llm.ThinkingContent{Thinking: "let me think", ThinkingSignature: "reasoning_content"},
				&llm.TextContent{Text: "it is sunny"},
			},
		},
	}}
	assistant := assistantWire(t, input, model, resolvedCompat{requiresThinkingAsText: true})

	parts, ok := assistant["content"].([]any)
	if !ok || len(parts) != 2 {
		t.Fatalf("content = %#v, want two text parts", assistant["content"])
	}
	first, _ := parts[0].(map[string]any)
	second, _ := parts[1].(map[string]any)
	if first["text"] != "let me think" {
		t.Fatalf("first part = %#v, want thinking", first)
	}
	if second["text"] != "it is sunny" {
		t.Fatalf("second part = %#v, want answer", second)
	}
	if _, present := assistant["reasoning_content"]; present {
		t.Fatalf("reasoning_content must not be set when rendered as text")
	}
}

// An assistant message with neither text nor tool calls is omitted entirely:
// some providers reject empty assistant messages.
func TestConvertMessagesSkipsEmptyAssistant(t *testing.T) {
	model := openAIReplayModel()
	input := llm.Context{Messages: []llm.Message{
		&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "hi"}}},
		&llm.AssistantMessage{
			Provider: model.Provider,
			Protocol: model.Protocol,
			Model:    model.ID,
			Content:  []llm.AssistantContent{},
		},
		&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "again"}}},
	}}
	messages, err := convertMessages(input, model, resolvedCompat{})
	if err != nil {
		t.Fatalf("convertMessages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("got %d messages, want 2 (empty assistant skipped)", len(messages))
	}
}

func TestConvertMessagesEmitsDeveloperSystemForReasoningModel(t *testing.T) {
	// With developer role support and a reasoning model, the system prompt is
	// sent as a developer message instead of system.
	model := openAIReplayModel()
	input := llm.Context{
		SystemPrompt: "be terse",
		Messages: []llm.Message{
			&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "hi"}}},
		},
	}
	messages, err := convertMessages(input, model, resolvedCompat{supportsDeveloperRole: true})
	if err != nil {
		t.Fatalf("convertMessages: %v", err)
	}
	raw, _ := json.Marshal(messages)
	var decoded []map[string]any
	_ = json.Unmarshal(raw, &decoded)
	if decoded[0]["role"] != "developer" {
		t.Fatalf("first message role = %v, want developer", decoded[0]["role"])
	}
}

func TestConvertMessagesEmitsSystemPromptForNonReasoningModel(t *testing.T) {
	// A non-reasoning model gets the plain system role regardless of developer
	// role support.
	model := openAIReplayModel()
	model.Reasoning = false
	input := llm.Context{
		SystemPrompt: "be terse",
		Messages: []llm.Message{
			&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "hi"}}},
		},
	}
	messages, err := convertMessages(input, model, resolvedCompat{supportsDeveloperRole: true})
	if err != nil {
		t.Fatalf("convertMessages: %v", err)
	}
	raw, _ := json.Marshal(messages)
	var decoded []map[string]any
	_ = json.Unmarshal(raw, &decoded)
	if decoded[0]["role"] != "system" {
		t.Fatalf("first message role = %v, want system", decoded[0]["role"])
	}
}

func TestConvertUserMessageSingleTextUsesStringContent(t *testing.T) {
	// A single text block is sent as a bare string content, not a multipart
	// array, so the wire stays compact and providers that reject arrays for
	// trivial text messages still accept it.
	got, err := convertUserMessage(&llm.UserMessage{
		Content: []llm.UserContent{&llm.TextContent{Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("convertUserMessage: %v", err)
	}
	raw, _ := json.Marshal(got)
	if !strings.Contains(string(raw), `"content":"hello"`) {
		t.Fatalf("expected bare-string content, got %s", raw)
	}
}

func TestConvertUserMessageMultipartIncludesImage(t *testing.T) {
	got, err := convertUserMessage(&llm.UserMessage{
		Content: []llm.UserContent{
			&llm.TextContent{Text: "describe this"},
			&llm.ImageContent{MIMEType: "image/png", Data: "AAAA"},
		},
	})
	if err != nil {
		t.Fatalf("convertUserMessage: %v", err)
	}
	raw, _ := json.Marshal(got)
	if !strings.Contains(string(raw), `data:image/png;base64,AAAA`) {
		t.Fatalf("multipart did not embed image: %s", raw)
	}
	if !strings.Contains(string(raw), `"text":"describe this"`) {
		t.Fatalf("multipart did not preserve text: %s", raw)
	}
}

func TestConvertUserMessageReturnsNilForEmpty(t *testing.T) {
	// An empty multipart message produces nil so the caller skips it instead of
	// sending a content-less user turn.
	got, err := convertUserMessage(&llm.UserMessage{Content: []llm.UserContent{}})
	if err != nil {
		t.Fatalf("convertUserMessage: %v", err)
	}
	if got != nil {
		t.Fatalf("empty content = %#v, want nil", got)
	}
}

func TestConvertUserMessageRejectsNilTextContent(t *testing.T) {
	_, err := convertUserMessage(&llm.UserMessage{
		Content: []llm.UserContent{nil, &llm.TextContent{Text: "ok"}},
	})
	if err == nil {
		t.Fatalf("expected error for nil text content")
	}
}

func TestConvertImageContentErrors(t *testing.T) {
	tests := []struct {
		name    string
		content *llm.ImageContent
	}{
		{name: "nil content", content: nil},
		{name: "missing mime", content: &llm.ImageContent{Data: "AAAA"}},
		{name: "missing data", content: &llm.ImageContent{MIMEType: "image/png"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := convertImageContent(test.content); err == nil {
				t.Fatalf("expected error for %s", test.name)
			}
		})
	}
}

func TestConvertImageContentSuccess(t *testing.T) {
	part, err := convertImageContent(&llm.ImageContent{MIMEType: "image/jpeg", Data: "XYZ"})
	if err != nil {
		t.Fatalf("convertImageContent: %v", err)
	}
	raw, _ := json.Marshal(part)
	if !strings.Contains(string(raw), `data:image/jpeg;base64,XYZ`) {
		t.Fatalf("image url not embedded: %s", raw)
	}
}

func TestConvertToolResultMessageRequiresToolCallID(t *testing.T) {
	if _, _, err := convertToolResultMessage(nil); err == nil {
		t.Fatalf("nil tool result must error")
	}
	if _, _, err := convertToolResultMessage(&llm.ToolResultMessage{}); err == nil {
		t.Fatalf("missing tool call id must error")
	}
}

func TestConvertToolResultMessageWithImagesProducesFollowupUserMessage(t *testing.T) {
	// A tool result carrying images is split into a tool message (text only) plus
	// a follow-up user message containing the images, since the tool role does
	// not accept image content parts on the OpenAI protocol.
	model := openAIReplayModel()
	model.Input = []llm.ModelInput{llm.Text, llm.Image}
	input := llm.Context{Messages: []llm.Message{
		&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "weather"}}},
		&llm.AssistantMessage{
			Provider:   model.Provider,
			Protocol:   model.Protocol,
			Model:      model.ID,
			StopReason: llm.StopReasonToolUse,
			Content: []llm.AssistantContent{
				&llm.ToolCall{ID: "call_1", Name: "snap", Arguments: map[string]any{}},
			},
		},
		&llm.ToolResultMessage{
			ToolCallID: "call_1", ToolName: "snap",
			Content: []llm.ToolResultContent{
				&llm.TextContent{Text: "here is a picture"},
				&llm.ImageContent{MIMEType: "image/png", Data: "AAAA"},
			},
		},
	}}
	messages, err := convertMessages(input, model, resolvedCompat{})
	if err != nil {
		t.Fatalf("convertMessages: %v", err)
	}
	raw, _ := json.Marshal(messages)
	var decoded []map[string]any
	_ = json.Unmarshal(raw, &decoded)

	// Layout: user, assistant, tool, user(image follow-up).
	if len(decoded) != 4 {
		t.Fatalf("got %d messages, want 4: %s", len(decoded), raw)
	}
	if decoded[2]["role"] != "tool" {
		t.Fatalf("third message role = %v, want tool", decoded[2]["role"])
	}
	if decoded[3]["role"] != "user" {
		t.Fatalf("fourth message role = %v, want user (image follow-up)", decoded[3]["role"])
	}
	if !strings.Contains(string(raw), `data:image/png;base64,AAAA`) {
		t.Fatalf("image not present in follow-up: %s", raw)
	}
	if !strings.Contains(string(raw), "Attached image(s) from tool result:") {
		t.Fatalf("image follow-up missing intro line: %s", raw)
	}
}

func TestConvertToolResultMessageImageOnlyFillsPlaceholderText(t *testing.T) {
	// When a tool returns only images, the tool message itself needs a string
	// body since the protocol requires one; the placeholder makes that explicit.
	msg, images, err := convertToolResultMessage(&llm.ToolResultMessage{
		ToolCallID: "call_1",
		Content:    []llm.ToolResultContent{&llm.ImageContent{MIMEType: "image/png", Data: "AAAA"}},
	})
	if err != nil {
		t.Fatalf("convertToolResultMessage: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("expected one image, got %d", len(images))
	}
	raw, _ := json.Marshal(msg)
	if !strings.Contains(string(raw), "(see attached image)") {
		t.Fatalf("placeholder text missing: %s", raw)
	}
}

func TestConvertTools(t *testing.T) {
	tools := []llm.ToolDefinition{
		{Name: "noop"},
		{Name: "weather", Description: "look up the weather", Parameters: json.RawMessage(`{"type":"object"}`)},
	}
	got, err := convertTools(tools, resolvedCompat{supportsStrictMode: true})
	if err != nil {
		t.Fatalf("convertTools: %v", err)
	}
	raw, _ := json.Marshal(got)
	wire := string(raw)
	if !strings.Contains(wire, `"name":"noop"`) || !strings.Contains(wire, `"name":"weather"`) {
		t.Fatalf("tool names missing: %s", wire)
	}
	// Strict mode is advertised but defaulted to false (matching pi), so the wire
	// must include the field with the value false.
	if !strings.Contains(wire, `"strict":false`) {
		t.Fatalf("expected strict=false in wire: %s", wire)
	}
	if !strings.Contains(wire, `"description":"look up the weather"`) {
		t.Fatalf("description missing for weather: %s", wire)
	}
}

func TestConvertToolsOmitsStrictWhenUnsupported(t *testing.T) {
	got, err := convertTools(
		[]llm.ToolDefinition{{Name: "noop"}},
		resolvedCompat{supportsStrictMode: false},
	)
	if err != nil {
		t.Fatalf("convertTools: %v", err)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), `"strict"`) {
		t.Fatalf("strict must be omitted: %s", raw)
	}
}

func TestConvertToolsRejectsEmpty(t *testing.T) {
	if got, err := convertTools(nil, resolvedCompat{}); err != nil || got != nil {
		t.Fatalf("nil tools = %v %v, want (nil, nil)", got, err)
	}
}

func TestConvertToolsRejectsMissingName(t *testing.T) {
	if _, err := convertTools([]llm.ToolDefinition{{Description: "no name"}}, resolvedCompat{}); err == nil {
		t.Fatalf("missing tool name must error")
	}
}

func TestConvertToolsRejectsBadParameters(t *testing.T) {
	if _, err := convertTools(
		[]llm.ToolDefinition{{Name: "bad", Parameters: json.RawMessage("not-json")}},
		resolvedCompat{},
	); err == nil {
		t.Fatalf("invalid parameters JSON must error")
	}
}

func TestEncodeToolArguments(t *testing.T) {
	if got, err := encodeToolArguments(nil); err != nil || got != "{}" {
		t.Fatalf("nil = %q %v, want {}", got, err)
	}
	got, err := encodeToolArguments(map[string]any{"a": 1})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if got != `{"a":1}` {
		t.Fatalf("encoded = %q, want {\"a\":1}", got)
	}
}

func TestToolCallIDNormalizerOpenAITruncatesLongIDs(t *testing.T) {
	model := llm.Model{Provider: "openai"}
	long := "call_" + strings.Repeat("a", 60)
	got := toolCallIDNormalizer(model)(long)
	if len(got) != 40 {
		t.Fatalf("truncated len = %d, want 40", len(got))
	}
	if got != long[:40] {
		t.Fatalf("truncation mismatch: got %q want %q", got, long[:40])
	}
}

func TestToolCallIDNormalizerOtherProviderPassesThrough(t *testing.T) {
	model := llm.Model{Provider: "deepseek"}
	id := "call_" + strings.Repeat("a", 60)
	if got := toolCallIDNormalizer(model)(id); got != id {
		t.Fatalf("non-openai provider must keep id unchanged: got %q", got)
	}
}

func TestToolCallIDNormalizerPipeSplitsAndSanitizes(t *testing.T) {
	// Responses-API style pipe-separated ids collapse to the sanitized call_id
	// prefix and get truncated; non-allowed characters become underscores.
	model := llm.Model{Provider: "deepseek"}
	id := "call/123|response-12345"
	got := toolCallIDNormalizer(model)(id)
	if got != "call_123" {
		t.Fatalf("normalized = %q, want call_123", got)
	}
}

func TestSanitizeToolCallID(t *testing.T) {
	// Allowed: a-zA-Z0-9_-; everything else becomes underscore.
	if got := sanitizeToolCallID("call/123 abc#xyz"); got != "call_123_abc_xyz" {
		t.Fatalf("sanitize = %q", got)
	}
	if got := sanitizeToolCallID("call_-1A"); got != "call_-1A" {
		t.Fatalf("allowed chars rewritten: %q", got)
	}
}

func TestTruncateASCII(t *testing.T) {
	if got := truncateASCII("short", 40); got != "short" {
		t.Fatalf("short string mutated: %q", got)
	}
	long := strings.Repeat("x", 60)
	if got := truncateASCII(long, 40); len(got) != 40 || got != long[:40] {
		t.Fatalf("truncate produced %q", got)
	}
}

func TestConvertAssistantMessageRejectsNilContent(t *testing.T) {
	// Each typed nil content branch must surface an explicit error; otherwise
	// callers might silently drop nil entries.
	model := openAIReplayModel()
	cases := []struct {
		name string
		msg  *llm.AssistantMessage
	}{
		{
			name: "text",
			msg: &llm.AssistantMessage{
				Content: []llm.AssistantContent{(*llm.TextContent)(nil)},
			},
		},
		{
			name: "thinking",
			msg: &llm.AssistantMessage{
				Content: []llm.AssistantContent{(*llm.ThinkingContent)(nil)},
			},
		},
		{
			name: "tool call",
			msg: &llm.AssistantMessage{
				Content: []llm.AssistantContent{(*llm.ToolCall)(nil)},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := convertAssistantMessage(c.msg, model, resolvedCompat{}); err == nil {
				t.Fatalf("expected error for nil %s content", c.name)
			}
		})
	}
}
