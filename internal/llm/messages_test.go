package llm

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// fullContext returns a Context that exercises every message and content
// variant the wire schema supports, so round-tripping it covers every encoder
// and decoder branch.
func fullContext() Context {
	return Context{
		SystemPrompt: "you are a helpful assistant",
		Messages: []Message{
			&UserMessage{Content: []UserContent{
				&TextContent{Text: "hi", TextSignature: "sig"},
				&ImageContent{Data: "AAAA", MIMEType: "image/png"},
			}},
			&AssistantMessage{
				Content: []AssistantContent{
					&ThinkingContent{Thinking: "let me think", ThinkingSignature: "ts", Redacted: true},
					&TextContent{Text: "hello"},
					&ToolCall{
						ID:               "call_1",
						Name:             "weather",
						Arguments:        map[string]any{"city": "Tokyo", "units": "c"},
						ThoughtSignature: "thought",
					},
				},
				Protocol:      ProtocolAnthropicMessages,
				Provider:      "anthropic",
				Model:         "claude-opus-4-8",
				ResponseModel: "claude-opus-4-8-2026-01-01",
				ResponseID:    "resp_123",
				Usage: Usage{
					Input: 10, Output: 20, CacheRead: 1, CacheWrite: 2,
					TotalTokens: 33,
					Cost:        UsageCost{Input: 0.1, Output: 0.2, Total: 0.3},
				},
				StopReason:   StopReasonToolUse,
				ErrorMessage: "",
				Diagnostics: []Diagnostic{{
					Type: DiagnosticToolArgumentsRecovered, Timestamp: 1, Message: "repaired",
					Details: map[string]any{"toolCallId": "call_1"},
				}},
				Timestamp: 1700000000000,
			},
			&ToolResultMessage{
				ToolCallID: "call_1",
				ToolName:   "weather",
				Content: []ToolResultContent{
					&TextContent{Text: "sunny"},
					&ImageContent{Data: "BBBB", MIMEType: "image/jpeg"},
				},
				IsError: false,
			},
		},
		Tools: []ToolDefinition{{
			Name:        "weather",
			Description: "Look up weather",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		}},
	}
}

func TestContextRoundTripPreservesEveryVariant(t *testing.T) {
	original := fullContext()
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal Context: %v", err)
	}

	var decoded Context
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal Context: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round-trip mismatch:\n  got  = %#v\n  want = %#v", decoded, original)
	}

	// Re-encoding must be byte-identical: catches accidental drift between the
	// marshaler and unmarshaler (e.g. a field renamed on only one side).
	reencoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("re-Marshal Context: %v", err)
	}
	if !reflect.DeepEqual(json.RawMessage(encoded), json.RawMessage(reencoded)) {
		t.Fatalf("re-encode drifted:\n  first  = %s\n  second = %s", encoded, reencoded)
	}
}

func TestToolCallMarshalNilArgumentsEncodesAsEmptyObject(t *testing.T) {
	call := ToolCall{ID: "x", Name: "noop", Arguments: nil}
	data, err := json.Marshal(call)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(data), `"arguments":{}`) {
		t.Fatalf("encoded = %s, want arguments:{}", data)
	}
}

func TestToolCallUnmarshalMissingArgumentsBecomesEmptyMap(t *testing.T) {
	// A wire payload that omits arguments must decode to an allocated empty map
	// when round-tripped through unmarshalContent, so callers can dereference
	// without nil checks.
	raw := json.RawMessage(`{"type":"toolCall","id":"x","name":"noop"}`)
	got, err := unmarshalContent(raw)
	if err != nil {
		t.Fatalf("unmarshalContent: %v", err)
	}
	call, ok := got.(*ToolCall)
	if !ok {
		t.Fatalf("type = %T, want *ToolCall", got)
	}
	if call.Arguments == nil {
		t.Fatalf("Arguments = nil, want empty non-nil map")
	}
	if len(call.Arguments) != 0 {
		t.Fatalf("Arguments = %v, want empty", call.Arguments)
	}
}

func TestUnmarshalUserMessageRejectsWrongRole(t *testing.T) {
	raw := []byte(`{"role":"assistant","content":[]}`)
	var msg UserMessage
	err := json.Unmarshal(raw, &msg)
	if err == nil || !strings.Contains(err.Error(), "expected role") {
		t.Fatalf("error = %v, want role mismatch", err)
	}
}

func TestUnmarshalAssistantMessageRejectsWrongRole(t *testing.T) {
	raw := []byte(`{"role":"user","content":[]}`)
	var msg AssistantMessage
	err := json.Unmarshal(raw, &msg)
	if err == nil || !strings.Contains(err.Error(), "expected role") {
		t.Fatalf("error = %v, want role mismatch", err)
	}
}

func TestUnmarshalToolResultMessageRejectsWrongRole(t *testing.T) {
	raw := []byte(`{"role":"user","content":[]}`)
	var msg ToolResultMessage
	err := json.Unmarshal(raw, &msg)
	if err == nil || !strings.Contains(err.Error(), "expected role") {
		t.Fatalf("error = %v, want role mismatch", err)
	}
}

func TestUnmarshalMessageDispatchesByRole(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want any
	}{
		{
			name: "user",
			raw:  `{"role":"user","content":[{"type":"text","text":"hi"}]}`,
			want: &UserMessage{},
		},
		{
			name: "assistant",
			raw:  `{"role":"assistant","content":[],"protocol":"anthropic-messages","provider":"anthropic","model":"x","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"stop","timestamp":0}`,
			want: &AssistantMessage{},
		},
		{
			name: "tool result",
			raw:  `{"role":"toolResult","toolCallId":"x","toolName":"y","content":[],"isError":false}`,
			want: &ToolResultMessage{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := unmarshalMessage(json.RawMessage(tc.raw))
			if err != nil {
				t.Fatalf("unmarshalMessage: %v", err)
			}
			if reflect.TypeOf(got) != reflect.TypeOf(tc.want) {
				t.Fatalf("type = %T, want %T", got, tc.want)
			}
		})
	}
}

func TestUnmarshalMessageRejectsBadHeaders(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"null", `null`, "null"},
		{"missing role", `{"content":[]}`, "role is missing"},
		{"unknown role", `{"role":"system"}`, "unknown message role"},
		{"bad json", `{`, "decode message header"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := unmarshalMessage(json.RawMessage(tc.raw))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestUnmarshalContentRejectsBadHeaders(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"null", `null`, "null"},
		{"missing type", `{"text":"hi"}`, "type is missing"},
		{"unknown type", `{"type":"audio"}`, "unknown content type"},
		{"bad json", `{`, "decode content header"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := unmarshalContent(json.RawMessage(tc.raw))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestUnmarshalUserContentRejectsDisallowedTypes(t *testing.T) {
	// A thinking block is valid content but not allowed in a user message.
	raw := []byte(`{"role":"user","content":[{"type":"thinking","thinking":"x"}]}`)
	var msg UserMessage
	err := json.Unmarshal(raw, &msg)
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("error = %v, want disallowed content", err)
	}
}

func TestUnmarshalAssistantContentRejectsDisallowedTypes(t *testing.T) {
	raw := []byte(`{"role":"assistant","content":[{"type":"image","data":"x","mimeType":"image/png"}],"protocol":"anthropic-messages","provider":"a","model":"m","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"stop","timestamp":0}`)
	var msg AssistantMessage
	err := json.Unmarshal(raw, &msg)
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("error = %v, want disallowed content", err)
	}
}

func TestUnmarshalToolResultContentRejectsDisallowedTypes(t *testing.T) {
	raw := []byte(`{"role":"toolResult","toolCallId":"x","toolName":"y","content":[{"type":"toolCall","id":"a","name":"b","arguments":{}}],"isError":false}`)
	var msg ToolResultMessage
	err := json.Unmarshal(raw, &msg)
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("error = %v, want disallowed content", err)
	}
}

func TestMarshalMessageRejectsUnknownAndNil(t *testing.T) {
	cases := []struct {
		name string
		msg  Message
		want string
	}{
		{"nil user", (*UserMessage)(nil), "user message is nil"},
		{"nil assistant", (*AssistantMessage)(nil), "assistant message is nil"},
		{"nil tool result", (*ToolResultMessage)(nil), "tool result message is nil"},
		{"unknown type", unknownMessage{}, "unsupported message type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := marshalMessage(tc.msg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

type unknownMessage struct{}

func (unknownMessage) isMessage() {}

func TestMarshalContentRejectsUnknownAndNil(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil text", (*TextContent)(nil), "text content is nil"},
		{"nil thinking", (*ThinkingContent)(nil), "thinking content is nil"},
		{"nil image", (*ImageContent)(nil), "image content is nil"},
		{"nil tool call", (*ToolCall)(nil), "tool call content is nil"},
		{"unknown", struct{}{}, "unsupported content type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := marshalContent(tc.in)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestContextEncodingShapeMatchesSchema(t *testing.T) {
	// Lightweight smoke test that the on-wire field names are stable; if any of
	// these change, persisted histories from older versions will break.
	input := Context{
		SystemPrompt: "sp",
		Messages: []Message{&UserMessage{Content: []UserContent{
			&TextContent{Text: "hi"},
		}}},
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, fragment := range []string{
		`"systemPrompt":"sp"`,
		`"messages":[`,
		`"role":"user"`,
		`"type":"text"`,
	} {
		if !strings.Contains(string(data), fragment) {
			t.Fatalf("encoded = %s, missing %q", data, fragment)
		}
	}
}

func TestUnmarshalContextRejectsBadMessage(t *testing.T) {
	raw := []byte(`{"messages":[{"role":"nope"}]}`)
	var ctx Context
	err := json.Unmarshal(raw, &ctx)
	if err == nil || !strings.Contains(err.Error(), "decode message 0") {
		t.Fatalf("error = %v, want decode message 0 error", err)
	}
}

func TestNilReceiverUnmarshalReturnsError(t *testing.T) {
	cases := []struct {
		name string
		call func() error
	}{
		{"user", func() error { return (*UserMessage)(nil).UnmarshalJSON([]byte(`{}`)) }},
		{"assistant", func() error { return (*AssistantMessage)(nil).UnmarshalJSON([]byte(`{}`)) }},
		{"tool result", func() error { return (*ToolResultMessage)(nil).UnmarshalJSON([]byte(`{}`)) }},
		{"context", func() error { return (*Context)(nil).UnmarshalJSON([]byte(`{}`)) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil {
				t.Fatalf("error = nil, want nil receiver error")
			}
		})
	}
}
