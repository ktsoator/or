package llm_test

import (
	"reflect"
	"testing"

	"github.com/ktsoator/or/llm"
)

func TestUserTextMatchesLonghand(t *testing.T) {
	got := llm.UserText("hello")
	want := &llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "hello"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("UserText = %#v, want %#v", got, want)
	}
}

func TestAssistantTextMatchesLonghand(t *testing.T) {
	got := llm.AssistantText("hi there")
	want := &llm.AssistantMessage{Content: []llm.AssistantContent{&llm.TextContent{Text: "hi there"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AssistantText = %#v, want %#v", got, want)
	}
}

func TestUserImageMatchesLonghand(t *testing.T) {
	got := llm.UserImage("base64data", "image/png")
	want := &llm.UserMessage{Content: []llm.UserContent{
		&llm.ImageContent{Data: "base64data", MIMEType: "image/png"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("UserImage = %#v, want %#v", got, want)
	}
}

func TestToolResultMatchesLonghand(t *testing.T) {
	got := llm.ToolResult("call_1", "get_weather", "sunny")
	want := &llm.ToolResultMessage{
		ToolCallID: "call_1",
		ToolName:   "get_weather",
		Content:    []llm.ToolResultContent{&llm.TextContent{Text: "sunny"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ToolResult = %#v, want %#v", got, want)
	}
}

func TestNewContextHoldsMessagesInOrder(t *testing.T) {
	got := llm.NewContext(llm.UserText("a"), llm.AssistantText("b"))
	want := llm.Context{Messages: []llm.Message{llm.UserText("a"), llm.AssistantText("b")}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NewContext = %#v, want %#v", got, want)
	}
}

func TestPromptMatchesLonghand(t *testing.T) {
	got := llm.Prompt("explain channels")
	want := llm.Context{Messages: []llm.Message{
		&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "explain channels"}}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Prompt = %#v, want %#v", got, want)
	}
}

func TestPromptWithSystemSetsSystemPrompt(t *testing.T) {
	got := llm.PromptWithSystem("be brief", "hi")
	if got.SystemPrompt != "be brief" {
		t.Fatalf("SystemPrompt = %q, want %q", got.SystemPrompt, "be brief")
	}
	want := llm.Prompt("hi").Messages
	if !reflect.DeepEqual(got.Messages, want) {
		t.Fatalf("messages = %#v, want %#v", got.Messages, want)
	}
}

func TestAssistantMessageText(t *testing.T) {
	message := &llm.AssistantMessage{Content: []llm.AssistantContent{
		&llm.TextContent{Text: "Hello, "},
		&llm.ThinkingContent{Thinking: "ignored"},
		&llm.TextContent{Text: "world"},
		&llm.ToolCall{ID: "1", Name: "x"},
	}}
	if got := message.Text(); got != "Hello, world" {
		t.Fatalf("Text() = %q, want %q", got, "Hello, world")
	}
}

func TestAssistantMessageTextEmpty(t *testing.T) {
	message := &llm.AssistantMessage{Content: []llm.AssistantContent{
		&llm.ToolCall{ID: "1", Name: "x"},
	}}
	if got := message.Text(); got != "" {
		t.Fatalf("Text() = %q, want empty", got)
	}
	var nilMessage *llm.AssistantMessage
	if got := nilMessage.Text(); got != "" {
		t.Fatalf("nil Text() = %q, want empty", got)
	}
}

func TestAssistantMessageToolCalls(t *testing.T) {
	message := &llm.AssistantMessage{Content: []llm.AssistantContent{
		&llm.TextContent{Text: "let me check"},
		&llm.ToolCall{ID: "a", Name: "first"},
		&llm.ToolCall{ID: "b", Name: "second"},
	}}
	got := message.ToolCalls()
	want := []llm.ToolCall{{ID: "a", Name: "first"}, {ID: "b", Name: "second"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ToolCalls() = %#v, want %#v", got, want)
	}
}

func TestAssistantMessageToolCallsNoneReturnsNil(t *testing.T) {
	message := &llm.AssistantMessage{Content: []llm.AssistantContent{&llm.TextContent{Text: "no tools"}}}
	if got := message.ToolCalls(); got != nil {
		t.Fatalf("ToolCalls() = %#v, want nil", got)
	}
}
