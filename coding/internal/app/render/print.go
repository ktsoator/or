// Package render turns agent events into terminal output for print mode. It is
// part of the product shell and must not be imported by the coding core.
package render

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// ANSI escape codes for muted and error styling.
const (
	dim   = "\033[2m"
	red   = "\033[31m"
	reset = "\033[0m"
)

// Printer renders a run's events to a writer as plain, lightly-styled text.
// Assistant text and reasoning stream token by token as they arrive; tool calls
// and results print as discrete lines.
type Printer struct {
	w io.Writer
	// streamKind tracks the kind of the last streamed delta for the current
	// assistant message: "" (nothing streamed yet), "text", or "thinking". It
	// drives separator newlines and lets MessageEnd skip reprinting streamed text.
	streamKind string
}

// New returns a Printer writing to w.
func New(w io.Writer) *Printer { return &Printer{w: w} }

// Handle renders a single agent event. It is safe to register directly with
// Session.Subscribe.
func (p *Printer) Handle(ev agent.AgentEvent) {
	switch ev.Type {
	case agent.MessageUpdate:
		p.handleUpdate(ev.LLMEvent)

	case agent.ToolStart:
		fmt.Fprintf(p.w, "\n%s→ %s %s%s\n", dim, ev.ToolName, compactArgs(ev.Args), reset)

	case agent.ToolEnd:
		text := firstLines(toolResultText(ev.Result), 8)
		style := dim
		if ev.IsError {
			style = red
		}
		if strings.TrimSpace(text) != "" {
			fmt.Fprintf(p.w, "%s%s%s\n", style, text, reset)
		}

	case agent.MessageEnd:
		p.handleMessageEnd(ev.Message)
	}
}

// handleUpdate streams an incremental assistant delta: answer text in the
// default color, reasoning dimmed. Other update kinds (tool-call argument
// fragments) are ignored, since the tool is shown when it starts executing.
func (p *Printer) handleUpdate(event *llm.Event) {
	if event == nil {
		return
	}
	switch event.Type {
	case llm.EventTextDelta:
		p.streamDelta("text", event.Delta, "")
	case llm.EventThinkingDelta:
		p.streamDelta("thinking", event.Delta, dim)
	}
}

// streamDelta writes one streamed fragment, inserting a separating newline
// before the first fragment and whenever the stream switches between reasoning
// and answer text.
func (p *Printer) streamDelta(kind, text, style string) {
	if p.streamKind == "" || p.streamKind != kind {
		fmt.Fprint(p.w, "\n")
	}
	p.streamKind = kind
	if style != "" {
		fmt.Fprintf(p.w, "%s%s%s", style, text, reset)
	} else {
		fmt.Fprint(p.w, text)
	}
}

// handleMessageEnd closes out an assistant message. If its text already streamed
// token by token, it just terminates the line; otherwise (a non-streaming
// provider or path) it prints the full text now.
func (p *Printer) handleMessageEnd(message agent.AgentMessage) {
	text, ok := assistantText(message)
	if !ok {
		return
	}
	if p.streamKind != "" {
		fmt.Fprintln(p.w) // streamed already; end the line
	} else if strings.TrimSpace(text) != "" {
		fmt.Fprintf(p.w, "\n%s\n", text)
	}
	p.streamKind = ""
}

// assistantText returns the text of an assistant message, or ok=false for any
// other message. An errored turn renders its error message instead.
func assistantText(message agent.AgentMessage) (string, bool) {
	llmMessage, ok := agent.ToLLM(message)
	if !ok {
		return "", false
	}
	assistant, ok := llmMessage.(*llm.AssistantMessage)
	if !ok {
		return "", false
	}
	if assistant.StopReason == llm.StopReasonError || assistant.StopReason == llm.StopReasonAborted {
		if assistant.ErrorMessage != "" {
			return "[" + string(assistant.StopReason) + "] " + assistant.ErrorMessage, true
		}
		return "[" + string(assistant.StopReason) + "]", true
	}
	return assistant.Text(), true
}

// toolResultText extracts the text content of a tool result carried on a tool
// event. A partial or unexpected value renders as empty.
func toolResultText(result any) string {
	toolResult, ok := result.(agent.ToolResult)
	if !ok {
		return ""
	}
	var parts []string
	for _, content := range toolResult.Content {
		if text, ok := content.(*llm.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// compactArgs renders tool arguments as a short single-line JSON snippet.
func compactArgs(args any) string {
	if args == nil {
		return ""
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return ""
	}
	s := string(encoded)
	const max = 120
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

// firstLines returns at most n lines of s, noting how many were dropped.
func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:n], "\n") + fmt.Sprintf("\n… (%d more lines)", len(lines)-n)
}
