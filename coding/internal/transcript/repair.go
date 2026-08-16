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

// RecoverSession validates one committed event prefix, synthesizes repairs for
// its interrupted tail, and returns a validator advanced through those repairs.
// The prefix is replayed once and is never mutated.
func RecoverSession(entries []Entry) (*SessionValidator, []Entry, error) {
	validator, err := ValidateSession(entries)
	if err != nil {
		return nil, nil, err
	}

	toolRepairs := interruptedToolCallRepairs(validator.reducer)
	toolRepairs, err = SequenceEntries(toolRepairs, validator.NextSeq())
	if err != nil {
		return nil, nil, err
	}
	preparedTools, err := validator.PrepareAppend(toolRepairs)
	if err != nil {
		return nil, nil, err
	}
	preparedTools.Commit()

	lifecycleRepairs, err := interruptedLifecycleRepairs(validator.reducer)
	if err != nil {
		return nil, nil, err
	}
	lifecycleRepairs, err = SequenceEntries(lifecycleRepairs, validator.NextSeq())
	if err != nil {
		return nil, nil, err
	}
	preparedLifecycle, err := validator.PrepareAppend(lifecycleRepairs)
	if err != nil {
		return nil, nil, err
	}
	preparedLifecycle.Commit()

	repairs := make([]Entry, 0, len(toolRepairs)+len(lifecycleRepairs))
	repairs = append(repairs, toolRepairs...)
	repairs = append(repairs, lifecycleRepairs...)
	return validator, repairs, nil
}

func interruptedToolCallRepairs(reducer *sessionReducer) []Entry {
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
	return repairs
}

func interruptedToolResult(started bool) (code, text string) {
	if !started {
		return ToolNotStarted,
			"The tool call did not start before the previous process was interrupted. Retry it if it is still needed."
	}
	return ToolOutcomeUnknown,
		"The previous process was interrupted after tool dispatch became possible, but no result was saved. The outcome is unknown. Do not retry a side-effecting operation until you verify external state or ask the user."
}
