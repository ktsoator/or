package transcript

import (
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

func TestBuildContextUsesLatestCompactionAndKeepsOriginalMessages(t *testing.T) {
	entries := linearMessages(
		agent.UserMessage("old user"),
		agent.FromLLM(assistant("old assistant")),
		agent.UserMessage("kept user"),
		agent.FromLLM(assistant("kept assistant")),
	)
	compact := NewCompaction(Compaction{
		Summary:          "old work summarized",
		FirstKeptEntryID: entries[2].ID,
		TokensBefore:     100,
		TokensAfter:      40,
	})
	entries = append(entries, compact)
	last := NewMessage(agent.UserMessage("new user"))
	entries = append(entries, last)

	context, err := BuildContext(entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(context) != 4 {
		t.Fatalf("context length = %d, want 4", len(context))
	}
	if text := messageText(t, context[0]); !strings.Contains(text, "old work summarized") {
		t.Fatalf("summary message = %q", text)
	}
	if got := messageText(t, context[1]); got != "kept user" {
		t.Fatalf("first kept message = %q", got)
	}
	if got := messageText(t, context[3]); got != "new user" {
		t.Fatalf("new message = %q", got)
	}

	full := Messages(entries)
	if len(full) != 5 {
		t.Fatalf("full message length = %d, want 5", len(full))
	}
	if got := messageText(t, full[0]); got != "old user" {
		t.Fatalf("oldest original message = %q", got)
	}
}

func linearMessages(messages ...agent.AgentMessage) []Entry {
	entries := make([]Entry, 0, len(messages))
	for _, message := range messages {
		entries = append(entries, NewMessage(message))
	}
	return entries
}

func assistant(text string) *llm.AssistantMessage {
	return &llm.AssistantMessage{
		Content:    []llm.AssistantContent{&llm.TextContent{Text: text}},
		StopReason: llm.StopReasonStop,
	}
}

func messageText(t *testing.T, wrapped agent.AgentMessage) string {
	t.Helper()
	message, ok := agent.ToLLM(wrapped)
	if !ok {
		t.Fatalf("message %T is not llm-backed", wrapped)
	}
	switch typed := message.(type) {
	case *llm.UserMessage:
		var result string
		for _, content := range typed.Content {
			if text, ok := content.(*llm.TextContent); ok {
				result += text.Text
			}
		}
		return result
	case *llm.AssistantMessage:
		return typed.Text()
	default:
		t.Fatalf("unexpected message type %T", message)
		return ""
	}
}
