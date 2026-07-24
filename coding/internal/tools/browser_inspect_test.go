package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

type resultBrowserInspector struct {
	result BrowserInspectionResult
	err    error
}

func (b resultBrowserInspector) InspectBrowser(context.Context) (BrowserInspectionResult, error) {
	return b.result, b.err
}

func TestInspectBrowserReturnsBoundedPageObservation(t *testing.T) {
	result := executeBrowserInspection(t, InspectBrowser(resultBrowserInspector{
		result: BrowserInspectionResult{
			Status:      BrowserInspectionCompleted,
			URL:         "https://example.com/final",
			Title:       "Example",
			PageStatus:  BrowserPageReady,
			Revision:    3,
			VisibleText: "Heading\nVisible button",
		},
	}))
	text := browserInspectionResultText(t, result)
	for _, want := range []string{
		"Browser URL: https://example.com/final",
		"Title: Example",
		"Page status: ready",
		"Heading\nVisible button",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("result = %q, want %q", text, want)
		}
	}
}

func TestInspectBrowserReportsTruncation(t *testing.T) {
	result := executeBrowserInspection(t, InspectBrowser(resultBrowserInspector{
		result: BrowserInspectionResult{
			Status:      BrowserInspectionCompleted,
			URL:         "https://example.com/",
			PageStatus:  BrowserPageReady,
			VisibleText: "partial page",
			Truncated:   true,
		},
	}))
	if text := browserInspectionResultText(t, result); !strings.Contains(text, "[Visible text truncated]") {
		t.Fatalf("result = %q, want truncation marker", text)
	}
}

func TestInspectBrowserReportsPageFailure(t *testing.T) {
	result := executeBrowserInspection(t, InspectBrowser(resultBrowserInspector{
		result: BrowserInspectionResult{
			Status: BrowserInspectionFailed,
			Error:  "Agent browser tab is not ready",
		},
	}))
	if text := browserInspectionResultText(t, result); text != "Could not inspect browser: Agent browser tab is not ready" {
		t.Fatalf("result = %q", text)
	}
}

func TestInspectBrowserReportsUnavailableController(t *testing.T) {
	result := executeBrowserInspection(t, InspectBrowser())
	if text := browserInspectionResultText(t, result); !strings.Contains(text, "browser observation is unavailable") {
		t.Fatalf("result = %q", text)
	}
}

func executeBrowserInspection(t *testing.T, tool Tool) agent.ToolResult {
	t.Helper()
	result, err := tool.Execute(
		context.Background(),
		"inspection-call",
		json.RawMessage(`{}`),
		func(agent.ToolResult) {},
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func browserInspectionResultText(t *testing.T, result agent.ToolResult) string {
	t.Helper()
	if len(result.Content) != 1 {
		t.Fatalf("content = %#v", result.Content)
	}
	content, ok := result.Content[0].(*llm.TextContent)
	if !ok {
		t.Fatalf("content = %#v", result.Content[0])
	}
	return content.Text
}
