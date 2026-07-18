package coding

import (
	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// ContextUsage describes the latest provider-measured context for the model
// currently selected by a Session. UsedTokens includes the prompt and response
// tokens from that request. Measured is false until the selected model has
// completed a request; switching models deliberately invalidates the previous
// model's count because tokenizers and context limits differ.
type ContextUsage struct {
	Provider      string
	Model         string
	UsedTokens    int64
	ContextWindow int64
	Measured      bool
}

// ContextUsage returns the newest context measurement when it belongs to the
// model currently selected by the Session.
func (s *Session) ContextUsage() ContextUsage {
	state := s.agent.Snapshot()
	result := ContextUsage{
		Provider:      state.Model.Provider,
		Model:         state.Model.ID,
		ContextWindow: state.Model.ContextWindow,
	}

	for index := len(state.Messages) - 1; index >= 0; index-- {
		message, ok := agent.ToLLM(state.Messages[index])
		if !ok {
			continue
		}
		assistant, ok := message.(*llm.AssistantMessage)
		if !ok || assistant == nil {
			continue
		}
		if assistant.Provider != result.Provider || assistant.Model != result.Model {
			return result
		}
		result.UsedTokens = usageTokens(assistant.Usage)
		result.Measured = result.UsedTokens > 0
		return result
	}
	return result
}

func usageTokens(usage llm.Usage) int64 {
	if usage.TotalTokens > 0 {
		return usage.TotalTokens
	}
	return usage.Input + usage.Output + usage.CacheRead + usage.CacheWrite
}
