package responses

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ktsoator/or/llm"
)

func TestResponsesRequestConvertsStatelessHistoryAndOptions(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("request path = %q, want /v1/responses", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &requestBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		writeResponsesSSE(w,
			`{"type":"response.created","response":{"id":"resp_request","model":"test-model","status":"in_progress"}}`,
			`{"type":"response.completed","response":{"id":"resp_request","model":"test-model","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":0,"total_tokens":1,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}`,
		)
	}))
	defer server.Close()

	textSignature := encodeResponsesSignature(responsesSignature{
		Kind: "text", ID: "msg_1", Phase: "final_answer",
	})
	reasoningSignature := encodeResponsesSignature(responsesSignature{
		Kind: "reasoning", ID: "rs_1", EncryptedContent: "encrypted_1",
	})
	temperature := 0.25
	strict := true
	model := responsesTestModel(server.URL + "/v1")
	model.Reasoning = true
	model.Input = []llm.ModelInput{llm.Text, llm.Image}
	input := llm.Context{
		SystemPrompt: "Be concise.",
		Messages: []llm.Message{
			&llm.UserMessage{Content: []llm.UserContent{
				&llm.TextContent{Text: "What is here?"},
				&llm.ImageContent{MIMEType: "image/png", Data: "AAAA"},
			}},
			&llm.AssistantMessage{
				Protocol: llm.ProtocolOpenAIResponses,
				Provider: model.Provider,
				Model:    model.ID,
				Content: []llm.AssistantContent{
					&llm.ThinkingContent{Thinking: "inspect", ThinkingSignature: reasoningSignature},
					&llm.TextContent{Text: "I will check.", TextSignature: textSignature},
					&llm.ToolCall{ID: "call_1|fc_1", Name: "weather", Arguments: map[string]any{"city": "Paris"}},
				},
				StopReason: llm.StopReasonToolUse,
			},
			&llm.ToolResultMessage{
				ToolCallID: "call_1|fc_1",
				ToolName:   "weather",
				Content: []llm.ToolResultContent{
					&llm.TextContent{Text: "sunny"},
					&llm.ImageContent{MIMEType: "image/jpeg", Data: "BBBB"},
				},
			},
		},
		Tools: []llm.ToolDefinition{{
			Name: "weather", Description: "Get weather",
			Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
			Strict:     &strict,
		}},
	}

	events := streamResponsesTest(t, model, input, llm.StreamOptions{
		APIKey:      "test",
		MaxTokens:   512,
		Temperature: &temperature,
		Reasoning:   llm.ModelThinkingHigh,
		ProtocolOptions: &llm.OpenAIResponsesStreamOptions{
			ToolChoice: llm.OpenAIToolChoiceFunction{Name: "weather"},
		},
	})
	assertSingleTerminalEvent(t, events, llm.EventDone)

	if requestBody["model"] != "test-model" || requestBody["instructions"] != "Be concise." {
		t.Fatalf("request identity/instructions = %#v", requestBody)
	}
	if requestBody["stream"] != true || requestBody["store"] != false {
		t.Fatalf("stream/store = %#v/%#v", requestBody["stream"], requestBody["store"])
	}
	if requestBody["max_output_tokens"] != float64(512) || requestBody["temperature"] != 0.25 {
		t.Fatalf("token/temperature options = %#v", requestBody)
	}
	if _, ok := requestBody["previous_response_id"]; ok {
		t.Fatal("stateless request must not send previous_response_id")
	}
	include := requestBody["include"].([]any)
	if len(include) != 1 || include[0] != "reasoning.encrypted_content" {
		t.Fatalf("include = %#v", include)
	}
	reasoning := requestBody["reasoning"].(map[string]any)
	if reasoning["effort"] != "high" || reasoning["summary"] != "auto" {
		t.Fatalf("reasoning = %#v", reasoning)
	}
	toolChoice := requestBody["tool_choice"].(map[string]any)
	if toolChoice["type"] != "function" || toolChoice["name"] != "weather" {
		t.Fatalf("tool_choice = %#v", toolChoice)
	}
	tools := requestBody["tools"].([]any)
	function := tools[0].(map[string]any)
	if function["strict"] != true || function["description"] != "Get weather" {
		t.Fatalf("tool = %#v", function)
	}

	items := requestBody["input"].([]any)
	if len(items) != 5 {
		t.Fatalf("input item count = %d, want 5: %#v", len(items), items)
	}
	user := items[0].(map[string]any)
	userContent := user["content"].([]any)
	if user["role"] != "user" || userContent[1].(map[string]any)["image_url"] != "data:image/png;base64,AAAA" {
		t.Fatalf("user input = %#v", user)
	}
	replayedReasoning := items[1].(map[string]any)
	if replayedReasoning["id"] != "rs_1" || replayedReasoning["encrypted_content"] != "encrypted_1" {
		t.Fatalf("reasoning replay = %#v", replayedReasoning)
	}
	if _, ok := replayedReasoning["summary"]; !ok {
		t.Fatalf("reasoning replay must preserve required summary: %#v", replayedReasoning)
	}
	replayedText := items[2].(map[string]any)
	if replayedText["id"] != "msg_1" || replayedText["phase"] != "final_answer" {
		t.Fatalf("text replay = %#v", replayedText)
	}
	textPart := replayedText["content"].([]any)[0].(map[string]any)
	if _, ok := textPart["annotations"]; !ok {
		t.Fatalf("output text replay must preserve required annotations: %#v", textPart)
	}
	call := items[3].(map[string]any)
	if call["call_id"] != "call_1" || call["id"] != "fc_1" || call["name"] != "weather" {
		t.Fatalf("function call replay = %#v", call)
	}
	result := items[4].(map[string]any)
	if result["call_id"] != "call_1" {
		t.Fatalf("function output replay = %#v", result)
	}
	resultContent := result["output"].([]any)
	if resultContent[1].(map[string]any)["image_url"] != "data:image/jpeg;base64,BBBB" {
		t.Fatalf("function output image = %#v", resultContent)
	}
}

func TestResponsesToolsOmitStrictByDefault(t *testing.T) {
	tools, err := convertResponsesTools([]llm.ToolDefinition{{
		Name:       "weather",
		Parameters: json.RawMessage(`{"type":"object"}`),
	}})
	if err != nil {
		t.Fatalf("convert responses tools: %v", err)
	}
	raw, err := json.Marshal(tools)
	if err != nil {
		t.Fatalf("marshal responses tools: %v", err)
	}
	if strings.Contains(string(raw), `"strict"`) {
		t.Fatalf("default Responses tool must omit strict: %s", raw)
	}
}

func TestResponsesStreamAggregatesTextReasoningToolAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeResponsesSSE(w,
			`{"type":"response.created","response":{"id":"resp_1","model":"test-model-2026","status":"in_progress"}}`,
			`{"type":"response.output_item.added","output_index":0,"item":{"id":"rs_1","type":"reasoning","summary":[],"encrypted_content":"encrypted_1","status":"in_progress"}}`,
			`{"type":"response.reasoning_summary_text.delta","item_id":"rs_1","output_index":0,"summary_index":0,"delta":"plan "}`,
			`{"type":"response.reasoning_summary_text.done","item_id":"rs_1","output_index":0,"summary_index":0,"text":"plan carefully"}`,
			`{"type":"response.output_item.done","output_index":0,"item":{"id":"rs_1","type":"reasoning","summary":[{"type":"summary_text","text":"plan carefully"}],"encrypted_content":"encrypted_1","status":"completed"}}`,
			`{"type":"response.output_item.added","output_index":1,"item":{"id":"msg_1","type":"message","role":"assistant","phase":"final_answer","content":[],"status":"in_progress"}}`,
			`{"type":"response.output_text.delta","item_id":"msg_1","output_index":1,"content_index":0,"delta":"Checking"}`,
			`{"type":"response.output_text.done","item_id":"msg_1","output_index":1,"content_index":0,"text":"Checking"}`,
			`{"type":"response.output_item.done","output_index":1,"item":{"id":"msg_1","type":"message","role":"assistant","phase":"final_answer","content":[{"type":"output_text","text":"Checking"}],"status":"completed"}}`,
			`{"type":"response.output_item.added","output_index":2,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"weather","arguments":"","status":"in_progress"}}`,
			`{"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":2,"delta":"{\"city\":"}`,
			`{"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":2,"delta":"\"Paris\"}"}`,
			`{"type":"response.function_call_arguments.done","item_id":"fc_1","output_index":2,"name":"weather","arguments":"{\"city\":\"Paris\"}"}`,
			`{"type":"response.output_item.done","output_index":2,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"weather","arguments":"{\"city\":\"Paris\"}","status":"completed"}}`,
			`{"type":"response.completed","response":{"id":"resp_1","model":"test-model-2026","status":"completed","output":[{"id":"rs_1","type":"reasoning","summary":[{"type":"summary_text","text":"plan carefully"}],"encrypted_content":"encrypted_1","status":"completed"},{"id":"msg_1","type":"message","role":"assistant","phase":"final_answer","content":[{"type":"output_text","text":"Checking"}],"status":"completed"},{"id":"fc_1","type":"function_call","call_id":"call_1","name":"weather","arguments":"{\"city\":\"Paris\"}","status":"completed"}],"usage":{"input_tokens":12,"output_tokens":5,"total_tokens":17,"input_tokens_details":{"cached_tokens":3,"cache_write_tokens":1},"output_tokens_details":{"reasoning_tokens":2}}}}`,
		)
	}))
	defer server.Close()

	events := streamResponsesTest(t, responsesTestModel(server.URL+"/v1"), responsesTestContext(), llm.StreamOptions{APIKey: "test"})
	assertSingleTerminalEvent(t, events, llm.EventDone)
	assertEventTypes(t, events, []llm.EventType{
		llm.EventStart,
		llm.EventThinkingStart,
		llm.EventThinkingDelta,
		llm.EventThinkingDelta,
		llm.EventThinkingEnd,
		llm.EventTextStart,
		llm.EventTextDelta,
		llm.EventTextEnd,
		llm.EventToolCallStart,
		llm.EventToolCallDelta,
		llm.EventToolCallDelta,
		llm.EventToolCallEnd,
		llm.EventDone,
	})

	message := events[len(events)-1].Message
	if message == nil || message.StopReason != llm.StopReasonToolUse {
		t.Fatalf("terminal message = %#v", message)
	}
	if message.ResponseID != "resp_1" || message.ResponseModel != "test-model-2026" {
		t.Fatalf("response metadata = %#v", message)
	}
	thinking := message.Content[0].(*llm.ThinkingContent)
	if thinking.Thinking != "plan carefully" || thinking.Redacted {
		t.Fatalf("thinking = %#v", thinking)
	}
	thinkingMetadata, ok := decodeResponsesSignature(thinking.ThinkingSignature, "reasoning")
	if !ok || thinkingMetadata.ID != "rs_1" || thinkingMetadata.EncryptedContent != "encrypted_1" {
		t.Fatalf("thinking signature = %#v, ok=%v", thinkingMetadata, ok)
	}
	text := message.Content[1].(*llm.TextContent)
	textMetadata, ok := decodeResponsesSignature(text.TextSignature, "text")
	if text.Text != "Checking" || !ok || textMetadata.ID != "msg_1" || textMetadata.Phase != "final_answer" {
		t.Fatalf("text/signature = %#v %#v", text, textMetadata)
	}
	call := message.Content[2].(*llm.ToolCall)
	if call.ID != "call_1|fc_1" || call.Name != "weather" || call.Arguments["city"] != "Paris" {
		t.Fatalf("tool call = %#v", call)
	}
	usage := message.Usage
	if usage.Input != 8 || usage.CacheRead != 3 || usage.CacheWrite != 1 || usage.Output != 5 || usage.TotalTokens != 17 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestResponsesCompletedEventBackfillsMissingDeltas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeResponsesSSE(w,
			`{"type":"response.completed","response":{"id":"resp_full","model":"test-model","status":"completed","output":[{"id":"rs_full","type":"reasoning","summary":[{"type":"summary_text","text":"full plan"}],"encrypted_content":"encrypted_full","status":"completed"},{"id":"msg_full","type":"message","role":"assistant","phase":"final_answer","content":[{"type":"output_text","text":"full answer"}],"status":"completed"},{"id":"fc_full","type":"function_call","call_id":"call_full","name":"weather","arguments":"{\"city\":\"Paris\"}","status":"completed"}]}}`,
		)
	}))
	defer server.Close()

	events := streamResponsesTest(t, responsesTestModel(server.URL+"/v1"), responsesTestContext(), llm.StreamOptions{APIKey: "test"})
	assertSingleTerminalEvent(t, events, llm.EventDone)
	assertEventTypes(t, events, []llm.EventType{
		llm.EventStart,
		llm.EventThinkingStart,
		llm.EventThinkingDelta,
		llm.EventThinkingEnd,
		llm.EventTextStart,
		llm.EventTextDelta,
		llm.EventTextEnd,
		llm.EventToolCallStart,
		llm.EventToolCallDelta,
		llm.EventToolCallEnd,
		llm.EventDone,
	})
	message := events[len(events)-1].Message
	if message == nil || message.StopReason != llm.StopReasonToolUse {
		t.Fatalf("terminal message = %#v", message)
	}
	if message.Content[0].(*llm.ThinkingContent).Thinking != "full plan" ||
		message.Content[1].(*llm.TextContent).Text != "full answer" ||
		message.Content[2].(*llm.ToolCall).Arguments["city"] != "Paris" {
		t.Fatalf("backfilled content = %#v", message.Content)
	}
}

func TestResponsesThinkingDisplayOmittedSkipsSummaryRequest(t *testing.T) {
	model := responsesTestModel("")
	model.Reasoning = true
	params := buildResponsesParams(model, "", nil, nil, llm.StreamOptions{
		ProtocolOptions: &llm.OpenAIResponsesStreamOptions{
			ThinkingDisplay: llm.ThinkingDisplayOmitted,
		},
	})
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	wire := string(raw)
	if strings.Contains(wire, `"summary"`) {
		t.Fatalf("omitted thinking must not request a summary: %s", wire)
	}
	if !strings.Contains(wire, `"reasoning.encrypted_content"`) {
		t.Fatalf("omitted thinking must preserve encrypted continuity: %s", wire)
	}
}

func TestResponsesReplaysEncryptedReasoningWithoutVisibleSummary(t *testing.T) {
	signature := encodeResponsesSignature(responsesSignature{
		Kind: "reasoning", ID: "rs_redacted", EncryptedContent: "encrypted_redacted",
	})
	items, err := convertResponsesAssistantMessage(&llm.AssistantMessage{
		Content: []llm.AssistantContent{&llm.ThinkingContent{
			Thinking:          "[Reasoning redacted]",
			ThinkingSignature: signature,
			Redacted:          true,
		}},
	})
	if err != nil {
		t.Fatalf("convert assistant message: %v", err)
	}
	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("marshal items: %v", err)
	}
	var wire []map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("decode wire: %v", err)
	}
	if len(wire) != 1 || wire[0]["encrypted_content"] != "encrypted_redacted" {
		t.Fatalf("reasoning replay = %#v", wire)
	}
	summary, ok := wire[0]["summary"].([]any)
	if !ok || len(summary) != 0 {
		t.Fatalf("reasoning summary = %#v, want required empty array", wire[0]["summary"])
	}
}

func TestResponsesStreamTerminalOutcomes(t *testing.T) {
	tests := []struct {
		name       string
		events     []string
		terminal   llm.EventType
		stopReason llm.StopReason
		wantError  string
	}{
		{
			name:     "max output tokens",
			events:   []string{`{"type":"response.incomplete","response":{"id":"resp_length","model":"test-model","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[]}}`},
			terminal: llm.EventDone, stopReason: llm.StopReasonLength,
		},
		{
			name:     "failed",
			events:   []string{`{"type":"response.failed","response":{"id":"resp_failed","model":"test-model","status":"failed","error":{"code":"server_error","message":"backend failed"}}}`},
			terminal: llm.EventError, stopReason: llm.StopReasonError, wantError: "backend failed",
		},
		{
			name:     "stream error",
			events:   []string{`{"type":"error","code":"server_error","message":"stream failed","param":""}`},
			terminal: llm.EventError, stopReason: llm.StopReasonError, wantError: "stream failed",
		},
		{
			name: "refusal",
			events: []string{
				`{"type":"response.refusal.delta","item_id":"msg_refusal","output_index":0,"content_index":0,"delta":"cannot comply"}`,
				`{"type":"response.completed","response":{"id":"resp_refusal","model":"test-model","status":"completed","output":[]}}`,
			},
			terminal: llm.EventError, stopReason: llm.StopReasonError, wantError: "refusal",
		},
		{
			name:     "missing terminal event",
			events:   []string{`{"type":"response.created","response":{"id":"resp_missing","model":"test-model","status":"in_progress"}}`},
			terminal: llm.EventError, stopReason: llm.StopReasonError, wantError: "without a terminal event",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeResponsesSSE(w, test.events...)
			}))
			defer server.Close()
			events := streamResponsesTest(t, responsesTestModel(server.URL+"/v1"), responsesTestContext(), llm.StreamOptions{APIKey: "test"})
			assertSingleTerminalEvent(t, events, test.terminal)
			terminal := events[len(events)-1]
			if terminal.Message == nil || terminal.Message.StopReason != test.stopReason {
				t.Fatalf("terminal message = %#v", terminal.Message)
			}
			if test.wantError != "" && (terminal.Err == nil || !strings.Contains(terminal.Err.Error(), test.wantError)) {
				t.Fatalf("terminal error = %v, want %q", terminal.Err, test.wantError)
			}
		})
	}
}

func TestResponsesMalformedToolArgumentsAreDiagnostic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeResponsesSSE(w,
			`{"type":"response.output_item.added","output_index":0,"item":{"id":"fc_bad","type":"function_call","call_id":"call_bad","name":"weather","arguments":"{\"city\":"}}`,
			`{"type":"response.output_item.done","output_index":0,"item":{"id":"fc_bad","type":"function_call","call_id":"call_bad","name":"weather","arguments":"{\"city\":"}}`,
			`{"type":"response.completed","response":{"id":"resp_bad","model":"test-model","status":"completed","output":[{"id":"fc_bad","type":"function_call","call_id":"call_bad","name":"weather","arguments":"{\"city\":"}]}}`,
		)
	}))
	defer server.Close()

	events := streamResponsesTest(t, responsesTestModel(server.URL+"/v1"), responsesTestContext(), llm.StreamOptions{APIKey: "test"})
	assertSingleTerminalEvent(t, events, llm.EventDone)
	message := events[len(events)-1].Message
	if message == nil || len(message.Diagnostics) != 1 || message.Diagnostics[0].Type != llm.DiagnosticToolArgumentsRecovered {
		t.Fatalf("diagnostics = %#v", message)
	}
}

func TestResponsesToolCallIDNormalizerDropsModelSpecificItemID(t *testing.T) {
	normalize := responsesToolCallIDNormalizer(llm.Model{Provider: "openai"})
	if got := normalize("call/unsafe|fc_model_specific"); got != "call_unsafe" {
		t.Fatalf("normalized tool call ID = %q, want %q", got, "call_unsafe")
	}
}

func writeResponsesSSE(w http.ResponseWriter, events ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, event := range events {
		fmt.Fprintf(w, "data: %s\n\n", event)
	}
}

func streamResponsesTest(
	t *testing.T,
	model llm.Model,
	input llm.Context,
	options llm.StreamOptions,
) []llm.Event {
	t.Helper()
	stream, err := NewAdapter(nil).Stream(context.Background(), model, input, options)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	var events []llm.Event
	for event := range stream {
		events = append(events, event)
	}
	return events
}

func responsesTestModel(baseURL string) llm.Model {
	return llm.Model{
		ID:       "test-model",
		Protocol: llm.ProtocolOpenAIResponses,
		Provider: "openai",
		BaseURL:  baseURL,
		Input:    []llm.ModelInput{llm.Text},
		Cost:     llm.ModelCost{Input: 1, Output: 2, CacheRead: 0.5, CacheWrite: 1.5},
	}
}

func responsesTestContext() llm.Context {
	return llm.Context{Messages: []llm.Message{&llm.UserMessage{
		Content: []llm.UserContent{&llm.TextContent{Text: "hello"}},
	}}}
}

func assertEventTypes(t *testing.T, events []llm.Event, want []llm.EventType) {
	t.Helper()
	got := make([]llm.EventType, len(events))
	for i := range events {
		got[i] = events[i].Type
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("event types = %v, want %v", got, want)
	}
}

func assertSingleTerminalEvent(t *testing.T, events []llm.Event, want llm.EventType) {
	t.Helper()
	terminals := 0
	for _, event := range events {
		if event.Type == llm.EventDone || event.Type == llm.EventError {
			terminals++
			if event.Type != want {
				t.Fatalf("terminal event = %q, want %q", event.Type, want)
			}
		}
	}
	if terminals != 1 {
		t.Fatalf("terminal event count = %d, want 1", terminals)
	}
	if len(events) == 0 || events[len(events)-1].Type != want {
		t.Fatalf("last event = %#v, want terminal %q", events, want)
	}
}
