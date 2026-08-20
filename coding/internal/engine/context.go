package engine

import (
	"encoding/json"
	"unicode/utf8"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/contextprojection"
	"github.com/ktsoator/or/llm"
)

const (
	messageTokenOverhead = int64(4)
	imageTokenEstimate   = int64(1_000)
)

// ContextBreakdown estimates how the latest measured context is distributed.
// The provider-measured total remains authoritative; these categories are
// proportionally calibrated to it because providers do not report attribution.
type ContextBreakdown struct {
	Messages       int64
	SystemTools    int64
	SystemPrompt   int64
	Skills         int64
	ProjectContext int64
}

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
	Breakdown     *ContextBreakdown
}

// ContextUsage returns the newest context measurement when it belongs to the
// model currently selected by the Session.
func (s *Session) ContextUsage() ContextUsage {
	state := s.agent.Snapshot()
	usageStart := s.journal.usageStartIndex()
	result := ContextUsage{
		Provider:      state.Model.Provider,
		Model:         state.Model.ID,
		ContextWindow: state.Model.ContextWindow,
	}

	for index := len(state.Messages) - 1; index >= usageStart; index-- {
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
		if result.Measured {
			result.Breakdown = estimateContextBreakdown(
				state,
				s.context.projectedAttachments(),
				result.UsedTokens,
			)
		}
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

func estimateContextBreakdown(
	state agent.State,
	attachments []contextprojection.Attachment,
	usedTokens int64,
) *ContextBreakdown {
	if usedTokens <= 0 {
		return nil
	}
	estimated := ContextBreakdown{
		Messages:     estimateMessages(state.Messages),
		SystemTools:  estimateTools(state.Tools),
		SystemPrompt: estimateTextTokens(state.SystemPrompt),
	}
	for _, attachment := range attachments {
		tokens := estimateTextTokens(attachment.Rendered) + messageTokenOverhead
		switch attachment.Kind {
		case contextprojection.SkillListing, contextprojection.SkillsUpdate, contextprojection.ActivatedSkill:
			estimated.Skills += tokens
		case contextprojection.BaseContext, contextprojection.ContextUpdate:
			estimated.ProjectContext += tokens
		case contextprojection.TaskStatus:
			estimated.Messages += tokens
		}
	}
	return estimated.calibrated(usedTokens)
}

func (breakdown ContextBreakdown) calibrated(total int64) *ContextBreakdown {
	estimatedTotal := breakdown.total()
	if total <= 0 || estimatedTotal <= 0 {
		return nil
	}
	calibrated := ContextBreakdown{
		Messages:       breakdown.Messages * total / estimatedTotal,
		SystemTools:    breakdown.SystemTools * total / estimatedTotal,
		SystemPrompt:   breakdown.SystemPrompt * total / estimatedTotal,
		Skills:         breakdown.Skills * total / estimatedTotal,
		ProjectContext: breakdown.ProjectContext * total / estimatedTotal,
	}
	// Integer division can leave at most four tokens undistributed. Messages is
	// always present in a measured request and absorbs that rounding remainder.
	calibrated.Messages += total - calibrated.total()
	return &calibrated
}

func (breakdown ContextBreakdown) total() int64 {
	return breakdown.Messages + breakdown.SystemTools + breakdown.SystemPrompt +
		breakdown.Skills + breakdown.ProjectContext
}

func estimateMessages(messages []agent.AgentMessage) int64 {
	var total int64
	for _, message := range messages {
		projected, ok := agent.ToLLM(message)
		if !ok {
			continue
		}
		total += estimateMessage(projected)
	}
	return total
}

func estimateMessage(message llm.Message) int64 {
	total := messageTokenOverhead
	switch message := message.(type) {
	case *llm.UserMessage:
		if message == nil {
			return 0
		}
		for _, content := range message.Content {
			total += estimateUserContent(content)
		}
	case *llm.AssistantMessage:
		if message == nil {
			return 0
		}
		for _, content := range message.Content {
			total += estimateAssistantContent(content)
		}
	case *llm.ToolResultMessage:
		if message == nil {
			return 0
		}
		total += estimateTextTokens(message.ToolCallID) + estimateTextTokens(message.ToolName)
		for _, content := range message.Content {
			total += estimateToolResultContent(content)
		}
	}
	return total
}

func estimateUserContent(content llm.UserContent) int64 {
	switch content := content.(type) {
	case *llm.TextContent:
		if content != nil {
			return estimateTextTokens(content.Text)
		}
	case *llm.ImageContent:
		if content != nil {
			return imageTokenEstimate
		}
	}
	return 0
}

func estimateAssistantContent(content llm.AssistantContent) int64 {
	switch content := content.(type) {
	case *llm.TextContent:
		if content != nil {
			return estimateTextTokens(content.Text) + estimateTextTokens(content.TextSignature)
		}
	case *llm.ThinkingContent:
		if content != nil {
			return estimateTextTokens(content.Thinking) + estimateTextTokens(content.ThinkingSignature)
		}
	case *llm.ToolCall:
		if content != nil {
			encoded, _ := json.Marshal(content.Arguments)
			return estimateTextTokens(content.ID) + estimateTextTokens(content.Name) +
				estimateTextTokens(string(encoded)) + estimateTextTokens(content.ThoughtSignature)
		}
	}
	return 0
}

func estimateToolResultContent(content llm.ToolResultContent) int64 {
	switch content := content.(type) {
	case *llm.TextContent:
		if content != nil {
			return estimateTextTokens(content.Text)
		}
	case *llm.ImageContent:
		if content != nil {
			return imageTokenEstimate
		}
	}
	return 0
}

func estimateTools(tools []agent.AgentTool) int64 {
	var total int64
	for _, tool := range tools {
		encoded, err := json.Marshal(tool.Definition)
		if err == nil {
			total += estimateTextTokens(string(encoded))
		}
	}
	return total
}

// estimateTextTokens approximates common code and prose at four ASCII
// characters per token while treating each non-ASCII rune as one token.
func estimateTextTokens(text string) int64 {
	if text == "" {
		return 0
	}
	var units int64
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		text = text[size:]
		if r <= utf8.RuneSelf {
			units++
		} else {
			units += 4
		}
	}
	return (units + 3) / 4
}
