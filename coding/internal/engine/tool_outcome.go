package engine

import (
	"encoding/json"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/transcript"
)

const (
	kindFileChange      = "file_change"
	kindMutationFailure = "mutation_failure"
	kindPreview         = "preview"
	kindQuestionAnswers = "question_answers"
	kindTodoList        = "todo_list"
	kindGenericData     = "generic"
)

func encodeOutcome(toolCallID string, outcome agent.ToolOutcome) transcript.ToolOutcome {
	if outcome.Status == "" {
		outcome.Status = agent.ToolOutcomeSuccess
	}
	dataKind, data := encodeOutcomeData(outcome.Data)
	return transcript.ToolOutcome{
		ToolCallID: toolCallID,
		Status:     outcome.Status,
		ErrorCode:  outcome.ErrorCode,
		ExitCode:   outcome.ExitCode,
		DataKind:   dataKind,
		Data:       data,
	}
}

func encodeOutcomeData(data any) (string, json.RawMessage) {
	if data == nil {
		return "", nil
	}
	kind := kindGenericData
	switch data.(type) {
	case tools.FileChange:
		kind = kindFileChange
	case tools.MutationFailure:
		kind = kindMutationFailure
	case tools.PreviewRequest:
		kind = kindPreview
	case tools.QuestionAnswers:
		kind = kindQuestionAnswers
	case tools.TodoSnapshot:
		kind = kindTodoList
	}
	raw, err := json.Marshal(data)
	if err != nil {
		// Status metadata is still durable even when an extension supplied data
		// that cannot be represented as JSON.
		return "", nil
	}
	return kind, raw
}

func decodeOutcome(persisted transcript.ToolOutcome) (agent.ToolOutcome, bool) {
	if persisted.ToolCallID == "" || persisted.Status == "" {
		return agent.ToolOutcome{}, false
	}
	return agent.ToolOutcome{
		Status:    persisted.Status,
		ErrorCode: persisted.ErrorCode,
		ExitCode:  persisted.ExitCode,
		Data:      decodeOutcomeData(persisted.DataKind, persisted.Data),
	}, true
}

func decodeOutcomeData(kind string, raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	switch kind {
	case kindFileChange:
		var value tools.FileChange
		if json.Unmarshal(raw, &value) == nil {
			return value
		}
	case kindMutationFailure:
		var value tools.MutationFailure
		if json.Unmarshal(raw, &value) == nil {
			return value
		}
	case kindPreview:
		var value tools.PreviewRequest
		if json.Unmarshal(raw, &value) == nil {
			return value
		}
	case kindQuestionAnswers:
		var value tools.QuestionAnswers
		if json.Unmarshal(raw, &value) == nil {
			return value
		}
	case kindTodoList:
		var value tools.TodoSnapshot
		if json.Unmarshal(raw, &value) == nil {
			return value
		}
	case kindGenericData:
		var value any
		if json.Unmarshal(raw, &value) == nil {
			return value
		}
	}
	return nil
}
