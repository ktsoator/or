package engine

import (
	"context"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/llm"
)

func TestProjectAgentEventProjectsToolInputLifecycle(t *testing.T) {
	tests := []struct {
		name      string
		llmEvent  llm.Event
		wantType  EventType
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
			if (got.ToolArgs != nil) != tt.wantArgs {
				t.Fatalf("ToolArgs = %#v, want present %v", got.ToolArgs, tt.wantArgs)
			}
		})
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
}
