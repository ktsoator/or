package agent

import "github.com/ktsoator/or/llm"

// defaultConvertToLLM is the projection used when LoopConfig.ConvertToLLM is nil.
// It unwraps messages created with FromLLM and drops every other AgentMessage,
// so UI-only messages stay in the transcript but never reach the model.
func defaultConvertToLLM(messages []AgentMessage) []llm.Message {
	result := make([]llm.Message, 0, len(messages))
	for _, message := range messages {
		if wrapped, ok := message.(llmMessage); ok {
			result = append(result, wrapped.Message)
		}
	}
	return result
}
