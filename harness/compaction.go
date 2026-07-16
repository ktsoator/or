package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// Compactor shrinks a transcript before it is projected to the model. It runs on
// every turn via the agent's TransformContext hook and returns the messages to
// send for that request. It must not mutate its input.
//
// Compaction here is projection-only: the harness and Session keep the full
// transcript, and only what travels to the model is shortened. Returning the
// input unchanged (the common case below the threshold) disables it for a turn.
type Compactor interface {
	Compact(ctx context.Context, messages []agent.AgentMessage) ([]agent.AgentMessage, error)
}

// CompactionSettings tunes when and how much SummarizingCompactor compacts.
type CompactionSettings struct {
	// ReserveTokens is headroom kept below the model's context window; compaction
	// triggers once the estimate exceeds contextWindow - ReserveTokens.
	ReserveTokens int
	// KeepRecentTokens is the approximate amount of recent context to retain
	// verbatim after compaction.
	KeepRecentTokens int
}

// DefaultCompactionSettings are used for any zero field on a SummarizingCompactor.
var DefaultCompactionSettings = CompactionSettings{
	ReserveTokens:    16384,
	KeepRecentTokens: 20000,
}

// SummarizeFunc condenses prior conversation into a single summary string.
type SummarizeFunc func(ctx context.Context, model llm.Model, prior []agent.AgentMessage) (string, error)

// SummarizingCompactor keeps the most recent context and replaces the older
// prefix with a model-generated summary once the estimated context exceeds the
// threshold. It is safe for concurrent use, though the harness drives it one run
// at a time.
type SummarizingCompactor struct {
	// Model is the model whose context window bounds compaction and that the
	// default summarizer calls. Required.
	Model llm.Model
	// Settings tunes the thresholds; zero fields fall back to defaults.
	Settings CompactionSettings
	// StreamOptions are passed to the default summarizer's model request.
	StreamOptions llm.StreamOptions
	// Summarize produces the summary. A nil value uses a default that calls the
	// model with a structured summarization prompt.
	Summarize SummarizeFunc

	// mu guards the single-entry summary cache, which avoids re-summarizing an
	// unchanged prefix on every turn while the cut point holds steady.
	mu            sync.Mutex
	cachedCut     int
	cachedSummary string
}

// Compact summarizes the older prefix when the estimated context exceeds the
// threshold, otherwise returns the messages unchanged.
func (c *SummarizingCompactor) Compact(ctx context.Context, messages []agent.AgentMessage) ([]agent.AgentMessage, error) {
	window := int(c.Model.ContextWindow)
	if window <= 0 {
		return messages, nil // unknown window: cannot decide, leave as is
	}
	settings := c.settings()
	if estimateContextTokens(messages) <= window-settings.ReserveTokens {
		return messages, nil
	}
	cut := findCutIndex(messages, settings.KeepRecentTokens)
	if cut <= 0 {
		return messages, nil // no safe boundary with anything to summarize
	}

	summary, err := c.summaryFor(ctx, messages[:cut])
	if err != nil {
		return nil, err
	}
	return mergeSummary(summary, messages[cut:]), nil
}

func (c *SummarizingCompactor) settings() CompactionSettings {
	s := c.Settings
	if s.ReserveTokens <= 0 {
		s.ReserveTokens = DefaultCompactionSettings.ReserveTokens
	}
	if s.KeepRecentTokens <= 0 {
		s.KeepRecentTokens = DefaultCompactionSettings.KeepRecentTokens
	}
	return s
}

// summaryFor returns the summary of prior, reusing the cached one when the cut
// point (and therefore the prefix, which only grows by appending) is unchanged.
func (c *SummarizingCompactor) summaryFor(ctx context.Context, prior []agent.AgentMessage) (string, error) {
	c.mu.Lock()
	if c.cachedSummary != "" && c.cachedCut == len(prior) {
		summary := c.cachedSummary
		c.mu.Unlock()
		return summary, nil
	}
	c.mu.Unlock()

	summarize := c.Summarize
	if summarize == nil {
		summarize = c.defaultSummarize
	}
	summary, err := summarize(ctx, c.Model, prior)
	if err != nil {
		return "", err
	}

	c.mu.Lock()
	c.cachedCut = len(prior)
	c.cachedSummary = summary
	c.mu.Unlock()
	return summary, nil
}

// mergeSummary folds the summary into the first kept message when it is a user
// message, so the projected transcript keeps alternating roles and never splits
// a tool-call/tool-result pair. It falls back to a standalone summary message.
func mergeSummary(summary string, kept []agent.AgentMessage) []agent.AgentMessage {
	preface := "Summary of the earlier conversation:\n\n" + summary + "\n\n---\n\n"
	out := make([]agent.AgentMessage, 0, len(kept)+1)

	if len(kept) > 0 {
		if first, ok := agent.ToLLM(kept[0]); ok {
			if user, isUser := first.(*llm.UserMessage); isUser {
				content := make([]llm.UserContent, 0, len(user.Content)+1)
				content = append(content, &llm.TextContent{Text: preface})
				content = append(content, user.Content...)
				out = append(out, agent.FromLLM(&llm.UserMessage{Content: content}))
				return append(out, kept[1:]...)
			}
		}
	}

	out = append(out, agent.FromLLM(&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: preface}}}))
	return append(out, kept...)
}

// findCutIndex returns the index of the first message to keep: the latest
// user-message boundary that still retains at least keepRecentTokens of recent
// context. It returns 0 when there is no such boundary with anything earlier to
// summarize.
func findCutIndex(messages []agent.AgentMessage, keepRecentTokens int) int {
	if len(messages) == 0 {
		return 0
	}
	cumulative := 0
	mustKeepFrom := len(messages) - 1
	for i := len(messages) - 1; i >= 0; i-- {
		cumulative += estimateMessageTokens(messages[i])
		mustKeepFrom = i
		if cumulative >= keepRecentTokens {
			break
		}
	}
	for i := mustKeepFrom; i >= 1; i-- {
		if isUserMessage(messages[i]) {
			return i
		}
	}
	return 0
}

func isUserMessage(m agent.AgentMessage) bool {
	msg, ok := agent.ToLLM(m)
	if !ok {
		return false
	}
	_, isUser := msg.(*llm.UserMessage)
	return isUser
}

// --- token estimation ----------------------------------------------------

const estimatedImageChars = 4800

func calculateContextTokens(u llm.Usage) int {
	if u.TotalTokens > 0 {
		return int(u.TotalTokens)
	}
	return int(u.Input + u.Output + u.CacheRead + u.CacheWrite)
}

// estimateContextTokens uses the most recent successful assistant usage as a
// precise base and adds a character heuristic for the messages after it. With no
// usage available it estimates every message.
func estimateContextTokens(messages []agent.AgentMessage) int {
	lastUsageIndex := -1
	var lastUsage llm.Usage
	for i := len(messages) - 1; i >= 0; i-- {
		if usage, ok := assistantUsage(messages[i]); ok {
			lastUsage = usage
			lastUsageIndex = i
			break
		}
	}
	if lastUsageIndex < 0 {
		total := 0
		for _, m := range messages {
			total += estimateMessageTokens(m)
		}
		return total
	}
	total := calculateContextTokens(lastUsage)
	for i := lastUsageIndex + 1; i < len(messages); i++ {
		total += estimateMessageTokens(messages[i])
	}
	return total
}

func assistantUsage(m agent.AgentMessage) (llm.Usage, bool) {
	msg, ok := agent.ToLLM(m)
	if !ok {
		return llm.Usage{}, false
	}
	assistant, ok := msg.(*llm.AssistantMessage)
	if !ok {
		return llm.Usage{}, false
	}
	if assistant.StopReason == llm.StopReasonError || assistant.StopReason == llm.StopReasonAborted {
		return llm.Usage{}, false
	}
	if calculateContextTokens(assistant.Usage) == 0 {
		return llm.Usage{}, false
	}
	return assistant.Usage, true
}

// estimateMessageTokens approximates a message's token count from its character
// length (roughly four characters per token), matching the heuristic the
// reference implementation uses.
func estimateMessageTokens(m agent.AgentMessage) int {
	msg, ok := agent.ToLLM(m)
	if !ok {
		return 0 // custom UI-only message: never projected to the model
	}
	chars := 0
	switch v := msg.(type) {
	case *llm.UserMessage:
		chars = userContentChars(v.Content)
	case *llm.AssistantMessage:
		for _, block := range v.Content {
			switch b := block.(type) {
			case *llm.TextContent:
				chars += len(b.Text)
			case *llm.ThinkingContent:
				chars += len(b.Thinking)
			case *llm.ToolCall:
				chars += len(b.Name) + marshalLen(b.Arguments)
			}
		}
	case *llm.ToolResultMessage:
		chars = toolResultChars(v.Content)
	}
	return (chars + 3) / 4
}

func userContentChars(content []llm.UserContent) int {
	chars := 0
	for _, block := range content {
		switch b := block.(type) {
		case *llm.TextContent:
			chars += len(b.Text)
		case *llm.ImageContent:
			chars += estimatedImageChars
		}
	}
	return chars
}

func toolResultChars(content []llm.ToolResultContent) int {
	chars := 0
	for _, block := range content {
		switch b := block.(type) {
		case *llm.TextContent:
			chars += len(b.Text)
		case *llm.ImageContent:
			chars += estimatedImageChars
		}
	}
	return chars
}

func marshalLen(v any) int {
	data, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return len(data)
}

// --- default summarizer --------------------------------------------------

const summarizationSystemPrompt = "You are a context summarization assistant. Read the conversation and produce a concise, structured summary another assistant can use to continue the work. Do not continue the conversation or answer questions in it; output only the summary."

const summarizationInstructions = `Summarize the conversation above as a context checkpoint another assistant will use to continue. Use this exact format:

## Goal
[What the user is trying to accomplish.]

## Progress
[What has been done, including key decisions and results.]

## Current state
[Files, data, or state in play needed to resume.]

## Next steps
[What remains.]`

func (c *SummarizingCompactor) defaultSummarize(ctx context.Context, model llm.Model, prior []agent.AgentMessage) (string, error) {
	promptText := "<conversation>\n" + serializeConversation(prior) + "\n</conversation>\n\n" + summarizationInstructions
	input := llm.Context{
		SystemPrompt: summarizationSystemPrompt,
		Messages: []llm.Message{
			&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: promptText}}},
		},
	}
	options := c.StreamOptions
	if reserve := c.settings().ReserveTokens; reserve > 0 {
		options.MaxTokens = int64(reserve * 4 / 5) // ~0.8 * reserve
	}

	response, err := llm.Complete(ctx, model, input, options)
	if err != nil {
		return "", fmt.Errorf("summarize: %w", err)
	}
	if response.StopReason == llm.StopReasonError {
		return "", fmt.Errorf("summarize: %s", response.ErrorMessage)
	}
	summary := assistantText(response)
	if strings.TrimSpace(summary) == "" {
		return "", fmt.Errorf("summarize: model returned an empty summary")
	}
	return summary, nil
}

// serializeConversation renders a transcript as plain role-prefixed text for the
// summarization prompt.
func serializeConversation(messages []agent.AgentMessage) string {
	var b strings.Builder
	for _, m := range messages {
		msg, ok := agent.ToLLM(m)
		if !ok {
			continue
		}
		switch v := msg.(type) {
		case *llm.UserMessage:
			fmt.Fprintf(&b, "User: %s\n", userText(v.Content))
		case *llm.AssistantMessage:
			fmt.Fprintf(&b, "Assistant: %s\n", assistantText(*v))
		case *llm.ToolResultMessage:
			fmt.Fprintf(&b, "Tool (%s): %s\n", v.ToolName, toolResultText(v.Content))
		}
	}
	return b.String()
}

func userText(content []llm.UserContent) string {
	var parts []string
	for _, block := range content {
		if t, ok := block.(*llm.TextContent); ok {
			parts = append(parts, t.Text)
		}
	}
	return strings.Join(parts, " ")
}

func toolResultText(content []llm.ToolResultContent) string {
	var parts []string
	for _, block := range content {
		if t, ok := block.(*llm.TextContent); ok {
			parts = append(parts, t.Text)
		}
	}
	return strings.Join(parts, " ")
}

func assistantText(m llm.AssistantMessage) string {
	var parts []string
	for _, block := range m.Content {
		switch b := block.(type) {
		case *llm.TextContent:
			parts = append(parts, b.Text)
		case *llm.ToolCall:
			parts = append(parts, fmt.Sprintf("[tool call: %s]", b.Name))
		}
	}
	return strings.Join(parts, " ")
}
