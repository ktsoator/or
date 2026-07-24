package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

const MaxBrowserInspectionTextRunes = 12_000

type inspectBrowserArgs struct{}

type BrowserInspectionStatus string

const (
	BrowserInspectionCompleted BrowserInspectionStatus = "completed"
	BrowserInspectionFailed    BrowserInspectionStatus = "failed"
	BrowserInspectionCancelled BrowserInspectionStatus = "cancelled"
	BrowserInspectionTimeout   BrowserInspectionStatus = "timeout"
)

type BrowserPageStatus string

const (
	BrowserPageReady      BrowserPageStatus = "ready"
	BrowserPageNavigating BrowserPageStatus = "navigating"
	BrowserPageFailed     BrowserPageStatus = "failed"
)

// BrowserInspectionResult is a bounded, read-only observation of the stable
// Agent browser tab. It intentionally contains no DOM, storage, cookies, form
// values, or executable page code.
type BrowserInspectionResult struct {
	ID          string
	Status      BrowserInspectionStatus
	URL         string
	Title       string
	PageStatus  BrowserPageStatus
	Revision    int
	VisibleText string
	Truncated   bool
	Error       string
}

type BrowserInspector interface {
	InspectBrowser(context.Context) (BrowserInspectionResult, error)
}

// InspectBrowser returns a product tool that observes the current session's
// stable Agent browser tab without granting control over user-owned tabs.
func InspectBrowser(inspectors ...BrowserInspector) Tool {
	var inspector BrowserInspector
	if len(inspectors) > 0 {
		inspector = inspectors[0]
	}
	def := llm.MustTool[inspectBrowserArgs]("inspect_browser", inspectBrowserText.description)
	return Tool{
		AgentTool: agent.AgentTool{
			Definition: def,
			Label:      "Inspect browser",
			Execute: func(ctx context.Context, _ string, _ json.RawMessage, _ func(agent.ToolResult)) (agent.ToolResult, error) {
				if inspector == nil {
					return textResult("Could not inspect browser: browser observation is unavailable"), nil
				}
				result, err := inspector.InspectBrowser(ctx)
				if err != nil {
					return agent.ToolResult{}, err
				}
				return browserInspectionToolResult(result), nil
			},
		},
		AccessFor:     InternalAccess,
		PromptSnippet: inspectBrowserText.snippet,
		Guidelines:    inspectBrowserText.guidelines,
	}
}

func browserInspectionToolResult(result BrowserInspectionResult) agent.ToolResult {
	switch result.Status {
	case BrowserInspectionCompleted:
		var out strings.Builder
		fmt.Fprintf(&out, "Browser URL: %s\n", result.URL)
		if result.Title != "" {
			fmt.Fprintf(&out, "Title: %s\n", result.Title)
		}
		fmt.Fprintf(&out, "Page status: %s\n", result.PageStatus)
		visibleText := strings.TrimSpace(result.VisibleText)
		if visibleText == "" {
			out.WriteString("Visible text: (none)")
		} else {
			out.WriteString("Visible text:\n")
			out.WriteString(visibleText)
		}
		if result.Truncated {
			out.WriteString("\n[Visible text truncated]")
		}
		return textResult(out.String())
	case BrowserInspectionFailed:
		detail := strings.TrimSpace(result.Error)
		if detail == "" {
			detail = "page observation failed"
		}
		return textResult("Could not inspect browser: " + detail)
	case BrowserInspectionTimeout:
		return textResult("The browser did not return an inspection result")
	case BrowserInspectionCancelled:
		return textResult("The browser inspection was cancelled")
	default:
		return textResult("Could not inspect browser: browser returned an invalid result")
	}
}
