package transcript

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// ForkMode selects the visible message boundary retained in a fork.
type ForkMode string

const (
	// ForkBeforeUser replaces the selected user message and drops later entries.
	ForkBeforeUser ForkMode = "before_user"
	// ForkAfterAssistant keeps the selected completed assistant response.
	ForkAfterAssistant ForkMode = "after_assistant"
)

var (
	// ErrForkMessageNotFound means the requested message ID is not in the transcript.
	ErrForkMessageNotFound = errors.New("transcript: fork message not found")
	// ErrInvalidForkBoundary means the requested fork would produce invalid context.
	ErrInvalidForkBoundary = errors.New("transcript: invalid fork boundary")
)

// Fork returns a transcript prefix at a visible message boundary without
// modifying the source entries.
// Editing replaces the selected user message with a newly identified message;
// branching after an assistant preserves the selected completed response.
func Fork(entries []Entry, messageID string, mode ForkMode, replacementText string) ([]Entry, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil, fmt.Errorf("%w: message id is empty", ErrInvalidForkBoundary)
	}
	if mode != ForkBeforeUser && mode != ForkAfterAssistant {
		return nil, fmt.Errorf("%w: unknown mode %q", ErrInvalidForkBoundary, mode)
	}

	target := -1
	for index, entry := range entries {
		if entry.Type == MessageEntry && entry.ID == messageID {
			target = index
			break
		}
	}
	if target < 0 {
		return nil, ErrForkMessageNotFound
	}

	var forked []Entry
	switch mode {
	case ForkBeforeUser:
		if err := validateBeforeUserBoundary(entries[:target]); err != nil {
			return nil, err
		}
		replacement, err := replaceUserText(entries[target], replacementText)
		if err != nil {
			return nil, err
		}
		forked = append([]Entry(nil), entries[:target]...)
		forked = append(forked, replacement)

	case ForkAfterAssistant:
		message, _ := agent.ToLLM(entries[target].Message)
		assistant, ok := message.(*llm.AssistantMessage)
		if !ok || !isCompletedAssistant(assistant) {
			return nil, fmt.Errorf("%w: after_assistant requires a completed assistant response", ErrInvalidForkBoundary)
		}
		forked = append([]Entry(nil), entries[:target+1]...)
		if tail := completedLifecycleTail(entries[target+1:]); len(tail) > 0 {
			forked = append(forked, tail...)
		}
	}

	context, err := BuildContext(forked)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidForkBoundary, err)
	}
	if hasUnresolvedToolCalls(context) {
		return nil, fmt.Errorf("%w: fork boundary leaves an unresolved tool call", ErrInvalidForkBoundary)
	}
	return forked, nil
}

func completedLifecycleTail(entries []Entry) []Entry {
	var tail []Entry
	for _, entry := range entries {
		switch entry.Type {
		case StepEndEntry, TurnEndEntry:
			tail = append(tail, entry)
		case RunEndEntry:
			if entry.Lifecycle.Status != LifecycleCompleted {
				return nil
			}
			tail = append(tail, entry)
			return tail
		default:
			return nil
		}
	}
	return nil
}

func validateBeforeUserBoundary(entries []Entry) error {
	for index := len(entries) - 1; index >= 0; index-- {
		if entries[index].Type != MessageEntry {
			continue
		}
		message, _ := agent.ToLLM(entries[index].Message)
		assistant, ok := message.(*llm.AssistantMessage)
		if ok && isCompletedAssistant(assistant) {
			return nil
		}
		return fmt.Errorf(
			"%w: before_user requires a completed turn before the selected message",
			ErrInvalidForkBoundary,
		)
	}
	return nil
}

func isCompletedAssistant(message *llm.AssistantMessage) bool {
	if message == nil || len(message.ToolCalls()) > 0 {
		return false
	}
	switch message.StopReason {
	case llm.StopReasonStop,
		llm.StopReasonLength,
		llm.StopReasonError,
		llm.StopReasonAborted:
		return true
	case llm.StopReasonToolUse, "":
		return false
	default:
		return false
	}
}

func hasUnresolvedToolCalls(messages []agent.AgentMessage) bool {
	pending := make(map[string]struct{})
	for _, wrapped := range messages {
		message, ok := agent.ToLLM(wrapped)
		if !ok {
			continue
		}
		switch typed := message.(type) {
		case *llm.AssistantMessage:
			if typed == nil {
				continue
			}
			calls := typed.ToolCalls()
			if typed.StopReason == llm.StopReasonToolUse && len(calls) == 0 {
				return true
			}
			for _, call := range calls {
				if call.ID == "" {
					return true
				}
				pending[call.ID] = struct{}{}
			}
		case *llm.ToolResultMessage:
			if typed != nil {
				delete(pending, typed.ToolCallID)
			}
		}
	}
	return len(pending) > 0
}

func replaceUserText(entry Entry, replacementText string) (Entry, error) {
	message, _ := agent.ToLLM(entry.Message)
	user, ok := message.(*llm.UserMessage)
	if !ok || user == nil {
		return Entry{}, fmt.Errorf("%w: before_user requires a user message", ErrInvalidForkBoundary)
	}

	copy := *user
	copy.Content = append([]llm.UserContent(nil), user.Content...)
	replaced := false
	for index, content := range copy.Content {
		text, ok := content.(*llm.TextContent)
		if !ok || text == nil {
			continue
		}
		cloned := *text
		cloned.Text = strings.TrimSpace(replacementText)
		cloned.TextSignature = ""
		copy.Content[index] = &cloned
		replaced = true
		break
	}
	if !replaced {
		copy.Content = append([]llm.UserContent{&llm.TextContent{
			Text: strings.TrimSpace(replacementText),
		}}, copy.Content...)
	}
	if !hasUserContent(copy.Content) {
		return Entry{}, fmt.Errorf("%w: replacement message is empty", ErrInvalidForkBoundary)
	}
	return NewMessage(agent.FromLLM(&copy)), nil
}

func hasUserContent(contents []llm.UserContent) bool {
	for _, content := range contents {
		switch block := content.(type) {
		case *llm.TextContent:
			if block != nil && strings.TrimSpace(block.Text) != "" {
				return true
			}
		case *llm.ImageContent:
			if block != nil && block.Data != "" {
				return true
			}
		}
	}
	return false
}
