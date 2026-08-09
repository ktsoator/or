package engine

import (
	"encoding/json"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/tools"
)

// Tool outcomes live in the details sidecar so transcript messages remain
// provider-neutral.

const (
	kindToolOutcome     = "tool_outcome"
	kindFileChange      = "file_change"
	kindMutationFailure = "mutation_failure"
	kindPreview         = "preview"
	kindQuestionAnswers = "question_answers"
	kindGenericData     = "generic"
)

type detailsEnvelope struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

type persistedToolOutcome struct {
	Status    agent.ToolOutcomeStatus `json:"status"`
	ErrorCode string                  `json:"errorCode,omitempty"`
	ExitCode  *int                    `json:"exitCode,omitempty"`
	DataKind  string                  `json:"dataKind,omitempty"`
	Data      json.RawMessage         `json:"data,omitempty"`
}

func encodeOutcome(outcome agent.ToolOutcome) (json.RawMessage, bool) {
	if outcome.Status == "" {
		outcome.Status = agent.ToolOutcomeSuccess
	}
	dataKind, data := encodeOutcomeData(outcome.Data)
	persisted := persistedToolOutcome{
		Status:    outcome.Status,
		ErrorCode: outcome.ErrorCode,
		ExitCode:  outcome.ExitCode,
		DataKind:  dataKind,
		Data:      data,
	}
	payload, err := json.Marshal(persisted)
	if err != nil {
		return nil, false
	}
	raw, err := json.Marshal(detailsEnvelope{Kind: kindToolOutcome, Data: payload})
	return raw, err == nil
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
	}
	raw, err := json.Marshal(data)
	if err != nil {
		// Status metadata is still durable even when an extension supplied data
		// that cannot be represented as JSON.
		return "", nil
	}
	return kind, raw
}

func decodeOutcome(raw json.RawMessage) (agent.ToolOutcome, bool) {
	var env detailsEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return agent.ToolOutcome{}, false
	}
	if env.Kind != kindToolOutcome {
		return agent.ToolOutcome{}, false
	}

	var persisted persistedToolOutcome
	if err := json.Unmarshal(env.Data, &persisted); err != nil {
		return agent.ToolOutcome{}, false
	}
	if persisted.Status == "" {
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
	case kindGenericData:
		var value any
		if json.Unmarshal(raw, &value) == nil {
			return value
		}
	}
	return nil
}
