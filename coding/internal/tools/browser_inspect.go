package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/llm"
)

const MaxBrowserInspectionTextRunes = 12_000

type inspectBrowserArgs struct {
	TabID string `json:"tabID,omitempty" jsonschema:"description=Stable session-local tab ID returned by tabs_context. An explicit ID temporarily attaches read access to that open tab for this inspection; omit it to inspect the selected controlled tab."`
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

// BrowserInspectionResult is a bounded, read-only observation of one open tab
// in the requesting session. It intentionally contains no DOM, storage,
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

// InspectBrowser returns a product tool that observes an explicit session-local
// tab or the selected controlled tab. The renderer may attach a request-scoped
// read lease to an explicit open tab and releases that lease after inspection.
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
			Execute: func(ctx context.Context, _ string, raw json.RawMessage, _ func(agent.ToolProgress)) (agent.ToolResult, error) {
				var in inspectBrowserArgs
				if err := json.Unmarshal(raw, &in); err != nil {
					return agent.ToolResult{}, err
				}
				tabID := strings.TrimSpace(in.TabID)
				if len([]rune(tabID)) > 256 {
					return failedResult("browser_tab_id_invalid", "Could not inspect browser: browser tab ID is too long", nil), nil
				}
				if inspector == nil {
					return failedResult("browser_unavailable", "Could not inspect browser: browser observation is unavailable", nil), nil
				}
				result, err := inspector.InspectBrowser(ctx, tabID)
				if err != nil {
					return agent.ToolResult{}, err
				}
				return browserInspectionToolResult(result), nil
			},
		},
		AccessFor: func(map[string]any) []permission.Access {
			return []permission.Access{{Action: permission.Network}}
		},
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
		return failedResult("browser_inspection_failed", "Could not inspect browser: "+detail, nil)
	case BrowserInspectionTimeout:
		return timeoutResult("browser_inspection_timeout", "The browser did not return an inspection result", nil)
	case BrowserInspectionCancelled:
		return cancelledResult("browser_inspection_cancelled", "The browser inspection was cancelled", nil)
	default:
		return failedResult("browser_inspection_result_invalid", "Could not inspect browser: browser returned an invalid result", nil)
	}
}
