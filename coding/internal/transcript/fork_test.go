package transcript

import (
	"errors"
	"testing"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

func TestForkBeforeUserReplacesTextAndPreservesAttachments(t *testing.T) {
	first := NewMessage(agent.UserMessage("first"))
	answer := NewMessage(forkAssistant("answer"))
	selected := NewMessage(agent.FromLLM(&llm.UserMessage{Content: []llm.UserContent{
		&llm.TextContent{Text: "old text", TextSignature: "source-signature"},
		&llm.TextContent{Text: "attachment contents"},
		&llm.ImageContent{Data: "AAAA", MIMEType: "image/png"},
	}}))
	later := NewMessage(forkAssistant("later"))
	source := []Entry{first, answer, selected, later}

	forked, err := Fork(
		source,
		selected.ID,
		ForkBeforeUser,
		"new text",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(forked) != 3 || forked[0].ID != first.ID || forked[1].ID != answer.ID {
		t.Fatalf("forked entries = %#v", forked)
	}
	if forked[2].ID == selected.ID {
		t.Fatal("replacement reused the source message ID")
	}
	message, _ := agent.ToLLM(forked[2].Message)
	user := message.(*llm.UserMessage)
	if len(user.Content) != 3 || user.Content[0].(*llm.TextContent).Text != "new text" ||
		user.Content[0].(*llm.TextContent).TextSignature != "" ||
		user.Content[1].(*llm.TextContent).Text != "attachment contents" ||
		user.Content[2].(*llm.ImageContent).Data != "AAAA" {
		t.Fatalf("replacement content = %#v", user.Content)
	}
	sourceMessage, _ := agent.ToLLM(source[2].Message)
	sourceUser := sourceMessage.(*llm.UserMessage)
	if sourceUser.Content[0].(*llm.TextContent).Text != "old text" ||
		sourceUser.Content[0].(*llm.TextContent).TextSignature != "source-signature" {
		t.Fatalf("source message was modified: %#v", sourceUser.Content)
	}
}

func TestForkBeforeFirstUserCreatesReplacementMessage(t *testing.T) {
	selected := NewMessage(agent.FromLLM(&llm.UserMessage{Content: []llm.UserContent{
		&llm.ImageContent{Data: "AAAA", MIMEType: "image/png"},
	}}))

	forked, err := Fork([]Entry{selected}, selected.ID, ForkBeforeUser, "describe this")
	if err != nil {
		t.Fatal(err)
	}
	if len(forked) != 1 || forked[0].ID == selected.ID {
		t.Fatalf("forked entries = %#v", forked)
	}
	message, _ := agent.ToLLM(forked[0].Message)
	user := message.(*llm.UserMessage)
	if len(user.Content) != 2 || user.Content[0].(*llm.TextContent).Text != "describe this" ||
		user.Content[1].(*llm.ImageContent).Data != "AAAA" {
		t.Fatalf("replacement content = %#v", user.Content)
	}
}

func TestForkBeforeUserPreservesValidCompaction(t *testing.T) {
	oldUser := NewMessage(agent.UserMessage("old question"))
	oldAnswer := NewMessage(forkAssistant("old answer"))
	keptUser := NewMessage(agent.UserMessage("kept question"))
	keptAnswer := NewMessage(forkAssistant("kept answer"))
	compaction := NewCompaction(Compaction{
		Summary:          "older work",
		FirstKeptEntryID: keptUser.ID,
		TokensBefore:     100,
		TokensAfter:      40,
	})
	selected := NewMessage(agent.UserMessage("edit me"))

	forked, err := Fork(
		[]Entry{oldUser, oldAnswer, keptUser, keptAnswer, compaction, selected},
		selected.ID,
		ForkBeforeUser,
		"replacement",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(forked) != 6 || forked[4].ID != compaction.ID {
		t.Fatalf("forked entries = %#v", forked)
	}
	context, err := BuildContext(forked)
	if err != nil {
		t.Fatal(err)
	}
	if len(context) != 4 || messageText(t, context[3]) != "replacement" {
		t.Fatalf("forked context = %#v", context)
	}
}

func TestForkBeforeUserRejectsEmptyReplacement(t *testing.T) {
	selected := NewMessage(agent.UserMessage("original"))

	_, err := Fork([]Entry{selected}, selected.ID, ForkBeforeUser, "  ")
	if !errors.Is(err, ErrInvalidForkBoundary) {
		t.Fatalf("Fork error = %v, want %v", err, ErrInvalidForkBoundary)
	}
}

func TestForkAfterAssistantKeepsOnlyAnImmediateCompletedRun(t *testing.T) {
	user := NewMessage(agent.UserMessage("question"))
	answer := NewMessage(forkAssistant("answer"))
	run := NewRun(user.ID, time.Now().Add(-time.Second), time.Now())
	later := NewMessage(agent.UserMessage("later"))

	forked, err := Fork([]Entry{user, answer, run, later}, answer.ID, ForkAfterAssistant, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(forked) != 3 || forked[2].Type != RunEntry || forked[2].ID != run.ID {
		t.Fatalf("forked entries = %#v, want completed run", forked)
	}

	forked, err = Fork([]Entry{user, answer, later, run}, answer.ID, ForkAfterAssistant, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(forked) != 2 {
		t.Fatalf("forked entries = %#v, want boundary at assistant", forked)
	}
}

func TestForkRejectsInvalidBoundaries(t *testing.T) {
	user := NewMessage(agent.UserMessage("question"))
	toolUse := NewMessage(agent.FromLLM(&llm.AssistantMessage{
		Content: []llm.AssistantContent{&llm.ToolCall{
			ID: "call-1", Name: "read", Arguments: map[string]any{"path": "README.md"},
		}},
		StopReason: llm.StopReasonToolUse,
	}))
	toolResult := NewMessage(agent.FromLLM(&llm.ToolResultMessage{
		ToolCallID: "call-1",
		ToolName:   "read",
		Content:    []llm.ToolResultContent{&llm.TextContent{Text: "contents"}},
	}))
	selected := NewMessage(agent.UserMessage("edit me"))
	answer := NewMessage(forkAssistant("answer"))

	tests := []struct {
		name      string
		entries   []Entry
		messageID string
		mode      ForkMode
		wantError error
	}{
		{
			name: "tool use assistant",
			entries: []Entry{
				user, toolUse,
			},
			messageID: toolUse.ID,
			mode:      ForkAfterAssistant,
			wantError: ErrInvalidForkBoundary,
		},
		{
			name:      "user after tool call",
			entries:   []Entry{user, toolUse, selected},
			messageID: selected.ID,
			mode:      ForkBeforeUser,
			wantError: ErrInvalidForkBoundary,
		},
		{
			name:      "user after tool result before final response",
			entries:   []Entry{user, toolUse, toolResult, selected},
			messageID: selected.ID,
			mode:      ForkBeforeUser,
			wantError: ErrInvalidForkBoundary,
		},
		{
			name:      "assistant in before user mode",
			entries:   []Entry{user, answer},
			messageID: answer.ID,
			mode:      ForkBeforeUser,
			wantError: ErrInvalidForkBoundary,
		},
		{
			name:      "user in after assistant mode",
			entries:   []Entry{user},
			messageID: user.ID,
			mode:      ForkAfterAssistant,
			wantError: ErrInvalidForkBoundary,
		},
		{
			name:      "unknown message",
			entries:   []Entry{user},
			messageID: "missing",
			mode:      ForkBeforeUser,
			wantError: ErrForkMessageNotFound,
		},
		{
			name:      "empty message id",
			entries:   []Entry{user},
			messageID: " ",
			mode:      ForkBeforeUser,
			wantError: ErrInvalidForkBoundary,
		},
		{
			name:      "unknown mode",
			entries:   []Entry{user},
			messageID: user.ID,
			mode:      ForkMode("unknown"),
			wantError: ErrInvalidForkBoundary,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Fork(test.entries, test.messageID, test.mode, "replacement")
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Fork error = %v, want %v", err, test.wantError)
			}
		})
	}
}

func TestForkAfterAssistantPreservesCompletedToolLoop(t *testing.T) {
	user := NewMessage(agent.UserMessage("question"))
	toolUse := NewMessage(agent.FromLLM(&llm.AssistantMessage{
		Content: []llm.AssistantContent{&llm.ToolCall{
			ID: "call-1", Name: "read", Arguments: map[string]any{"path": "README.md"},
		}},
		StopReason: llm.StopReasonToolUse,
	}))
	toolResult := NewMessage(agent.FromLLM(&llm.ToolResultMessage{
		ToolCallID: "call-1",
		ToolName:   "read",
		Content:    []llm.ToolResultContent{&llm.TextContent{Text: "contents"}},
	}))
	answer := NewMessage(forkAssistant("answer"))

	forked, err := Fork(
		[]Entry{user, toolUse, toolResult, answer},
		answer.ID,
		ForkAfterAssistant,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(forked) != 4 || forked[3].ID != answer.ID {
		t.Fatalf("forked entries = %#v", forked)
	}
}

func forkAssistant(text string) agent.AgentMessage {
	return agent.FromLLM(&llm.AssistantMessage{
		Content:    []llm.AssistantContent{&llm.TextContent{Text: text}},
		StopReason: llm.StopReasonStop,
	})
}
