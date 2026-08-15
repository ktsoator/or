package engine

import (
	"context"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/llm"
)

func TestAddUsagePreservesUnknownInput(t *testing.T) {
	total := llm.Usage{Input: 2, Output: 3, TotalTokens: 5}
	addUsage(&total, llm.Usage{InputUnknown: true, Output: 4, TotalTokens: 4})

	if !total.InputUnknown || total.Input != 2 || total.Output != 7 || total.TotalTokens != 9 {
		t.Fatalf("usage = %#v, want partial input retained and marked unknown", total)
	}
}

func TestProjectAgentEventProjectsToolInputLifecycle(t *testing.T) {
	tests := []struct {
		name      string
		llmEvent  llm.Event
		wantType  EventType
		wantDelta string
		wantBytes int
		wantArgs  bool
	}{
		{
			name: "start",
			llmEvent: llm.Event{
				Type:         llm.EventToolCallStart,
				ContentIndex: 2,
				ToolCall:     &llm.ToolCall{ID: "call-1", Name: "write"},
			},
			wantType: ToolInputStarted,
		},
		{
			name: "delta counts utf8 bytes",
			llmEvent: llm.Event{
				Type:         llm.EventToolCallDelta,
				ContentIndex: 2,
				Delta:        "\u4f60a",
				ToolCall:     &llm.ToolCall{ID: "call-1", Name: "write"},
			},
			wantType:  ToolInputDelta,
			wantDelta: "\u4f60a",
			wantBytes: 4,
		},
		{
			name: "end includes parsed arguments",
			llmEvent: llm.Event{
				Type:         llm.EventToolCallEnd,
				ContentIndex: 2,
				ToolCall: &llm.ToolCall{
					ID:        "call-1",
					Name:      "write",
					Arguments: map[string]any{"path": "main.go", "content": "package main\n"},
				},
			},
			wantType: ToolInputCompleted,
			wantArgs: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := projectAgentEvent(agent.AgentEvent{
				Type:     agent.MessageUpdate,
				LLMEvent: &tt.llmEvent,
			})
			if !ok {
				t.Fatal("event was not projected")
			}
			if got.Type != tt.wantType || got.ToolContentIndex != 2 {
				t.Fatalf("event = %#v", got)
			}
			if got.ToolCallID != "call-1" || got.ToolName != "write" {
				t.Fatalf("tool identity = %q/%q", got.ToolCallID, got.ToolName)
			}
			if got.ToolInputBytes != tt.wantBytes {
				t.Fatalf("ToolInputBytes = %d, want %d", got.ToolInputBytes, tt.wantBytes)
			}
			if got.Delta != tt.wantDelta {
				t.Fatalf("Delta = %q, want %q", got.Delta, tt.wantDelta)
			}
			if (got.ToolArgs != nil) != tt.wantArgs {
				t.Fatalf("ToolArgs = %#v, want present %v", got.ToolArgs, tt.wantArgs)
			}
		})
	}
}

func TestProjectAgentEventPreservesToolResultImages(t *testing.T) {
	event, ok := projectAgentEvent(agent.AgentEvent{
		Type:       agent.ToolEnd,
		ToolCallID: "image-call",
		ToolName:   "mcp__everything__get_tiny_image",
		Result: agent.ToolResult{Content: []llm.ToolResultContent{
			&llm.TextContent{Text: "tiny image"},
			&llm.ImageContent{Data: "aW1hZ2U=", MIMEType: "image/png"},
		}},
	})
	if !ok {
		t.Fatal("tool result event was not projected")
	}
	if event.ToolResult != "tiny image" || len(event.Images) != 1 {
		t.Fatalf("event = %#v", event)
	}
	if event.Images[0].Data != "aW1hZ2U=" || event.Images[0].MIMEType != "image/png" {
		t.Fatalf("images = %#v", event.Images)
	}
}

func TestProjectHistoryPreservesToolResultImages(t *testing.T) {
	items := projectHistory([]agent.AgentMessage{agent.FromLLM(&llm.ToolResultMessage{
		ToolCallID: "image-call",
		ToolName:   "mcp__everything__get_tiny_image",
		Content: []llm.ToolResultContent{
			&llm.ImageContent{Data: "aW1hZ2U=", MIMEType: "image/png"},
		},
	})}, nil)
	if len(items) != 1 || items[0].Type != HistoryToolResult || len(items[0].Images) != 1 {
		t.Fatalf("history = %#v", items)
	}
	if items[0].Images[0].Data != "aW1hZ2U=" || items[0].Images[0].MIMEType != "image/png" {
		t.Fatalf("images = %#v", items[0].Images)
	}
}

func TestSessionProjectsQueuedUserMessageHandle(t *testing.T) {
	session, err := New(context.Background(), Options{
		Model:    llm.Model{Provider: "test", ID: "model"},
		Tools:    []tools.Tool{},
		StreamFn: fixedResponse("answer"),
	})
	if err != nil {
		t.Fatal(err)
	}

	handle := session.FollowUp("same content")
	var userEvents []Event
	session.Subscribe(func(event Event) {
		if event.Type == UserMessageCompleted {
			userEvents = append(userEvents, event)
		}
	})
	if err := session.Prompt(context.Background(), "same content"); err != nil {
		t.Fatal(err)
	}

	if len(userEvents) != 2 {
		t.Fatalf("user completion events = %#v, want initial and queued messages", userEvents)
	}
	if userEvents[0].QueueHandle != (QueueHandle{}) {
		t.Fatalf("initial prompt handle = %#v, want zero", userEvents[0].QueueHandle)
	}
	if userEvents[1].QueueHandle != handle {
		t.Fatalf("queued prompt handle = %#v, want %#v", userEvents[1].QueueHandle, handle)
	}
	for index, event := range userEvents {
		if event.SentAt.IsZero() {
			t.Fatalf("user completion event %d has no sent time: %#v", index, event)
		}
	}
}

func TestSessionCorrelatesVisibleEventsWithProviderRequest(t *testing.T) {
	stream := func(
		_ context.Context,
		model llm.Model,
		_ llm.Context,
		_ llm.StreamOptions,
	) (<-chan llm.Event, error) {
		partial := llm.NewAssistantMessage(model)
		partial.Content = []llm.AssistantContent{&llm.TextContent{Text: "answer"}}
		message := llm.NewAssistantMessage(model)
		message.Content = []llm.AssistantContent{&llm.TextContent{Text: "answer"}}
		message.StopReason = llm.StopReasonStop
		events := make(chan llm.Event, 2)
		events <- llm.Event{Type: llm.EventTextDelta, Delta: "answer", Partial: &partial}
		events <- llm.Event{Type: llm.EventDone, Message: &message}
		close(events)
		return events, nil
	}
	session, err := New(context.Background(), Options{
		Model: llm.Model{Provider: "test", ID: "model"}, Tools: []tools.Tool{}, StreamFn: stream,
	})
	if err != nil {
		t.Fatal(err)
	}

	var requestIDs []string
	session.Subscribe(func(event Event) {
		if event.Type == TextDelta || event.Type == MessageCompleted {
			requestIDs = append(requestIDs, event.ProviderRequestID)
		}
	})
	if err := session.Prompt(context.Background(), "question"); err != nil {
		t.Fatal(err)
	}
	if len(requestIDs) != 2 || requestIDs[0] == "" || requestIDs[0] != requestIDs[1] {
		t.Fatalf("provider request IDs = %#v, want one stable non-empty ID", requestIDs)
	}
}
