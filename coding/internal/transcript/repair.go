package transcript

import (
	"fmt"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

const (
	// ToolNotStarted means no durable dispatch intent exists for an interrupted
	// assistant tool request.
	ToolNotStarted = "TOOL_NOT_STARTED"
	// ToolOutcomeUnknown means dispatch became possible but no terminal result
	// was durably recorded.
	ToolOutcomeUnknown = "TOOL_OUTCOME_UNKNOWN"
)

type pendingToolCall struct {
	call     llm.ToolCall
	started  bool
	resolved bool
}

// RepairInterruptedToolCalls validates tool-call ordering and returns synthetic
// result/outcome entries that close an interrupted tail. It never mutates the
// supplied committed prefix.
func RepairInterruptedToolCalls(entries []Entry) ([]Entry, error) {
	pendingByID := make(map[string]*pendingToolCall)
	var pendingInOrder []*pendingToolCall

	for index, entry := range entries {
		if err := entry.Validate(); err != nil {
			return nil, err
		}
		switch entry.Type {
		case MessageEntry:
			message, ok := agent.ToLLM(entry.Message)
			if !ok {
				return nil, fmt.Errorf("transcript: entry %s has an unsupported message", entry.ID)
			}
			switch typed := message.(type) {
			case *llm.AssistantMessage:
				if len(pendingByID) > 0 {
					return nil, fmt.Errorf(
						"transcript: assistant entry %s follows unresolved tool calls",
						entry.ID,
					)
				}
				for _, call := range typed.ToolCalls() {
					if call.ID == "" {
						return nil, fmt.Errorf("transcript: assistant entry %s has a tool call without an id", entry.ID)
					}
					if _, exists := pendingByID[call.ID]; exists {
						return nil, fmt.Errorf("transcript: assistant entry %s repeats tool call id %s", entry.ID, call.ID)
					}
					pending := &pendingToolCall{call: call}
					pendingByID[call.ID] = pending
					pendingInOrder = append(pendingInOrder, pending)
				}
			case *llm.ToolResultMessage:
				pending, exists := pendingByID[typed.ToolCallID]
				if !exists {
					return nil, fmt.Errorf(
						"transcript: tool result entry %s has no unresolved call %s",
						entry.ID,
						typed.ToolCallID,
					)
				}
				if firstPending(pendingInOrder) != pending {
					return nil, fmt.Errorf(
						"transcript: tool result entry %s resolves call %s out of model order",
						entry.ID,
						typed.ToolCallID,
					)
				}
				pending.resolved = true
				delete(pendingByID, typed.ToolCallID)
			case *llm.UserMessage:
				if len(pendingByID) > 0 {
					return nil, fmt.Errorf("transcript: user entry %s follows unresolved tool calls", entry.ID)
				}
			default:
				return nil, fmt.Errorf("transcript: message entry %s has unsupported type %T", entry.ID, message)
			}

		case ToolCallEntry:
			pending, exists := pendingByID[entry.ToolCall.ToolCallID]
			if !exists {
				return nil, fmt.Errorf(
					"transcript: tool call entry %s has no unresolved assistant call %s",
					entry.ID,
					entry.ToolCall.ToolCallID,
				)
			}
			if pending.started {
				return nil, fmt.Errorf(
					"transcript: tool call entry %s repeats dispatch intent for %s",
					entry.ID,
					entry.ToolCall.ToolCallID,
				)
			}
			if pending.call.Name != entry.ToolCall.ToolName {
				return nil, fmt.Errorf(
					"transcript: tool call entry %s names %q, want %q",
					entry.ID,
					entry.ToolCall.ToolName,
					pending.call.Name,
				)
			}
			pending.started = true

		case ContextEntry:
			// Tools may durably attach product context while their result is still
			// pending. The attachment stays inside the current tool step.
		case CompactionEntry,
			RunStartEntry, RunEndEntry,
			TurnStartEntry, TurnEndEntry,
			StepStartEntry, StepEndEntry:
			if len(pendingByID) > 0 {
				return nil, fmt.Errorf(
					"transcript: %s entry %s follows unresolved tool calls at index %d",
					entry.Type,
					entry.ID,
					index,
				)
			}
		case ToolOutcomeEntry:
			// Tool outcomes accompany a preceding model-facing result and do not
			// change the unresolved-call cursor.
		default:
			return nil, fmt.Errorf("transcript: entry %s has unsupported type %q", entry.ID, entry.Type)
		}
	}

	if len(pendingByID) == 0 {
		return nil, nil
	}
	repairs := make([]Entry, 0, 2*len(pendingByID))
	for _, pending := range pendingInOrder {
		if pending.resolved {
			continue
		}
		code, text := interruptedToolResult(pending.started)
		repairs = append(repairs,
			NewMessage(agent.FromLLM(&llm.ToolResultMessage{
				ToolCallID: pending.call.ID,
				ToolName:   pending.call.Name,
				IsError:    true,
				Content: []llm.ToolResultContent{
					&llm.TextContent{Text: text},
				},
			})),
			NewToolOutcome(ToolOutcome{
				ToolCallID: pending.call.ID,
				Status:     agent.ToolOutcomeFailed,
				ErrorCode:  code,
			}),
		)
	}
	return repairs, nil
}

func firstPending(calls []*pendingToolCall) *pendingToolCall {
	for _, call := range calls {
		if !call.resolved {
			return call
		}
	}
	return nil
}

func interruptedToolResult(started bool) (code, text string) {
	if !started {
		return ToolNotStarted,
			"The tool call did not start before the previous process was interrupted. Retry it if it is still needed."
	}
	return ToolOutcomeUnknown,
		"The previous process was interrupted after tool dispatch became possible, but no result was saved. The outcome is unknown. Do not retry a side-effecting operation until you verify external state or ask the user."
}
