package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

type resultBrowserTabsProvider struct {
	result BrowserTabsResult
}

func (p resultBrowserTabsProvider) BrowserTabs(context.Context) (BrowserTabsResult, error) {
	return p.result, nil
}

func TestBrowserTabsReturnsStableSessionTabMetadata(t *testing.T) {
	result, err := BrowserTabs(resultBrowserTabsProvider{result: BrowserTabsResult{
		Status: BrowserTabsCompleted,
		OpenTabs: []BrowserOpenTab{
			{
				TabID:  "stable-tab-1",
				URL:    "https://example.com/",
				Title:  "Example",
				Status: BrowserTabReady,
			},
		},
		ControlledTabs: []BrowserControlledTab{
			{TabID: "stable-tab-1", Capabilities: []BrowserControlCapability{BrowserControlRead, BrowserControlNavigate}},
		},
		Selected: "stable-tab-1",
	}}).Execute(context.Background(), "tabs-call", json.RawMessage(`{}`), func(agent.ToolProgress) {})
	if err != nil {
		t.Fatal(err)
	}
	text := browserTabsResultText(t, result)
	for _, want := range []string{
		`"openTabs"`,
		`"controlledTabs"`,
		`"capabilities"`,
		`"selected": "stable-tab-1"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("result = %q, want %q", text, want)
		}
	}
}

func TestBrowserTabsUsesNullSelectionWhenNoTabIsControlled(t *testing.T) {
	result, err := BrowserTabs(resultBrowserTabsProvider{result: BrowserTabsResult{
		Status:   BrowserTabsCompleted,
		OpenTabs: []BrowserOpenTab{},
	}}).Execute(context.Background(), "tabs-call", json.RawMessage(`{}`), func(agent.ToolProgress) {})
	if err != nil {
		t.Fatal(err)
	}
	if text := browserTabsResultText(t, result); !strings.Contains(text, `"selected": null`) {
		t.Fatalf("result = %q", text)
	}
}

func TestBrowserTabsReportsUnavailableProvider(t *testing.T) {
	result, err := BrowserTabs().Execute(
		context.Background(),
		"tabs-call",
		json.RawMessage(`{}`),
		func(agent.ToolProgress) {},
	)
	if err != nil {
		t.Fatal(err)
	}
	if text := browserTabsResultText(t, result); !strings.Contains(text, "browser observation is unavailable") {
		t.Fatalf("result = %q", text)
	}
}

func browserTabsResultText(t *testing.T, result agent.ToolResult) string {
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
