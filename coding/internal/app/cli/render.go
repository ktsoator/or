package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/ktsoator/or/coding"
)

const (
	dim   = "\033[2m"
	red   = "\033[31m"
	reset = "\033[0m"
)

// Renderer projects UI-neutral coding events as lightly styled terminal text.
type Renderer struct {
	w          io.Writer
	streamKind string
}

// NewRenderer returns a Renderer writing to w.
func NewRenderer(w io.Writer) *Renderer { return &Renderer{w: w} }

// Handle renders a single coding event.
func (p *Renderer) Handle(ev coding.Event) {
	switch ev.Type {
	case coding.TextDelta:
		p.streamDelta("text", ev.Delta, "")

	case coding.ThinkingDelta:
		p.streamDelta("thinking", ev.Delta, dim)

	case coding.ToolStarted:
		fmt.Fprintf(p.w, "\n%s→ %s %s%s\n", dim, ev.ToolName, compactArgs(ev.ToolArgs), reset)

	case coding.ToolFinished:
		text := firstLines(ev.ToolResult, 8)
		style := dim
		if ev.IsError {
			style = red
		}
		if strings.TrimSpace(text) != "" {
			fmt.Fprintf(p.w, "%s%s%s\n", style, text, reset)
		}

	case coding.MessageCompleted:
		p.handleMessageCompleted(ev.Text)
	}
}

func (p *Renderer) streamDelta(kind, text, style string) {
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

func (p *Renderer) handleMessageCompleted(text string) {
	if p.streamKind != "" {
		fmt.Fprintln(p.w)
	} else if strings.TrimSpace(text) != "" {
		fmt.Fprintf(p.w, "\n%s\n", text)
	}
	p.streamKind = ""
}

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

func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:n], "\n") + fmt.Sprintf("\n… (%d more lines)", len(lines)-n)
}
