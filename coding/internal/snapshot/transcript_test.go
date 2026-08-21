package snapshot

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

func TestFromTranscriptReconstructsCommittedProviderExchange(t *testing.T) {
	contextEntry := transcript.NewContext(transcript.ContextAttachment{
		AttachmentID: "base:1:revision", Epoch: 1, Kind: "base",
		Placement: "prefix", Revision: "revision", Rendered: "hidden context",
	})
	requestEntry := transcript.NewRequestHeader(transcript.RequestHeader{
		ProviderRequestID: "request-1",
		RunID:             "run-1", TurnID: "turn-1", StepID: "step-1",
		Provider: "test", Model: "model", Protocol: llm.ProtocolOpenAIResponses,
		SystemPrompt: "system prompt",
		Tools: []llm.ToolDefinition{{
			Name: "read", Description: "read a file",
			Parameters: json.RawMessage(`{"type":"object"}`),
		}},
		Attachments: []transcript.RequestAttachment{{
			AttachmentID: contextEntry.Context.AttachmentID,
			MessageIndex: 0,
		}},
	})
	assistant := llm.NewAssistantMessage(llm.Model{
		Provider: "test", ID: "model", Protocol: llm.ProtocolOpenAIResponses,
	})
	assistant.ProviderRequestID = "request-1"
	assistant.Content = []llm.AssistantContent{
		&llm.ThinkingContent{Thinking: "reason", ThinkingSignature: "private-signature"},
		&llm.TextContent{Text: "answer"},
	}
	assistant.StopReason = llm.StopReasonStop

	entries, err := transcript.SequenceEntries([]transcript.Entry{
		transcript.NewRunStart("run-1"),
		transcript.NewTurnStart("run-1", "turn-1"),
		transcript.NewMessage(agent.UserMessage("question")),
		transcript.NewStepStart("run-1", "turn-1", "step-1"),
		contextEntry,
		requestEntry,
		transcript.NewMessage(agent.FromLLM(&assistant)),
		transcript.NewStepEnd(
			"run-1", "turn-1", "step-1", transcript.LifecycleCompleted, "",
		),
		transcript.NewTurnEnd(
			"run-1", "turn-1", transcript.LifecycleCompleted, "",
		),
		transcript.NewRunEnd("run-1", transcript.LifecycleCompleted, ""),
	}, 0)
	if err != nil {
		t.Fatal(err)
	}

	got, err := FromTranscript("session-1", "request-1", entries)
	if err != nil {
		t.Fatal(err)
	}
	if got.SessionID != "session-1" || got.RunID != "run-1" ||
		got.TurnID != "turn-1" || got.StepID != "step-1" ||
		got.ProviderRequestID != "request-1" || got.Provider != "test" ||
		got.Model != "model" {
		t.Fatalf("snapshot identity = %#v", got)
	}
	if !got.CapturedAt.Equal(entries[5].Timestamp) {
		t.Fatalf("captured at = %v, want %v", got.CapturedAt, entries[5].Timestamp)
	}
	if got.Input.SystemPrompt != "system prompt" || len(got.Input.Tools) != 1 ||
		len(got.Input.Messages) != 2 || got.Input.Messages[0].Content[0].Text != "hidden context" ||
		got.Input.Messages[1].Content[0].Text != "question" {
		t.Fatalf("snapshot input = %#v", got.Input)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].ID != "base:1:revision" ||
		got.Attachments[0].MessageIndex != 0 {
		t.Fatalf("snapshot attachments = %#v", got.Attachments)
	}
	if got.Output == nil || !got.Output.CapturedAt.Equal(entries[6].Timestamp) ||
		got.Output.StopReason != "stop" || len(got.Output.Message.Content) != 2 ||
		got.Output.Message.Content[0].Thinking != "reason" ||
		got.Output.Message.Content[1].Text != "answer" {
		t.Fatalf("snapshot output = %#v", got.Output)
	}
}

func TestFromTranscriptRejectsUnknownOrInvalidRequest(t *testing.T) {
	if _, err := FromTranscript("session-1", "bad/request", nil); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("invalid request error = %v, want ErrInvalidID", err)
	}
	if _, err := FromTranscript("session-1", "request-missing", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing request error = %v, want ErrNotFound", err)
	}
}
