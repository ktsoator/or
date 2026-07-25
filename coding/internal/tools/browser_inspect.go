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

type inspectBrowserArgs struct {
	TabID string `json:"tabID,omitempty" jsonschema:"description=Stable tab ID returned by tabs_context. Omit it only to inspect the selected Agent-controlled tab."`
}

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

// BrowserInspectionResult is a bounded, read-only observation of an
// Agent-controlled browser tab. It intentionally contains no DOM, storage,
// cookies, form values, or executable page code.
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
	InspectBrowser(context.Context, string) (BrowserInspectionResult, error)
}

// InspectBrowser returns a product tool that observes an explicit or selected
// Agent-controlled tab without granting control over user-owned tabs.
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
			Execute: func(ctx context.Context, _ string, raw json.RawMessage, _ func(agent.ToolResult)) (agent.ToolResult, error) {
				var in inspectBrowserArgs
				if err := json.Unmarshal(raw, &in); err != nil {
					return agent.ToolResult{}, err
				}
				tabID := strings.TrimSpace(in.TabID)
				if len([]rune(tabID)) > 256 {
					return textResult("Could not inspect browser: browser tab ID is too long"), nil
				}
				if inspector == nil {
					return textResult("Could not inspect browser: browser observation is unavailable"), nil
				}
				result, err := inspector.InspectBrowser(ctx, tabID)
				if err != nil {
					return agent.ToolResult{}, err
				}
				return browserInspectionToolResult(result), nil
			},
		},
		AccessFor:  InternalAccess,
		Guidelines: inspectBrowserText.guidelines,
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
