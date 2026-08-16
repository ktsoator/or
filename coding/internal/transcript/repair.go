package transcript

import (
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

// RepairInterruptedToolCalls validates the complete event prefix through the
// canonical reducer and returns synthetic result/outcome pairs for unresolved
// calls. It never mutates the supplied committed prefix.
func RepairInterruptedToolCalls(entries []Entry) ([]Entry, error) {
	reducer := newSessionReducer(len(entries))
	for index, entry := range entries {
		if _, err := reducer.Apply(index, entry); err != nil {
			return nil, err
		}
	}

	repairs := make([]Entry, 0, 2*len(reducer.pendingTools))
	for _, toolCallID := range reducer.pendingTools {
		tool := reducer.tools[toolCallID]
		code, text := interruptedToolResult(tool.DispatchEntryID != "")
		repairs = append(repairs,
			NewMessage(agent.FromLLM(&llm.ToolResultMessage{
				ToolCallID: tool.Request.ID,
				ToolName:   tool.Request.Name,
				IsError:    true,
				Content: []llm.ToolResultContent{
					&llm.TextContent{Text: text},
				},
			})),
			NewToolOutcome(ToolOutcome{
				ToolCallID: tool.Request.ID,
				Status:     agent.ToolOutcomeFailed,
				ErrorCode:  code,
			}),
		)
	}
	return repairs, nil
}

func interruptedToolResult(started bool) (code, text string) {
	if !started {
		return ToolNotStarted,
			"The tool call did not start before the previous process was interrupted. Retry it if it is still needed."
	}
	return ToolOutcomeUnknown,
		"The previous process was interrupted after tool dispatch became possible, but no result was saved. The outcome is unknown. Do not retry a side-effecting operation until you verify external state or ask the user."
}
